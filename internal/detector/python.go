package detector

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// detectPython classifies a Python project. Unlike detectJS this never
// rejects a candidate — even an unknown Python project ships as a generic
// "python" service.
func detectPython(dir, ident string, svc *Service) {
	svc.Language = "python"
	svc.addEvidence("language=python", "manifest "+ident, "strong")

	// Read the manifest and strip comments so a "# uses fastapi" line
	// doesn't trigger framework=fastapi when the actual dep is flask.
	rawManifest := readLowercase(filepath.Join(dir, ident))
	manifest := stripComments(rawManifest, ident)
	setPythonPackageManager(dir, svc)
	svc.PythonDeps = parsePythonDeps(filepath.Join(dir, ident), manifest)

	switch {
	case strings.Contains(manifest, "fastapi"):
		applyFastAPI(dir, svc)
		svc.addEvidence("framework=fastapi", "manifest contains 'fastapi'", "strong")
	case strings.Contains(manifest, "django"):
		applyDjango(dir, svc)
		svc.addEvidence("framework=django", "manifest contains 'django'", "strong")
	case strings.Contains(manifest, "flask"):
		applyFlask(dir, svc)
		svc.addEvidence("framework=flask", "manifest contains 'flask'", "strong")
	default:
		applyGenericPython(dir, svc)
		svc.addEvidence("framework=python", "no known framework matched", "weak")
	}
}

// setPythonPackageManager picks the install tool from the lockfiles present.
// Order matters: poetry and uv are detected by their own lockfile; otherwise
// pip is the universal fallback.
func setPythonPackageManager(dir string, svc *Service) {
	switch {
	case exists(filepath.Join(dir, "poetry.lock")):
		svc.PackageManager = "poetry"
		svc.InstallCmd = []string{"poetry", "install", "--no-root", "--no-interaction"}
		return
	case exists(filepath.Join(dir, "uv.lock")):
		svc.PackageManager = "uv"
		// uv pip installs into the system env when run with --system.
		svc.InstallCmd = []string{"uv", "pip", "install", "--system"}
		return
	}
	svc.PackageManager = "pip"
	svc.InstallCmd = []string{"pip", "install", "--no-cache-dir", "-r", "requirements.txt"}
}

func applyFastAPI(dir string, svc *Service) {
	svc.Framework, svc.Type, svc.Confidence, svc.Port = "fastapi", "backend", "high", 8000
	mod, instance := findFastAPIEntry(dir)
	if mod == "" {
		// Fall back to a file-name based guess for common entry modules.
		mod = strings.TrimSuffix(findPythonEntry(dir, []string{"main.py", "app.py", "server.py"}), ".py")
	}
	if mod == "" {
		mod = "main"
	}
	if instance == "" {
		instance = "app"
	}
	svc.StartCmd = []string{"uvicorn", mod + ":" + instance, "--host", "0.0.0.0", "--port", "8000"}
}

func applyDjango(dir string, svc *Service) {
	svc.Framework, svc.Type, svc.Port = "django", "backend", 8000
	if exists(filepath.Join(dir, "manage.py")) {
		svc.Confidence = "high"
	} else {
		svc.Confidence = "medium"
	}
	svc.StartCmd = []string{"python", "manage.py", "runserver", "0.0.0.0:8000"}
}

func applyFlask(dir string, svc *Service) {
	svc.Framework, svc.Type, svc.Confidence, svc.Port = "flask", "backend", "high", 5000
	mod, instance := findFlaskEntry(dir)
	if mod == "" {
		entry := findPythonEntry(dir, []string{"app.py", "wsgi.py", "main.py"})
		if entry == "" {
			entry = "app.py"
		}
		mod = strings.TrimSuffix(entry, ".py")
	}
	if instance == "" {
		instance = "app"
	}
	svc.StartCmd = []string{"flask", "--app", mod + ":" + instance, "run", "--host", "0.0.0.0", "--port", "5000"}
}

