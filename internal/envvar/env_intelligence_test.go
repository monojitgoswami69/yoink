package envvar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yoink/internal/detector"
)

// TestLargeEnvSurfaceOnlyOneRequiredDoesNotBlockAll verifies that a repository
// with many static env references does not produce requirements for all of
// them. Static discovery alone must produce UNKNOWN candidates.
func TestLargeEnvSurfaceOnlyOneRequiredDoesNotBlockAll(t *testing.T) {
	root := t.TempDir()
	// Generate 50+ process.env references in source code.
	var src strings.Builder
	for i := 0; i < 50; i++ {
		src.WriteString("process.env.VAR_")
		src.WriteString(string(rune('A' + (i % 26))))
		src.WriteString(";\n")
	}
	src.WriteString("process.env.DATABASE_URL;\n")
	writeFile(t, filepath.Join(root, "app.js"), src.String())

	result := Detect(root, []detector.Service{{ID: "s1", Language: "javascript", Framework: "node", Type: "backend"}})[0]
	if len(result.Vars) < 20 {
		t.Fatalf("expected 20+ candidates, got %d", len(result.Vars))
	}
	// None should be REQUIRED from static discovery alone.
	required := 0
	for _, v := range result.Vars {
		if v.Status == StatusRequired {
			required++
		}
	}
	if required != 0 {
		t.Fatalf("static discovery should not produce REQUIRED, got %d", required)
	}
}

