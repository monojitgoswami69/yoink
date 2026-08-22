package envvar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yoink/internal/detector"
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

func TestNextPublicCapturesFullName(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "page.tsx"), `
		const url = process.env.NEXT_PUBLIC_API_URL;
		const x = process.env.NEXT_PUBLIC_FOO;
	`)
	svcs := []detector.Service{{ID: "service-1", Directory: "", Language: "javascript", Framework: "next", Type: "frontend"}}
	got := Detect(root, svcs)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	names := map[string]bool{}
	for _, v := range got[0].Vars {
		names[v.Name] = true
	}
	if !names["NEXT_PUBLIC_API_URL"] {
		t.Errorf("did not capture NEXT_PUBLIC_API_URL; got: %v", names)
	}
	if !names["NEXT_PUBLIC_FOO"] {
		t.Errorf("did not capture NEXT_PUBLIC_FOO; got: %v", names)
	}
	if names["API_URL"] {
		t.Errorf("captured stripped suffix API_URL, want full name")
	}
}

func TestPythonOsEnviron(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.py"), `
import os
DB = os.environ["DATABASE_URL"]
SECRET = os.getenv("SECRET_KEY")
PORT = os.environ.get("PORT")
`)
	svcs := []detector.Service{{ID: "service-1", Directory: "", Language: "python", Framework: "fastapi", Type: "backend"}}
	got := Detect(root, svcs)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	names := map[string]bool{}
	for _, v := range got[0].Vars {
		names[v.Name] = true
	}
	for _, want := range []string{"DATABASE_URL", "SECRET_KEY", "PORT"} {
		if !names[want] {
			t.Errorf("missing %s in detected vars: %v", want, names)
		}
	}
}

// Regression: Django-style `os.getenv("X", default)` calls were missed by the
// earlier patterns because they required a closing paren right after the
// closing quote.
func TestPythonGetenvWithDefault(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "settings.py"), `
import os
SECRET_KEY = os.getenv("DJANGO_SECRET_KEY", get_random_secret_key())
DEBUG = os.getenv("DEBUG", "False") == "True"
ALLOWED_HOSTS = os.getenv("DJANGO_ALLOWED_HOSTS", "127.0.0.1,localhost").split(",")
if os.getenv("DATABASE_URL", None) is None:
    raise Exception("missing")
TIMEOUT = os.environ.get("REQUEST_TIMEOUT", 30)
`)
	svcs := []detector.Service{{ID: "service-1", Directory: "", Language: "python", Framework: "django", Type: "backend"}}
	got := Detect(root, svcs)[0].Vars
	names := map[string]bool{}
	for _, v := range got {
		names[v.Name] = true
	}
	for _, want := range []string{"DJANGO_SECRET_KEY", "DEBUG", "DJANGO_ALLOWED_HOSTS", "DATABASE_URL", "REQUEST_TIMEOUT"} {
		if !names[want] {
			t.Errorf("missing %s in detected vars: %v", want, names)
		}
	}
}

func TestDeterministicOrdering(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.js"), `
		process.env.ZED;
		process.env.ALPHA;
		process.env.MIKE;
	`)
	svcs := []detector.Service{{ID: "service-1", Language: "javascript", Framework: "node"}}
	first := Detect(root, svcs)[0].Vars

	// Re-run and confirm the order is identical across calls.
	second := Detect(root, svcs)[0].Vars
	if len(first) != len(second) {
		t.Fatalf("len mismatch: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Name != second[i].Name {
			t.Errorf("nondeterministic ordering at %d: %s vs %s", i, first[i].Name, second[i].Name)
		}
	}
	if first[0].Name != "ALPHA" || first[len(first)-1].Name != "ZED" {
		t.Errorf("not sorted: %v", first)
	}
}

func TestDetectPreservesExistingEnvTemplateValues(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte("TWILIO_ENABLED=false\nSMTP_PORT=587\nSECRET_KEY=\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app.py"), []byte("import os\nos.getenv('TWILIO_ENABLED')\n"), 0644); err != nil {
		t.Fatal(err)
	}
	results := Detect(root, []detector.Service{{ID: "service-1", Directory: "", Language: "python", Framework: "fastapi", Type: "backend"}})
	if len(results) != 1 || !strings.Contains(results[0].EnvContent, "TWILIO_ENABLED=false") || !strings.Contains(results[0].EnvContent, "SMTP_PORT=587") {
		t.Fatalf("existing template values were not preserved: %+v", results)
	}
}

func TestCommonVarsDoNotDuplicate(t *testing.T) {
	out := GenerateEnvExample([]EnvVar{{Name: "DATABASE_URL"}}, "fastapi")
	if c := strings.Count(out, "DATABASE_URL="); c != 1 {
		t.Errorf("DATABASE_URL appeared %d times in:\n%s", c, out)
	}
}

// TestPydanticSettingsFieldsCaptured verifies the envvar scanner catches
// env vars declared as pydantic BaseSettings class fields — the pattern
// modern FastAPI apps use instead of os.getenv.
func TestPydanticSettingsFieldsCaptured(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "config.py"), `from pydantic_settings import BaseSettings

class Settings(BaseSettings):
    DATABASE_URL: str = "postgresql://user:pass@localhost/app"
    SECRET_KEY: str = "change-me"
    API_KEY: str = ""
    PORT: int = 8000

# Not a settings class:
class Other:
    pass
`)
	svcs := []detector.Service{{ID: "s1", Directory: "", Language: "python", Framework: "fastapi", Type: "backend"}}
	got := Detect(root, svcs)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	names := map[string]bool{}
	for _, v := range got[0].Vars {
		names[v.Name] = true
	}
	for _, want := range []string{"DATABASE_URL", "SECRET_KEY", "API_KEY", "PORT"} {
		if !names[want] {
			t.Errorf("pydantic-settings field %s not captured; got %v", want, names)
		}
	}
}

func TestPydanticSettingsSkipsNonSettingsClasses(t *testing.T) {
	// A class that doesn't inherit BaseSettings should not contribute fields.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "app.py"), `class Model:
    DATABASE_URL: str
    API_KEY: str
`)
	svcs := []detector.Service{{ID: "s1", Directory: "", Language: "python", Framework: "fastapi", Type: "backend"}}
	got := Detect(root, svcs)
	for _, v := range got[0].Vars {
		if v.Name == "DATABASE_URL" || v.Name == "API_KEY" {
			t.Errorf("non-BaseSettings field %s should not be captured", v.Name)
		}
	}
}
