// Package preflight validates generated Docker artifacts before an expensive
// `docker compose build` is attempted. It catches malformed YAML, missing
// Dockerfiles, missing env files, duplicate service names, and invalid ports
// so that trivial configuration errors never waste a build round.
package preflight

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Issue is one validation problem. Severity "error" blocks the build;
// "warning" is surfaced but does not block.
type Issue struct {
	Severity string // "error" | "warning"
	Service  string
	Message  string
}

// Check validates the generated compose YAML and the files map (which holds
// the Dockerfiles and compose). It returns a list of issues; if any have
// severity "error", the caller should not attempt a build.
func Check(composeYAML string, files map[string]string, outputDir string) []Issue {
	var issues []Issue

	// 1. Parse the compose YAML.
	var compose struct {
		Services map[string]struct {
			Build struct {
				Context    string `yaml:"context"`
				Dockerfile string `yaml:"dockerfile"`
			} `yaml:"build"`
			Ports   []string `yaml:"ports"`
			EnvFile []string `yaml:"env_file"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(composeYAML), &compose); err != nil {
		issues = append(issues, Issue{Severity: "error", Message: fmt.Sprintf("compose YAML parse error: %v", err)})
		return issues // can't check further without a parse
	}

	// 2. Check each referenced Dockerfile exists in the files map.
	for name, svc := range compose.Services {
		df := svc.Build.Dockerfile
		if df == "" {
			continue
		}
		// The dockerfile path in compose is relative to the context (repo
		// root), e.g. "yoink-outputs/Dockerfile.service-1". In the files
		// map it's keyed by basename, e.g. "Dockerfile.service-1".
		base := filepath.Base(df)
		if _, ok := files[base]; !ok {
			issues = append(issues, Issue{
				Severity: "error", Service: name,
				Message: fmt.Sprintf("Dockerfile %s referenced in compose but not generated", base),
			})
		}
	}

	// 3. Check env files exist on disk (they're written by writeOutputs
	// before the build, so they should be present).
	for name, svc := range compose.Services {
		for _, ef := range svc.EnvFile {
			envPath := filepath.Join(outputDir, ef)
			if _, err := os.Stat(envPath); err != nil {
				issues = append(issues, Issue{
					Severity: "warning", Service: name,
					Message: fmt.Sprintf("env file %s not found at %s", ef, envPath),
				})
			}
		}
	}

	// 4. Check for duplicate host ports.
	hostPorts := map[string]string{} // port → first service that used it
	for name, svc := range compose.Services {
		for _, p := range svc.Ports {
			parts := strings.Split(p, ":")
			if len(parts) < 1 {
				continue
			}
			hostPort := parts[0]
			if prev, ok := hostPorts[hostPort]; ok {
				issues = append(issues, Issue{
					Severity: "warning", Service: name,
					Message: fmt.Sprintf("host port %s also bound by %s", hostPort, prev),
				})
			} else {
				hostPorts[hostPort] = name
			}
		}
	}

	return issues
}

// HasErrors returns true if any issue has severity "error".
func HasErrors(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == "error" {
			return true
		}
	}
	return false
}

// FormatIssues renders the issue list as a human-readable string for CLI output.
func FormatIssues(issues []Issue) string {
	if len(issues) == 0 {
		return ""
	}
	var b strings.Builder
	for _, i := range issues {
		label := "⚠"
		if i.Severity == "error" {
			label = "×"
		}
		svc := ""
		if i.Service != "" {
			svc = i.Service + ": "
		}
		fmt.Fprintf(&b, "  %s %s%s\n", label, svc, i.Message)
	}
	return b.String()
}
