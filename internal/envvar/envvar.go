// Package envvar finds environment variables referenced in source code.
package envvar

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"yoink/internal/detector"
)

// JS/TS patterns. Each pattern captures group #1 as the variable name.
// IMPORTANT: NEXT_PUBLIC_ matches must capture the whole name including the prefix.
var jsPatterns = []*regexp.Regexp{
	regexp.MustCompile(`process\.env\.([A-Z][A-Z0-9_]*)`),
	regexp.MustCompile(`process\.env\[['"]([A-Z][A-Z0-9_]*)['"]\]`),
	regexp.MustCompile(`import\.meta\.env\.([A-Z][A-Z0-9_]*)`),
	regexp.MustCompile(`import\.meta\.env\[['"]([A-Z][A-Z0-9_]*)['"]\]`),
	regexp.MustCompile(`\b(NEXT_PUBLIC_[A-Z0-9_]+)\b`),
	regexp.MustCompile(`\b(VITE_[A-Z0-9_]+)\b`),
	regexp.MustCompile(`\b(REACT_APP_[A-Z0-9_]+)\b`),
}

// Python patterns. The lookups that take a default argument
// (os.getenv("X", default), os.environ.get("X", default)) end with `,` rather
// than `)`, so the patterns stop at the closing quote instead of requiring a
// closing paren — otherwise the common `os.getenv("FOO", "bar")` idiom is
// missed.
var pythonPatterns = []*regexp.Regexp{
	regexp.MustCompile(`os\.environ\.get\(['"]([A-Z][A-Z0-9_]*)['"]`),
	regexp.MustCompile(`os\.environ\[['"]([A-Z][A-Z0-9_]*)['"]\]`),
	regexp.MustCompile(`os\.getenv\(['"]([A-Z][A-Z0-9_]*)['"]`),
	regexp.MustCompile(`getenv\(['"]([A-Z][A-Z0-9_]*)['"]`),
	regexp.MustCompile(`env\(['"]([A-Z][A-Z0-9_]*)['"]`),
	regexp.MustCompile(`config\(['"]([A-Z][A-Z0-9_]*)['"]`),
}

var excludedDirs = map[string]bool{
	"node_modules":  true,
	".git":          true,
	".next":         true,
	".nuxt":         true,
	"dist":          true,
	"build":         true,
	"__pycache__":   true,
	".venv":         true,
	"venv":          true,
	"env":           true,
	".cache":        true,
	"vendor":        true,
	".pytest_cache": true,
	"yoink-outputs": true,
	".idea":         true,
	".vscode":       true,
}

// Result holds env vars detected for a single service.
type Result struct {
	ServiceID   string   `json:"service_id"`
	Directory   string   `json:"directory"`
	ServiceType string   `json:"service_type"`
	Technology  string   `json:"technology"`
	Vars        []EnvVar `json:"vars"`
	EnvContent  string   `json:"env_content,omitempty"` // populated when LLM rewrites the .env.example
	// Deps are lowercased runtime dependency names (from package.json or
	// requirements.txt/pyproject.toml). Used by infra inference as a second
	// evidence source: e.g. "psycopg" → postgres, "redis" → redis, even
	// when no DATABASE_URL env var is referenced.
	Deps []string `json:"deps,omitempty"`
}

// EnvVar is a single discovered variable.
type EnvVar struct {
	Name        string   `json:"name"`
	Occurrences int      `json:"occurrences"`
	Files       []string `json:"files"`
	// Classification is the determined role of this variable.
	// "public" (NEXT_PUBLIC_*, VITE_*), "private" (non-secret config),
	// "secret" (contains KEY/SECRET/PASSWORD/TOKEN), "unknown".
	Classification string `json:"classification,omitempty"`
	// BuildTime is true when the variable is likely accessed during build
	// (e.g. in Next.js getStaticProps, generateStaticParams, generateMetadata,
	// or top-level server component code). Conservative: false = unknown.
	BuildTime bool `json:"build_time,omitempty"`
	// Status is an evidence-based requirement classification. Static discovery
	// alone produces UNKNOWN; runtime/build evidence can promote a variable to
	// REQUIRED without treating every template entry as mandatory.
	Status string `json:"status,omitempty"`
	// Value is populated only for repository-provided non-secret templates.
	Value       string   `json:"value,omitempty"`
	Provided    bool     `json:"provided,omitempty"`
	Placeholder bool     `json:"placeholder,omitempty"`
	Evidence    []string `json:"evidence,omitempty"`
}

