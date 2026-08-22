package agent

import (
	"strings"
	"testing"
	"time"

	"yoink/internal/detector"
	"yoink/internal/healer"
)

func TestDetermineFinalStateConfigRequired(t *testing.T) {
	// Next.js build failure with secret env vars → CONFIGURATION_REQUIRED
	failure := &healer.Failure{
		Category: "nextjs-build",
		Error:    "Failed to collect page data for /_not-found",
		EnvRefs:  []string{}, // no specific variable named in error
	}
	envReqs := []EnvRequirement{
		{Name: "UPSTASH_REDIS_REST_TOKEN", Secret: true, SafePlaceholder: false, Status: "REQUIRED"},
		{Name: "GEMINI_API_KEY", Secret: true, SafePlaceholder: false, Status: "REQUIRED"},
	}
	state := determineFinalState(false, failure, envReqs)
	if state != StateConfigRequired {
		t.Errorf("want configuration_required, got %s", state)
	}
}

func TestDetermineFinalStateSuccess(t *testing.T) {
	state := determineFinalState(true, nil, nil)
	if state != StateSuccess {
		t.Errorf("want success, got %s", state)
	}
}

func TestDetermineFinalStateBlocked(t *testing.T) {
	failure := &healer.Failure{
		Category: "compilation",
		Error:    "error TS2307: Cannot find module 'X'",
	}
	state := determineFinalState(false, failure, nil)
	if state != StateBlocked {
		t.Errorf("want blocked, got %s", state)
	}
}

func TestDetermineFinalStateMissingEnv(t *testing.T) {
	failure := &healer.Failure{
		Category: "missing-environment",
		Error:    "DATABASE_URL is not defined",
		EnvRefs:  []string{"DATABASE_URL"},
	}
	state := determineFinalState(false, failure, nil)
	if state != StateConfigRequired {
		t.Errorf("want configuration_required, got %s", state)
	}
}

func TestDetermineFinalStateGenericConfigurationDoesNotGuess(t *testing.T) {
	failure := &healer.Failure{Category: "configuration", Error: "environment configuration required"}
	if got := determineFinalState(false, failure, nil); got != StateBlocked {
		t.Fatalf("generic unlocated configuration should not guess required vars: %s", got)
	}
}

func TestDetermineFinalStateOnlyRequiredFindingsBlock(t *testing.T) {
	failure := &healer.Failure{Category: "runtime", Error: "worker failed"}
	reqs := []EnvRequirement{
		{Name: "WORKER_SECRET", Secret: true, Status: "UNKNOWN"},
		{Name: "DATABASE_URL", Status: "REQUIRED", Evidence: []string{"startup validation"}},
	}
	if got := determineFinalState(false, failure, reqs); got != StateConfigRequired {
		t.Fatalf("explicit required finding should block: %s", got)
	}
}

func TestWorkerRequirementDoesNotBecomeFrontendRequirement(t *testing.T) {
	ag := &Agent{State: &AgentState{Services: []detector.Service{{ID: "frontend"}, {ID: "worker"}}, ToolHistory: []ToolCall{{Tool: "read_file"}}}}
	ag.applyEnvironmentFindings([]EnvironmentFinding{{ServiceID: "worker", Name: "WORKER_SECRET", Status: "REQUIRED", Phase: "runtime", Evidence: []string{"worker startup validation"}}})
	if len(ag.State.EnvReqs) != 1 || ag.State.EnvReqs[0].Name != "WORKER_SECRET" {
		t.Fatalf("unexpected worker requirement state: %+v", ag.State.EnvReqs)
	}
}

func TestFinalReportRenderConfigRequired(t *testing.T) {
	report := &FinalReport{
		State:              StateConfigRequired,
		ProjectName:        "portfolio",
		DetectedFrameworks: []string{"next"},
		ServicesTotal:      1,
		RequiredEnvVars: []EnvRequirement{
			{Name: "UPSTASH_REDIS_REST_TOKEN", Secret: true, Phase: "build", Status: "REQUIRED"},
			{Name: "GEMINI_API_KEY", Secret: true, Phase: "build", Status: "REQUIRED"},
			{Name: "NEXT_PUBLIC_SITE_URL", Secret: false, Phase: "build"},
		},
	}
	output := report.Render()
	if !contains(output, "configuration required") {
		t.Errorf("report should say 'configuration required'; got:\n%s", output)
	}
	if !contains(output, "UPSTASH_REDIS_REST_TOKEN") {
		t.Errorf("report should list UPSTASH_REDIS_REST_TOKEN")
	}
	if !contains(output, "secret") {
		t.Errorf("report should mark secrets")
	}
	if !contains(output, "yoink env portfolio") {
		t.Errorf("report should suggest yoink env command")
	}
}

func TestFinalReportRenderSuccess(t *testing.T) {
	report := &FinalReport{
		State:              StateSuccess,
		ProjectName:        "certify",
		DetectedFrameworks: []string{"vite"},
		ServicesTotal:      1,
		ServicesHealthy:    1,
		URLs:               []string{"http://localhost:80"},
		StartedAt:          time.Now(),
	}
	output := report.Render()
	if !contains(output, "Project ready") {
		t.Errorf("success report should say 'Project ready'")
	}
	if !contains(output, "http://localhost:80") {
		t.Errorf("success report should show URL")
	}
}

func TestCollectEnvRequirementsDoesNotGuessFromBuildEnv(t *testing.T) {
	svc := detector.Service{
		ID:        "s1",
		Framework: "next",
		BuildEnv: map[string]string{
			"UPSTASH_REDIS_REST_TOKEN": "",           // secret, empty
			"GEMINI_API_KEY":           "",           // secret, empty
			"PORT":                     "3000",       // non-secret, has value
			"NODE_ENV":                 "production", // non-secret, has value
		},
	}
	ag := &Agent{
		State: &AgentState{
			Services: []detector.Service{svc},
			CurrentFailure: &healer.Failure{
				Category: "nextjs-build",
				Error:    "Failed to collect page data",
			},
			Generated: nil,
		},
	}
	ag.collectEnvRequirements()
	// Static build env candidates alone are insufficient evidence. The runtime
	// error must name the required variable before it is reported.
	if len(ag.State.EnvReqs) != 0 {
		t.Fatalf("unexpected guessed environment requirements: %+v", ag.State.EnvReqs)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && strings.Contains(s, substr)
}
