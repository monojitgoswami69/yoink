package detector

import (
	"os"
	"path/filepath"
	"testing"
)

// Adversarial tests: verify that misleading signals do NOT cause false
// detection. The detector must use evidence (dependencies, scripts,
// config files), not naive substring matching.

// TestNextThemesDoesNotTriggerNextJS: the "next-themes" package is a
// dark-mode library, NOT Next.js. The detector must NOT classify it as "next".
func TestNextThemesDoesNotTriggerNextJS(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"dependencies":{"next-themes":"0.3","react":"18"},"scripts":{"start":"vite"}}`)
	res, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, svc := range res.Services {
		if svc.Framework == "next" {
			t.Errorf("next-themes must NOT trigger framework=next; got %+v", svc)
		}
	}
}

// TestNextIntlDoesNotTriggerNextJS: "next-intl" is an i18n library.
func TestNextIntlDoesNotTriggerNextJS(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"dependencies":{"next-intl":"3","react":"18"}}`)
	res, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, svc := range res.Services {
		if svc.Framework == "next" {
			t.Errorf("next-intl must NOT trigger framework=next")
		}
	}
}

// TestLibraryWithoutStartScriptSkipped: a package.json with deps but no
// start/dev script and no framework should be skipped (library, not deployable).
func TestLibraryWithoutStartScriptSkipped(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"my-lib","main":"index.js","dependencies":{"lodash":"4"}}`)
	res, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Services) != 0 {
		t.Errorf("library without start script should be skipped, got %d services", len(res.Services))
	}
}

// TestWorkspaceRootSkipped: a workspace root package.json should be skipped.
func TestWorkspaceRootSkipped(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"workspaces":["packages/*"]}`)
	writeFile(t, filepath.Join(root, "packages", "app", "package.json"), `{"dependencies":{"express":"4"},"scripts":{"start":"node index.js"}}`)
	writeFile(t, filepath.Join(root, "packages", "app", "index.js"), "express.listen(3000);")
	res, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Services) != 1 {
		t.Fatalf("expected 1 service (the app), got %d", len(res.Services))
	}
	if res.Services[0].Directory != "packages/app" {
		t.Errorf("service should be in packages/app, got %s", res.Services[0].Directory)
	}
}

// TestExcludedDirNotDetected: package.json in examples/ should be excluded.
func TestExcludedDirNotDetected(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "examples", "demo", "package.json"), `{"dependencies":{"express":"4"},"scripts":{"start":"node index.js"}}`)
	res, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Services) != 0 {
		t.Errorf("examples/ should be excluded, got %d services", len(res.Services))
	}
}

// TestFastAPISubstringInComment: requirements.txt mentioning "fastapi" in a
// comment should still detect it (comments are stripped).
func TestFastAPISubstringInComment(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "requirements.txt"), "# uses fastapi\nflask\n")
	res, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(res.Services))
	}
	// The comment "# uses fastapi" should not cause framework=fastapi.
	// The actual dep "flask" should.
	if res.Services[0].Framework != "flask" {
		t.Errorf("expected flask, got %s (comment should not trigger fastapi)", res.Services[0].Framework)
	}
}

// TestMultipleFrameworks: when multiple framework deps exist, the strongest
// (highest priority) should win.
func TestMultipleFrameworks(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"dependencies":{"next":"14","express":"4"},"scripts":{"start":"next start"}}`)
	res, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Services) != 1 {
		t.Fatalf("expected 1, got %d", len(res.Services))
	}
	if res.Services[0].Framework != "next" {
		t.Errorf("next should win over express; got %s", res.Services[0].Framework)
	}
}

// TestStaleDependencyNotFramework: a dep named "next" in an unrelated context
// (like a tool) should not trigger if no start script and no config.
// This is already handled by the library-skip logic.
func TestStaleDependencySkippedAsLibrary(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"tool","dependencies":{"next":"14"}}`)
	res, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Services) != 0 {
		t.Errorf("package with 'next' dep but no start script should be skipped as a library")
	}
}

// suppress unused import
var _ = os.Stat