// Detect scans each service's directory for env-var references.
func Detect(rootDir string, services []detector.Service) []Result {
	results := make([]Result, 0, len(services))
	for _, svc := range services {
		serviceDir := filepath.Join(rootDir, svc.Directory)
		vars := scanDirectory(serviceDir, svc.Language, rootDir)
		r := Result{
			ServiceID:   svc.ID,
			Directory:   svc.Directory,
			ServiceType: svc.Type,
			Technology:  svc.Framework,
			Vars:        vars,
			EnvContent:  existingEnvContent(serviceDir),
		}
		// Populate Deps for infra inference (second evidence source:
		// package deps → backing service, even without env vars).
		switch svc.Language {
		case "python":
			r.Deps = svc.PythonDeps
		case "javascript", "typescript":
			r.Deps = readJSDeps(filepath.Join(serviceDir, "package.json"))
		}
		// Classify each variable: public/private/secret, build-time/runtime.
		classifyVars(r.Vars, serviceDir, svc.Framework, svc.Language)
		applyTemplateMetadata(r.Vars, serviceDir)
		results = append(results, r)
	}
	return results
}

const (
	StatusProvidedDefault = "PROVIDED_DEFAULT"
	StatusRequired        = "REQUIRED"
	StatusOptional        = "OPTIONAL"
	StatusFeatureSpecific = "FEATURE_SPECIFIC"
	StatusUnknown         = "UNKNOWN"
)

// classifyVars enriches each EnvVar with classification and build-time
// assessment based on the variable name and its usage context.
func classifyVars(vars []EnvVar, serviceDir, framework, language string) {
	for i := range vars {
		v := &vars[i]
		v.Status = StatusUnknown
		// Classification by name.
		upper := strings.ToUpper(v.Name)
		switch {
		case strings.HasPrefix(upper, "NEXT_PUBLIC_") || strings.HasPrefix(upper, "VITE_") || strings.HasPrefix(upper, "REACT_APP_"):
			v.Classification = "public"
		case isSecretName(upper):
			v.Classification = "secret"
		default:
			v.Classification = "private"
		}
		// Build-time assessment: only for JS/TS frameworks where Next.js
		// static generation can execute application code at build time.
		if language == "javascript" || language == "typescript" {
			v.BuildTime = assessBuildTime(v, serviceDir, framework)
		}
	}
}

// applyTemplateMetadata attaches safe repository-provided values to detected
// variables. Real .env files are read only for names; their values are never
// copied into generated output or exposed in diagnostics.
func applyTemplateMetadata(vars []EnvVar, serviceDir string) {
	values := map[string]string{}
	for _, name := range []string{".env.example", ".env.sample", ".env.template", ".env.local.example"} {
		readEnvAssignments(filepath.Join(serviceDir, name), values)
	}
	for i := range vars {
		v := &vars[i]
		if value, ok := values[v.Name]; ok && strings.TrimSpace(value) != "" {
			v.Provided = true
			v.Value = value
			v.Status = StatusProvidedDefault
			v.Placeholder = obviousPlaceholder(value)
			v.Evidence = append(v.Evidence, "repository template provides a value")
		}
	}
}

func readEnvAssignments(path string, values map[string]string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(key) != "" {
			values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"'")
		}
	}
}

func obviousPlaceholder(value string) bool {
	low := strings.ToLower(strings.TrimSpace(value))
	if low == "" {
		return false
	}
	for _, marker := range []string{"placeholder", "your-", "change-me", "example", "dummy", "test", "xxx", "your_"} {
		if strings.Contains(low, marker) {
			return true
		}
	}
	return false
}

