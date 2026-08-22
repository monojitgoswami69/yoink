package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	maxAttempts    = 3
	maxFilesPerReq = 5
	maxFileBytes   = 16 * 1024
)

// FileReader is supplied by the caller and is constrained to the repo root so
// the LLM cannot read arbitrary files on disk.
type FileReader func(path string) (string, error)

// ValidationResponse is the schema we ask all validators to return.
type ValidationResponse struct {
	Valid               bool                 `json:"valid"`
	NeedsFiles          []string             `json:"needs_files,omitempty"`
	CorrectedServices   []CorrectedService   `json:"corrected_services,omitempty"`
	CorrectedDockerfile string               `json:"corrected_dockerfile,omitempty"`
	CorrectedCompose    string               `json:"corrected_compose,omitempty"`
	CorrectedEnvVars    []CorrectedEnvResult `json:"corrected_env_vars,omitempty"`
}

// CorrectedService is a single service the LLM proposes in place of detector output.
type CorrectedService struct {
	Type       string  `json:"type"`
	Directory  string  `json:"directory"`
	Framework  string  `json:"framework"`
	Confidence float64 `json:"confidence"`
}

// CorrectedEnvResult is a rewritten .env.example file the LLM proposes.
type CorrectedEnvResult struct {
	Directory   string `json:"directory"`
	ServiceType string `json:"service_type"`
	Technology  string `json:"technology"`
	EnvContent  string `json:"env_content"`
}

func (v *ValidationResponse) hasCorrections() bool {
	return len(v.CorrectedServices) > 0 ||
		v.CorrectedDockerfile != "" ||
		v.CorrectedCompose != "" ||
		len(v.CorrectedEnvVars) > 0
}

// ValidateDetection asks the LLM to confirm or correct the static detection.
func (c *Client) ValidateDetection(ctx context.Context, tree, analysis string, read FileReader) (*ValidationResponse, error) {
	const sys = `You are a senior DevOps engineer validating static analysis of a code repository.

Your role is to CONFIRM or CORRECT the static detection. Be conservative: the static detector is usually right. Only propose a correction when you have concrete evidence (an actual dependency or entry file) that the static analysis got wrong.

Framework-classification pitfalls to avoid:
- "next-themes", "next-intl", "next-seo" are LIBRARIES, not the Next.js framework. Only classify as "next" when the "next" package itself is a dependency.
- "vite" as a devDependency with "@vitejs/plugin-react-swc" or "build": "vite build" means a Vite app — NOT Next.
- "@remix-run/*", "@sveltejs/kit", "nuxt", "astro" are the real framework markers.
- Do NOT change a framework the static analysis already identified correctly just to "tidy up".

Your response must be valid JSON in one of these formats:

Format A (need files to validate):
{"valid": false, "needs_files": ["path/to/file1"]}

Format B (validation passed):
{"valid": true}

Format C (corrections needed):
{"valid": false, "corrected_services": [
  {"type": "frontend|backend", "directory": "relative/path", "framework": "next|express|node|fastapi|flask|django|vite|react", "confidence": 0.95}
]}

RULES:
- Request files if you need to see actual code to validate
- Maximum 5 files per request
- Only request files that materially affect the outcome
- Only return a correction with confidence >= 0.9 when you have direct evidence
- Respond ONLY with valid JSON, no markdown, no backticks`
	user := fmt.Sprintf("Validate this static analysis:\n\nTREE:\n%s\n\nANALYSIS:\n%s", tree, analysis)
	return c.runValidationLoop(ctx, sys, user, read)
}

// ValidateDockerfiles asks the LLM to confirm or correct generated Docker configs.
func (c *Client) ValidateDockerfiles(ctx context.Context, tree, dockerfile, compose string, read FileReader) (*ValidationResponse, error) {
	const sys = `You are a senior DevOps engineer validating generated Dockerfile and docker-compose.yml.

LAYOUT INVARIANT (do not violate):
- docker-compose.yml lives at <repo>/yoink-outputs/docker-compose.yml.
- Each app service uses build.context: ".." (the repo root) and build.dockerfile: "yoink-outputs/Dockerfile.<service-id>".
- All COPY paths inside the Dockerfile are therefore relative to the repo root (e.g. COPY requirements.txt ./ when the file is at the repo root; COPY apps/web/package*.json ./ for a monorepo).
- Never rewrite build.context to "." — that would break monorepo support and break paths.
- Never move the Dockerfile out of the yoink-outputs directory.
- If a COPY fails because the path is wrong, fix the COPY path, not the context.

Your response must be valid JSON in one of these formats:

Format A (need files):
{"valid": false, "needs_files": ["path/to/file"]}

Format B (valid):
{"valid": true}

Format C (corrections):
{"valid": false, "corrected_dockerfile": "...", "corrected_compose": "..."}

Only return corrected_compose when the compose itself has a real defect (e.g. missing depends_on, conflicting port). Do NOT return corrected_compose merely to "tidy up" the build context or paths.

Respond ONLY with valid JSON, no markdown, no backticks.`
	user := fmt.Sprintf("Validate these Docker configurations:\n\nTREE:\n%s\n\nDOCKERFILE:\n%s\n\nCOMPOSE:\n%s", tree, dockerfile, compose)
	return c.runValidationLoop(ctx, sys, user, read)
}

