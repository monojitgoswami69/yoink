package healer

import (
	"regexp"
	"strings"
)

// Failure is a structured representation of a build or runtime failure,
// extracted deterministically from Docker/Compose output. It replaces the
// raw "last 80 lines of build log" that was previously sent to the LLM,
// giving the model high-quality, categorized evidence instead of noise.
type Failure struct {
	Category      string   // "docker-build" | "dependency" | "compilation" | "configuration" | "environment" | "runtime" | "healthcheck" | "unknown"
	Service       string   // the failing service ID
	Stage         string   // Dockerfile stage (e.g. "builder", "deps")
	Command       string   // the build command that was run
	ExitCode      int      // exit code (usually 1)
	Error         string   // the primary error message (first meaningful error line)
	RelevantLog   string   // relevant log excerpt (around the error, ~15 lines)
	FileRefs      []string // file paths referenced in the error
	PackageRefs   []string // package/module names referenced (e.g. "@napi-rs/canvas")
	PathRefs      []string // COPY paths or other filesystem paths referenced
	EnvRefs       []string // env var names referenced (e.g. "JWT_SECRET")
	PortRefs      []string // port numbers referenced
	Progression   string   // "same_failure" | "progressed" | "regressed" | "new_failure" | "success" | "first"
	PrevDiagnosis string   // previous attempt's diagnosis (for diff-aware context)
}

// AnalyzeFailure parses raw Docker/Compose build output and extracts a
// structured Failure. This is the deterministic pre-processing that happens
// BEFORE the LLM is called — the model receives the results, not the raw
// log, so it can focus on reasoning instead of log archaeology.
func AnalyzeFailure(buildOut, service string) Failure {
	f := Failure{
		Service:     service,
		Command:     "docker compose build",
		ExitCode:    1,
		Progression: "first",
	}

	lines := strings.Split(buildOut, "\n")

	// Extract the primary error line (first meaningful error).
	f.Error = extractPrimaryError(lines)
	f.RelevantLog = extractRelevantLog(lines)
	f.Stage = extractStage(lines, service)
	f.Category = categorize(f.Error, lines)

	// Extract structured references.
	f.FileRefs = extractFileRefs(buildOut)
	f.PackageRefs = extractPackageRefs(buildOut)
	f.PathRefs = extractPathRefs(buildOut)
	f.EnvRefs = extractEnvRefs(buildOut)
	f.PortRefs = extractPortRefs(buildOut)

	return f
}

// Fingerprint produces a normalized digest of the failure for progression
// comparison. Normalizes away volatile details (paths, line numbers,
// timestamps, container IDs, BuildKit prefixes) so the same root failure
// is recognized even if the surrounding text varies.
func (f *Failure) Fingerprint() string {
	// Normalize: lowercase, strip paths, line numbers, BuildKit prefixes.
	fp := strings.ToLower(f.Error)
	// Strip file paths like "ingestion-worker/src/pipeline.ts(6,29):"
	fp = regexp.MustCompile(`[\w/.-]+\.\w+\(\d+,\d+\):`).ReplaceAllString(fp, "<file>:")
	// Strip BuildKit prefixes "#16 8.311 "
	fp = regexp.MustCompile(`#\d+\s+\d+\.\d+\s+`).ReplaceAllString(fp, "")
	// Strip absolute paths
	fp = regexp.MustCompile(`/[^\s"]+/`).ReplaceAllString(fp, "<path>")
	// Strip hex hashes
	fp = regexp.MustCompile(`[0-9a-f]{12,}`).ReplaceAllString(fp, "<hash>")
	return f.Category + ":" + fp
}

// ClassifyProgression compares the current failure to the previous one and
// returns a progression classification. Uses Fingerprint() for normalized
// comparison instead of raw error string equality.
func ClassifyProgression(current, previous *Failure) string {
	if previous == nil {
		return "first"
	}
	// Fingerprint comparison: same root failure?
	if current.Fingerprint() == previous.Fingerprint() {
		return "same_failure"
	}
	// Category change = progressed to a new stage.
	if current.Category != previous.Category {
		return "progressed"
	}
	// Same category, different fingerprint — new failure in same stage.
	return "new_failure"
}