// isSecretName checks if the variable name looks like a secret.
// Mirrors the generator's isSecretEnvVar but lives here for classification.
func isSecretName(upper string) bool {
	for _, marker := range []string{"SECRET", "PASSWORD", "TOKEN", "CREDENTIAL", "PRIVATE_KEY", "API_KEY"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

// nextBuildTimePatterns are function/method names that Next.js executes
// during static generation (build time). If process.env.X appears inside
// or near these patterns, X is a build-time candidate.
var nextBuildTimePatterns = []string{
	"getStaticProps", "getStaticPaths", "generateStaticParams",
	"generateMetadata", "getServerSideProps",
}

// assessBuildTime checks whether a variable is likely accessed during
// Next.js build-time (static generation). Conservative: only marks true
// when the variable appears in a file that contains build-time patterns.
func assessBuildTime(v *EnvVar, serviceDir, framework string) bool {
	if framework != "next" && framework != "nuxt" {
		return false // Only Next.js/Nuxt do static generation at build time
	}
	for _, file := range v.Files {
		data, err := os.ReadFile(filepath.Join(serviceDir, file))
		if err != nil {
			continue
		}
		content := string(data)
		for _, pattern := range nextBuildTimePatterns {
			if strings.Contains(content, pattern) {
				return true
			}
		}
	}
	return false
}

// readJSDeps reads a package.json and returns lowercased dependency names
// from both dependencies and devDependencies. Used by infra inference.
func readJSDeps(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var pkg struct {
		Dependencies    map[string]any `json:"dependencies"`
		DevDependencies map[string]any `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}
	deps := make([]string, 0, len(pkg.Dependencies)+len(pkg.DevDependencies))
	for k := range pkg.Dependencies {
		deps = append(deps, strings.ToLower(k))
	}
	for k := range pkg.DevDependencies {
		deps = append(deps, strings.ToLower(k))
	}
	return deps
}

const maxSourceFileBytes = 1 << 20

var (
	jsExts     = map[string]bool{".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".mjs": true, ".cjs": true}
	pythonExts = map[string]bool{".py": true}
)

func scanDirectory(dir, language, rootDir string) []EnvVar {
	vars := map[string]*EnvVar{}
	scanEnvExampleFiles(dir, vars)

	patterns, exts := jsPatterns, jsExts
	if language == "python" {
		patterns, exts = pythonPatterns, pythonExts
	}

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if excludedDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !exts[filepath.Ext(path)] {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxSourceFileBytes {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(rootDir, path)
		text := string(data)
		for _, pat := range patterns {
			for _, m := range pat.FindAllStringSubmatch(text, -1) {
				name := strings.TrimSpace(m[1])
				if name == "" {
					continue
				}
				record(vars, name, rel)
			}
		}
		// Python: also scan pydantic-settings field declarations. Modern
		// FastAPI/Django apps declare env vars as BaseSettings class fields
		// (e.g. `DATABASE_URL: str`) rather than os.getenv calls, which the
		// regex patterns above miss entirely.
		if language == "python" {
			for _, name := range scanPydanticSettings(text) {
				record(vars, name, rel)
			}
		}
		return nil
	})

	out := make([]EnvVar, 0, len(vars))
	for _, v := range vars {
		sort.Strings(v.Files)
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func record(vars map[string]*EnvVar, name, source string) {
	if v, ok := vars[name]; ok {
		v.Occurrences++
		if !contains(v.Files, source) {
			v.Files = append(v.Files, source)
		}
		return
	}
	vars[name] = &EnvVar{Name: name, Occurrences: 1, Files: []string{source}}
}

func scanEnvExampleFiles(dir string, vars map[string]*EnvVar) {
	for _, name := range []string{".env.example", ".env.sample", ".env.template", ".env.local.example"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, _, ok := strings.Cut(line, "=")
			key = strings.TrimSpace(key)
			if !ok || key == "" {
				continue
			}
			record(vars, key, name)
		}
	}
}

// existingEnvContent preserves committed templates as the authoritative base
// for generated env files. This keeps safe defaults such as booleans, integer
// ports, and disabled integrations from becoming invalid empty assignments.
func existingEnvContent(dir string) string {
	for _, name := range []string{".env.example", ".env.sample", ".env.template", ".env.local.example"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err == nil && strings.TrimSpace(string(data)) != "" {
			return string(data)
		}
	}
	return ""
}

// MergeProvidedValues keeps non-empty repository template assignments when a
// later semantic/template rewrite omits them or leaves them blank.
func MergeProvidedValues(original, proposed string) string {
	if strings.TrimSpace(original) == "" || strings.TrimSpace(proposed) == "" {
		return proposed
	}
	values := map[string]string{}
	readEnvAssignmentsFromContent(original, values)
	var b strings.Builder
	for i, line := range strings.Split(proposed, "\n") {
		trimmed := strings.TrimSpace(line)
		if key, value, ok := strings.Cut(trimmed, "="); ok {
			key = strings.TrimSpace(key)
			if strings.TrimSpace(value) == "" && values[key] != "" {
				line = key + "=" + values[key]
			}
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	return b.String()
}

// EnsureCommonVars preserves repository templates while adding only missing
// technology defaults needed by deterministic infra/generation inference.
func EnsureCommonVars(existing string, vars []EnvVar, technology string) string {
	if strings.TrimSpace(existing) == "" {
		return GenerateEnvExample(vars, technology)
	}
	have := map[string]bool{}
	for _, line := range strings.Split(existing, "\n") {
		if key, _, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			have[strings.TrimSpace(key)] = true
		}
	}
	generated := GenerateEnvExample(nil, technology)
	var missing []string
	for _, line := range strings.Split(generated, "\n") {
		line = strings.TrimSpace(line)
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(line, "#") || have[strings.TrimSpace(key)] {
			continue
		}
		missing = append(missing, strings.TrimSpace(key)+"="+value)
	}
	if len(missing) == 0 {
		return existing
	}
	return strings.TrimRight(existing, "\n") + "\n\n# Yoink technology defaults\n" + strings.Join(missing, "\n") + "\n"
}

func readEnvAssignmentsFromContent(content string, values map[string]string) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"'")
		}
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// pydanticClassRe matches a class declaration that inherits from
// BaseSettings (pydantic v2 pydantic-settings or pydantic v1 BaseSettings).
var pydanticClassRe = regexp.MustCompile(`(?m)^class\s+\w+\s*\([^)]*\bBaseSettings\b[^)]*\)\s*:`)

// pydanticFieldRe matches an indented uppercase field declaration inside a
// BaseSettings class body, e.g. "    DATABASE_URL: str" or
// "    SECRET_KEY: str = \"change-me\". Captures the field name.
var pydanticFieldRe = regexp.MustCompile(`(?m)^\s+([A-Z][A-Z0-9_]{2,})\s*:`)

// scanPydanticSettings finds env-var names declared as fields of a
// pydantic BaseSettings class. Modern FastAPI apps use this pattern instead
// of os.getenv, so the standard os.getenv/os.environ regex patterns miss
// them. Returns the uppercased field names (deduplicated).
func scanPydanticSettings(text string) []string {
	if !strings.Contains(text, "BaseSettings") {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	// Find each BaseSettings class declaration, then walk the indented block
	// that follows it until the indentation drops back to the class level.
	classLocs := pydanticClassRe.FindAllStringIndex(text, -1)
	for _, loc := range classLocs {
		// Start scanning after the class declaration line.
		block := text[loc[1]:]
		lines := strings.Split(block, "\n")
		for _, line := range lines {
			// The class body is indented. A non-blank, non-comment line at
			// zero indentation means the class body ended.
			trimmed := strings.TrimLeft(line, " \t")
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			if line == trimmed {
				break // dedent → class body ended
			}
			if m := pydanticFieldRe.FindStringSubmatch(line); len(m) == 2 {
				name := m[1]
				if !seen[name] {
					seen[name] = true
					out = append(out, name)
				}
			}
		}
	}
	return out
}

// GenerateEnvExample renders a .env.example file for the given technology.
// Common variables for the technology are added before discovered variables,
// with no duplicates.
func GenerateEnvExample(vars []EnvVar, technology string) string {
	seen := map[string]bool{}

	var b strings.Builder
	fmt.Fprintf(&b, "# .env.example for %s service\n", technology)
	b.WriteString("# Copy to .env and fill in real values before running docker compose up.\n\n")

	common := getCommonVars(technology)
	if len(common) > 0 {
		fmt.Fprintf(&b, "# Common defaults for %s\n", technology)
		for _, v := range common {
			if seen[v.name] {
				continue
			}
			seen[v.name] = true
			if v.comment != "" {
				fmt.Fprintf(&b, "# %s\n", v.comment)
			}
			fmt.Fprintf(&b, "%s=%s\n", v.name, v.example)
		}
		b.WriteString("\n")
	}

	if len(vars) > 0 {
		b.WriteString("# Detected from source code\n")
		for _, v := range vars {
			if seen[v.Name] {
				continue
			}
			seen[v.Name] = true
			fmt.Fprintf(&b, "%s=\n", v.Name)
		}
	}
	return b.String()
}

type commonVar struct {
	name    string
	example string
	comment string
}

func getCommonVars(technology string) []commonVar {
	switch technology {
	case "fastapi":
		return []commonVar{
			{"DATABASE_URL", "postgresql://user:pass@db:5432/app", "App database connection string"},
			{"SECRET_KEY", "change-me", "Application secret"},
		}
	case "flask":
		return []commonVar{
			{"FLASK_ENV", "production", ""},
			{"DATABASE_URL", "postgresql://user:pass@db:5432/app", ""},
			{"SECRET_KEY", "change-me", ""},
		}
	case "django":
		return []commonVar{
			{"DJANGO_SETTINGS_MODULE", "project.settings", ""},
			{"DEBUG", "0", ""},
			{"SECRET_KEY", "change-me", ""},
			{"DATABASE_URL", "postgresql://user:pass@db:5432/app", ""},
			{"ALLOWED_HOSTS", "*", ""},
		}
	case "next":
		return []commonVar{
			{"NODE_ENV", "production", ""},
			{"NEXT_PUBLIC_API_URL", "http://localhost:3000/api", "URL exposed to the browser"},
		}
	case "express", "fastify", "nest", "node":
		return []commonVar{
			{"NODE_ENV", "production", ""},
			{"PORT", "3000", ""},
			{"DATABASE_URL", "postgresql://user:pass@db:5432/app", ""},
		}
	case "react", "vite", "cra":
		return []commonVar{
			{"VITE_API_URL", "http://localhost:3000/api", "(Vite) URL exposed to the browser"},
		}
	}
	return nil
}