// ValidateEnvVars asks the LLM to produce richer .env.example files.
func (c *Client) ValidateEnvVars(ctx context.Context, tree, envAnalysis, existingEnvFiles string, read FileReader) (*ValidationResponse, error) {
	const sys = `You are a senior DevOps engineer creating .env.example files.

Your response must be valid JSON in one of these formats:

Format A (need files):
{"valid": false, "needs_files": ["path/to/file"]}

Format B (corrections with full .env.example content):
{"valid": false, "corrected_env_vars": [
  {"directory": "path", "service_type": "frontend|backend", "technology": "next|fastapi|...", "env_content": "# Database\nDATABASE_URL=postgresql://...\n\n# Auth\nSECRET_KEY=..."}
]}

RULES:
- Create complete .env.example files with proper comments
- Group related variables with comment headers
- Provide example values that show the expected format
- Include all detected variables plus any common ones for the technology
- Respond ONLY with valid JSON, no markdown, no backticks`
	user := fmt.Sprintf("Create .env.example files for these services:\n\nTREE:\n%s\n\nDETECTED VARS:\n%s\n\nEXISTING ENV FILES:\n%s\n\nGenerate complete, well-commented .env.example files.", tree, envAnalysis, existingEnvFiles)
	return c.runValidationLoop(ctx, sys, user, read)
}

func (c *Client) runValidationLoop(ctx context.Context, systemPrompt, userPrompt string, read FileReader) (*ValidationResponse, error) {
	served := map[string]bool{} // files the model has already been shown
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		raw, err := c.Call(ctx, systemPrompt, userPrompt)
		if err != nil {
			return nil, err
		}

		var resp ValidationResponse
		if err := json.Unmarshal([]byte(ExtractJSON(raw)), &resp); err != nil {
			if attempt < maxAttempts {
				userPrompt += "\n\nYour previous response could not be parsed as JSON. Respond with a single JSON object only, no prose, no markdown, no backticks. Start with { and end with }."
				continue
			}
			return nil, fmt.Errorf("failed to parse LLM response as JSON after %d attempts: %w\nlast response: %s", maxAttempts, err, truncateForLog(raw, 400))
		}

		if resp.Valid || resp.hasCorrections() {
			return &resp, nil
		}

		if len(resp.NeedsFiles) > 0 && read != nil {
			// Only (re)serve files the model hasn't seen yet. Re-requesting
			// already-served files is the main cause of non-convergence, so
			// when there's nothing new to show we instead nudge the model to
			// decide rather than spin forever.
			newFiles := make([]string, 0, len(resp.NeedsFiles))
			for _, p := range resp.NeedsFiles {
				if !served[p] {
					newFiles = append(newFiles, p)
					served[p] = true
				}
			}
			if len(newFiles) > 0 {
				userPrompt += renderRequestedFiles(newFiles, read)
			} else {
				userPrompt += "\n\nYou have already been shown every file you requested. Do not request more files. Respond now with a final JSON verdict: {\"valid\": true} if the configuration is acceptable, or the corrections object."
			}
			continue
		}

		// {"valid": false} with no corrections and no file request: treat as
		// "no opinion" so we don't drop the static output.
		return &ValidationResponse{Valid: true}, nil
	}
	return nil, fmt.Errorf("LLM did not converge after %d rounds (keeping static output)", maxAttempts)
}

func renderRequestedFiles(needed []string, read FileReader) string {
	if len(needed) > maxFilesPerReq {
		needed = needed[:maxFilesPerReq]
	}
	var b strings.Builder
	b.WriteString("\n\nREQUESTED FILES:\n")
	for _, p := range needed {
		content, err := read(p)
		if err != nil {
			fmt.Fprintf(&b, "\n=== %s ===\n[unavailable: %v]\n", p, err)
			continue
		}
		if len(content) > maxFileBytes {
			content = content[:maxFileBytes] + "\n[...truncated...]\n"
		}
		fmt.Fprintf(&b, "\n=== %s ===\n%s\n", p, content)
	}
	return b.String()
}
