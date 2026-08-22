package detector

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func newMonorepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "apps/web/package.json"), `{
        "name":"web",
        "scripts":{"build":"next build","start":"next start"},
        "dependencies":{"next":"14","react":"18"}
    }`)
	writeFile(t, filepath.Join(root, "apps/web/tsconfig.json"), `{}`)
	writeFile(t, filepath.Join(root, "services/api/requirements.txt"), "fastapi==0.111.0\nuvicorn==0.30.0\n")
	writeFile(t, filepath.Join(root, "services/api/main.py"), "import os\nfrom fastapi import FastAPI\napp = FastAPI()\nDB = os.environ[\"DATABASE_URL\"]\n")
	return root
}

func TestDetectsMonorepo(t *testing.T) {
	root := newMonorepo(t)

	res, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(res.Services) != 2 {
		t.Fatalf("expected 2 services, got %d: %+v", len(res.Services), res.Services)
	}

	var web, api *Service
	for i := range res.Services {
		s := &res.Services[i]
		switch s.Framework {
		case "next":
			web = s
		case "fastapi":
			api = s
		}
	}
	if web == nil {
		t.Fatal("did not detect next frontend")
	}
	if api == nil {
		t.Fatal("did not detect fastapi backend")
	}
	if web.Language != "typescript" {
		t.Errorf("web language: want typescript, got %s", web.Language)
	}
	if web.Type != "frontend" {
		t.Errorf("web type: want frontend, got %s", web.Type)
	}
	if web.Port != 3000 {
		t.Errorf("web port: want 3000, got %d", web.Port)
	}
	if api.Type != "backend" {
		t.Errorf("api type: want backend, got %s", api.Type)
	}
	if api.Port != 8000 {
		t.Errorf("api port: want 8000, got %d", api.Port)
	}
	if len(api.StartCmd) == 0 || api.StartCmd[0] != "uvicorn" {
		t.Errorf("api start cmd: %v", api.StartCmd)
	}
}

func TestDeduplicatesPython(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "requirements.txt"), "flask\n")
	writeFile(t, filepath.Join(root, "pyproject.toml"), "[tool.poetry]\n")

	res, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(res.Services) != 1 {
		t.Fatalf("expected 1 python service, got %d", len(res.Services))
	}
	if res.Services[0].Framework != "flask" {
		t.Errorf("framework: want flask, got %s", res.Services[0].Framework)
	}
}

func TestSkipsExcludedDirs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"dependencies":{"express":"4"}}`)
	writeFile(t, filepath.Join(root, "node_modules/foo/package.json"), `{"dependencies":{"next":"14"}}`)

	res, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(res.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(res.Services))
	}
	if res.Services[0].Framework != "express" {
		t.Errorf("framework: want express, got %s", res.Services[0].Framework)
	}
}

func TestNextWithApiDirGetsBackendType(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "apps/api/package.json"), `{"dependencies":{"next":"14"},"scripts":{"start":"next start"}}`)
	res, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(res.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(res.Services))
	}
	if res.Services[0].Type != "backend" {
		t.Errorf("type: want backend (api/ directory hint), got %s", res.Services[0].Type)
	}
}

func TestSkipsLibraryPackagesAndMonorepoNoise(t *testing.T) {
	root := t.TempDir()
	// Workspace root with workspaces: skipped.
	writeFile(t, filepath.Join(root, "package.json"), `{"workspaces":["packages/*"]}`)
	// A library: no framework dep, no start script — skipped.
	writeFile(t, filepath.Join(root, "packages/utils/package.json"), `{"name":"utils","main":"index.js"}`)
	// An actual app: kept.
	writeFile(t, filepath.Join(root, "packages/web/package.json"), `{"dependencies":{"next":"14"},"scripts":{"start":"next start"}}`)
	// Example dir: skipped (excluded dir).
	writeFile(t, filepath.Join(root, "examples/foo/package.json"), `{"dependencies":{"express":"4"}}`)

	res, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(res.Services) != 1 {
		t.Fatalf("expected 1 service (only the web app), got %d: %+v", len(res.Services), res.Services)
	}
	if res.Services[0].Directory != "packages/web" {
		t.Errorf("unexpected service directory: %s", res.Services[0].Directory)
	}
}

func TestKeepsServiceWithStartScriptEvenWithoutKnownFramework(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"scripts":{"start":"node server.js"}}`)
	res, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(res.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(res.Services))
	}
	if res.Services[0].Framework != "node" {
		t.Errorf("expected fallback to 'node' framework, got %s", res.Services[0].Framework)
	}
}

