package healer

import (
	"strings"
	"testing"
)

// TestSnapshotIncludesRelevantFiles proves Phase 1: the context snapshot
// hashes ALL files supplied to the LLM, not just Dockerfile + compose.
// When a relevant file (package.json) changes, staleness is detected.
func TestSnapshotIncludesRelevantFiles(t *testing.T) {
	files := map[string]string{
		"Dockerfile.service-1": "FROM node:20\nRUN npm ci\n",
		"docker-compose.yml":   "services:\n  s1:\n    build: ..\n",
		"package.json":         `{"dependencies":{"express":"4"}}`,
		"tsconfig.json":        `{"compilerOptions":{}}`,
	}
	snap := NewSnapshot(files)
	// Mutate package.json only.
	current := map[string]string{
		"Dockerfile.service-1": "FROM node:20\nRUN npm ci\n",
		"docker-compose.yml":   "services:\n  s1:\n    build: ..\n",
		"package.json":         `{"dependencies":{"express":"5"}}`, // changed!
		"tsconfig.json":        `{"compilerOptions":{}}`,
	}
	staleFile, isStale := snap.IsStale(current)
	if !isStale {
		t.Fatal("snapshot should detect package.json mutation")
	}
	if staleFile != "package.json" {
		t.Errorf("stale file should be package.json, got %s", staleFile)
	}
}

// TestSnapshotIgnoresIrrelevantFiles proves that files NOT in the snapshot
// don't trigger staleness (even if they change on disk).
func TestSnapshotIgnoresIrrelevantFiles(t *testing.T) {
	files := map[string]string{
		"Dockerfile.service-1": "FROM node:20\n",
		"docker-compose.yml":   "services:\n",
	}
	snap := NewSnapshot(files)
	// Only Dockerfile + compose were snapshotted. A changed README.md
	// should NOT trigger staleness.
	current := map[string]string{
		"Dockerfile.service-1": "FROM node:20\n",
		"docker-compose.yml":   "services:\n",
		"README.md":            "# Changed! This file was not in the snapshot.",
	}
	_, isStale := snap.IsStale(current)
	if isStale {
		t.Error("irrelevant file change should NOT trigger staleness")
	}
}

// TestFingerprintNormalization proves Phase 5: failure fingerprints are
// normalized so the same root failure is recognized despite volatile details
// (line numbers, BuildKit prefixes, file paths).
func TestFingerprintNormalization(t *testing.T) {
	f1 := Failure{Category: "compilation", Error: "ingestion-worker/src/pipeline.ts(6,29): error TS2307: Cannot find module '@napi-rs/canvas'"}
	f2 := Failure{Category: "compilation", Error: "ingestion-worker/src/pipeline.ts(12,5): error TS2307: Cannot find module '@napi-rs/canvas'"}
	// Different line numbers, same root failure → same fingerprint.
	if f1.Fingerprint() != f2.Fingerprint() {
		t.Errorf("same root failure should have same fingerprint:\n  %s\n  %s", f1.Fingerprint(), f2.Fingerprint())
	}
}

// TestFingerprintDifferentFailures proves different root failures get
// different fingerprints.
func TestFingerprintDifferentFailures(t *testing.T) {
	f1 := Failure{Category: "compilation", Error: "Cannot find module 'X'"}
	f2 := Failure{Category: "dependency", Error: "Cannot find module 'Y'"}
	if f1.Fingerprint() == f2.Fingerprint() {
		t.Error("different failures should have different fingerprints")
	}
}

// TestScopeBroadRejected proves Phase 3: broad repairs are now REJECTED,
// not just warned.
func TestScopeBroadRejected(t *testing.T) {
	bigContent := strings.Repeat("RUN echo line\n", 60)
	changes := []Change{{Content: bigContent}}
	scope := EvaluateScope(changes, nil, "")
	if !scope.ShouldReject() {
		t.Errorf("60-line repair should be rejected (class=%s)", scope.Class)
	}
	if scope.Class != ScopeBroad {
		t.Errorf("class: want broad, got %s", scope.Class)
	}
}

// TestScopeForbiddenRejected proves sleep-CMD is classified as forbidden.
func TestScopeForbiddenRejected(t *testing.T) {
	changes := []Change{{Content: `CMD ["sleep", "infinity"]`}}
	scope := EvaluateScope(changes, nil, "")
	if !scope.ShouldReject() {
		t.Errorf("sleep CMD should be forbidden (class=%s)", scope.Class)
	}
}

// TestScopeSafeAccepted proves small legitimate repairs pass.
func TestScopeSafeAccepted(t *testing.T) {
	changes := []Change{{Content: "RUN apk add --no-cache curl"}}
	scope := EvaluateScope(changes, nil, "")
	if scope.ShouldReject() {
		t.Errorf("small repair should be safe, not rejected (class=%s)", scope.Class)
	}
	if scope.Class != ScopeSafe {
		t.Errorf("class: want safe, got %s", scope.Class)
	}
}
