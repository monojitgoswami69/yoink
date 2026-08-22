package healer

import (
	"strings"
	"testing"
)

func TestApplyInsertAfter(t *testing.T) {
	content := "FROM node:20-alpine\nWORKDIR /app\nCOPY . .\n"
	change := Change{
		Operation: "insert_after",
		Anchor:    "WORKDIR /app",
		Content:   "RUN apk add --no-cache curl",
	}
	result, err := ApplyPatch(content, change)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "WORKDIR /app\nRUN apk add --no-cache curl\nCOPY") {
		t.Errorf("insert_after should place content between WORKDIR and COPY; got:\n%s", result)
	}
}

func TestApplyInsertBefore(t *testing.T) {
	content := "FROM node:20-alpine\nWORKDIR /app\nCOPY . .\n"
	change := Change{
		Operation: "insert_before",
		Anchor:    "COPY . .",
		Content:   "RUN npm ci",
	}
	result, err := ApplyPatch(content, change)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "RUN npm ci\nCOPY . .") {
		t.Errorf("insert_before should place content before COPY; got:\n%s", result)
	}
}

func TestApplyReplaceLine(t *testing.T) {
	content := "FROM node:20-alpine\nRUN npm ci\nCMD [\"npm\", \"start\"]\n"
	change := Change{
		Operation: "replace_line",
		Anchor:    "RUN npm ci",
		Content:   "RUN npm install --no-audit --no-fund",
	}
	result, err := ApplyPatch(content, change)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "RUN npm install --no-audit --no-fund") {
		t.Errorf("replace_line should replace npm ci; got:\n%s", result)
	}
	if strings.Contains(result, "npm ci\n") {
		t.Errorf("old line should be gone; got:\n%s", result)
	}
}

func TestApplyReplaceExact(t *testing.T) {
	content := "FROM python:3.12-slim\nWORKDIR /app\n"
	change := Change{
		Operation: "replace_exact",
		Anchor:    "python:3.12-slim",
		Content:   "python:3.14-slim",
	}
	result, err := ApplyPatch(content, change)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "FROM python:3.14-slim") {
		t.Errorf("replace_exact should bump python version; got:\n%s", result)
	}
}

func TestApplyCreateFile(t *testing.T) {
	change := Change{
		Operation: "create_file",
		Content:   "FROM alpine\nCMD [\"echo\", \"hello\"]\n",
	}
	result, err := ApplyPatch("", change)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "FROM alpine") {
		t.Errorf("create_file should return the content; got:\n%s", result)
	}
}

func TestApplyAnchorNotFound(t *testing.T) {
	content := "FROM node:20-alpine\n"
	change := Change{Operation: "insert_after", Anchor: "WORKDIR /app", Content: "RUN npm ci"}
	_, err := ApplyPatch(content, change)
	if err == nil {
		t.Error("expected error when anchor not found")
	}
}

func TestApplyAmbiguousAnchor(t *testing.T) {
	content := "RUN npm ci\nRUN npm ci\n"
	change := Change{Operation: "insert_after", Anchor: "RUN npm ci", Content: "RUN echo done"}
	_, err := ApplyPatch(content, change)
	if err == nil {
		t.Error("expected error when anchor matches multiple locations")
	}
}

// --- Invariant tests ---

func TestInvariantRejectsDisableTypeCheck(t *testing.T) {
	changes := []Change{{
		File:      "Dockerfile.service-2",
		Operation: "insert_after",
		Anchor:    "RUN npm run build",
		Content:   "ENV NEXT_TYPESCRIPT_IGNORE_BUILD_ERRORS=true",
	}}
	originals := map[string]string{"Dockerfile.service-2": "FROM node:20\nRUN npm run build\n"}
	violations := CheckInvariants(changes, originals)
	if !HasRejections(violations) {
		t.Errorf("should reject disabling type checking; got %v", violations)
	}
}

func TestInvariantRejectsHealthcheckRemoval(t *testing.T) {
	original := `services:
  web:
    build: ..
    healthcheck:
      test: ["CMD", "curl", "http://localhost:3000"]
`
	changes := []Change{{
		File:      "docker-compose.yml",
		Operation: "replace_exact",
		Anchor:    "    healthcheck:\n      test: [\"CMD\", \"curl\", \"http://localhost:3000\"]\n",
		Content:   "",
	}}
	violations := CheckInvariants(changes, map[string]string{"docker-compose.yml": original})
	if !HasRejections(violations) {
		t.Errorf("should reject healthcheck removal; got %v", violations)
	}
}

