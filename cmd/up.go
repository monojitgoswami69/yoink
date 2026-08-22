package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"yoink/internal/docker"
	"yoink/internal/project"
	"yoink/internal/state"
	"yoink/internal/ui"

	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up [project]",
	Short: "Start a project",
	Long: `Start a previously-initialised project. Applies saved environment
overrides, runs docker compose up, waits for healthchecks, and prints the
application URL(s).

When no project is given, the most recently initialised one is used.

Flags:
  --build     Rebuild images before starting (compose --build)
  --no-wait   Don't wait for healthchecks; return as soon as compose exits`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runUp(cmd, args); err != nil {
			fmt.Println(ui.ErrorBox.Render("Error: " + err.Error()))
			os.Exit(1)
		}
	},
}

var (
	upBuild  bool
	upNoWait bool
)

func init() {
	upCmd.Flags().BoolVar(&upBuild, "build", false, "Rebuild images before starting")
	upCmd.Flags().BoolVar(&upNoWait, "no-wait", false, "Don't wait for healthchecks")
}

func runUp(cmd *cobra.Command, args []string) error {
	io := &initIO{verbose: GetVerbose(cmd), quiet: GetQuiet(cmd)}
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Already running? Surface the URL and exit without a rebuild.
	if running, _ := p.IsRunning(ctx); running {
		fmt.Println(ui.ProjectHeader(p.Name, ""))
		fmt.Println("\n  " + ui.SuccessStyle.Render(ui.SymRun+" Already running"))
		printURLs(ctx, p, io)
		return nil
	}

	if !io.quiet {
		fmt.Println(ui.ProjectHeader(p.Name, ""))
		fmt.Println("\n  " + ui.MutedStyle.Render("Starting services…"))
	}

	if err := renderMergedEnvs(p.Manager, p.Lock, io); err != nil {
		return err
	}

	if _, err := os.Stat(p.Compose.File); err != nil {
		return fmt.Errorf("compose file missing at %s — run `yoink init <url> --force`", p.Compose.File)
	}

	extra := []string{}
	if upBuild {
		extra = append(extra, "--build")
	}
	out, err := p.Compose.Up(ctx, extra...)
	if io.verbose && strings.TrimSpace(out) != "" {
		fmt.Println(ui.DimStyle.Render(strings.TrimRight(out, "\n")))
	}
	if err != nil {
		return fmt.Errorf("could not start %s\n\n%s\n\nrun `yoink logs %s` or `yoink heal %s`",
			p.Name, docker.TailLines(out, 20), p.Name, p.Name)
	}

	if !upNoWait {
		if err := waitForHealthy(ctx, p, io); err != nil {
			io.warn(err.Error())
		}
	}

	p.Lock.LastUp = time.Now().UTC()
	_ = p.Manager.SaveLock(p.Lock)

	if !io.quiet {
		printHealthSummary(ctx, p, io)
		printURLs(ctx, p, io)
	}
	return nil
}

// renderMergedEnvs takes each service's .env.example (template) plus
// per-service overrides from env-overrides.json and writes the merged
// content to .env, which is what compose actually reads.
func renderMergedEnvs(mgr *state.Manager, lock *state.Lock, io *initIO) error {
	overrides, err := mgr.LoadOverrides()
	if err != nil {
		return fmt.Errorf("could not read env overrides: %w", err)
	}
	envRoot := filepath.Join(lock.RepoPath, lock.OutputSubdir)
	for _, svc := range lock.Services {
		example := filepath.Join(envRoot, "env-vars", svc.ID, ".env.example")
		dotenv := filepath.Join(envRoot, "env-vars", svc.ID, ".env")
		data, err := os.ReadFile(example)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("read %s: %w", example, err)
		}
		merged := state.MergedEnv(string(data), overrides[svc.ID])
		if err := os.WriteFile(dotenv, []byte(merged), 0600); err != nil {
			return fmt.Errorf("write %s: %w", dotenv, err)
		}
	}
	if len(overrides) > 0 {
		io.success("Applied env overrides")
	}
	return nil
}

func waitForHealthy(ctx context.Context, p *project.Project, io *initIO) error {
	deadline := time.Now().Add(90 * time.Second)
	io.info("Waiting for healthchecks…")
	for time.Now().Before(deadline) {
		h, err := p.Health(ctx)
		if err != nil {
			return err
		}
		if h.Overall == "running" {
			io.success(fmt.Sprintf("%d service(s) healthy", len(h.Services)))
			return nil
		}
		for _, s := range h.Services {
			if s.Health == "unhealthy" {
				return fmt.Errorf("service %s reported unhealthy: %s", s.Service, s.Status)
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timed out waiting for healthchecks — use `yoink dash %s` to inspect", p.Name)
}

// printHealthSummary renders a per-service health list followed by the overall
// badge, mirroring the spec's `yoink up` output.
func printHealthSummary(ctx context.Context, p *project.Project, io *initIO) {
	h, err := p.Health(ctx)
	if err != nil || len(h.Services) == 0 {
		return
	}
	fmt.Println()
	for _, s := range h.Services {
		dot, label := ui.ServiceStatus(s.State, s.Health)
		fmt.Printf("  %s %s  %s\n", dot, ui.BoldStyle.Render(s.Service), label)
	}
	fmt.Println("\n  " + ui.Rule(36))
	fmt.Println("\n  " + ui.OverallStatus(h.Overall))
}

// printURLs shows the public-facing service URLs derived from the generated
// port map and the live compose state.
func printURLs(ctx context.Context, p *project.Project, io *initIO) {
	urls, err := p.URLs(ctx)
	if err != nil || len(urls) == 0 {
		return
	}
	fmt.Println()
	if len(urls) == 1 {
		fmt.Println(ui.URL(urls[0].URL))
		return
	}
	fmt.Print(ui.URLList(toURLPairs(urls)))
	fmt.Println()
}

func toURLPairs(urls []project.ServiceURL) []ui.URLPair {
	out := make([]ui.URLPair, len(urls))
	for i, u := range urls {
		out[i] = ui.URLPair{Service: u.Service, URL: u.URL}
	}
	return out
}