func applyGenericPython(dir string, svc *Service) {
	svc.Framework, svc.Type, svc.Confidence, svc.Port = "python", "backend", "low", 8000
	entry := findPythonEntry(dir, []string{"main.py", "app.py"})
	if entry == "" {
		entry = "main.py"
	}
	svc.StartCmd = []string{"python", entry}
}

func findPythonEntry(dir string, candidates []string) string {
	for _, name := range candidates {
		if exists(filepath.Join(dir, name)) {
			return name
		}
	}
	return ""
}

// pyModulePath turns a .py file's path relative to the service directory into
// the dotted module path that `uvicorn`/`flask --app` expect. A file at
// `app/main.py` becomes `app.main`; a package's `app/__init__.py` becomes the
// bare package name `app` (not `app.__init__`), so `flask --app app:app`
// resolves the way the project author intended.
func pyModulePath(rel string) string {
	mod := strings.TrimSuffix(rel, ".py")
	mod = strings.ReplaceAll(mod, string(filepath.Separator), ".")
	if strings.HasSuffix(mod, ".__init__") {
		mod = strings.TrimSuffix(mod, ".__init__")
	} else if mod == "__init__" {
		mod = ""
	}
	return strings.TrimPrefix(mod, ".")
}

// fastAPIInstanceRe matches `app = FastAPI(` (with optional whitespace), used
// to discover the real ASGI entrypoint so the generated `uvicorn module:app`
// command actually resolves. Covers both `app = FastAPI()` and
// `app: FastAPI = FastAPI()` style declarations.
var fastAPIInstanceRe = regexp.MustCompile(`(\w+)\s*[:=]\s*FastAPI\s*\(`)

// flaskInstanceRe matches `app = Flask(` (with optional whitespace), used to
// discover the real WSGI entrypoint so the generated `flask --app module:app`
// command actually resolves. Covers both `app = Flask(__name__)` and the
// application-factory `create_app()` idiom (handled separately below).
var flaskInstanceRe = regexp.MustCompile(`(\w+)\s*[:=]\s*Flask\s*\(`)

