package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// BuildFixResponse is what FixBuildFailure returns. Exactly one of the
// (dockerfile, compose) fields should be set for the field the model wants
// to change. If NeedsFiles is set the caller should refetch and call again.
type BuildFixResponse struct {
	// NeedsFiles asks the caller to read repo files and resubmit. Mirrors the
	// existing validation flow.
	NeedsFiles []string `json:"needs_files,omitempty"`
	// Service is the compose service ID the fix applies to. It may be empty
	// when the compose-level change spans services.
	Service string `json:"service,omitempty"`
	// Dockerfile, when non-empty, replaces the Dockerfile for `Service`.
	Dockerfile string `json:"dockerfile,omitempty"`
	// Compose, when non-empty, replaces docker-compose.yml.
	Compose string `json:"compose,omitempty"`
	// Summary is a 1-line explanation of the change for human consumption.
	Summary string `json:"summary,omitempty"`
	// Unfixable is true when the model decides the failure isn't something
	// it can resolve (e.g. a network outage during apt-get). The loop should
	// stop rather than spin.
	Unfixable bool `json:"unfixable,omitempty"`
}

func (r *BuildFixResponse) hasFix() bool {
	return r.Dockerfile != "" || r.Compose != ""
}

// FixBuildFailure asks the LLM to repair a Docker build failure. The model
// receives the current Dockerfile and compose for the failing service plus
// the tail of the build output and is asked to return either a new
// Dockerfile, a new compose, or a "needs_files" request.
//
// failingService should be the compose service ID the failure was attributed
// to ("" is acceptable if we couldn't extract one).
func (c *Client) FixBuildFailure(
	ctx context.Context,
	tree, dockerfile, compose, failingService, errorTail string,
	read FileReader,
	previousFixes []string,
) (*BuildFixResponse, error) {
	const sys = `You are Yoink's repository environment repair agent.

Your objective is to make the target repository build and run correctly inside Yoink's generated environment. You are operating inside a deterministic engineering system. You are NOT responsible for inventing the entire environment from scratch. You are responsible for diagnosing failures and making the smallest correct changes necessary.

PRIORITY ORDER:
1. Preserve the repository's intended behavior.
2. Preserve existing working configuration.
3. Prefer deterministic fixes (version bumps, missing OS packages, path corrections).
4. Prefer changes supported by repository evidence.
5. Make the smallest change that resolves the actual root cause.
6. Never hide a failure merely to make the build pass.
7. Never invent dependencies without evidence.
8. Never remove functionality merely to make a build succeed.
9. Never expose secrets.
10. Never overwrite newer changes with stale context.
11. Do not repeatedly apply the same unsuccessful strategy.
12. Stop when the environment is actually healthy.

LAYOUT INVARIANT (do not violate):
- docker-compose.yml lives at <repo>/yoink-outputs/docker-compose.yml.
- Each app service uses build.context: ".." (the repo root) and build.dockerfile: "yoink-outputs/Dockerfile.<service-id>".
- All COPY paths inside the Dockerfile are therefore relative to the repo root.
- Never rewrite build.context to "." — that breaks monorepos and breaks paths.
- "COPY ... not found" almost always means the COPY path is wrong, NOT that the context needs to change. Fix the COPY path.

ROOT CAUSE ANALYSIS:
- Distinguish symptoms from root causes. "Cannot find module X" might mean the wrong package is being built, not that X needs installing.
- "DATABASE_URL missing" — check if the variable is declared, if there's a generated postgres service, whether it's needed at build or runtime.
- A version mismatch error ("requires a different Python: 3.12 not in '>=3.14'") → bump the base image. NEVER work around it with --no-deps or --no-build-isolation.

Common failure modes:
- Missing OS package (apt/apk) for a native wheel (libpq-dev + gcc for psycopg, libffi-dev for cryptography)
- Wrong Python/Node version → bump the FROM image to the required version
- Lockfile mismatch (npm ci fails → switch to npm install if no lockfile)
- Missing files in COPY (fix the COPY path, not the context)
- Wrong start command for the framework (uvicorn target module, next start vs node server)
- Monorepo sub-package deps not installed (cd <subdir> && npm ci + COPY the sub-package's node_modules)
- Build-time env validation (app checks JWT_SECRET/DATABASE_URL during next build → add ENV directives)
- pyproject.toml/Pipfile projects needing COPY of those files instead of requirements.txt

Your response MUST be valid JSON in ONE of these formats:

Format A — request files to see real code:
{"needs_files": ["path/to/file1", "path/to/file2"]}

Format B — return a corrected Dockerfile:
{"service": "service-1", "dockerfile": "FROM ...\\n...", "summary": "Added libffi-dev for bcrypt", "diagnosis": "Missing OS package for bcrypt C extension"}

Format C — return a corrected compose:
{"compose": "services:\\n...\\n", "summary": "Removed conflicting port binding", "diagnosis": "Duplicate port binding"}

Format D — give up (rare; only when the failure isn't a build problem):
{"unfixable": true, "summary": "Build failed because Docker Hub is unreachable"}

RULES:
- Maximum 5 files per request
- Only request files that materially affect the fix
- Respond ONLY with the JSON object — no markdown, no backticks, no prose
- Do NOT escape newlines in the dockerfile/compose strings beyond what JSON requires
- Do NOT repeat a fix that was already attempted (see PREVIOUS FIXES below)
- When returning a Dockerfile, return the COMPLETE corrected Dockerfile, not a diff
- Preserve the LAYOUT INVARIANT in any compose you return`

	user := fmt.Sprintf(
		"FAILING SERVICE: %s\n\nDOCKERFILE:\n%s\n\nDOCKER-COMPOSE.YML:\n%s\n\nBUILD ERROR (tail):\n%s\n\nTREE:\n%s",
		failingService,
		dockerfile,
		compose,
		errorTail,
		tree,
	)

	if len(previousFixes) > 0 {
		user += "\n\nPREVIOUS FIX ATTEMPTS (do NOT repeat these):\n"
		for _, pf := range previousFixes {
			user += "- " + pf + "\n"
		}
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		raw, err := c.Call(ctx, sys, user)
		if err != nil {
			return nil, err
		}

		var resp BuildFixResponse
		if err := json.Unmarshal([]byte(ExtractJSON(raw)), &resp); err != nil {
			if attempt < maxAttempts {
				user += "\n\nYour previous response could not be parsed as JSON. Respond with a single JSON object only, no prose, no markdown, no backticks. Start with { and end with }."
				continue
			}
			return nil, fmt.Errorf("LLM heal response not parseable after %d attempts: %w\nlast: %s", maxAttempts, err, truncateForLog(raw, 400))
		}

		if resp.hasFix() || resp.Unfixable {
			return &resp, nil
		}

		if len(resp.NeedsFiles) > 0 && read != nil {
			user += renderRequestedFiles(resp.NeedsFiles, read)
			continue
		}

		// Empty response — bail rather than loop.
		return &resp, nil
	}
	return nil, fmt.Errorf("LLM heal did not converge in %d rounds", maxAttempts)
}

// CleanContent strips the kind of markdown fences models occasionally wrap
// large content in. It is permissive: a plain block is returned unchanged.
func CleanContent(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if nl := strings.IndexByte(s, '\n'); nl >= 0 {
			s = s[nl+1:]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
	}
	return strings.TrimSpace(s) + "\n"
}
