package healer

import (
	"strings"
	"testing"

	"yoink/internal/generator"
)

func TestNonEmptyFallback(t *testing.T) {
	if got := nonEmpty("", "default"); got != "default" {
		t.Errorf("expected fallback, got %q", got)
	}
	if got := nonEmpty("  ", "default"); got != "default" {
		t.Errorf("expected fallback on whitespace, got %q", got)
	}
	if got := nonEmpty("real", "default"); got != "real" {
		t.Errorf("expected real value, got %q", got)
	}
}

func TestDockerfileForServiceFindsByID(t *testing.T) {
	l := &Loop{Output: &generator.Output{Files: map[string]string{
		"Dockerfile.service-1": "FROM alpine\n",
		"Dockerfile.service-2": "FROM debian\n",
		"docker-compose.yml":   "services: {}",
	}}}
	if got := l.dockerfileForService("service-2"); got != "FROM debian\n" {
		t.Errorf("expected debian dockerfile, got %q", got)
	}
}

func TestDockerfileForServiceFallsBackToBlob(t *testing.T) {
	l := &Loop{Output: &generator.Output{Files: map[string]string{
		"Dockerfile.service-1": "FROM alpine\n",
		"Dockerfile.service-2": "FROM debian\n",
	}}}
	got := l.dockerfileForService("")
	if got == "" || got == "FROM alpine\n" {
		t.Errorf("blob fallback should contain both, got %q", got)
	}
}

func TestDeterministicFixBumpsPythonForRequiresPython(t *testing.T) {
	errTail := `Collecting python-multipart<1.0.0,>=0.0.27 (from app==0.1.0)
ERROR: Package 'app' requires a different Python: 3.12.14 not in '<4.0,>=3.14'
The command '/bin/sh -c pip install --no-cache-dir .' returned a non-zero code: 1`
	df := "# syntax=docker/dockerfile:1.6\nFROM python:3.12-slim\nWORKDIR /app\nRUN pip install .\n"
	fixed, summary, ok := deterministicFix(errTail, df, "service-1")
	if !ok {
		t.Fatalf("expected a deterministic fix for requires-python mismatch")
	}
	if !strings.Contains(fixed, "FROM python:3.14-slim") {
		t.Errorf("should bump to python:3.14-slim; got:\n%s", fixed)
	}
	if !strings.Contains(summary, "3.14") {
		t.Errorf("summary should mention 3.14; got %s", summary)
	}
}

func TestDeterministicFixNoOpWhenAlreadyNewEnough(t *testing.T) {
	errTail := "requires a different Python: 3.14.1 not in '>=3.14'"
	df := "FROM python:3.14-slim\n"
	if _, _, ok := deterministicFix(errTail, df, "s1"); ok {
		t.Errorf("should not re-bump when base already satisfies the constraint")
	}
}

func TestDeterministicFixNoOpForUnrelatedError(t *testing.T) {
	errTail := "ERROR: npm ci failed because package-lock.json is missing"
	df := "FROM node:20-alpine\n"
	if _, _, ok := deterministicFix(errTail, df, "s1"); ok {
		t.Errorf("should not apply a fix for an unrelated error")
	}
}
