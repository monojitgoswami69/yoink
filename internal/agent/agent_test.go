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