// --- extraction helpers ---

// specificErrorRe matches lines with concrete, actionable error information
// (TS errors, module-not-found, Python exceptions, Next.js build failures,
// missing environment variables, etc.).
var specificErrorRe = regexp.MustCompile(`(?i)(?:error TS\d+:|Cannot find module|ModuleNotFoundError|ImportError|RuntimeError|Exception|requires a different Python|CRITICAL.*ERROR|No module named|failed to calculate checksum|not found|does not exist|is not defined|is undefined|missing environment variable|failed to collect page data|error occurred prerendering|build error occurred)`)

// genericErrorRe matches generic BuildKit/compose wrapper lines that
// always appear at the bottom of a failed build. These are NOT the primary
// error — the real error is above them.
var genericErrorRe = regexp.MustCompile(`(?i)(?:failed to solve|returned a non-zero code|exited with code)`)

func extractPrimaryError(lines []string) string {
	// Phase 1: walk bottom-up, skip generic wrappers, return the first
	// line matching a SPECIFIC error pattern.
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if genericErrorRe.MatchString(line) {
			continue // skip BuildKit wrapper — the real error is above
		}
		if specificErrorRe.MatchString(line) {
			return stripBuildKitPrefix(line)
		}
	}
	// Phase 2: if no specific error found, return the first generic error
	// (or last non-empty line) as a fallback.
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if genericErrorRe.MatchString(line) {
			return stripBuildKitPrefix(line)
		}
	}
	// Phase 3: last non-empty, non-comment line.
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	return "unknown error"
}

func extractRelevantLog(lines []string) string {
	// Find the error line index (specific first, then generic).
	errIdx := -1
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if genericErrorRe.MatchString(line) {
			continue
		}
		if specificErrorRe.MatchString(line) {
			errIdx = i
			break
		}
	}
	if errIdx < 0 {
		// Try generic error as fallback.
		for i := len(lines) - 1; i >= 0; i-- {
			if genericErrorRe.MatchString(strings.TrimSpace(lines[i])) {
				errIdx = i
				break
			}
		}
	}
	if errIdx < 0 {
		start := len(lines) - 15
		if start < 0 {
			start = 0
		}
		return strings.Join(lines[start:], "\n")
	}
	start := errIdx - 5
	if start < 0 {
		start = 0
	}
	end := errIdx + 10
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start:end], "\n")
}

var stageRe = regexp.MustCompile(`\[([a-z0-9-]+)\s+(\w+)\s+\d+/\d+\]`)

func extractStage(lines []string, service string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if m := stageRe.FindStringSubmatch(lines[i]); len(m) == 3 {
			if m[1] == service || service == "" {
				return m[2] // the stage name (e.g. "builder", "deps")
			}
		}
	}
	return ""
}

