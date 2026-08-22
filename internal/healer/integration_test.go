package healer

import (
	"testing"

	"yoink/internal/detector"
	"yoink/internal/generator"
	"yoink/internal/llm"
)

// TestApplyFixRejectsDisableTypeCheck proves that the production applyFix
// path now calls CheckInvariants and rejects repairs that weaken validation.
// This is the integration test that proves the invariant checker is wired
// into the real healer execution path.
func TestApplyFixRejectsDisableTypeCheck(t *testing.T) {
	l := &Loop{
		Output: &generator.Output{Files: map[string]string{
			"Dockerfile.service-1": "FROM node:20-alpine\nRUN npm run build\nCMD [\"npm\", \"start\"]\n",
		}},
		Services:  []detector.Service{{ID: "service-1"}},
		OutputDir: t.TempDir(),
	}

	// LLM proposes a Dockerfile that disables TypeScript checking.
	fix := &llm.BuildFixResponse{
		Service:    "service-1",
		Dockerfile: "FROM node:20-alpine\nENV NEXT_TYPESCRIPT_IGNORE_BUILD_ERRORS=true\nRUN npm run build\nCMD [\"npm\", \"start\"]\n",
	}

	applied, err := l.applyFix(fix, "service-1")
	if err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Error("applyFix should reject a Dockerfile that disables type checking")
	}

	// Verify the original Dockerfile is unchanged.
	if l.Output.Files["Dockerfile.service-1"] != "FROM node:20-alpine\nRUN npm run build\nCMD [\"npm\", \"start\"]\n" {
		t.Error("original Dockerfile should not be modified when fix is rejected")
	}
}

// TestApplyFixAcceptsLegitimateFix proves that legitimate repairs pass the
// invariant checks in the production path.
func TestApplyFixAcceptsLegitimateFix(t *testing.T) {
	l := &Loop{
		Output: &generator.Output{Files: map[string]string{
			"Dockerfile.service-1": "FROM node:20-alpine\nRUN npm ci\nCOPY . .\nCMD [\"npm\", \"start\"]\n",
		}},
		Services:  []detector.Service{{ID: "service-1"}},
		OutputDir: t.TempDir(),
	}

	// LLM proposes a legitimate fix: add libc6-compat for native modules.
	fix := &llm.BuildFixResponse{
		Service:    "service-1",
		Dockerfile: "FROM node:20-alpine\nRUN apk add --no-cache libc6-compat\nRUN npm ci\nCOPY . .\nCMD [\"npm\", \"start\"]\n",
	}

	applied, err := l.applyFix(fix, "service-1")
	if err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Error("applyFix should accept a legitimate fix")
	}
}

// TestApplyFixRejectsHealthcheckRemoval proves the compose invariant is
// wired into the production path.
func TestApplyFixRejectsHealthcheckRemoval(t *testing.T) {
	originalCompose := `services:
  web:
    build: ..
    healthcheck:
      test: ["CMD", "curl", "http://localhost:3000"]
    ports:
      - "3000:3000"
`
	l := &Loop{
		Output: &generator.Output{Files: map[string]string{
			"docker-compose.yml": originalCompose,
		}},
		Services:  []detector.Service{{ID: "web"}},
		OutputDir: t.TempDir(),
	}

	// LLM proposes removing the healthcheck.
	badCompose := `services:
  web:
    build: ..
    ports:
      - "3000:3000"
`
	fix := &llm.BuildFixResponse{
		Compose: badCompose,
	}

	applied, _ := l.applyFix(fix, "web")
	if applied {
		t.Error("applyFix should reject a compose that removes a healthcheck")
	}

	// Verify the original compose is unchanged.
	if l.Output.Files["docker-compose.yml"] != originalCompose {
		t.Error("original compose should not be modified when fix is rejected")
	}
}

// TestApplyFixRejectsSleepCMD proves the command-replacement invariant is
// wired in.
func TestApplyFixRejectsSleepCMD(t *testing.T) {
	l := &Loop{
		Output: &generator.Output{Files: map[string]string{
			"Dockerfile.service-1": "FROM node:20-alpine\nCMD [\"node\", \"index.js\"]\n",
		}},
		Services:  []detector.Service{{ID: "service-1"}},
		OutputDir: t.TempDir(),
	}

	fix := &llm.BuildFixResponse{
		Service:    "service-1",
		Dockerfile: "FROM node:20-alpine\nCMD [\"sleep\", \"infinity\"]\n",
	}

	applied, _ := l.applyFix(fix, "service-1")
	if applied {
		t.Error("applyFix should reject replacing CMD with sleep")
	}
}
