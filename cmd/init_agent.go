package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"yoink/internal/config"
	"yoink/internal/detector"
	"yoink/internal/envvar"
	"yoink/internal/generator"
	"yoink/internal/llm"
	"yoink/internal/safefs"
	"yoink/internal/ui"
)

// initAgent bundles a single LLM client and a sandboxed file reader so the three
// validation passes don't each re-create them.
type initAgent struct {
	client *llm.Client
	read   llm.FileReader
	root   string
	io     *initIO
}

func newInitAgent(cfg *config.Config, root string, io *initIO) *initAgent {
	if initNoAgent {
		return nil
	}
	client, err := llm.NewClient(cfg.LLMProvider, cfg.LLMModel, cfg.LLMAPIKey)
	if err != nil {
		io.warn(fmt.Sprintf("LLM disabled: %v", err))
		return nil
	}
	reader, err := safefs.New(root)
	if err != nil {
		io.warn(fmt.Sprintf("LLM disabled: %v", err))
		return nil
	}
	a := &initAgent{client: client, root: root, io: io}
	a.read = func(p string) (string, error) {
		if io.verbose {
			fmt.Println(ui.DimStyle.Render("  → LLM reading " + p))
		}
		return reader.Read(p)
	}
	return a
}

func (a *initAgent) validateDetection(ctx context.Context, detection *detector.Result, treeStr string) error {
	detJSON, _ := detection.ToJSON()
	resp, err := a.client.ValidateDetection(ctx, treeStr, detJSON, a.read)
	if err != nil {
		a.io.warn(fmt.Sprintf("LLM validation skipped: %v", err))
		return err
	}
	if resp.Valid {
		a.io.success("LLM confirmed detection")
		return nil
	}
	if len(resp.CorrectedServices) == 0 {
		return nil
	}
	merged, changed := mergeServiceCorrections(detection.Services, resp.CorrectedServices)
	if !changed {
		// LLM agreed with detection in substance — don't surface a noisy update.
		return nil
	}
	detection.Services = merged
	a.io.success("LLM provided corrections to detection")
	if !a.io.quiet {
		printDetectionTable(detection)
	}
	return nil
}

func (a *initAgent) validateDocker(ctx context.Context, out *generator.Output, treeStr string) error {
	var dockerfileBlob strings.Builder
	for _, name := range generator.SortedFilenames(out.Files) {
		if strings.HasPrefix(name, "Dockerfile.") {
			fmt.Fprintf(&dockerfileBlob, "=== %s ===\n%s\n\n", name, out.Files[name])
		}
	}
	resp, err := a.client.ValidateDockerfiles(ctx, treeStr, dockerfileBlob.String(), out.Files["docker-compose.yml"], a.read)
	if err != nil {
		a.io.warn(fmt.Sprintf("LLM Docker validation skipped: %v", err))
		return err
	}
	if resp.Valid {
		a.io.success("LLM confirmed Docker configurations")
		return nil
	}
	// The LLM occasionally proposes a compose rewrite at this stage. Without
	// any build-error signal we have no reason to trust it: in practice these
	// rewrites have stripped env_file refs, healthchecks, and rewritten the
	// build context to "." which breaks every COPY. The heal loop will get a
	// chance to fix compose later if a build does fail, where it sees real
	// error output to ground the change. Surface that the model wanted a
	// change so the user can inspect, but don't apply it.
	if resp.CorrectedCompose != "" {
		out.Files["docker-compose.llm-review.yml"] = resp.CorrectedCompose
		a.io.warn("LLM proposed a compose rewrite — saved as docker-compose.llm-review.yml (not applied; heal loop will fix compose if a build fails)")
	}
	// The LLM may return a multi-Dockerfile blob (concatenated text). Save it
	// under a distinct name rather than overwriting an existing per-service
	// Dockerfile, so reviewers can compare before adopting.
	if resp.CorrectedDockerfile != "" {
		out.Files["Dockerfile.llm-review"] = resp.CorrectedDockerfile
		a.io.warn("LLM returned a Dockerfile review — saved as Dockerfile.llm-review (not wired into compose)")
	}
	return nil
}

