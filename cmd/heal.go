package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"yoink/internal/config"
	"yoink/internal/generator"
	"yoink/internal/healer"
	"yoink/internal/llm"
	"yoink/internal/preflight"
	"yoink/internal/project"
	"yoink/internal/safefs"
	"yoink/internal/tree"
	"yoink/internal/ui"

	"github.com/spf13/cobra"
)

var healCmd = &cobra.Command{
	Use:   "heal [project]",
	Short: "Diagnose and repair build failures",
	Long: `Re-run the LLM build/heal loop against an existing project. Useful when
you have edited Dockerfiles by hand or want another attempt after the
initial init couldn't get to green.

Flags:
  --tries N    Maximum heal attempts (default 3)`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runHeal(cmd, args); err != nil {
			fmt.Println(ui.ErrorBox.Render("Error: " + err.Error()))
			os.Exit(1)
		}
	},
}

var healTries int

func init() {
	healCmd.Flags().IntVar(&healTries, "tries", healer.DefaultMaxTries, "Maximum heal-loop attempts")
}

func runHeal(cmd *cobra.Command, args []string) error {
	io := &initIO{verbose: GetVerbose(cmd), quiet: GetQuiet(cmd)}
	if !io.quiet {
		fmt.Print(ui.Header(ui.HeaderArgs{Command: "heal", Version: Version}))
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

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("%s\n\nrun `yoink setup` first", err)
	}

	files, err := readOutputDir(p.OutputDir)
	if err != nil {
		return err
	}
	out := &generator.Output{Files: files, Dockerignore: generator.DockerignoreContent()}

	// Pre-flight: validate before wasting a build round.
	if issues := preflight.Check(out.Files["docker-compose.yml"], out.Files, p.OutputDir); preflight.HasErrors(issues) {
		return fmt.Errorf("pre-flight validation failed:\n%s", preflight.FormatIssues(issues))
	}

	// Ensure a .dockerignore exists at the repo root so `COPY . ./` stays
	// lean. init writes one, but a stack initialised by an older Yoink or
	// hand-edited may not have it.
	dockerignorePath := filepath.Join(p.Lock.RepoPath, ".dockerignore")
	if _, statErr := os.Stat(dockerignorePath); statErr != nil {
		if err := os.WriteFile(dockerignorePath, []byte(out.Dockerignore), 0644); err != nil {
			return fmt.Errorf("write .dockerignore: %w", err)
		}
		io.info("Wrote .dockerignore at repo root")
	}

	client, err := llm.NewClient(cfg.LLMProvider, cfg.LLMModel, cfg.LLMAPIKey)
	if err != nil {
		return fmt.Errorf("LLM client: %w", err)
	}
	reader, err := safefs.New(p.Lock.RepoPath)
	if err != nil {
		return fmt.Errorf("safefs: %w", err)
	}
	read := func(path string) (string, error) { return reader.Read(path) }

	treeStr, _, _ := tree.Generate(p.Lock.RepoPath, 10000)

	loop := &healer.Loop{
		Output:    out,
		Services:  p.Lock.Services,
		Compose:   p.Compose,
		LLM:       client,
		Reader:    read,
		OutputDir: p.OutputDir,
		Tree:      treeStr,
		MaxTries:  healTries,
		Tee:       func(line string) { io.info(line) },
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	res, err := loop.Run(ctx)
	if err != nil {
		return fmt.Errorf("heal loop: %w", err)
	}
	if res.Success {
		io.success("Build is green")
	} else {
		io.warn("Build still failing after heal — see the tail below")
		if res.FinalOutput != "" {
			fmt.Println(ui.DimStyle.Render(res.FinalOutput))
		}
		return fmt.Errorf("build still failing after %d heal attempt(s)", len(res.Attempts))
	}
	return nil
}

// readOutputDir loads every file in the yoink-outputs directory (excluding
// subdirectories) into a map. The heal loop needs Dockerfile.* and
// docker-compose.yml in memory so it can rewrite them in place.
func readOutputDir(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	files := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		files[e.Name()] = string(data)
	}
	return files, nil
}