func TestInvariantRejectsSleepCMD(t *testing.T) {
	changes := []Change{{
		File:      "Dockerfile.service-1",
		Operation: "replace_exact",
		Anchor:    `CMD ["node", "index.js"]`,
		Content:   `CMD ["sleep", "infinity"]`,
	}}
	originals := map[string]string{"Dockerfile.service-1": `CMD ["node", "index.js"]`}
	violations := CheckInvariants(changes, originals)
	if !HasRejections(violations) {
		t.Errorf("should reject sleep CMD; got %v", violations)
	}
}

func TestInvariantAcceptsLegitimateFix(t *testing.T) {
	changes := []Change{{
		File:      "Dockerfile.service-2",
		Operation: "insert_after",
		Anchor:    "RUN npm ci --no-audit --no-fund",
		Content:   "RUN cd ingestion-worker && npm ci --no-audit --no-fund",
	}}
	originals := map[string]string{"Dockerfile.service-2": "RUN npm ci --no-audit --no-fund\n"}
	violations := CheckInvariants(changes, originals)
	if HasRejections(violations) {
		t.Errorf("should accept a legitimate dependency fix; got %v", violations)
	}
}

func TestInvariantWarnsOnSourceFileModification(t *testing.T) {
	changes := []Change{{
		File:      "src/index.ts",
		Operation: "replace_exact",
		Anchor:    "import { X } from 'missing'",
		Content:   "import { X } from 'available'",
	}}
	originals := map[string]string{"src/index.ts": "import { X } from 'missing'\n"}
	violations := CheckInvariants(changes, originals)
	found := false
	for _, v := range violations {
		if v.Severity == "warn" && strings.Contains(v.Violation, "non-generated") {
			found = true
		}
	}
	if !found {
		t.Errorf("should warn on source file modification; got %v", violations)
	}
}

// --- Stale context tests ---

func TestSnapshotIsStaleAfterMutation(t *testing.T) {
	files := map[string]string{
		"Dockerfile.service-1": "FROM node:20-alpine\n",
		"docker-compose.yml":   "services:\n  web:\n    build: ..\n",
	}
	snap := NewSnapshot(files)
	// Mutate the Dockerfile.
	current := map[string]string{
		"Dockerfile.service-1": "FROM node:22-alpine\n", // changed!
		"docker-compose.yml":   "services:\n  web:\n    build: ..\n",
	}
	staleFile, isStale := snap.IsStale(current)
	if !isStale {
		t.Error("snapshot should be stale after Dockerfile mutation")
	}
	if staleFile != "Dockerfile.service-1" {
		t.Errorf("stale file should be Dockerfile.service-1, got %s", staleFile)
	}
}

func TestSnapshotNotStaleWhenUnchanged(t *testing.T) {
	files := map[string]string{
		"Dockerfile.service-1": "FROM node:20-alpine\n",
		"docker-compose.yml":   "services:\n",
	}
	snap := NewSnapshot(files)
	_, isStale := snap.IsStale(files)
	if isStale {
		t.Error("snapshot should NOT be stale when files are unchanged")
	}
}

func TestSnapshotStaleOnMultiFileChange(t *testing.T) {
	// Dockerfile unchanged, package.json changed.
	files := map[string]string{
		"Dockerfile.service-1": "FROM node:20-alpine\n",
		"package.json":         `{"dependencies":{"express":"4"}}`,
	}
	snap := NewSnapshot(files)
	current := map[string]string{
		"Dockerfile.service-1": "FROM node:20-alpine\n",            // unchanged
		"package.json":         `{"dependencies":{"express":"5"}}`, // changed!
	}
	staleFile, isStale := snap.IsStale(current)
	if !isStale {
		t.Error("snapshot should be stale when package.json changed")
	}
	if staleFile != "package.json" {
		t.Errorf("stale file should be package.json, got %s", staleFile)
	}
}

// --- Scope evaluation tests ---

func TestEvaluateScopeNotBroadForSmallPatch(t *testing.T) {
	changes := []Change{{Content: "RUN npm ci"}}
	originals := map[string]string{"Dockerfile.service-1": "FROM node:20\n"}
	assessment := EvaluateScope(changes, originals, "dependency")
	if assessment.ShouldReject() {
		t.Errorf("small patch should not be flagged as broad: %s", assessment.Reason)
	}
}

func TestEvaluateScopeBroadForLargePatch(t *testing.T) {
	big := strings.Repeat("RUN echo line\n", 60)
	changes := []Change{{Content: big}}
	originals := map[string]string{"Dockerfile.service-1": "FROM node:20\n"}
	assessment := EvaluateScope(changes, originals, "dependency")
	if !assessment.ShouldReject() {
		t.Error("60-line patch should be flagged as broad")
	}
}