func (a *initAgent) validateEnvVars(ctx context.Context, results *[]envvar.Result, treeStr string) error {
	existing := readExistingEnvFiles(a.root)
	envJSON, _ := json.Marshal(*results)
	resp, err := a.client.ValidateEnvVars(ctx, treeStr, string(envJSON), existing, a.read)
	if err != nil {
		a.io.warn(fmt.Sprintf("LLM env enhancement skipped: %v", err))
		return err
	}
	if len(resp.CorrectedEnvVars) == 0 {
		return nil
	}
	byDir := map[string]int{}
	for i, r := range *results {
		byDir[r.Directory] = i
	}
	for _, c := range resp.CorrectedEnvVars {
		if idx, ok := byDir[c.Directory]; ok {
			(*results)[idx].EnvContent = envvar.MergeProvidedValues((*results)[idx].EnvContent, c.EnvContent)
			(*results)[idx].Technology = c.Technology
		}
	}
	a.io.success("LLM enhanced environment variables")
	return nil
}

// mergeServiceCorrections folds the LLM's high-level corrections (type,
// directory, framework, confidence) into the existing detector records so
// that install/start commands and package-manager state survive. New
// services proposed by the LLM (no matching directory in the detection)
// fall back to a bare record with default port/language; they will likely
// generate empty Dockerfiles, so we surface them but they are rare in
// practice. Returns the merged slice and a boolean indicating whether
// anything actually changed compared to the original detection.
// normDir treats "" and "." (and trailing slashes) as the same repo-root
// directory. The detector emits ""; LLMs frequently emit "." instead.
func normDir(d string) string {
	d = strings.TrimRight(d, "/")
	if d == "." {
		return ""
	}
	return d
}

func mergeServiceCorrections(existing []detector.Service, corrections []llm.CorrectedService) ([]detector.Service, bool) {
	byDir := map[string]int{}
	for i, s := range existing {
		byDir[normDir(s.Directory)] = i
	}
	out := make([]detector.Service, 0, len(corrections))
	changed := len(corrections) != len(existing)
	for i, c := range corrections {
		if idx, ok := byDir[normDir(c.Directory)]; ok {
			svc := existing[idx]
			if c.Type != "" && c.Type != svc.Type {
				svc.Type = c.Type
				changed = true
			}
			// Only let the LLM override a static framework pick when it is
			// highly confident. Low/medium-confidence overrides (often
			// false positives like "next-themes" → "next") must not clobber
			// an already-correct high-confidence static detection.
			if c.Framework != "" && c.Framework != svc.Framework && confidenceForScore(c.Confidence) == "high" {
				svc.Framework = c.Framework
				svc.Language = languageFor(c.Framework)
				changed = true
			}
			if newConf := confidenceForScore(c.Confidence); c.Confidence > 0 && newConf != svc.Confidence {
				svc.Confidence = newConf
				changed = true
			}
			out = append(out, svc)
			continue
		}
		// LLM added a service we didn't detect; build a bare record.
		out = append(out, detector.Service{
			ID:         fmt.Sprintf("service-%d", i+1),
			Type:       c.Type,
			Directory:  c.Directory,
			Language:   languageFor(c.Framework),
			Framework:  c.Framework,
			Confidence: confidenceForScore(c.Confidence),
			Port:       defaultPort(c.Framework),
		})
		changed = true
	}
	// Re-assign stable IDs (service-1, service-2, …) so they line up with the
	// rest of the pipeline that keys off these IDs.
	for i := range out {
		out[i].ID = fmt.Sprintf("service-%d", i+1)
	}
	return out, changed
}

func languageFor(framework string) string {
	switch framework {
	case "fastapi", "flask", "django", "python":
		return "python"
	}
	return "javascript"
}

func confidenceForScore(score float64) string {
	switch {
	case score >= 0.9:
		return "high"
	case score < 0.6:
		return "low"
	}
	return "medium"
}

func defaultPort(framework string) int {
	switch framework {
	case "next", "react", "express", "node", "fastify", "nest", "cra":
		return 3000
	case "vite":
		return 5173
	case "fastapi", "django":
		return 8000
	case "flask":
		return 5000
	}
	return 8080
}

func readExistingEnvFiles(targetDir string) string {
	patterns := map[string]bool{".env.example": true, ".env.sample": true, ".env.template": true, ".env.local.example": true}
	var found []string
	_ = filepath.WalkDir(targetDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if patterns[d.Name()] {
			rel, _ := filepath.Rel(targetDir, path)
			found = append(found, rel)
		}
		return nil
	})
	sort.Strings(found)

	reader, err := safefs.New(targetDir)
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, f := range found {
		content, err := reader.Read(f)
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "\n=== %s ===\n%s\n", f, content)
	}
	return b.String()
}
