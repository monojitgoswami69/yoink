package healer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yoink/internal/detector"
)

func TestAnalyzeFailureTS2307(t *testing.T) {
	buildOut := `#1 [internal] load build definition
#2 [internal] load build definition from Dockerfile.service-2
#16 [service-2 builder 5/6] RUN npm run build
#16 9.678 ingestion-worker/src/pipeline.ts(6,29): error TS2307: Cannot find module '@napi-rs/canvas' or its corresponding type declarations.
#16 9.678 ingestion-worker/src/pipeline.ts(7,60): error TS2307: Cannot find module 'mammoth' or its corresponding type declarations.
failed to solve: process "/bin/sh -c npm run build" did not complete successfully: exit code: 1
`
	f := AnalyzeFailure(buildOut, "service-2")
	if f.Category != "compilation" {
		t.Errorf("category: want compilation, got %s", f.Category)
	}
	if f.Service != "service-2" {
		t.Errorf("service: want service-2, got %s", f.Service)
	}
	if f.Stage != "builder" {
		t.Errorf("stage: want builder, got %s", f.Stage)
	}
	if !strings.Contains(f.Error, "TS2307") {
		t.Errorf("error should mention TS2307; got %s", f.Error)
	}
	if !contains(f.FileRefs, "ingestion-worker/src/pipeline.ts") {
		t.Errorf("file refs should include pipeline.ts; got %v", f.FileRefs)
	}
	if !contains(f.PackageRefs, "@napi-rs/canvas") {
		t.Errorf("package refs should include @napi-rs/canvas; got %v", f.PackageRefs)
	}
}

func TestAnalyzeFailurePythonVersionMismatch(t *testing.T) {
	buildOut := `Step 8/11 : RUN pip install --no-cache-dir .
ERROR: Package 'app' requires a different Python: 3.12.14 not in '<4.0,>=3.14'
The command '/bin/sh -c pip install --no-cache-dir .' returned a non-zero code: 1`
	f := AnalyzeFailure(buildOut, "service-1")
	if f.Category != "environment" {
		t.Errorf("category: want environment, got %s", f.Category)
	}
	if !strings.Contains(f.Error, "requires a different Python") {
		t.Errorf("error should mention Python mismatch; got %s", f.Error)
	}
}

func TestAnalyzeFailureCopyNotFound(t *testing.T) {
	buildOut := `#17 ERROR: failed to calculate checksum: "/app/public": not found
failed to solve: failed to compute cache key`
	f := AnalyzeFailure(buildOut, "service-2")
	if f.Category != "configuration" {
		t.Errorf("category: want configuration, got %s", f.Category)
	}
	if !contains(f.PathRefs, "/app/public") {
		t.Errorf("path refs should include /app/public; got %v", f.PathRefs)
	}
}

func TestAnalyzeFailureEarlierTypeScriptBeatsNextOutputArtifact(t *testing.T) {
	buildOut := `#16 [service-1 builder 6/7] RUN npm run build
#16 8.000 src/page.tsx(7,4): error TS2322: Type string is not assignable to type number
#16 8.400 failed to calculate checksum: "/app/.next": not found
failed to solve: process "/bin/sh -c npm run build" did not complete successfully: exit code: 1`
	f := AnalyzeFailure(buildOut, "service-1")
	if f.Category != "compilation" || !strings.Contains(f.Error, "TS2322") {
		t.Fatalf("expected TypeScript root cause, got category=%s error=%s", f.Category, f.Error)
	}
	if f.RawLog != buildOut || !strings.Contains(f.RelevantLog, ".next") {
		t.Fatal("failure should preserve raw log and downstream context")
	}
}

func TestAnalyzeFailureEarlierViteBeatsDistChecksum(t *testing.T) {
	buildOut := `#18 [service-1 builder 5/6] RUN npm run build
#18 2.000 vite v7.0.0 building for production...
#18 2.400 error during build: Rollup failed to resolve import "./missing" from "src/main.ts"
#18 2.500 failed to calculate checksum: "/app/dist": not found
failed to solve: process "/bin/sh -c npm run build" did not complete successfully: exit code: 1`
	f := AnalyzeFailure(buildOut, "service-1")
	if f.Category != "compilation" || !strings.Contains(strings.ToLower(f.Error), "rollup") {
		t.Fatalf("expected Vite/Rollup root cause, got category=%s error=%s", f.Category, f.Error)
	}
}

