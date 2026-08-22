package detector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectsGoService(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module myapp\n\ngo 1.23\n")
	writeFile(t, filepath.Join(root, "main.go"), "package main\nimport \"net/http\"\nfunc main() { http.ListenAndServe(\":8080\", nil) }\n")

	res, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(res.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(res.Services))
	}
	svc := res.Services[0]
	if svc.Framework != "go" {
		t.Errorf("framework: want go, got %s", svc.Framework)
	}
	if svc.Language != "go" {
		t.Errorf("language: want go, got %s", svc.Language)
	}
	if svc.PackageManager != "go" {
		t.Errorf("pm: want go, got %s", svc.PackageManager)
	}
	if len(svc.BuildCmd) == 0 || svc.BuildCmd[0] != "go" {
		t.Errorf("BuildCmd: %v", svc.BuildCmd)
	}
	if len(svc.StartCmd) == 0 || svc.StartCmd[0][:2] != "./" {
		t.Errorf("StartCmd: %v", svc.StartCmd)
	}
}

func TestDetectsRustService(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Cargo.toml"), `[package]
name = "my-rust-app"
version = "0.1.0"
[dependencies]
axum = "0.7"`)

	res, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(res.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(res.Services))
	}
	svc := res.Services[0]
	if svc.Framework != "rust" {
		t.Errorf("framework: want rust, got %s", svc.Framework)
	}
	if svc.Language != "rust" {
		t.Errorf("language: want rust, got %s", svc.Language)
	}
	if svc.PackageManager != "cargo" {
		t.Errorf("pm: want cargo, got %s", svc.PackageManager)
	}
	if len(svc.StartCmd) == 0 || svc.StartCmd[0][:2] != "./" {
		t.Errorf("StartCmd should be ./my-rust-app, got %v", svc.StartCmd)
	}
}

func TestBunPackageManager(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"dependencies":{"express":"4"}}`)
	writeFile(t, filepath.Join(root, "bun.lock"), "")
	res, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if pm := res.Services[0].PackageManager; pm != "bun" {
		t.Errorf("expected pm=bun, got %s", pm)
	}
}

func TestBunLockbPackageManager(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"dependencies":{"express":"4"}}`)
	writeFile(t, filepath.Join(root, "bun.lockb"), "")
	res, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if pm := res.Services[0].PackageManager; pm != "bun" {
		t.Errorf("expected pm=bun from bun.lockb, got %s", pm)
	}
}

func TestEvidencePopulatedForJSDetection(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"dependencies":{"next":"14"},"scripts":{"start":"next start"}}`)
	res, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	svc := res.Services[0]
	if len(svc.Evidence) == 0 {
		t.Fatal("expected evidence to be populated")
	}
	foundFramework := false
	for _, e := range svc.Evidence {
		if e.Fact == "framework=next" {
			foundFramework = true
			if e.Weight != "strong" {
				t.Errorf("framework evidence weight: want strong, got %s", e.Weight)
			}
		}
	}
	if !foundFramework {
		t.Errorf("evidence missing framework=next; got %v", svc.Evidence)
	}
}

func TestEvidencePopulatedForGoDetection(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module myapp\n\ngo 1.23\n")
	res, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	svc := res.Services[0]
	foundLang := false
	for _, e := range svc.Evidence {
		if e.Fact == "language=go" {
			foundLang = true
		}
	}
	if !foundLang {
		t.Errorf("evidence missing language=go; got %v", svc.Evidence)
	}
}

// suppress unused import warnings for os (used by writeFile via t.TempDir)
var _ = os.Stat
