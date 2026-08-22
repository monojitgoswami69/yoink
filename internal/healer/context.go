package healer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"yoink/internal/detector"
)

// ContextPack is the structured context sent to the LLM. It replaces the
// ad-hoc "here's the Dockerfile + raw error" with a rich, evidence-driven
// package that includes service metadata, structured failure analysis,
// relevant source files, and diff-aware iteration context.
type ContextPack struct {
	Service          detector.Service
	Failure          Failure
	Dockerfile       string
	Compose          string
	RelevantFiles    []FileEntry
	PreviousAttempts []AttemptContext
}

// FileEntry is one file selected for the LLM context, with the reason it
// was included.
type FileEntry struct {
	Path    string
	Reason  string
	Content string
}

// AttemptContext is the diff-aware representation of a previous repair attempt.
type AttemptContext struct {
	N            int
	Diagnosis    string
	ChangedFiles []string
	Result       string // "build progressed to TS errors", "same failure", etc.
	Progression  string
}

// BuildContextPack constructs the structured context for the LLM from the
// current failure, service metadata, and relevant files. It reads files
// from DISK (not in-memory) to guarantee current-state freshness.
func BuildContextPack(svc detector.Service, failure Failure, dockerfile, compose, repoRoot, outputDir string, previous []AttemptContext) *ContextPack {
	pack := &ContextPack{
		Service:          svc,
		Failure:          failure,
		Dockerfile:       dockerfile,
		Compose:          compose,
		PreviousAttempts: previous,
	}

	// Select relevant files based on the failure category and references.
	pack.RelevantFiles = selectRelevantFiles(svc, failure, repoRoot)

	return pack
}

// selectRelevantFiles chooses files to include in the LLM context based on
// the failure type. This is deterministic — the same failure always selects
// the same files, so the LLM doesn't have to request them manually.
func selectRelevantFiles(svc detector.Service, failure Failure, repoRoot string) []FileEntry {
	var entries []FileEntry
	seen := map[string]bool{}

	addFile := func(relPath, reason string) {
		if seen[relPath] {
			return
		}
		seen[relPath] = true
		full := filepath.Join(repoRoot, relPath)
		data, err := os.ReadFile(full)
		if err != nil {
			return
		}
		content := string(data)
		if len(content) > 16*1024 {
			content = content[:16*1024] + "\n[...truncated...]\n"
		}
		entries = append(entries, FileEntry{Path: relPath, Reason: reason, Content: content})
	}

	// Service directory prefix.
	prefix := svc.Directory
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	// Always include the service's manifest (package.json or pyproject.toml).
	switch svc.Language {
	case "javascript", "typescript":
		addFile(prefix+"package.json", "service manifest — dependency declarations and scripts")
		if svc.HasLockfile {
			for _, lock := range []string{"package-lock.json", "pnpm-lock.yaml", "yarn.lock", "bun.lock", "bun.lockb"} {
				if _, err := os.Stat(filepath.Join(repoRoot, prefix+lock)); err == nil {
					addFile(prefix+lock, "lockfile — dependency resolution")
					break
				}
			}
		}
		addFile(prefix+"tsconfig.json", "TypeScript configuration — type checking scope")
	case "python":
		manifest := svc.PythonManifest
		if manifest == "" {
			manifest = "requirements.txt"
		}
		addFile(prefix+manifest, "Python dependency manifest")
	}

	// For COPY/path failures, include the referenced files.
	for _, pathRef := range failure.PathRefs {
		// Dockerfile paths start with /app/ — map to repo-relative.
		rel := strings.TrimPrefix(pathRef, "/app/")
		if rel != pathRef {
			addFile(rel, "referenced in COPY failure")
		}
	}

	// For TS2307/dependency failures, include the failing source files.
	for _, fileRef := range failure.FileRefs {
		addFile(fileRef, "source file referenced in the error")
	}

	// For dependency failures, include workspace config.
	if failure.Category == "dependency" || failure.Category == "compilation" {
		// Check for workspace config at the repo root.
		for _, cfg := range []string{"pnpm-workspace.yaml", "lerna.json", "turbo.json", "nx.json"} {
			if _, err := os.Stat(filepath.Join(repoRoot, cfg)); err == nil {
				addFile(cfg, "workspace configuration — monorepo package resolution")
			}
		}
	}

	// Include framework config files.
	switch svc.Framework {
	case "next":
		for _, cfg := range []string{"next.config.js", "next.config.mjs", "next.config.ts"} {
			addFile(prefix+cfg, "Next.js configuration")
		}
	case "vite":
		for _, cfg := range []string{"vite.config.ts", "vite.config.js", "vite.config.mjs"} {
			addFile(prefix+cfg, "Vite configuration")
		}
	}

	return entries
}

