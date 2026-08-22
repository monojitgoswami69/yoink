// Package detector identifies the services that make up a project.
//
// The detector walks the cloned repository, picks up the files that signal a
// deployable unit (package.json, requirements.txt, pyproject.toml, Pipfile),
// classifies each one, and returns a deterministic list of Service records
// that downstream packages (generator, envvar, the LLM validator) consume.
package detector

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
)

// MaxServices is the default cap on the number of services returned. It can
// be overridden per-call via DetectWithCap.
const MaxServices = 12

// Result is the output of Detect.
type Result struct {
	Repo     string    `json:"repo"`
	Services []Service `json:"services"`
	// TruncatedFrom is non-zero when Detect found more services than the cap
	// and dropped the surplus. Callers should surface this to the user.
	TruncatedFrom int `json:"truncated_from,omitempty"`
}

// Evidence records why a fact was inferred. The healer uses this to
// distinguish "detector is confident" from "detector guessed" and to avoid
// re-deriving facts the static analysis already established. It also makes
// `yoink explain` possible later.
type Evidence struct {
	Fact   string `json:"fact"`   // e.g. "framework=next", "port=3000"
	Source string `json:"source"` // e.g. "package.json dependency 'next'"
	Weight string `json:"weight"` // "strong" | "medium" | "weak"
}

// Service is the canonical description of a single deployable unit. All
// downstream subsystems read from this struct, so changes here ripple
// consistently across generation, env-var extraction, and LLM validation.
type Service struct {
	ID             string   `json:"id"`                        // stable id, e.g. service-1
	Type           string   `json:"type"`                      // frontend | backend
	Directory      string   `json:"directory"`                 // path relative to repo root ("" == root)
	Language       string   `json:"language"`                  // javascript | typescript | python | go | rust | ...
	Framework      string   `json:"framework"`                 // next | express | react | fastapi | flask | django | node | python | ...
	PackageManager string   `json:"package_manager"`           // npm | yarn | pnpm | bun | pip | poetry | uv | go | cargo | ...
	HasLockfile    bool     `json:"has_lockfile"`              // true when a lockfile for the detected PM exists on disk
	PythonManifest string   `json:"python_manifest,omitempty"` // requirements.txt | pyproject.toml | Pipfile (Python only)
	PythonDeps     []string `json:"python_deps,omitempty"`     // lowercased runtime dep names (for native-apt inference)
	InstallCmd     []string `json:"install_cmd"`               // e.g. ["npm","ci"]
	BuildCmd       []string `json:"build_cmd,omitempty"`
	StartCmd       []string `json:"start_cmd"`  // e.g. ["npm","start"]
	Port           int      `json:"port"`       // container port
	Confidence     string   `json:"confidence"` // high | medium | low
	// BuildEnv holds KEY→VALUE pairs injected as ENV directives before the
	// build step. Many apps validate env vars at build time (e.g. Next.js
	// `next build` evaluates API routes that check JWT_SECRET/DATABASE_URL).
	// Without these, the build crashes on a missing-var check even though
	// the var is only needed at runtime. Values are placeholders overridden
	// at runtime by compose's env_file.
	BuildEnv map[string]string `json:"build_env,omitempty"`
	// Evidence records why each important fact was inferred, so the healer
	// can reason about detection confidence and avoid re-deriving known
	// facts. Populated during detection.
	Evidence []Evidence `json:"evidence,omitempty"`
}

// Detect performs static analysis over rootDir, returning at most MaxServices
// services. Use DetectWithCap to override the cap.
func Detect(rootDir string) (*Result, error) {
	return DetectWithCap(rootDir, MaxServices)
}

// DetectWithCap performs static analysis with an explicit cap on services.
// A cap of 0 or negative means no cap.
func DetectWithCap(rootDir string, maxServices int) (*Result, error) {
	candidates, err := scanCandidates(rootDir)
	if err != nil {
		return nil, err
	}

	all := buildServices(candidates)

	result := &Result{
		Repo:     filepath.Base(rootDir),
		Services: all,
	}

	if maxServices > 0 && len(all) > maxServices {
		result.TruncatedFrom = len(all)
		result.Services = capByConfidence(all, maxServices)
	}
	return result, nil
}

