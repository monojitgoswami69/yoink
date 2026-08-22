package testdata

import (
	"strings"
	"testing"

	"yoink/internal/detector"
	"yoink/internal/envvar"
	"yoink/internal/generator"
	"yoink/internal/infra"
)

// TestFixtureCorpus runs each synthetic fixture through the full
// detect → infer → generate pipeline and verifies the expected output.
// This is the regression corpus that catches deterministic bugs before
// they require LLM healing.
func TestFixtureCorpus(t *testing.T) {
	for _, fx := range Fixtures {
		t.Run(fx.Name, func(t *testing.T) {
			dir := fx.Write(t)

			// 1. Detect.
			res, err := detector.Detect(dir)
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if len(res.Services) != fx.Expect.ServiceCount {
				t.Fatalf("service count: want %d, got %d (%+v)", fx.Expect.ServiceCount, len(res.Services), res.Services)
			}

			// 2. Verify expected frameworks.
			for i, want := range fx.Expect.Frameworks {
				if i >= len(res.Services) {
					break
				}
				if res.Services[i].Framework != want {
					t.Errorf("framework[%d]: want %s, got %s", i, want, res.Services[i].Framework)
				}
			}

			// 3. Verify expected languages.
			for i, want := range fx.Expect.Languages {
				if i >= len(res.Services) {
					break
				}
				if res.Services[i].Language != want {
					t.Errorf("language[%d]: want %s, got %s", i, want, res.Services[i].Language)
				}
			}

			// 4. Extract env vars + infer infra.
			envResults := envvar.Detect(dir, res.Services)
			inference := infra.Infer(envResults)

			// 5. Verify expected infra.
			if len(fx.Expect.Infra) > 0 {
				infraNames := make(map[string]bool)
				for _, s := range inference.Services {
					infraNames[s.Name] = true
				}
				for _, want := range fx.Expect.Infra {
					if !infraNames[want] {
						t.Errorf("missing inferred infra: %s (got %v)", want, infraNames)
					}
				}
			}

			// 6. Generate Dockerfiles + compose.
			out := generator.Build(res.Services, generator.Options{
				Repo:         "test",
				OutputSubdir: "yoink-outputs",
				Infra:        inference.Services,
				Links:        inference.Links,
			})
			if len(out.Files) == 0 {
				t.Fatal("no files generated")
			}

			// 7. Verify expected Dockerfile substrings.
			for _, want := range fx.Expect.DockerfileContains {
				found := false
				for name, content := range out.Files {
					if strings.HasPrefix(name, "Dockerfile.") && strings.Contains(content, want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Dockerfile should contain %q; files: %v", want, fileNames(out.Files))
				}
			}

			// 8. Verify .dockerignore is generated.
			if out.Dockerignore == "" {
				t.Error("Dockerignore should be non-empty")
			}
		})
	}
}

func fileNames(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
