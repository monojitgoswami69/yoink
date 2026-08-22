package preflight

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckValidCompose(t *testing.T) {
	compose := `services:
  service-1:
    build:
      context: ..
      dockerfile: yoink-outputs/Dockerfile.service-1
    ports:
      - "8000:8000"
    env_file:
      - env-vars/service-1/.env
`
	files := map[string]string{"Dockerfile.service-1": "FROM python:3.12-slim\n"}
	dir := t.TempDir()
	// create the env file
	os.MkdirAll(filepath.Join(dir, "env-vars", "service-1"), 0755)
	os.WriteFile(filepath.Join(dir, "env-vars", "service-1", ".env"), []byte("KEY=val"), 0600)

	issues := Check(compose, files, dir)
	if HasErrors(issues) {
		t.Errorf("valid compose should have no errors; got %v", issues)
	}
}

func TestCheckMissingDockerfile(t *testing.T) {
	compose := `services:
  service-1:
    build:
      context: ..
      dockerfile: yoink-outputs/Dockerfile.service-1
`
	files := map[string]string{} // no Dockerfile!
	issues := Check(compose, files, t.TempDir())
	if !HasErrors(issues) {
		t.Errorf("missing Dockerfile should be an error; got %v", issues)
	}
}

func TestCheckDuplicatePorts(t *testing.T) {
	compose := `services:
  a:
    build: {context: .., dockerfile: yoink-outputs/Dockerfile.a}
    ports: ["3000:3000"]
  b:
    build: {context: .., dockerfile: yoink-outputs/Dockerfile.b}
    ports: ["3000:3000"]
`
	files := map[string]string{"Dockerfile.a": "FROM alpine\n", "Dockerfile.b": "FROM alpine\n"}
	issues := Check(compose, files, t.TempDir())
	found := false
	for _, i := range issues {
		if i.Severity == "warning" && i.Service == "b" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a warning for duplicate port; got %v", issues)
	}
}

func TestCheckInvalidYAML(t *testing.T) {
	compose := `services: [this is not valid`
	issues := Check(compose, map[string]string{}, t.TempDir())
	if !HasErrors(issues) {
		t.Errorf("invalid YAML should be an error")
	}
}