// candidate is a single project marker we picked up during the walk.
type candidate struct {
	dir    string // absolute
	relDir string
	kind   string // js | python
	ident  string // file name that triggered detection
}

// scanCandidates walks rootDir for project markers and returns one candidate
// per (directory, kind) pair. When both requirements.txt and pyproject.toml
// live in the same directory, requirements.txt wins.
func scanCandidates(rootDir string) ([]*candidate, error) {
	hits := map[string]*candidate{}
	add := func(c *candidate) {
		key := c.relDir + "|" + c.kind
		existing, ok := hits[key]
		if !ok {
			hits[key] = c
			return
		}
		if existing.ident == "pyproject.toml" && c.ident == "requirements.txt" {
			hits[key] = c
		}
	}

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) && path != rootDir {
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		dir := filepath.Dir(path)
		rel, _ := filepath.Rel(rootDir, dir)
		if rel == "." {
			rel = ""
		}
		switch name {
		case "package.json":
			add(&candidate{dir: dir, relDir: rel, kind: "js", ident: name})
		case "requirements.txt", "pyproject.toml", "Pipfile":
			add(&candidate{dir: dir, relDir: rel, kind: "python", ident: name})
		case "go.mod":
			add(&candidate{dir: dir, relDir: rel, kind: "go", ident: name})
		case "Cargo.toml":
			add(&candidate{dir: dir, relDir: rel, kind: "rust", ident: name})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk failed: %w", err)
	}

	keys := make([]string, 0, len(hits))
	for k := range hits {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]*candidate, 0, len(keys))
	for _, k := range keys {
		out = append(out, hits[k])
	}
	return out, nil
}

// buildServices runs the kind-specific classifier on each candidate and
// returns the kept Service records with stable IDs.
func buildServices(candidates []*candidate) []Service {
	services := make([]Service, 0, len(candidates))
	for _, c := range candidates {
		svc := Service{Directory: c.relDir}
		keep := true
		switch c.kind {
		case "js":
			keep = detectJS(c.dir, &svc)
		case "python":
			svc.PythonManifest = c.ident
			detectPython(c.dir, c.ident, &svc)
		case "go":
			keep = detectGo(c.dir, &svc)
		case "rust":
			keep = detectRust(c.dir, &svc)
		}
		if !keep {
			continue
		}
		svc.ID = fmt.Sprintf("service-%d", len(services)+1)
		services = append(services, svc)
	}
	return services
}

// capByConfidence keeps the top n services by confidence, then restores
// directory ordering and reassigns IDs.
func capByConfidence(services []Service, n int) []Service {
	ranked := make([]Service, len(services))
	copy(ranked, services)
	sort.SliceStable(ranked, func(i, j int) bool {
		return confidenceRank(ranked[i].Confidence) > confidenceRank(ranked[j].Confidence)
	})
	ranked = ranked[:n]
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].Directory < ranked[j].Directory })
	for i := range ranked {
		ranked[i].ID = fmt.Sprintf("service-%d", i+1)
	}
	return ranked
}

func confidenceRank(c string) int {
	switch c {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	}
	return 0
}

var skipDirs = map[string]bool{
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
	// Common monorepo noise. These directories contain package.json files that
	// describe demos/tests/docs/tooling, not deployable services.
	"examples":         true,
	"example":          true,
	"e2e":              true,
	"test":             true,
	"tests":            true,
	"__tests__":        true,
	"__fixtures__":     true,
	"__testfixtures__": true,
	"fixtures":         true,
	"docs":             true,
	"website":          true,
	"benchmarks":       true,
	"bench":            true,
	"playground":       true,
	"scripts":          true,
	"tools":            true,
	"evals":            true,
}

func shouldSkipDir(name string) bool {
	return skipDirs[name]
}

// ToJSON renders the result as pretty JSON. It is used to feed the static
// analysis to the LLM validator.
func (r *Result) ToJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// addEvidence records why a fact was inferred on the service. Weight is
// "strong" (direct evidence like a dependency or config file), "medium"
// (heuristic like a script name or directory hint), or "weak" (fallback
// default).
func (s *Service) addEvidence(fact, source, weight string) {
	s.Evidence = append(s.Evidence, Evidence{Fact: fact, Source: source, Weight: weight})
}
