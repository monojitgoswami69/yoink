package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yoink/internal/detector"
	"yoink/internal/generator"
	"yoink/internal/healer"
	"yoink/internal/safefs"
)

type recordingLLM struct {
	responses []string
	prompts   []string
}

func (m *recordingLLM) Call(_ context.Context, _, user string) (string, error) {
	m.prompts = append(m.prompts, user)
	response := m.responses[0]
	m.responses = m.responses[1:]
	return response, nil
}

func testAgent(t *testing.T, llmClient LLMClient) (*Agent, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"app"}`), 0644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "yoink-outputs")
	if err := os.MkdirAll(output, 0755); err != nil {
		t.Fatal(err)
	}
	reader, err := safefs.New(root)
	if err != nil {
		t.Fatal(err)
	}
	ag := New(llmClient, nil, root, output, nil, nil)
	ag.SetReader(reader.Read)
	ag.SetGenerated(&generator.Output{Files: map[string]string{
		"Dockerfile.service-1": "FROM node:20-alpine\nCMD [\"node\", \"index.js\"]\n",
		"Dockerfile.service-2": "FROM node:20-alpine\nCMD [\"node\", \"server.js\"]\n",
		"docker-compose.yml":   "services:\n  service-1:\n    image: app\n",
	}})
	ag.SetDetection(&detector.Result{Services: []detector.Service{{ID: "service-1"}, {ID: "service-2"}}}, []detector.Service{{ID: "service-1"}, {ID: "service-2"}}, nil)
	return ag, root
}

func TestAgentToolsRejectTraversal(t *testing.T) {
	ag, root := testAgent(t, &recordingLLM{})
	outside := filepath.Join(filepath.Dir(root), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := ag.toolReadFile("../outside.txt", 0, 0); !strings.Contains(got, "escapes repository root") {
		t.Fatalf("read traversal was not rejected: %s", got)
	}
	if got := ag.toolSearch("outside", "../"); !strings.Contains(got, "escapes repository root") {
		t.Fatalf("search traversal was not rejected: %s", got)
	}
	if got := ag.toolListTree("../", 1); !strings.Contains(got, "escapes repository root") {
		t.Fatalf("tree traversal was not rejected: %s", got)
	}
}

func TestAgentPatchPathMustBeGeneratedArtifact(t *testing.T) {
	ag, _ := testAgent(t, &recordingLLM{})
	if ag.applyAgentPatches([]healer.Change{{File: "../escaped", Operation: "create_file", Content: "bad"}}) {
		t.Fatal("escaped patch path was applied")
	}
	if ag.applyAgentPatches([]healer.Change{{File: "package.json", Operation: "create_file", Content: "bad"}}) {
		t.Fatal("source-file patch was applied")
	}
}

func TestAgentContextUsesFailingService(t *testing.T) {
	ag, _ := testAgent(t, &recordingLLM{})
	ag.State.CurrentFailure = &healer.Failure{Service: "service-2", Category: "dependency", Error: "failed"}
	prompt := ag.buildContextPrompt()
	if !strings.Contains(prompt, "SERVICE: service-2") {
		t.Fatalf("prompt targeted the wrong service: %s", prompt)
	}
}

func TestAgentToolResultsReturnToNextLLMCall(t *testing.T) {
	model := &recordingLLM{responses: []string{
		`{"tool_calls":[{"tool":"read_file","args":{"path":"package.json"}}]}`,
		`{"done":true}`,
	}}
	ag, _ := testAgent(t, model)
	ag.State.CurrentFailure = &healer.Failure{Service: "service-1", Category: "dependency", Error: "failed"}
	if _, err := ag.agentIteration(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(model.prompts) != 2 || !strings.Contains(model.prompts[1], `{"name":"app"}`) {
		t.Fatalf("tool result was not returned to next call: %#v", model.prompts)
	}
}

func TestAgentComposeOnlyReplacementPreservesDockerfile(t *testing.T) {
	model := &recordingLLM{responses: []string{`{"compose":"services:\n  service-1:\n    image: fixed\n","summary":"compose fix"}`}}
	ag, _ := testAgent(t, model)
	ag.State.CurrentFailure = &healer.Failure{Service: "service-1", Category: "configuration", Error: "failed"}
	original := ag.State.Generated.Files["Dockerfile.service-1"]
	if _, err := ag.agentIteration(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ag.State.Generated.Files["Dockerfile.service-1"] != original {
		t.Fatal("compose-only response overwrote Dockerfile")
	}
}

// TestAgentSystemPromptContainsEnvInvestigationGuidance verifies the system
// prompt is technology-agnostic and instructs the agent to investigate env vars
// across JS/TS and Python stacks.
func TestAgentSystemPromptContainsEnvInvestigationGuidance(t *testing.T) {
	if !strings.Contains(agentSystemPrompt, "ENVIRONMENT VARIABLE INVESTIGATION") {
		t.Fatal("system prompt should contain environment investigation guidance")
	}
	if !strings.Contains(agentSystemPrompt, "process.env.X") {
		t.Fatal("system prompt should mention JS/TS patterns")
	}
	if !strings.Contains(agentSystemPrompt, "os.environ") {
		t.Fatal("system prompt should mention Python patterns")
	}
	if !strings.Contains(agentSystemPrompt, "import.meta.env") {
		t.Fatal("system prompt should mention Vite patterns")
	}
	if !strings.Contains(agentSystemPrompt, "FEATURE_SPECIFIC") {
		t.Fatal("system prompt should mention feature-specific classification")
	}
}

// TestAgentRequiredFindingRequiresPriorInspection verifies that the agent
// cannot promote a REQUIRED finding without having actually inspected files.
func TestAgentRequiredFindingRequiresPriorInspection(t *testing.T) {
	ag, _ := testAgent(t, &recordingLLM{})
	ag.State.Services = []detector.Service{{ID: "service-1"}}
	// No tool history — no prior inspection.
	ag.applyEnvironmentFindings([]EnvironmentFinding{{
		ServiceID: "service-1", Name: "DATABASE_URL", Status: "REQUIRED",
		Phase: "runtime", Evidence: []string{"test"},
	}})
	if len(ag.State.EnvReqs) != 0 {
		t.Fatal("REQUIRED finding accepted without prior tool inspection")
	}
}

// TestAgentRequiredFindingAcceptedAfterInspection verifies that after actual
// file inspection, a well-evidenced REQUIRED finding is accepted.
func TestAgentRequiredFindingAcceptedAfterInspection(t *testing.T) {
	ag, _ := testAgent(t, &recordingLLM{})
	ag.State.Services = []detector.Service{{ID: "service-1"}}
	ag.State.ToolHistory = []ToolCall{{Tool: "read_file", Success: true}}
	ag.applyEnvironmentFindings([]EnvironmentFinding{{
		ServiceID: "service-1", Name: "DATABASE_URL", Status: "REQUIRED",
		Phase: "runtime", Evidence: []string{"config.py:12 declares required field"},
	}})
	if len(ag.State.EnvReqs) != 1 {
		t.Fatalf("expected 1 accepted requirement, got %d", len(ag.State.EnvReqs))
	}
	if ag.State.EnvReqs[0].Name != "DATABASE_URL" || ag.State.EnvReqs[0].Status != "REQUIRED" {
		t.Fatalf("wrong requirement: %+v", ag.State.EnvReqs[0])
	}
}

// TestAgentOptionalFindingDoesNotBlock verifies that OPTIONAL/FEATURE_SPECIFIC
// findings never produce EnvReqs.
func TestAgentOptionalAndFeatureSpecificFindingsDoNotBlock(t *testing.T) {
	ag, _ := testAgent(t, &recordingLLM{})
	ag.State.Services = []detector.Service{{ID: "service-1"}}
	ag.State.ToolHistory = []ToolCall{{Tool: "read_file", Success: true}}
	ag.applyEnvironmentFindings([]EnvironmentFinding{
		{ServiceID: "service-1", Name: "SENTRY_DSN", Status: "OPTIONAL", Phase: "runtime", Evidence: []string{"guarded"}},
		{ServiceID: "service-1", Name: "STRIPE_SECRET_KEY", Status: "FEATURE_SPECIFIC", Phase: "feature", Evidence: []string{"only in /api/payments"}},
		{ServiceID: "service-1", Name: "UNKNOWN_VAR", Status: "UNKNOWN", Phase: "runtime", Evidence: []string{"unclear"}},
	})
	if len(ag.State.EnvReqs) != 0 {
		t.Fatalf("non-REQUIRED findings should not produce env requirements: %+v", ag.State.EnvReqs)
	}
}

// TestAgentWorkerFindingDoesNotAffectFrontend verifies service scoping.
func TestAgentWorkerFindingDoesNotAffectFrontend(t *testing.T) {
	ag, _ := testAgent(t, &recordingLLM{})
	ag.State.Services = []detector.Service{{ID: "frontend"}, {ID: "worker"}}
	ag.State.ToolHistory = []ToolCall{{Tool: "read_file", Success: true}}
	ag.applyEnvironmentFindings([]EnvironmentFinding{{
		ServiceID: "worker", Name: "WORKER_DATABASE_URL", Status: "REQUIRED",
		Phase: "runtime", Evidence: []string{"worker startup validation"},
	}})
	if len(ag.State.EnvReqs) != 1 || ag.State.EnvReqs[0].ServiceID != "worker" {
		t.Fatalf("worker requirement should be scoped to worker, got: %+v", ag.State.EnvReqs)
	}
}

// TestAgentMultipleRequiredFindingsAllReported verifies that structured
// validation exposing multiple missing variables reports all of them.
func TestAgentMultipleRequiredFindingsAllReported(t *testing.T) {
	ag, _ := testAgent(t, &recordingLLM{})
	ag.State.Services = []detector.Service{{ID: "backend"}}
	ag.State.ToolHistory = []ToolCall{{Tool: "read_file", Success: true}}
	ag.applyEnvironmentFindings([]EnvironmentFinding{
		{ServiceID: "backend", Name: "DATABASE_URL", Status: "REQUIRED", Phase: "runtime", Evidence: []string{"config.py:5 required field"}},
		{ServiceID: "backend", Name: "JWT_SECRET", Status: "REQUIRED", Phase: "runtime", Evidence: []string{"config.py:8 required field"}},
		{ServiceID: "backend", Name: "REDIS_URL", Status: "REQUIRED", Phase: "runtime", Evidence: []string{"config.py:12 required field"}},
	})
	if len(ag.State.EnvReqs) != 3 {
		t.Fatalf("expected 3 requirements, got %d", len(ag.State.EnvReqs))
	}
}

// TestAgentContextPackUsesFailingService verifies that the pre-selected
// relevant files target the failing service, not Services[0].
func TestAgentContextPackUsesFailingService(t *testing.T) {
	root := t.TempDir()
	// Create manifests for two services.
	os.MkdirAll(filepath.Join(root, "backend"), 0755)
	os.WriteFile(filepath.Join(root, "backend", "requirements.txt"), []byte("fastapi\n"), 0644)
	os.WriteFile(filepath.Join(root, "frontend", "package.json"), []byte(`{"name":"web"}`), 0644)
	reader, _ := safefs.New(root)
	ag := New(nil, nil, root, filepath.Join(root, "yoink-outputs"), nil, nil)
	ag.SetReader(reader.Read)
	ag.SetGenerated(&generator.Output{Files: map[string]string{
		"Dockerfile.service-1": "FROM node:20-alpine\n",
		"Dockerfile.service-2": "FROM python:3.12-slim\n",
		"docker-compose.yml":   "services: {}\n",
	}})
	ag.SetDetection(nil, []detector.Service{
		{ID: "service-1", Directory: "frontend", Language: "typescript", Framework: "next"},
		{ID: "service-2", Directory: "backend", Language: "python", Framework: "fastapi"},
	}, nil)
	ag.State.CurrentFailure = &healer.Failure{Service: "service-2", Category: "dependency", Error: "failed"}
	prompt := ag.buildContextPrompt()
	// The pre-selected files should include backend/requirements.txt, not frontend/package.json.
	if strings.Contains(prompt, "package.json") && !strings.Contains(prompt, "requirements.txt") {
		t.Fatal("context pack selected frontend manifest for backend failure")
	}
}