func TestAnalyzeFailureNpmErrorBeatsDockerWrapper(t *testing.T) {
	buildOut := `#9 [service-1 builder 4/6] RUN npm ci
#9 0.300 npm error code EUSAGE
#9 0.300 npm error The npm ci command can only install with an existing package-lock.json
failed to solve: process "/bin/sh -c npm ci" did not complete successfully: exit code: 1`
	f := AnalyzeFailure(buildOut, "service-1")
	if !strings.Contains(strings.ToLower(f.Error), "npm error") || f.Category != "dependency" {
		t.Fatalf("expected npm root cause, got category=%s error=%s", f.Category, f.Error)
	}
}

func TestAnalyzeFailurePythonTracebackBeatsDockerWrapper(t *testing.T) {
	buildOut := `#12 [service-1 builder 6/7] RUN pip install .
#12 1.000 Traceback (most recent call last):
#12 1.100   File "app/config.py", line 4, in <module>
#12 1.100 ImportError: missing optional dependency
failed to solve: process "/bin/sh -c pip install ." did not complete successfully: exit code: 1`
	f := AnalyzeFailure(buildOut, "service-1")
	if !strings.Contains(strings.ToLower(f.Error), "importerror") || f.Category != "dependency" {
		t.Fatalf("expected Python import root cause, got category=%s error=%s", f.Category, f.Error)
	}
}

func TestAnalyzeFailureDockerfileErrorWhenOnlyWrapperExists(t *testing.T) {
	buildOut := `Dockerfile:7
unknown instruction: BROKEN
failed to solve: Dockerfile parse error`
	f := AnalyzeFailure(buildOut, "service-1")
	if f.Category != "docker-build" || !strings.Contains(strings.ToLower(f.Error), "unknown instruction") {
		t.Fatalf("expected Dockerfile root cause, got category=%s error=%s", f.Category, f.Error)
	}
}

func TestAnalyzeFailureMissingEnvironmentIsConfiguration(t *testing.T) {
	buildOut := `#16 [service-1 builder 6/7] RUN npm run build
Error: DATABASE_URL is required
failed to solve: process "/bin/sh -c npm run build" did not complete successfully: exit code: 1`
	f := AnalyzeFailure(buildOut, "service-1")
	if f.Category != "missing-environment" || !contains(f.EnvRefs, "DATABASE_URL") {
		t.Fatalf("expected missing env configuration error, got category=%s env=%v error=%s", f.Category, f.EnvRefs, f.Error)
	}
}

func TestClassifyProgressionSameFailure(t *testing.T) {
	prev := &Failure{Error: "Cannot find module 'X'", Category: "compilation"}
	curr := &Failure{Error: "Cannot find module 'X'", Category: "compilation"}
	if got := ClassifyProgression(curr, prev); got != "same_failure" {
		t.Errorf("want same_failure, got %s", got)
	}
}

func TestClassifyProgressionProgressed(t *testing.T) {
	prev := &Failure{Error: "npm ci failed", Category: "dependency"}
	curr := &Failure{Error: "Cannot find module 'X'", Category: "compilation"}
	if got := ClassifyProgression(curr, prev); got != "progressed" {
		t.Errorf("want progressed (category changed), got %s", got)
	}
}

func TestClassifyProgressionFirst(t *testing.T) {
	curr := &Failure{Error: "some error", Category: "docker-build"}
	if got := ClassifyProgression(curr, nil); got != "first" {
		t.Errorf("want first, got %s", got)
	}
}

func TestHashStringStable(t *testing.T) {
	h1 := hashString("FROM node:20-alpine\n")
	h2 := hashString("FROM node:20-alpine\n")
	h3 := hashString("FROM node:22-alpine\n")
	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	if h1 == h3 {
		t.Error("different input should produce different hash")
	}
}

func TestRelevantFileSelectionForTS2307(t *testing.T) {
	// Create a fixture repo root with a package.json.
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"dependencies":{"next":"14"}}`), 0644)
	os.MkdirAll(filepath.Join(root, "ingestion-worker", "src"), 0755)
	os.WriteFile(filepath.Join(root, "ingestion-worker", "src", "pipeline.ts"),
		[]byte(`import '@napi-rs/canvas'`), 0644)

	f := Failure{
		Category:    "compilation",
		FileRefs:    []string{"ingestion-worker/src/pipeline.ts"},
		PackageRefs: []string{"@napi-rs/canvas"},
	}
	svc := detector.Service{
		ID:        "s1",
		Directory: "",
		Language:  "typescript",
		Framework: "next",
	}
	files := selectRelevantFiles(svc, f, root)
	foundManifest := false
	for _, fe := range files {
		if strings.HasSuffix(fe.Path, "package.json") {
			foundManifest = true
		}
	}
	if !foundManifest {
		t.Errorf("expected package.json to be selected for a TS dependency failure; got %d files", len(files))
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