func TestPortLearnedFromJSEntry(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"main":"server.js","scripts":{"start":"node server.js"}}`)
	writeFile(t, filepath.Join(root, "server.js"), `
		const PORT = process.env.PORT || 4000;
		server.listen(PORT);
	`)
	res, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(res.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(res.Services))
	}
	if res.Services[0].Port != 4000 {
		t.Errorf("expected port 4000 (read from server.js), got %d", res.Services[0].Port)
	}
}

// TestFastAPIEntryScanFindsDottedModule covers the real-world crash where a
// FastAPI app lives in app/main.py but the detector used to default to the
// non-existent `main:app`. The scanner must discover `app.main:app`.
func TestFastAPIEntryScanFindsDottedModule(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "requirements.txt"), "fastapi\nuvicorn\n")
	writeFile(t, filepath.Join(root, "app", "__init__.py"), "")
	writeFile(t, filepath.Join(root, "app", "main.py"), "from fastapi import FastAPI\napp = FastAPI()\n")

	res, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(res.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(res.Services))
	}
	svc := res.Services[0]
	if len(svc.StartCmd) < 2 || svc.StartCmd[0] != "uvicorn" || svc.StartCmd[1] != "app.main:app" {
		t.Errorf("expected uvicorn app.main:app, got %v", svc.StartCmd)
	}
}

// TestFastAPIEntryScanHonoursCustomInstance ensures a project that names its
// FastAPI instance `api` (e.g. `api = FastAPI(`) produces `module:api`.
func TestFastAPIEntryScanHonoursCustomInstance(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "requirements.txt"), "fastapi\n")
	writeFile(t, filepath.Join(root, "server.py"), "from fastapi import FastAPI\napi = FastAPI()\n")

	res, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got := res.Services[0].StartCmd[1]; got != "server:api" {
		t.Errorf("expected server:api, got %s", got)
	}
}

// TestFlaskEntryScanFindsPackage covers a Flask app whose factory lives in
// app/__init__.py (`app = Flask(__name__)`).
func TestFlaskEntryScanFindsPackage(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "requirements.txt"), "flask\n")
	writeFile(t, filepath.Join(root, "app", "__init__.py"), "from flask import Flask\napp = Flask(__name__)\n")

	res, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	svc := res.Services[0]
	// flask --app app:app run ...
	if len(svc.StartCmd) < 3 || svc.StartCmd[0] != "flask" || svc.StartCmd[2] != "app:app" {
		t.Errorf("expected flask --app app:app ..., got %v", svc.StartCmd)
	}
}

// TestUVLockfileDetected verifies a uv.lock makes the package manager "uv".
func TestUVLockfileDetected(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "requirements.txt"), "fastapi\nuvicorn\n")
	writeFile(t, filepath.Join(root, "uv.lock"), "")
	res, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if pm := res.Services[0].PackageManager; pm != "uv" {
		t.Errorf("expected PackageManager=uv, got %s", pm)
	}
}

// TestPythonDepsParsedFromRequirements verifies the native-dep inference
// pipeline picks up package names from a requirements.txt file.
func TestPythonDepsParsedFromRequirements(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "requirements.txt"), "fastapi==0.111.0\npsycopg[binary]>=3.1\n# comment\n-r extra.txt\nuvicorn\n")
	res, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	deps := res.Services[0].PythonDeps
	if !containsStr(deps, "psycopg") {
		t.Errorf("expected psycopg in parsed deps, got %v", deps)
	}
	if !containsStr(deps, "fastapi") {
		t.Errorf("expected fastapi in parsed deps, got %v", deps)
	}
	// option lines (-r) and comments must not produce dep names
	if containsStr(deps, "-r") || containsStr(deps, "comment") {
		t.Errorf("option/comment leaked into deps: %v", deps)
	}
}

// TestPythonDepsParsedFromPyproject verifies PEP 621 pyproject.toml deps are
// extracted (used for native-apt inference on pyproject projects).
func TestPythonDepsParsedFromPyproject(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "pyproject.toml"), `[project]
name = "app"
dependencies = [
    "fastapi>=0.100",
    "psycopg[binary]>=3.1",
    "cryptography>=42",
]
[build-system]
requires = ["hatchling"]
`)
	writeFile(t, filepath.Join(root, "app", "__init__.py"), "")
	writeFile(t, filepath.Join(root, "app", "main.py"), "from fastapi import FastAPI\napp = FastAPI()\n")
	res, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	deps := res.Services[0].PythonDeps
	if !containsStr(deps, "psycopg") || !containsStr(deps, "cryptography") {
		t.Errorf("expected psycopg + cryptography in deps, got %v", deps)
	}
}

func containsStr(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
