package agent

import (
	"fmt"
	"strings"
	"time"

	"yoink/internal/healer"
)

// FinalState is the terminal project state after yoink init.
type FinalState string

const (
	StateSuccess        FinalState = "success"
	StateConfigRequired FinalState = "configuration_required"
	StateBlocked        FinalState = "blocked"
	StateFailed         FinalState = "failed"
)

// FinalReport is the structured result presented to the user at the end
// of yoink init. It distinguishes "build failed" from "credentials
// unavailable" so missing secrets are never treated as product failures.
type FinalReport struct {
	State              FinalState
	ProjectName        string
	DetectedFrameworks []string
	ServicesBuilt      int
	ServicesTotal      int
	ServicesHealthy    int
	AgentIterations    int
	DeterministicFixes []string
	AgentPatches       []PatchRecord
	RequiredEnvVars    []EnvRequirement
	URLs               []string
	BuildLogTail       string
	StartedAt          time.Time
	Duration           time.Duration
}

// Render produces the user-facing summary string.
func (r *FinalReport) Render() string {
	var b strings.Builder

	// Header based on state.
	switch r.State {
	case StateSuccess:
		b.WriteString("✓ Project ready\n\n")
	case StateConfigRequired:
		b.WriteString("✓ Project prepared — configuration required\n\n")
	case StateBlocked:
		b.WriteString("⚠ Project blocked\n\n")
	case StateFailed:
		b.WriteString("✗ Project failed\n\n")
	}

	// What was accomplished.
	if len(r.DetectedFrameworks) > 0 {
		fmt.Fprintf(&b, "✓ Detected: %s\n", strings.Join(r.DetectedFrameworks, ", "))
	}
	if r.ServicesTotal > 0 {
		fmt.Fprintf(&b, "✓ %d/%d services built\n", r.ServicesBuilt, r.ServicesTotal)
	}
	if r.ServicesHealthy > 0 {
		fmt.Fprintf(&b, "✓ %d services healthy\n", r.ServicesHealthy)
	}
	if len(r.DeterministicFixes) > 0 {
		b.WriteString("✓ Deterministic fixes applied:\n")
		for _, f := range r.DeterministicFixes {
			fmt.Fprintf(&b, "    %s\n", f)
		}
	}
	if len(r.AgentPatches) > 0 {
		b.WriteString("✓ Agent repairs applied:\n")
		for _, p := range r.AgentPatches {
			fmt.Fprintf(&b, "    %s (%s): %s\n", p.File, p.Operation, p.Reason)
		}
	}

	// URLs for successful projects.
	if len(r.URLs) > 0 && r.State == StateSuccess {
		b.WriteString("\n")
		for _, url := range r.URLs {
			fmt.Fprintf(&b, "→ %s\n", url)
		}
	}

	// Required configuration (the key UX for CONFIGURATION_REQUIRED).
	if len(r.RequiredEnvVars) > 0 {
		b.WriteString("\nConfiguration required:\n\n")
		for _, req := range r.RequiredEnvVars {
			label := req.Name
			if req.Secret {
				label += "    (secret)"
			} else {
				label += "    (configuration)"
			}
			switch req.Phase {
			case "build":
				label += " [build-time]"
			case "runtime":
				label += " [runtime]"
			}
			if req.Provider != "" {
				label += " [provider: " + req.Provider + "]"
			}
			fmt.Fprintf(&b, "  %s\n", label)
		}
		b.WriteString("\nThese values cannot safely be inferred from the repository.\n")
		fmt.Fprintf(&b, "\nRun:\n  yoink env %s\n\nThen:\n  yoink up %s\n", r.ProjectName, r.ProjectName)
	}

	// Build log for failed/blocked.
	if r.BuildLogTail != "" && (r.State == StateBlocked || r.State == StateFailed) {
		fmt.Fprintf(&b, "\nBuild output (tail):\n%s\n", r.BuildLogTail)
	}

	return b.String()
}

// determineFinalState decides the project state from the agent's results.
// If the failure is due to missing secrets/credentials → CONFIGURATION_REQUIRED.
// If the failure is a build error the agent couldn't solve → BLOCKED.
// If the build succeeded + runtime healthy → SUCCESS.
func determineFinalState(success bool, failure *healer.Failure, envReqs []EnvRequirement) FinalState {
	if success {
		return StateSuccess
	}
	// If there are required env vars that are secrets and unavailable,
	// this is a configuration requirement, not a failure.
	for _, req := range envReqs {
		if req.Secret && !req.SafePlaceholder {
			return StateConfigRequired
		}
	}
	// If the failure category is missing-environment or nextjs-build
	// (which often means env vars are needed during static generation),
	// and we have env requirements that couldn't be satisfied:
	if failure != nil {
		switch failure.Category {
		case "missing-environment":
			return StateConfigRequired
		case "nextjs-build":
			// Next.js build failures are often env-related. Check if any
			// env refs point to secrets.
			for _, ref := range failure.EnvRefs {
				for _, req := range envReqs {
					if strings.EqualFold(req.Name, ref) && req.Secret {
						return StateConfigRequired
					}
				}
			}
			// If we have env refs but don't know if they're secrets, still
			// lean toward config_required (the user can provide them).
			if len(failure.EnvRefs) > 0 {
				return StateConfigRequired
			}
		}
	}
	return StateBlocked
}
