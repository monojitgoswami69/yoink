package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"yoink/internal/config"
	"yoink/internal/detector"
	"yoink/internal/docker"
	"yoink/internal/envvar"
	"yoink/internal/generator"
	"yoink/internal/healer"
	"yoink/internal/infra"
	"yoink/internal/llm"
	"yoink/internal/portprobe"
	"yoink/internal/preflight"
	"yoink/internal/project"
	"yoink/internal/safefs"
	"yoink/internal/state"
	"yoink/internal/tree"
	"yoink/internal/ui"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update [project]",
	Short: "Update the repository and rebuild",
	Long: `Pull the latest changes for a project's repository, regenerate the
Docker configuration when the repo has changed, rebuild, run the heal loop
if the build fails, and restart the stack.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runUpdate(cmd, args); err != nil {
			fmt.Println(ui.ErrorBox.Render("Error: " + err.Error()))
			os.Exit(1)
		}
	},
}

func runUpdate(cmd *cobra.Command, args []string) error {
	io := &initIO{verbose: GetVerbose(cmd), quiet: GetQuiet(cmd)}
	if !io.quiet {
		fmt.Print(ui.Header(ui.HeaderArgs{Command: "update", Version: Version}))
	}
	if err := requireDocker(); err != nil {
		return err
	}
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	p, err := project.Resolve(name)
	if err != nil {
		return err
	}
	if !io.quiet {
		fmt.Printf("Updating %s...\n\n", ui.HighlightStyle.Render(p.Name))
	}

	// 1. Pull latest changes.
	pullCtx, pullCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer pullCancel()
	pull := exec.CommandContext(pullCtx, "git", "-C", p.Lock.RepoPath, "pull", "--ff-only")
	pull.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, pullErr := pull.CombinedOutput()
	if pullErr != nil {
		return fmt.Errorf("git pull failed: %s", strings.TrimSpace(string(out)))
	}
	if strings.Contains(string(out), "Already up to date") {
		io.success("Already up to date.")
		return nil
	}
	io.success("Repository updated.")

	// 2. Re-detect + regenerate (the repo may have changed shape).
	detection, err := detector.DetectWithCap(p.Lock.RepoPath, detector.MaxServices)
	if err != nil {
		return fmt.Errorf("re-detection failed: %w", err)
	}
	envResults := envvar.Detect(p.Lock.RepoPath, detection.Services)
	inference := infra.Infer(envResults)
	allocator := portprobe.New()
	out2 := generator.Build(detection.Services, generator.Options{
		Repo:         p.Lock.Repo,
		OutputSubdir: p.Lock.OutputSubdir,
		Infra:        inference.Services,
		Links:        inference.Links,
		PortFn:       allocator.Allocate,
	})
	// 3. Write regenerated outputs + .dockerignore.
	// Check repair provenance: don't silently overwrite healed artifacts.
	healedFiles := p.Manager.HealedFiles()
	if len(healedFiles) > 0 {
		for _, name := range generator.SortedFilenames(out2.Files) {
			if containsStr(healedFiles, name) {
				artState := p.Manager.ClassifyArtifact(name, out2.Files[name])
				switch artState {
				case state.ArtifactHealed:
					io.warn(fmt.Sprintf("Preserving healed %s (healer modification detected). Regenerated version saved as %s.gen.", name, name))
					_ = os.WriteFile(filepath.Join(p.OutputDir, name+".gen"), []byte(out2.Files[name]), 0644)
					out2.Files[name] = readFileFromDisk(p.OutputDir, name)
				case state.ArtifactDiverged:
					io.warn(fmt.Sprintf("Preserving user-modified %s. Regenerated version saved as %s.gen.", name, name))
					_ = os.WriteFile(filepath.Join(p.OutputDir, name+".gen"), []byte(out2.Files[name]), 0644)
					out2.Files[name] = readFileFromDisk(p.OutputDir, name)
				}
			}
		}
	}
	if err := writeOutputs(p.OutputDir, out2, envResults, p.Lock.Repo, detection.Services, p.Lock.OutputSubdir, io); err != nil {
		return err
	}
	if out2.Dockerignore != "" {
		_ = os.WriteFile(filepath.Join(p.Lock.RepoPath, ".dockerignore"), []byte(out2.Dockerignore), 0644)
	}
	io.success("Docker configuration regenerated.")

	// 3.5 Preflight validation.
	if issues := preflight.Check(out2.Files["docker-compose.yml"], out2.Files, p.OutputDir); preflight.HasErrors(issues) {
		io.warn("Pre-flight validation found errors:")
		fmt.Print(preflight.FormatIssues(issues))
	} else if len(issues) > 0 {
		io.info("Pre-flight warnings:")
		fmt.Print(preflight.FormatIssues(issues))
	}

	// 4. Rebuild.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	buildOut, buildErr := p.Compose.Build(ctx)
	if buildErr != nil {
		io.warn("Build failed — attempting heal...")
		// 4.5 Self-heal: run the heal loop on the regenerated output.
		cfg, cfgErr := config.Load()
		if cfgErr == nil && cfg.LLMProvider != "" {
			client, llmErr := llm.NewClient(cfg.LLMProvider, cfg.LLMModel, cfg.LLMAPIKey)
			if llmErr == nil {
				reader, _ := safefs.New(p.Lock.RepoPath)
				read := func(path string) (string, error) { return reader.Read(path) }
				treeStr, _, _ := tree.Generate(p.Lock.RepoPath, 10000)
				loop := &healer.Loop{
					Output: out2, Services: detection.Services, Compose: p.Compose,
					LLM: client, Reader: read, OutputDir: p.OutputDir, Tree: treeStr,
					MaxTries: 3, Manager: p.Manager,
					Tee: func(line string) { io.info(line) },
				}
				healRes, healErr := loop.Run(ctx)
				if healErr == nil && healRes.Success {
					io.success("Heal succeeded — build is green")
				} else if healRes != nil {
					io.warn(fmt.Sprintf("Heal: %d attempts, success=%v", len(healRes.Attempts), healRes.Success))
					if healRes.FinalOutput != "" {
						fmt.Println(ui.DimStyle.Render(healRes.FinalOutput))
					}
				}
			}
		}
		// If heal didn't succeed, fall through with the error.
		if buildErr != nil {
			fmt.Println(ui.DimStyle.Render(docker.TailLines(buildOut, 40)))
			return nil
		}
	}
	io.success("Build succeeded.")

	// 5. Restart.
	if _, err := p.Compose.Down(ctx, false); err != nil {
		io.warn(fmt.Sprintf("compose down: %v", err))
	}
	if err := renderMergedEnvs(p.Manager, p.Lock, io); err != nil {
		return err
	}
	if _, err := p.Compose.Up(ctx); err != nil {
		return fmt.Errorf("compose up failed after rebuild")
	}
	p.Lock.Services = detection.Services
	p.Lock.Hash = state.HashDetection(detection.Services)
	p.Lock.LastUp = time.Now().UTC()
	_ = p.Manager.SaveLock(p.Lock)
	if !io.quiet {
		printURLs(ctx, p, io)
	}
	return nil
}
