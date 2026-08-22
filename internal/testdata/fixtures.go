// Package testdata provides synthetic repository fixtures that reproduce
// real-world patterns for regression testing. Each fixture is a minimal
// file tree that triggers a specific detection/generation scenario.
package testdata

import (
	"os"
	"path/filepath"
	"testing"
)

// Fixture is a synthetic repository structure for regression testing.
type Fixture struct {
	Name   string
	Files  map[string]string // relative path → content
	Expect Expectation
}

// Expectation describes what the detector and generator should produce
// for a given fixture.
type Expectation struct {
	ServiceCount       int
	Frameworks         []string
	Languages          []string
	Infra              []string // expected infra service names
	DockerfileContains []string // substrings that must appear in generated Dockerfile
}

// Write creates the fixture in a temp directory and returns the path.
func (f Fixture) Write(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for path, content := range f.Files {
		full := filepath.Join(dir, path)
		os.MkdirAll(filepath.Dir(full), 0755)
		os.WriteFile(full, []byte(content), 0644)
	}
	return dir
}

// Fixtures is the corpus of regression test cases.
var Fixtures = []Fixture{
	{
		Name: "simple-fastapi-requirements",
		Files: map[string]string{
			"requirements.txt": "fastapi\nuvicorn\n",
			"main.py":          "from fastapi import FastAPI\napp = FastAPI()\n",
		},
		Expect: Expectation{
			ServiceCount: 1, Frameworks: []string{"fastapi"}, Languages: []string{"python"},
			DockerfileContains: []string{"python:3.12-slim", "uvicorn"},
		},
	},
	{
		Name: "simple-express-npm",
		Files: map[string]string{
			"package.json":      `{"dependencies":{"express":"4"},"scripts":{"start":"node index.js"}}`,
			"package-lock.json": `{"name":"app","lockfileVersion":3}`,
			"index.js":          "const express = require('express'); express.listen(3000);",
		},
		Expect: Expectation{
			ServiceCount: 1, Frameworks: []string{"express"}, Languages: []string{"javascript"},
			DockerfileContains: []string{"node:20-alpine", "npm ci"},
		},
	},
	{
		Name: "nextjs-with-lockfile",
		Files: map[string]string{
			"package.json":      `{"dependencies":{"next":"14","react":"18"},"scripts":{"build":"next build","start":"next start"}}`,
			"package-lock.json": `{"name":"app","lockfileVersion":3}`,
			"next.config.mjs":   "export default {}",
		},
		Expect: Expectation{
			ServiceCount: 1, Frameworks: []string{"next"}, Languages: []string{"javascript"},
			DockerfileContains: []string{"node:20-alpine", "npm ci", "/staged/"},
		},
	},
	{
		Name: "go-simple",
		Files: map[string]string{
			"go.mod":  "module myapp\n\ngo 1.23\n",
			"main.go": "package main\nimport \"net/http\"\nfunc main() { http.ListenAndServe(\":8080\", nil) }\n",
		},
		Expect: Expectation{
			ServiceCount: 1, Frameworks: []string{"go"}, Languages: []string{"go"},
			DockerfileContains: []string{"golang:1.23-alpine", "go build"},
		},
	},
	{
		Name: "rust-cargo",
		Files: map[string]string{
			"Cargo.toml":  `[package]\nname = "my-app"\nversion = "0.1.0"\n[dependencies]\naxum = "0.7"`,
			"src/main.rs": "fn main() { println!(\"hello\"); }",
		},
		Expect: Expectation{
			ServiceCount: 1, Frameworks: []string{"rust"}, Languages: []string{"rust"},
			DockerfileContains: []string{"rust:1.83-slim", "cargo build"},
		},
	},
	{
		Name: "fastapi-with-postgres-dep",
		Files: map[string]string{
			"requirements.txt": "fastapi\nuvicorn\npsycopg\n",
			"main.py":          "from fastapi import FastAPI\napp = FastAPI()\n",
		},
		Expect: Expectation{
			ServiceCount: 1, Frameworks: []string{"fastapi"}, Languages: []string{"python"},
			Infra:              []string{"postgres"},
			DockerfileContains: []string{"libpq-dev"},
		},
	},
	{
		Name: "bun-express",
		Files: map[string]string{
			"package.json": `{"dependencies":{"express":"4"},"scripts":{"start":"node index.js"}}`,
			"bun.lock":     "",
			"index.js":     "const express = require('express'); express.listen(3000);",
		},
		Expect: Expectation{
			ServiceCount: 1, Frameworks: []string{"express"}, Languages: []string{"javascript"},
			DockerfileContains: []string{"node:20-alpine"},
		},
	},
	{
		Name: "vite-react-static",
		Files: map[string]string{
			"package.json":      `{"devDependencies":{"vite":"5","@vitejs/plugin-react":"4"},"dependencies":{"react":"18"},"scripts":{"build":"vite build","preview":"vite preview"}}`,
			"package-lock.json": `{"name":"app","lockfileVersion":3}`,
			"vite.config.ts":    `import { defineConfig } from 'vite'; export default defineConfig({});`,
		},
		Expect: Expectation{
			ServiceCount: 1, Frameworks: []string{"vite"}, Languages: []string{"javascript"},
			DockerfileContains: []string{"nginx:alpine"},
		},
	},
}