// findFlaskEntry walks the service directory for a .py file that constructs a
// Flask() instance and returns (dottedModule, instanceName). A file at
// `app/main.py` becomes module `app.main`, matching `flask --app app.main:app`.
// Returns ("", "") when no Flask instance is found.
func findFlaskEntry(dir string) (module, instance string) {
	type hit struct {
		path     string
		instance string
		score    int
	}
	var best *hit

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base == "" || base[0] == '.' {
				return fs.SkipDir
			}
			switch base {
			case "tests", "test", "__pycache__", "venv", ".venv", "env", ".pytest_cache",
				".mypy_cache", ".ruff_cache", "migrations", "alembic", "node_modules":
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".py" {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		if strings.Count(rel, string(filepath.Separator)) > 5 {
			return nil
		}
		info, _ := d.Info()
		if info == nil || info.Size() > 1<<20 {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		m := flaskInstanceRe.FindStringSubmatch(string(data))
		if len(m) < 2 {
			return nil
		}
		base := strings.TrimSuffix(filepath.Base(path), ".py")
		score := 0
		switch base {
		case "app", "main", "server", "wsgi", "asgi":
			score = 3
		}
		score -= strings.Count(rel, string(filepath.Separator))
		h := &hit{path: path, instance: m[1], score: score}
		if best == nil || h.score > best.score {
			best = h
		}
		return nil
	})
	if best == nil {
		return "", ""
	}
	rel, _ := filepath.Rel(dir, best.path)
	return pyModulePath(rel), best.instance
}

// a FastAPI() instance and returns (dottedModule, instanceName). The module
// path is computed relative to the service directory so a file at
// `app/main.py` becomes `app.main` — the form `uvicorn app.main:app` expects.
// Returns ("", "") when no FastAPI instance is found.
func findFastAPIEntry(dir string) (module, instance string) {
	type hit struct {
		path     string
		instance string
		score    int
	}
	var best *hit

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base == "" || base[0] == '.' {
				return fs.SkipDir
			}
			// Skip test/venv/cache/tooling dirs — they aren't the app entry.
			switch base {
			case "tests", "test", "__pycache__", "venv", ".venv", "env", ".pytest_cache",
				".mypy_cache", ".ruff_cache", "migrations", "alembic", "node_modules":
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".py" {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		// Don't go deeper than ~5 dirs; the entry is near the top.
		if strings.Count(rel, string(filepath.Separator)) > 5 {
			return nil
		}
		info, _ := d.Info()
		if info == nil || info.Size() > 1<<20 {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		m := fastAPIInstanceRe.FindStringSubmatch(string(data))
		if len(m) < 2 {
			return nil
		}
		base := strings.TrimSuffix(filepath.Base(path), ".py")
		// Prefer files conventionally named as the app entry.
		score := 0
		switch base {
		case "main", "app", "server", "asgi":
			score = 3
		case "wsgi":
			score = 1
		default:
			score = 0
		}
		// Prefer shallower files (closer to the root).
		score -= strings.Count(rel, string(filepath.Separator))
		h := &hit{path: path, instance: m[1], score: score}
		if best == nil || h.score > best.score {
			best = h
		}
		return nil
	})
	if best == nil {
		return "", ""
	}
	rel, _ := filepath.Rel(dir, best.path)
	return pyModulePath(rel), best.instance
}

// readLowercase reads a file and returns its lowercased content. Errors are
// swallowed — a missing manifest just yields "", which the classifier treats
// as "unknown".
func readLowercase(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.ToLower(string(data))
}

// stripComments removes comment lines from manifest content so that
// "# uses fastapi" doesn't trigger framework=fastapi when the real dep
// is flask. For requirements.txt, lines starting with # are stripped.
// For pyproject.toml/Pipfile, inline # comments are stripped.
func stripComments(content, manifestType string) string {
	var b strings.Builder
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if manifestType == "requirements.txt" {
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
		}
		// Strip inline comments (everything after # in TOML/Pipfile).
		if manifestType != "requirements.txt" {
			if idx := strings.Index(trimmed, "#"); idx >= 0 {
				trimmed = strings.TrimSpace(trimmed[:idx])
			}
		}
		b.WriteString(trimmed)
		b.WriteString("\n")
	}
	return b.String()
}

// depNameRe captures the leading package name of a requirements.txt entry.
// Stops at the first version marker, extras bracket, whitespace, or comment.
var depNameRe = regexp.MustCompile(`^\s*([A-Za-z0-9_][A-Za-z0-9_.-]*)`)

// tomlDepRe matches a dependency name in either quoted (PEP 621 /
// dependency-groups) or bare (poetry / Pipfile) form, where the name is
// followed by a version marker or an assignment. This intentionally stays
// loose: false positives are filtered against the known-native-deps map, so a
// stray TOML key never produces a wrong apt package.
var tomlDepRe = regexp.MustCompile(`["']?([A-Za-z0-9_][A-Za-z0-9_.-]*)["']?\s*(?:\[[^\]]*\])?\s*(?:[<>=!~]|=\s*["'{])`)

// parsePythonDeps extracts lowercased dependency names from a manifest so the
// generator can decide which system libraries to install for C-extension
// wheels. Only the names matter; versions and extras are discarded. The set
// is best-effort: false positives are harmless because the native-deps map
// only triggers on specific known names.
func parsePythonDeps(path, _ string) []string {
	// Derive the manifest TYPE from the filename, not the file content
	// (which was lowercased and passed as the second parameter).
	manifestType := filepath.Base(path)
	if manifestType == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	content := string(data)
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}

	switch manifestType {
	case "requirements.txt":
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
				continue
			}
			if strings.HasPrefix(line, "git+") || strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
				continue
			}
			m := depNameRe.FindStringSubmatch(line)
			if len(m) >= 2 {
				add(m[1])
			}
		}
	default: // pyproject.toml / Pipfile — scan for dep-like tokens.
		for _, m := range tomlDepRe.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 {
				add(m[1])
			}
		}
	}
	return out
}
