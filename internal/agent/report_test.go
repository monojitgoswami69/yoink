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
		{Name: "UPSTASH_REDIS_REST_TOKEN", Secret: true, SafePlaceholder: false},
		{Name: "GEMINI_API_KEY", Secret: true, SafePlaceholder: false},
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

func TestFinalReportRenderConfigRequired(t *testing.T) {
	report := &FinalReport{
		State:              StateConfigRequired,
		ProjectName:        "portfolio",
		DetectedFrameworks: []string{"next"},
		ServicesTotal:      1,
		RequiredEnvVars: []EnvRequirement{
			{Name: "UPSTASH_REDIS_REST_TOKEN", Secret: true, Phase: "build"},
			{Name: "GEMINI_API_KEY", Secret: true, Phase: "build"},
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

func TestCollectEnvRequirementsFromBuildEnv(t *testing.T) {
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
	// Should include the two secrets (empty + secret) but not PORT/NODE_ENV.
	foundToken := false
	foundGeminiKey := false
	foundPort := false
	for _, req := range ag.State.EnvReqs {
		switch req.Name {
		case "UPSTASH_REDIS_REST_TOKEN":
			foundToken = true
			if !req.Secret {
				t.Error("UPSTASH_REDIS_REST_TOKEN should be secret")
			}
		case "GEMINI_API_KEY":
			foundGeminiKey = true
			if !req.Secret {
				t.Error("GEMINI_API_KEY should be secret")
			}
		case "PORT":
			foundPort = true
		}
	}
	if !foundToken {
		t.Error("should include UPSTASH_REDIS_REST_TOKEN")
	}
	if !foundGeminiKey {
		t.Error("should include GEMINI_API_KEY")
	}
	if foundPort {
		t.Error("should NOT include PORT (non-secret with value)")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && strings.Contains(s, substr)
}