// Render constructs the user prompt string from the context pack. This is
// what the LLM actually sees.
func (p *ContextPack) Render() string {
	var b strings.Builder

	// 1. Service metadata.
	fmt.Fprintf(&b, "SERVICE: %s\n", p.Service.ID)
	fmt.Fprintf(&b, "Framework: %s\n", p.Service.Framework)
	fmt.Fprintf(&b, "Language: %s\n", p.Service.Language)
	fmt.Fprintf(&b, "Package Manager: %s\n", p.Service.PackageManager)
	fmt.Fprintf(&b, "Port: %d\n", p.Service.Port)
	if len(p.Service.StartCmd) > 0 {
		fmt.Fprintf(&b, "Start Command: %s\n", strings.Join(p.Service.StartCmd, " "))
	}
	if len(p.Service.BuildCmd) > 0 {
		fmt.Fprintf(&b, "Build Command: %s\n", strings.Join(p.Service.BuildCmd, " "))
	}

	// 2. Detection evidence (so the LLM knows WHY facts were inferred).
	if len(p.Service.Evidence) > 0 {
		b.WriteString("\nDETECTION EVIDENCE:\n")
		for _, e := range p.Service.Evidence {
			fmt.Fprintf(&b, "  %s (%s, source: %s)\n", e.Fact, e.Weight, e.Source)
		}
	}

	// 3. Structured failure analysis.
	f := p.Failure
	fmt.Fprintf(&b, "\nFAILURE ANALYSIS:\n")
	fmt.Fprintf(&b, "  Category: %s\n", f.Category)
	fmt.Fprintf(&b, "  Service: %s\n", f.Service)
	if f.Stage != "" {
		fmt.Fprintf(&b, "  Stage: %s\n", f.Stage)
	}
	fmt.Fprintf(&b, "  Primary Error: %s\n", f.Error)
	if len(f.FileRefs) > 0 {
		fmt.Fprintf(&b, "  File References: %s\n", strings.Join(f.FileRefs, ", "))
	}
	if len(f.PackageRefs) > 0 {
		fmt.Fprintf(&b, "  Package References: %s\n", strings.Join(f.PackageRefs, ", "))
	}
	if len(f.EnvRefs) > 0 {
		fmt.Fprintf(&b, "  Env Var References: %s\n", strings.Join(f.EnvRefs, ", "))
	}
	if len(f.PathRefs) > 0 {
		fmt.Fprintf(&b, "  Path References: %s\n", strings.Join(f.PathRefs, ", "))
	}

	// 4. Relevant log excerpt.
	fmt.Fprintf(&b, "\nRELEVANT BUILD LOG:\n%s\n", f.RelevantLog)

	// 5. Current Dockerfile.
	fmt.Fprintf(&b, "\nCURRENT DOCKERFILE:\n%s\n", p.Dockerfile)

	// 6. Current compose.
	fmt.Fprintf(&b, "\nCURRENT DOCKER-COMPOSE.YML:\n%s\n", p.Compose)

	// 7. Relevant files (with reasons).
	if len(p.RelevantFiles) > 0 {
		b.WriteString("\nRELEVANT FILES:\n")
		for _, f := range p.RelevantFiles {
			fmt.Fprintf(&b, "\n=== %s ===\n", f.Path)
			fmt.Fprintf(&b, "(included because: %s)\n", f.Reason)
			b.WriteString(f.Content)
			b.WriteString("\n")
		}
	}

	// 8. Previous attempts (diff-aware).
	if len(p.PreviousAttempts) > 0 {
		b.WriteString("\nPREVIOUS ATTEMPTS:\n")
		for _, a := range p.PreviousAttempts {
			fmt.Fprintf(&b, "\nAttempt %d:\n", a.N)
			fmt.Fprintf(&b, "  Diagnosis: %s\n", a.Diagnosis)
			fmt.Fprintf(&b, "  Changed: %s\n", strings.Join(a.ChangedFiles, ", "))
			fmt.Fprintf(&b, "  Result: %s\n", a.Result)
			fmt.Fprintf(&b, "  Progression: %s\n", a.Progression)
		}
		b.WriteString("\nDo NOT repeat fixes that did not resolve the failure.\n")
	}

	return b.String()
}