func categorize(errorLine string, lines []string) string {
	low := strings.ToLower(errorLine)
	// Check specific patterns BEFORE "failed to solve" (which is a generic
	// BuildKit wrapper that also appears in COPY/config errors).
	// "error TS" is checked before "cannot find module" because TS2307
	// errors say "Cannot find module" but are fundamentally compilation
	// errors, not dependency issues.
	switch {
	case strings.Contains(low, "error ts"):
		return "compilation"
	case strings.Contains(low, "copy") && (strings.Contains(low, "not found") || strings.Contains(low, "does not exist") || strings.Contains(low, "checksum")):
		return "configuration"
	case strings.Contains(low, "checksum") && strings.Contains(low, "not found"):
		return "configuration"
	case strings.Contains(low, "cannot find module") || strings.Contains(low, "modulenotfounderror"):
		return "dependency"
	case strings.Contains(low, "requires a different python") || strings.Contains(low, "requires-python"):
		return "environment"
	case strings.Contains(low, "is not defined") || strings.Contains(low, "is undefined") || strings.Contains(low, "missing environment variable") || strings.Contains(low, "env variable"):
		return "missing-environment"
	case strings.Contains(low, "failed to collect page data") || strings.Contains(low, "error occurred prerendering") || strings.Contains(low, "build error occurred"):
		return "nextjs-build"
	case strings.Contains(low, "failed to solve") || strings.Contains(low, "returned a non-zero code"):
		return "docker-build"
	case strings.Contains(low, "connection refused") || strings.Contains(low, "could not connect"):
		return "runtime"
	case strings.Contains(low, "unhealthy") || strings.Contains(low, "healthcheck"):
		return "healthcheck"
	}
	// If the primary error line is generic, scan all lines for a more
	// specific category.
	for _, line := range lines {
		ll := strings.ToLower(line)
		if strings.Contains(ll, "failed to collect page data") || strings.Contains(ll, "error occurred prerendering") {
			return "nextjs-build"
		}
		if strings.Contains(ll, "is not defined") || strings.Contains(ll, "missing environment variable") {
			return "missing-environment"
		}
		if strings.Contains(ll, "copy") && (strings.Contains(ll, "not found") || strings.Contains(ll, "checksum")) {
			return "configuration"
		}
		if strings.Contains(ll, "error ts") {
			return "compilation"
		}
	}
	return "unknown"
}

var (
	fileRefRe    = regexp.MustCompile(`([a-zA-Z0-9_][\w/.-]*\.(?:ts|tsx|js|jsx|py|go|rs|java|rb|php))\(\d+`)
	packageRefRe = regexp.MustCompile(`(?:Cannot find module|No module named)\s+['"]?([@a-zA-Z0-9_][\w/.-]*)['"]?`)
	pathRefRe    = regexp.MustCompile(`"(\/[^"]+)"`)
	// envRefRe catches env var names near "missing"/"not defined"/"required".
	envRefRe = regexp.MustCompile(`\b([A-Z][A-Z0-9_]{3,})\b.*(?:missing|not set|not found|required|not provided)`)
	// envNotDefinedRe catches "X is not defined" / "X is undefined" where X
	// is an uppercase env-var-like name (at least 4 chars).
	envNotDefinedRe = regexp.MustCompile(`\b([A-Z][A-Z0-9_]{3,})\s+is\s+(?:not\s+defined|undefined)`)
	portRefRe       = regexp.MustCompile(`:(\d{4,5})\b`)
)

func extractFileRefs(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range fileRefRe.FindAllStringSubmatch(s, -1) {
		if len(m) == 2 && !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

func extractPackageRefs(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range packageRefRe.FindAllStringSubmatch(s, -1) {
		if len(m) == 2 && !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

func extractPathRefs(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range pathRefRe.FindAllStringSubmatch(s, -1) {
		if len(m) == 2 && !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

func extractEnvRefs(s string) []string {
	seen := map[string]bool{}
	var out []string
	// Pattern 1: "X is missing"/"X is not defined"/"X is required"
	for _, m := range envRefRe.FindAllStringSubmatch(s, -1) {
		if len(m) == 2 && !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	// Pattern 2: "X is not defined" / "X is undefined"
	for _, m := range envNotDefinedRe.FindAllStringSubmatch(s, -1) {
		if len(m) == 2 && !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

func extractPortRefs(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range portRefRe.FindAllStringSubmatch(s, -1) {
		if len(m) == 2 && !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

func stripBuildKitPrefix(line string) string {
	// "#16 8.311 error TS2307: ..." → "error TS2307: ..."
	if strings.HasPrefix(line, "#") {
		parts := strings.SplitN(line, " ", 3)
		if len(parts) >= 3 {
			return parts[2]
		}
	}
	return line
}