// TestViteManyImportMetaEnvNoneRequired verifies Vite import.meta.env
// references do not become requirements.
func TestViteManyImportMetaEnvNoneRequired(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.ts"), `
		import.meta.env.VITE_API_URL;
		import.meta.env.VITE_APP_TITLE;
		import.meta.env.VITE_BACKEND_URL;
		import.meta.env.VITE_FEATURE_FLAG;
	`)
	result := Detect(root, []detector.Service{{ID: "s1", Language: "typescript", Framework: "vite", Type: "frontend"}})[0]
	for _, v := range result.Vars {
		if v.Status == StatusRequired {
			t.Fatalf("Vite env var %s should not be REQUIRED from static discovery", v.Name)
		}
	}
}

// TestNodeStartupValidationOneRequired verifies that a Node.js file with
// explicit startup validation is still just a candidate — static discovery
// does not conclude REQUIRED from the pattern alone.
func TestNodeStartupValidationOneRequired(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "server.js"), `
		if (!process.env.DATABASE_URL) throw new Error("DATABASE_URL is required");
		const port = process.env.PORT || 3000;
	`)
	result := Detect(root, []detector.Service{{ID: "s1", Language: "javascript", Framework: "node", Type: "backend"}})[0]
	// Static discovery must not promote DATABASE_URL to REQUIRED even though
	// the source has an explicit throw. That classification comes from the
	// agent's semantic investigation or the actual runtime failure.
	for _, v := range result.Vars {
		if v.Name == "DATABASE_URL" && v.Status == StatusRequired {
			t.Fatal("static discovery should not classify as REQUIRED")
		}
	}
}

// TestPythonRequiredVariableIsCandidate verifies that os.environ["DATABASE_URL"]
// (unconditional access) is a candidate but not statically REQUIRED.
func TestPythonRequiredVariableIsCandidate(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "app.py"), `
		import os
		db = os.environ["DATABASE_URL"]
		debug = os.getenv("DEBUG", "false")
	`)
	result := Detect(root, []detector.Service{{ID: "s1", Language: "python", Framework: "fastapi", Type: "backend"}})[0]
	for _, v := range result.Vars {
		if v.Status == StatusRequired {
			t.Fatalf("static discovery should not classify %s as REQUIRED", v.Name)
		}
	}
}

// TestPythonOptionalWithDefaultIsCandidate verifies that os.getenv with a default
// value is discovered as a candidate but not REQUIRED.
func TestPythonOptionalWithDefaultIsCandidate(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "settings.py"), `
		import os
		DEBUG = os.getenv("DEBUG", "false")
		PORT = os.getenv("PORT", "3000")
	`)
	result := Detect(root, []detector.Service{{ID: "s1", Language: "python", Framework: "fastapi", Type: "backend"}})[0]
	for _, v := range result.Vars {
		if v.Status == StatusRequired {
			t.Fatalf("optional variable %s should not be REQUIRED", v.Name)
		}
	}
}

// TestPydanticMixedRequiredAndOptional verifies that Pydantic BaseSettings
// fields with defaults are discovered as candidates. The required-vs-optional
// distinction is for the agent to determine from source inspection.
func TestPydanticMixedRequiredAndOptional(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "config.py"), `from pydantic_settings import BaseSettings

class Settings(BaseSettings):
    DATABASE_URL: str
    SECRET_KEY: str = "change-me"
    DEBUG: bool = False
    PORT: int = 8000
    OPTIONAL_FEATURE_KEY: str = ""
`)
	result := Detect(root, []detector.Service{{ID: "s1", Language: "python", Framework: "fastapi", Type: "backend"}})[0]
	names := map[string]bool{}
	for _, v := range result.Vars {
		names[v.Name] = true
		if v.Status == StatusRequired {
			t.Fatalf("static discovery should not classify %s as REQUIRED — agent must investigate", v.Name)
		}
	}
	for _, want := range []string{"DATABASE_URL", "SECRET_KEY", "DEBUG", "PORT", "OPTIONAL_FEATURE_KEY"} {
		if !names[want] {
			t.Errorf("missing pydantic field %s", want)
		}
	}
}

// TestRepositoryDefaultsPlusPlaceholderCredentials verifies that .env.example
// with ordinary defaults and placeholder credentials are both preserved,
// and placeholders are flagged but not treated as valid credentials.
func TestRepositoryDefaultsPlusPlaceholderCredentials(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".env.example"), `PORT=3000
DEBUG=false
GEMINI_API_KEY=placeholder-key-here
DATABASE_URL=postgresql://user:pass@db:5432/app
`)
	writeFile(t, filepath.Join(root, "app.js"), `
		process.env.PORT;
		process.env.DEBUG;
		process.env.GEMINI_API_KEY;
		process.env.DATABASE_URL;
	`)
	result := Detect(root, []detector.Service{{ID: "s1", Language: "javascript", Framework: "node", Type: "backend"}})[0]
	for _, v := range result.Vars {
		switch v.Name {
		case "PORT":
			if !v.Provided || v.Value != "3000" || v.Placeholder {
				t.Fatalf("PORT should be provided default without placeholder: %+v", v)
			}
		case "DEBUG":
			if !v.Provided || v.Value != "false" || v.Placeholder {
				t.Fatalf("DEBUG should be provided default without placeholder: %+v", v)
			}
		case "GEMINI_API_KEY":
			if !v.Provided || !v.Placeholder {
				t.Fatalf("GEMINI_API_KEY should be provided placeholder: %+v", v)
			}
		case "DATABASE_URL":
			if !v.Provided {
				t.Fatalf("DATABASE_URL should be provided: %+v", v)
			}
		}
	}
}

// TestMultiServiceWorkerEnvDoesNotLeakToOtherService verifies that env vars
// discovered in one service directory do not appear in another service's results.
func TestMultiServiceWorkerEnvDoesNotLeakToOtherService(t *testing.T) {
	root := t.TempDir()
	workerDir := filepath.Join(root, "worker")
	webDir := filepath.Join(root, "web")
	os.MkdirAll(workerDir, 0755)
	os.MkdirAll(webDir, 0755)
	writeFile(t, filepath.Join(workerDir, "index.js"), "process.env.WORKER_DATABASE_URL;\n")
	writeFile(t, filepath.Join(webDir, "app.js"), "process.env.NEXT_PUBLIC_API_URL;\n")

	results := Detect(root, []detector.Service{
		{ID: "worker", Directory: "worker", Language: "javascript", Framework: "node", Type: "backend"},
		{ID: "web", Directory: "web", Language: "typescript", Framework: "next", Type: "frontend"},
	})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		for _, v := range r.Vars {
			if r.ServiceID == "web" && v.Name == "WORKER_DATABASE_URL" {
				t.Fatal("worker env var leaked into frontend service")
			}
			if r.ServiceID == "worker" && v.Name == "NEXT_PUBLIC_API_URL" {
				t.Fatal("frontend env var leaked into worker service")
			}
		}
	}
}

// TestLLMRewriteDoesNotEraseRepositoryDefaults verifies that MergeProvidedValues
// preserves non-empty repository template assignments when the LLM returns
// blank or missing values.
func TestLLMRewriteDoesNotEraseRepositoryDefaults(t *testing.T) {
	original := "PORT=3000\nDEBUG=false\nGEMINI_API_KEY=placeholder-key-here\nSECRET_KEY=\n"
	proposed := "PORT=\nDEBUG=\nGEMINI_API_KEY=\nSECRET_KEY=\n"
	merged := MergeProvidedValues(original, proposed)
	if !strings.Contains(merged, "PORT=3000") {
		t.Error("PORT default was erased")
	}
	if !strings.Contains(merged, "DEBUG=false") {
		t.Error("DEBUG default was erased")
	}
	if !strings.Contains(merged, "GEMINI_API_KEY=placeholder-key-here") {
		t.Error("GEMINI_API_KEY placeholder was erased")
	}
	// Empty values should remain empty (not guessed).
	if !strings.Contains(merged, "SECRET_KEY=") {
		t.Error("SECRET_KEY empty value should remain")
	}
}
