package healer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"yoink/internal/llm"
)

// RepairResponse is the hybrid LLM response: the model can return either a
// structured RepairPlan (preferred, with Changes) or a full-file replacement
// (fallback, with Dockerfile/Compose). This lets us transition to patch-based
// repairs without breaking the model's ability to do full rewrites when needed.
type RepairResponse struct {
	// RepairPlan fields (preferred)
	Diagnosis  Diagnosis `json:"diagnosis,omitempty"`
	Changes    []Change  `json:"changes,omitempty"`
	Validation []string  `json:"validation,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	Unfixable  bool      `json:"unfixable,omitempty"`
	NeedsFiles []string  `json:"needs_files,omitempty"`
	// Fallback: full-file replacement (when the model can't produce patches)
	Dockerfile string `json:"dockerfile,omitempty"`
	Compose    string `json:"compose,omitempty"`
	Service    string `json:"service,omitempty"`
}

// hasChanges reports whether the response contains patch-based changes.
func (r *RepairResponse) hasChanges() bool {
	return len(r.Changes) > 0
}

// hasFullReplacement reports whether the response contains a full-file
// replacement (the legacy BuildFixResponse path).
func (r *RepairResponse) hasFullReplacement() bool {
	return r.Dockerfile != "" || r.Compose != ""
}

// toBuildFixResponse converts the fallback fields to the legacy format so
// the existing applyFix path can handle them.
func (r *RepairResponse) toBuildFixResponse() *llm.BuildFixResponse {
	return &llm.BuildFixResponse{
		Service:    r.Service,
		Dockerfile: r.Dockerfile,
		Compose:    r.Compose,
		Summary:    r.Summary,
		Unfixable:  r.Unfixable,
	}
}

// callLLMForRepair is the WIRED production LLM call that:
//  1. Uses pack.Render() as the structured user prompt (replacing the old
//     ad-hoc "FAILING SERVICE + raw Dockerfile + 80-line tail" format)
//  2. Asks the model for a RepairPlan (patch-based) with a fallback to
//     full-file replacement
//  3. Handles the NeedsFiles loop
//  4. Returns a RepairResponse that the caller can dispatch to ApplyPatch
//     (for changes) or applyFix (for full-file replacement)
func (l *Loop) callLLMForRepair(ctx context.Context, pack *ContextPack, read llm.FileReader, previousFixes []string) (*RepairResponse, error) {
	const sys = repairSystemPrompt

	// WIRE: pack.Render() produces the structured context — service metadata,
	// detection evidence, structured failure analysis, relevant files with
	// reasons, current Dockerfile + compose, and diff-aware previous attempts.
	user := pack.Render()

	if len(previousFixes) > 0 {
		user += "\n\nPREVIOUS FIX ATTEMPTS (do NOT repeat these):\n"
		for _, pf := range previousFixes {
			user += "- " + pf + "\n"
		}
	}

	for attempt := 1; attempt <= 3; attempt++ {
		raw, err := l.LLM.Call(ctx, sys, user)
		if err != nil {
			return nil, err
		}

		var resp RepairResponse
		if err := json.Unmarshal([]byte(llm.ExtractJSON(raw)), &resp); err != nil {
			if attempt < 3 {
				user += "\n\nYour previous response could not be parsed as JSON. Respond with a single JSON object only, no prose, no markdown, no backticks. Start with { and end with }."
				continue
			}
			return nil, fmt.Errorf("LLM repair response not parseable after 3 attempts: %w\nlast: %s", err, truncateForLog(raw, 400))
		}

		if resp.hasChanges() || resp.hasFullReplacement() || resp.Unfixable {
			return &resp, nil
		}

		if len(resp.NeedsFiles) > 0 && read != nil {
			user += renderRequestedFilesForRepair(resp.NeedsFiles, read)
			continue
		}

		return &resp, nil
	}
	return nil, fmt.Errorf("LLM repair did not converge in 3 rounds")
}

// repairSystemPrompt is the system prompt that asks for patch-based repairs
// with a fallback to full-file replacement. It supersedes the old
// FixBuildFailure prompt by requesting structured Changes[] instead of (or
// in addition to) full Dockerfile/compose replacement.
const repairSystemPrompt = `You are Yoink's repository environment repair agent.

Your objective is to diagnose the build failure and propose the SMALLEST evidence-backed change that fixes the root cause. You are operating inside a controlled engineering system. Yoink validates and applies your proposed changes — you do not edit files directly.

PRIORITY ORDER:
1. Preserve the repository's intended behavior.
2. Prefer deterministic fixes (version bumps, missing OS packages, path corrections).
3. Make the smallest change that resolves the actual root cause.
4. Never hide a failure merely to make the build pass.
5. Never disable type checking, remove healthchecks, or replace commands with sleep.
6. Do NOT repeat a fix that was already attempted.

LAYOUT INVARIANT (do not violate):
- docker-compose.yml lives at <repo>/yoink-outputs/docker-compose.yml.
- Each app service uses build.context: ".." (the repo root) and build.dockerfile: "yoink-outputs/Dockerfile.<service-id>".
- All COPY paths inside the Dockerfile are relative to the repo root.
- Never rewrite build.context to "." — that breaks monorepos.

ROOT CAUSE ANALYSIS:
- Distinguish symptoms from root causes. "Cannot find module X" might mean the wrong package is being built, not that X needs installing.
- "Requires-python >=3.14" → bump the FROM image. NEVER work around with --no-deps.
- Monorepo sub-package deps → install in the sub-package dir + copy node_modules to builder.

RESPONSE FORMATS:

Format A — request files to see real code:
{"needs_files": ["path/to/file1"]}

Format B — structured repair plan (PREFERRED):
{"diagnosis": {"category": "dependency", "root_cause": "...", "confidence": 0.9, "evidence": ["..."], "source_of_truth": "generator", "risk": "low"}, "changes": [{"file": "Dockerfile.service-2", "operation": "insert_after", "anchor": "RUN npm ci", "content": "RUN cd ingestion-worker && npm ci", "reason": "Install sub-package deps"}], "summary": "Added sub-package dependency installation", "validation": ["preflight", "docker build"]}

Format C — full-file replacement (FALLBACK, only when patches are insufficient):
{"service": "service-1", "dockerfile": "FROM ...\\n...", "summary": "Complete rewrite needed", "diagnosis": {"category": "configuration", "root_cause": "...", "confidence": 0.8}}

Format D — give up (rare):
{"unfixable": true, "summary": "Docker Hub unreachable"}

SUPPORTED OPERATIONS for changes[]:
- insert_after: insert content after the line matching "anchor"
- insert_before: insert before the line matching "anchor"
- replace_line: replace the line matching "anchor" with "content"
- replace_exact: replace the exact string "anchor" with "content"
- create_file: replace the entire file content

RULES:
- Prefer Format B (patch-based) over Format C (full replacement)
- Anchor must match EXACTLY ONE line — if it matches multiple, the patch is rejected
- Return the COMPLETE corrected Dockerfile only as a last resort (Format C)
- Maximum 5 files per needs_files request
- Respond ONLY with JSON — no markdown, no backticks, no prose
- Do NOT escape newlines beyond what JSON requires`

func renderRequestedFilesForRepair(needed []string, read llm.FileReader) string {
	if len(needed) > 5 {
		needed = needed[:5]
	}
	var b strings.Builder
	b.WriteString("\n\nREQUESTED FILES:\n")
	for _, p := range needed {
		content, err := read(p)
		if err != nil {
			fmt.Fprintf(&b, "\n=== %s ===\n[unavailable: %v]\n", p, err)
			continue
		}
		if len(content) > 16*1024 {
			content = content[:16*1024] + "\n[...truncated...]\n"
		}
		fmt.Fprintf(&b, "\n=== %s ===\n%s\n", p, content)
	}
	return b.String()
}

func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...[truncated]"
}
