package healer

import (
	"strings"
	"testing"
)

// TestAnalyzeFailureMissingEnvVar proves that "X is not defined" errors
// are categorized as "missing-environment" and the variable name is
// extracted into EnvRefs.
func TestAnalyzeFailureMissingEnvVar(t *testing.T) {
	buildOut := `#16 [service-1 builder 6/7] RUN npm run build
#16 5.123 Error: DATABASE_URL is not defined
failed to solve: process "/bin/sh -c npm run build" did not complete successfully: exit code: 1`
	f := AnalyzeFailure(buildOut, "service-1")
	if f.Category != "missing-environment" {
		t.Errorf("category: want missing-environment, got %s", f.Category)
	}
	if !contains(f.EnvRefs, "DATABASE_URL") {
		t.Errorf("EnvRefs should include DATABASE_URL; got %v", f.EnvRefs)
	}
}

// TestAnalyzeFailureNextjsBuildFailure proves that "Failed to collect
// page data" errors are categorized as "nextjs-build" (NOT blindly
// classified as missing-environment).
func TestAnalyzeFailureNextjsBuildFailure(t *testing.T) {
	buildOut := `#16 [service-1 builder 6/7] RUN npm run build
#16 8.136 > Build error occurred
#16 8.138 Error: Failed to collect page data for /_not-found
failed to solve: process "/bin/sh -c npm run build" did not complete successfully: exit code: 1`
	f := AnalyzeFailure(buildOut, "service-1")
	if f.Category != "nextjs-build" {
		t.Errorf("category: want nextjs-build, got %s", f.Category)
	}
}

// TestFixMissingEnvInjectsNonSecretVar proves that a non-secret env var
// explicitly named in the error ("X is not defined") gets a deterministic
// ENV placeholder injected into the Dockerfile.
func TestFixMissingEnvInjectsNonSecretVar(t *testing.T) {
	errTail := "Error: DATABASE_URL is not defined"
	df := "FROM node:20-alpine\nWORKDIR /app\nRUN npm run build\n"
	fixed, summary, ok := deterministicFix(errTail, df, "s1")
	if !ok {
		t.Fatal("expected deterministic fix for non-secret missing env var")
	}
	if !strings.Contains(fixed, "ENV DATABASE_URL=yoink-build-placeholder") {
		t.Errorf("Dockerfile should contain ENV DATABASE_URL=placeholder; got:\n%s", fixed)
	}
	if !strings.Contains(summary, "DATABASE_URL") {
		t.Errorf("summary should mention DATABASE_URL; got %s", summary)
	}
}

// TestFixMissingEnvRejectsSecretVar proves that secrets named in errors
// ("JWT_SECRET is not defined") are NOT fabricated — the fixer returns false
// so the LLM gets the structured failure instead.
func TestFixMissingEnvRejectsSecretVar(t *testing.T) {
	errTail := "Error: JWT_SECRET is not defined"
	df := "FROM node:20-alpine\nRUN npm run build\n"
	_, _, ok := deterministicFix(errTail, df, "s1")
	if ok {
		t.Error("should NOT fabricate JWT_SECRET — credentials must never be invented")
	}
}

// TestFixMissingEnvRejectsEnglishWord proves that "ERROR is not defined"
// doesn't trigger a false-positive env var injection.
func TestFixMissingEnvRejectsEnglishWord(t *testing.T) {
	errTail := "ERROR: something is not defined properly"
	df := "FROM node:20-alpine\nRUN npm run build\n"
	_, _, ok := deterministicFix(errTail, df, "s1")
	if ok {
		t.Error("should NOT treat 'ERROR' as an env var name")
	}
}

// TestFixMissingEnvNoOpWhenAlreadyPresent proves the fixer doesn't
// duplicate ENV directives that already exist.
func TestFixMissingEnvNoOpWhenAlreadyPresent(t *testing.T) {
	errTail := "Error: DATABASE_URL is not defined"
	df := "FROM node:20-alpine\nENV DATABASE_URL=yoink-build-placeholder\nRUN npm run build\n"
	_, _, ok := deterministicFix(errTail, df, "s1")
	if ok {
		t.Error("should not add duplicate ENV when it already exists")
	}
}
