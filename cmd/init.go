package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"yoink/internal/agent"
	"yoink/internal/config"
	"yoink/internal/detector"
	"yoink/internal/docker"
	"yoink/internal/envvar"
	"yoink/internal/generator"
	"yoink/internal/git"
	"yoink/internal/graph"
	"yoink/internal/healer"
	"yoink/internal/httpcheck"
	"yoink/internal/infra"
	"yoink/internal/llm"
	"yoink/internal/portprobe"
	"yoink/internal/preflight"
	"yoink/internal/safefs"
	"yoink/internal/state"
	"yoink/internal/tree"
	"yoink/internal/ui"

	"github.com/spf13/cobra"
)

const (
	defaultOutputSubdir = "yoink-outputs"
	treeLineCap         = 10000
)

var initCmd = &cobra.Command{
	Use:   "init <github-url>",
	Short: "Clone, analyse, and generate a runnable Docker stack",
	Long: `Clone a GitHub repository, analyse its stack, generate Dockerfiles +
docker-compose.yml, and (when Docker is available) run a build/heal loop
that asks the LLM to repair build failures automatically.

URL forms accepted:
  https://github.com/<owner>/<repo>
  https://github.com/<owner>/<repo>.git
  https://github.com/<owner>/<repo>/tree/<branch>[/<subdir>]
  git@github.com:<owner>/<repo>[.git]

Local repositories:
  yoink init            # the current working directory
  yoink init .          # the current working directory
  yoink init <dir>      # a local directory (no clone performed)

Flags:
  --force         Force re-initialisation even if the cloned directory exists
  --no-agent      Disable LLM validation (static analysis only)
  --output        Override the output directory (default: yoink-outputs)
  --max-services  Cap on detected services
  --build         Run docker compose build + heal loop after generation (default: auto)
  --no-build      Skip the build/heal loop even when docker is available
  --heal-tries    Maximum heal-loop attempts (default: 3)`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repo := ""
		if len(args) > 0 {
			repo = args[0]
		}
		if err := runInit(cmd, repo); err != nil {
			fmt.Println()
			fmt.Println(ui.ErrorBox.Render(fmt.Sprintf("Error: %s", err.Error())))
			return err
		}
		return nil
	},
}

var (
	initForce       bool
	initNoAgent     bool
	initOutputDir   string
	initMaxServices int
	initBuild       bool
	initNoBuild     bool
	initHealTries   int
	initName        string
)

func init() {
	initCmd.Flags().BoolVar(&initForce, "force", false, "Force re-initialisation even if directory exists")
	initCmd.Flags().BoolVar(&initNoAgent, "no-agent", false, "Disable LLM validation (use static analysis only)")
	initCmd.Flags().StringVar(&initOutputDir, "output", defaultOutputSubdir, "Output sub-directory (relative to the repo root)")
	initCmd.Flags().IntVar(&initMaxServices, "max-services", detector.MaxServices, "Cap on the number of detected services")
	initCmd.Flags().BoolVar(&initBuild, "build", true, "Run docker compose build + heal loop after generation")
	initCmd.Flags().BoolVar(&initNoBuild, "no-build", false, "Skip docker compose build")
	initCmd.Flags().IntVar(&initHealTries, "heal-tries", healer.DefaultMaxTries, "Maximum heal-loop attempts")
	initCmd.Flags().StringVar(&initName, "name", "", "Override the project name (default: repository name)")
}

func runInit(cmd *cobra.Command, repoURL string) error {
	start := time.Now()
	ctx := context.Background()

	io := &initIO{
		verbose: GetVerbose(cmd),
		quiet:   GetQuiet(cmd),
	}

	if !io.quiet {
		fmt.Print(ui.Header(ui.HeaderArgs{Command: "init", Version: Version}))
	}

	parsed, err := parseRepoRef(repoURL)
	if err != nil {
		return err
	}

	projectName := initName
	if projectName == "" {
		projectName = parsed.Repo
	}

	cfg, err := loadInitConfig(io)
	if err != nil {
		return err
	}

	// Local mode reuses the repository in place (no clone, no copy); remote
	// mode materialises the repo under ~/.yoink/repos/<name>.
	var targetDir string
	if parsed.LocalPath != "" {
		targetDir = parsed.LocalPath
	} else {
		targetDir, err = prepareTargetDir(projectName, io)
		if err != nil {
			return err
		}
	}

	totalSteps := 7
	doBuild := initBuild && !initNoBuild && docker.Available()
	if doBuild {
		totalSteps = 8
	}

	// Step 1: clone (remote) or load (local).
	if parsed.LocalPath != "" {
		io.step(1, totalSteps, "Load local repository")
		io.success(fmt.Sprintf("Using %s", targetDir))
	} else {
		io.step(1, totalSteps, "Clone repository")
		if err := io.withSpinner(fmt.Sprintf("Cloning %s/%s...", parsed.Owner, parsed.Repo), func() error {
			return git.Clone(ctx, parsed.Clone, targetDir, cfg.GitHubPAT)
		}); err != nil {
			return err
		}
	}

	// Agent construction needs the cloned tree (safefs roots the sandboxed
	// reader at targetDir), so build it only after the clone succeeds.
	ia := newInitAgent(cfg, targetDir, io)
	fileCount, sizeBytes, _ := git.CountFiles(targetDir)
	io.success(fmt.Sprintf("%s cloned (%d files · %.1f MB)", parsed.Repo, fileCount, float64(sizeBytes)/1024/1024))
	if existing := existingDockerConfig(targetDir); len(existing) > 0 {
		io.warn("Existing Docker configuration detected: " + strings.Join(existing, ", "))
		io.info("Yoink will replace the generated stack in yoink-outputs; existing files are preserved.")
	}

	// Step 2: tree.
	io.step(2, totalSteps, "Generate repository tree")
	treeStr, totalLines, err := tree.Generate(targetDir, treeLineCap)
	if err != nil {
		return fmt.Errorf("tree generation failed: %w", err)
	}
	io.success(fmt.Sprintf("Tree generated (%d lines)", totalLines))
	if io.verbose {
		for _, line := range strings.Split(treeStr, "\n") {
			if line != "" {
				fmt.Println(ui.DimStyle.Render("  " + line))
			}
		}
	}

	// Step 3: detection.
	io.step(3, totalSteps, "Detect services and frameworks")
	detection, err := detector.DetectWithCap(targetDir, initMaxServices)
	if err != nil {
		return fmt.Errorf("detection failed: %w", err)
	}
	if len(detection.Services) == 0 {
		fmt.Println(ui.WarningBox.Render(
			"No supported project types detected.\n\n" +
				"Yoink currently supports Node-based (Next, React, Vite, Express, NestJS, etc.) and Python (FastAPI, Flask, Django).\n\n" +
				"If this repo uses a supported technology, please open an issue.",
		))
		return nil
	}
	if !io.quiet {
		printDetectionTable(detection)
		if detection.TruncatedFrom > 0 {
			io.warn(fmt.Sprintf(
				"Found %d candidate services — kept the top %d by confidence. Re-run with --max-services to adjust.",
				detection.TruncatedFrom, len(detection.Services),
			))
		}
	}

	if ia != nil {
		_ = io.withSpinner("Validating detection with LLM...", func() error {
			return ia.validateDetection(ctx, detection, treeStr)
		})
	}

	// Step 4: env vars.
	io.step(4, totalSteps, "Extract environment variables")
	envResults := envvar.Detect(targetDir, detection.Services)
	totalVars := 0
	for _, r := range envResults {
		totalVars += len(r.Vars)
	}
	io.success(fmt.Sprintf("Detected %d variable reference(s) across %d service(s)", totalVars, len(envResults)))

	if ia != nil {
		_ = io.withSpinner("Enhancing env files with LLM...", func() error {
			return ia.validateEnvVars(ctx, &envResults, treeStr)
		})
	}

	if reconcilePortsFromEnv(detection.Services, envResults) {
		io.info("Updated service ports from .env content")
	}

	// Seed EnvContent from the static template BEFORE infra inference so the
	// common vars for the technology (e.g. DATABASE_URL for FastAPI) are
	// visible to infra.Infer even when the static os.getenv scan missed them
	// (modern apps use pydantic-settings field names, not os.getenv) or the
	// LLM enhancement failed/timed out.
	for i, r := range envResults {
		envResults[i].EnvContent = envvar.EnsureCommonVars(r.EnvContent, r.Vars, r.Technology)
	}

	// Step 5: infrastructure inference.
	io.step(5, totalSteps, "Infer backing services")
	inference := infra.Infer(envResults)
	if len(inference.Services) == 0 {
		io.info("No backing services inferred from env vars")
	} else {
		names := make([]string, 0, len(inference.Services))
		for _, s := range inference.Services {
			names = append(names, fmt.Sprintf("%s (%s)", s.Name, s.Reason))
		}
		io.success("Inferred: " + strings.Join(names, ", "))
	}
	// Inject connection-string env vars into each app service's env content.
	// The connection-string keys (DATABASE_URL, etc.) overwrite the
	// placeholder common-var values so the app points at the inferred infra.
	for i, r := range envResults {
		injected := map[string]string{}
		for _, link := range inference.Links[r.ServiceID] {
			if service := findInfra(inference.Services, link.ServiceName); service != nil && service.Mode == "external" {
				envResults[i].EnvContent = infra.ClearGeneratedConnectionPlaceholders(envResults[i].EnvContent)
				continue
			}
			for k, v := range link.EnvVars {
				if _, dupe := injected[k]; !dupe {
					injected[k] = v
				}
			}
		}
		if len(injected) > 0 {
			envResults[i].EnvContent = infra.EnrichEnvContent(envResults[i].EnvContent, injected)
		}
	}

	// Step 6: generation.
	io.step(6, totalSteps, "Generate Dockerfile(s) and docker-compose.yml")
	allocator := portprobe.New()
	// Populate BuildEnv on each service from the finalized env content so
	// the generator can emit ENV directives before build steps. This solves
	// the "next build fails because JWT_SECRET is missing" problem — the
	// build-time ENVs are placeholders, overridden at runtime by compose's
	// env_file.
	envBySvc := map[string]map[string]string{}
	for _, r := range envResults {
		envBySvc[r.ServiceID] = parseEnvContent(r.EnvContent)
	}
	for i := range detection.Services {
		if env, ok := envBySvc[detection.Services[i].ID]; ok {
			detection.Services[i].BuildEnv = env
		}
	}

	serviceGraph := graph.Build(detection.Services, inference.Services, inference.Links, nil)
	for i, result := range envResults {
		if bindings := serviceGraph.InternalBindings(detection.Services)[result.ServiceID]; len(bindings) > 0 {
			envResults[i].EnvContent = infra.ReplaceEnvValues(result.EnvContent, bindings)
			for serviceIndex := range detection.Services {
				if detection.Services[serviceIndex].ID == result.ServiceID {
					detection.Services[serviceIndex].BuildEnv = parseEnvContent(envResults[i].EnvContent)
					break
				}
			}
		}
	}
	out := generator.Build(detection.Services, generator.Options{
		Repo:         parsed.Repo,
		OutputSubdir: initOutputDir,
		Infra:        inference.Services,
		Links:        inference.Links,
		AppLinks:     serviceGraph.AppLinks(),
		PortFn:       allocator.Allocate,
	})
	if !io.quiet {
		for _, name := range generator.SortedFilenames(out.Files) {
			io.success("  " + initOutputDir + "/" + name)
		}
	}
	if configured, placeholders := envSummary(envResults); configured > 0 {
		io.info(fmt.Sprintf("Environment configured automatically: %d repository-provided value(s), %d placeholder(s)", configured, placeholders))
		if placeholders > 0 {
			io.info("Some repository values are placeholders or development defaults; replace them for full feature functionality.")
		}
	}

	if ia != nil {
		_ = io.withSpinner("Validating Dockerfile and compose with LLM...", func() error {
			return ia.validateDocker(ctx, out, treeStr)
		})
	}

	// Step 7: write outputs.
	io.step(7, totalSteps, "Write outputs")
	outputDir := filepath.Join(targetDir, initOutputDir)
	if err := writeOutputs(outputDir, out, envResults, parsed.Repo, detection.Services, initOutputDir, io); err != nil {
		return err
	}
	// .dockerignore lives at the repo root (the compose build context is
	// ".."), NOT inside yoink-outputs, so it must be written separately.
	if out.Dockerignore != "" {
		if err := os.WriteFile(filepath.Join(targetDir, ".dockerignore"), []byte(out.Dockerignore), 0644); err != nil {
			io.warn(fmt.Sprintf("could not write .dockerignore: %v", err))
		} else {
			io.success("  .dockerignore (repo root)")
		}
	}

	// Persist state so `yoink up` / `dash` work later.
	portMap := extractPortMap(out.Files["docker-compose.yml"], detection.Services)
	if err := persistState(parsed, projectName, targetDir, initOutputDir, detection.Services, inference.Services, inference.Links, portMap); err != nil {
		io.warn(fmt.Sprintf("Could not persist state: %v", err))
	}

	// Pre-flight: validate the generated compose + Dockerfiles before
	// wasting an expensive build round on malformed output.
	preflightBlocked := false
	if issues := preflight.Check(out.Files["docker-compose.yml"], out.Files, outputDir); preflight.HasErrors(issues) {
		preflightBlocked = true
		io.warn("Pre-flight validation found errors:")
		fmt.Print(preflight.FormatIssues(issues))
		io.info("Fix the above or re-run with --no-build to skip the build.")
		if !doBuild {
			// already skipping
		} else {
			doBuild = false
		}
	} else if len(issues) > 0 {
		io.info("Pre-flight warnings:")
		fmt.Print(preflight.FormatIssues(issues))
	}

	// Step 8 (optional): build/heal.
	healResult, healErr := maybeRunHeal(ctx, doBuild, projectName, targetDir, outputDir, out, detection.Services, treeStr, ia, totalSteps, portMap, io)
	if healErr != nil {
		io.warn(fmt.Sprintf("Build/heal aborted: %v", healErr))
	}

	if !io.quiet {
		fmt.Println()
		fmt.Println(ui.SuccessBox.Render(renderCompletionSummary(projectName, outputDir, targetDir, initOutputDir, time.Since(start), healResult)))
		fmt.Println()
	}
	if preflightBlocked {
		return initStateError{state: "blocked", message: "generated artifacts failed pre-flight validation"}
	}
	if healErr != nil {
		return initStateError{state: "failed", message: healErr.Error()}
	}
	if healResult != nil && !healResult.Success {
		state := healResult.Summary
		if state == "" {
			state = "blocked"
		}
		code := 3
		if state == "configuration_required" {
			code = 2
		} else if state == "failed" {
			code = 4
		}
		return initStateError{state: state, code: code, message: "initialization did not reach a verified running state"}
	}
	return nil
}

func envSummary(results []envvar.Result) (configured, placeholders int) {
	for _, result := range results {
		for _, value := range result.Vars {
			if value.Status == envvar.StatusProvidedDefault {
				configured++
				if value.Placeholder {
					placeholders++
				}
			}
		}
	}
	return configured, placeholders
}

func existingDockerConfig(root string) []string {
	names := []string{"Dockerfile", "docker-compose.yml", "docker-compose.yaml", "compose.yaml", "docker-compose.override.yml", "docker-compose.override.yaml"}
	var found []string
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			found = append(found, name)
		}
	}
	return found
}

func findInfra(services []infra.Service, name string) *infra.Service {
	for i := range services {
		if services[i].Name == name {
			return &services[i]
		}
	}
	return nil
}

type initStateError struct {
	state   string
	code    int
	message string
}

func (e initStateError) Error() string {
	return fmt.Sprintf("%s: %s", strings.ToUpper(e.state), e.message)
}

func (e initStateError) ExitCode() int {
	if e.code != 0 {
		return e.code
	}
	switch e.state {
	case "configuration_required":
		return 2
	case "failed":
		return 4
	}
	return 3
}

// extractPortMap reads the generated compose YAML to map service ID -> host
// port. Compose output is canonical (we emit it ourselves), so a line-by-
// line scan is reliable enough to avoid a YAML round-trip.
func extractPortMap(compose string, services []detector.Service) map[string]int {
	wanted := map[string]bool{}
	for _, s := range services {
		wanted[s.ID] = true
	}
	portMap := map[string]int{}
	current := ""
	for _, line := range strings.Split(compose, "\n") {
		// Top-level keys ("services:", "networks:") reset the active service.
		if len(line) > 0 && line[0] != ' ' && line[0] != '#' {
			current = ""
			continue
		}
		// Service header: exactly 2-space indent + name + ':'.
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") {
			trimmed := strings.TrimSpace(line)
			if strings.HasSuffix(trimmed, ":") {
				name := strings.TrimSuffix(trimmed, ":")
				if wanted[name] {
					current = name
				} else {
					current = ""
				}
			}
			continue
		}
		if current == "" {
			continue
		}
		// Port line within an indented ports block: `- "host:container"`.
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "- \"") {
			continue
		}
		pair := strings.Trim(t[len("- "):], "\"")
		host, _, ok := strings.Cut(pair, ":")
		if !ok {
			continue
		}
		if n, err := strconv.Atoi(host); err == nil {
			if _, already := portMap[current]; !already {
				portMap[current] = n
			}
		}
	}
	return portMap
}

func persistState(parsed *git.ParsedURL, projectName, targetDir, outputSubdir string, services []detector.Service, infraSvcs []infra.Service, links map[string][]infra.AppLink, portMap map[string]int) error {
	mgr, err := state.For(projectName)
	if err != nil {
		return err
	}
	infraNames := make([]string, 0, len(infraSvcs))
	details := make([]state.InfraDetail, 0, len(infraSvcs))
	for _, s := range infraSvcs {
		infraNames = append(infraNames, s.Name)
		details = append(details, state.InfraDetail{
			Name: s.Name, Kind: string(s.Kind), Mode: s.Mode,
			Provider: s.Provider, Port: s.Port, Reason: s.Reason,
		})
	}
	lockLinks := make(map[string][]state.LinkRef, len(links))
	for appID, ls := range links {
		refs := make([]state.LinkRef, 0, len(ls))
		for _, l := range ls {
			refs = append(refs, state.LinkRef{To: l.ServiceName})
		}
		lockLinks[appID] = refs
	}
	lock := &state.Lock{
		Project:      projectName,
		Repo:         parsed.Repo,
		RepoURL:      parsed.Clone,
		RepoPath:     targetDir,
		OutputSubdir: outputSubdir,
		Services:     services,
		Infra:        infraNames,
		InfraDetails: details,
		Links:        lockLinks,
		PortMap:      portMap,
		Hash:         state.HashDetection(services),
		LastInit:     time.Now().UTC(),
		Version:      Version,
	}
	return mgr.SaveLock(lock)
}

func maybeRunHeal(
	ctx context.Context,
	doBuild bool,
	repo, targetDir, outputDir string,
	out *generator.Output,
	services []detector.Service,
	treeStr string,
	ia *initAgent,
	totalSteps int,
	portMap map[string]int,
	io *initIO,
) (*healer.Result, error) {
	if !doBuild {
		if initNoBuild {
			io.info("Skipping build/heal (--no-build)")
		} else if !docker.Available() {
			io.info("Docker not detected on this machine — skipping build/heal")
		}
		return nil, nil
	}
	io.step(totalSteps, totalSteps, "Build & heal")
	composePath := filepath.Join(outputDir, "docker-compose.yml")
	cm := docker.New(composePath, targetDir, "yoink-"+repo)

	// leaveRunning starts the stack, waits for container health, runs HTTP
	// verification on every published app service, and LEAVES THE STACK
	// RUNNING on success (Phase 6). HTTP 5xx / refused / timeout are not
	// success (Phase 5): a healthy container with a broken app is still a
	// failure. Returns a blocked Result (stack torn down) when unhealthy.
	leaveRunning := func() *healer.Result {
		if _, err := cm.Up(ctx, "-d"); err != nil {
			return &healer.Result{Success: false, Summary: "blocked", FinalOutput: "failed to start the stack: " + err.Error()}
		}
		if !waitForHealthQuick(ctx, cm, io) {
			_, _ = cm.Down(ctx, false)
			return &healer.Result{Success: false, Summary: "blocked", FinalOutput: "containers did not become healthy"}
		}
		checks := httpcheck.Services(ctx, services, portMap)
		// Mark the project as up so `yoink status`/`yoink list` agree.
		if mgr, err := state.For(repo); err == nil {
			if lock, _ := mgr.LoadLock(); lock != nil {
				lock.LastUp = time.Now().UTC()
				_ = mgr.SaveLock(lock)
			}
		}
		if !httpcheck.AllHealthy(checks) {
			_, _ = cm.Down(ctx, false)
			return &healer.Result{Success: false, Summary: "blocked", FinalOutput: blockedHTTPSummary(checks)}
		}
		return &healer.Result{Success: true, Summary: "success", FinalOutput: runningSummary(checks)}
	}

	// First try the build. If it succeeds (with runtime + HTTP verification),
	// we're done — no agent needed — and the stack is left running.
	buildOut, buildErr := cm.Build(ctx)
	if buildErr == nil {
		res := leaveRunning()
		if res.Success {
			io.success("Build succeeded")
			return res, nil
		}
		_, _ = cm.Down(ctx, false)
		io.warn("Build succeeded but runtime unhealthy — invoking agent")
	} else {
		io.info("Build failed — invoking agent for diagnosis and repair")
	}

	// If an LLM is configured, use the agent runtime for multi-tool-call
	// diagnosis and repair. The agent can inspect files, apply patches,
	// rebuild, and verify runtime health — all within bounded budgets.
	var client *llm.Client
	var reader llm.FileReader
	if ia != nil {
		client = ia.client
		reader = ia.read
	}
	if client == nil {
		// No LLM — fall back to the old healer loop (deterministic fixers only).
		loop := &healer.Loop{
			Output:    out,
			Services:  services,
			Compose:   cm,
			Reader:    reader,
			OutputDir: outputDir,
			Tree:      treeStr,
			MaxTries:  initHealTries,
			Tee:       func(line string) { io.info(line) },
		}
		res, err := loop.Run(ctx)
		if err != nil {
			return res, err
		}
		if res.Success {
			// Deterministic healer repaired the build — start + verify +
			// leave running (Phase 5+6).
			res = leaveRunning()
		}
		if res.Success {
			io.success("Build succeeded")
		} else {
			io.warn("Build still failing after heal attempts — see summary")
		}
		return res, nil
	}

	// Agent runtime: multi-tool-call, bounded, evidence-driven.
	mgr, _ := state.For(repo)
	ag := agent.New(client, cm, targetDir, outputDir, mgr, func(line string) { io.info(line) })
	ag.SetDetection(&detector.Result{Services: services}, services, nil)
	ag.SetGenerated(out)
	ag.SetProjectName(repo)
	if reader != nil {
		ag.SetReader(reader)
	} else if safefsReader, err := safefs.New(targetDir); err == nil {
		ag.SetReader(func(p string) (string, error) { return safefsReader.Read(p) })
	}

	healRes, healErr := ag.RunHealLoop(ctx, buildOut, initHealTries)
	if healErr != nil {
		return healRes, healErr
	}
	if healRes.Success {
		// Agent verified health during its cycles (stack is currently down).
		// Phase 5+6: do the final start + HTTP verify + leave running. A
		// container that is healthy but returns 5xx on HTTP is NOT success.
		final := leaveRunning()
		if final.Success {
			io.success("Build succeeded — agent verified runtime health")
			return final, nil
		}
		// Agent said success but the final HTTP probe failed: surface it as
		// blocked rather than claiming success.
		io.warn("Agent repaired the build but runtime verification failed")
		return final, nil
	} else if healRes.Summary == "configuration_required" {
		// Configuration required — not a failure. Show the actionable report.
		fmt.Println()
		fmt.Println(ui.SuccessBox.Render(healRes.FinalOutput))
	} else {
		io.warn("Build still failing after agent attempts")
		if healRes.FinalOutput != "" {
			fmt.Println(ui.DimStyle.Render(healRes.FinalOutput))
		}
	}
	return healRes, nil
}

// waitForHealthQuick polls docker compose ps for up to 60 seconds.
func waitForHealthQuick(ctx context.Context, cm *docker.Compose, io *initIO) bool {
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		ps, err := cm.Ps(ctx)
		if err != nil || len(ps) == 0 {
			time.Sleep(2 * time.Second)
			continue
		}
		allHealthy := true
		for _, c := range ps {
			if c.State != "running" {
				return false
			}
			switch c.Health {
			case "healthy":
			case "unhealthy":
				return false
			default:
				allHealthy = false
			}
		}
		if allHealthy {
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

// initIO bundles verbose/quiet flags and the surface area for printing
// step/spinner/line output, so call sites stay terse.
type initIO struct {
	verbose, quiet bool
}

func (i *initIO) step(n, total int, title string) {
	if !i.quiet {
		fmt.Println(ui.StepLine(n, total, title))
	}
}

func (i *initIO) success(msg string) {
	if !i.quiet {
		fmt.Println(ui.SuccessLine(msg))
	}
}

func (i *initIO) warn(msg string) {
	if !i.quiet {
		fmt.Println(ui.WarningLine(msg))
	}
}

func (i *initIO) info(msg string) {
	if !i.quiet {
		fmt.Println(ui.InfoLine(msg))
	}
}

// withSpinner runs fn while showing a spinner. In quiet mode the spinner is
// suppressed but fn still runs.
func (i *initIO) withSpinner(label string, fn func() error) error {
	if i.quiet {
		return fn()
	}
	sp := ui.StartSpinner(label)
	err := fn()
	sp.Stop()
	return err
}

func loadInitConfig(io *initIO) (*config.Config, error) {
	if initNoAgent {
		cfg := config.LoadOptional()
		io.info("Static-only mode (--no-agent) — LLM configuration not required.")
		return cfg, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("%s\n\nrun  yoink setup  to configure a provider, or pass --no-agent to skip the LLM", err)
	}
	io.success("Configuration loaded")
	return cfg, nil
}

// parseRepoRef resolves the init argument to a repository reference. Empty,
// ".", or an existing directory is treated as a local repository (no clone);
// anything else is parsed as a GitHub URL.
func parseRepoRef(repoURL string) (*git.ParsedURL, error) {
	if git.IsLocalRef(repoURL) {
		return git.ParseLocal(repoURL)
	}
	return git.ParseURL(repoURL)
}

func prepareTargetDir(repoName string, io *initIO) (string, error) {
	reposDir, err := config.ReposDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(reposDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create repos directory: %w", err)
	}
	target := filepath.Join(reposDir, repoName)
	if _, err := os.Stat(target); err == nil {
		if !initForce {
			return "", fmt.Errorf("%s already exists — use --force to re-initialise", target)
		}
		io.warn("Removing existing directory (--force)")
		if err := os.RemoveAll(target); err != nil {
			return "", fmt.Errorf("failed to remove existing directory: %w", err)
		}
	}
	return target, nil
}

func printDetectionTable(d *detector.Result) {
	t := &ui.Table{Headers: []string{"ID", "Type", "Directory", "Framework", "Port", "Confidence"}}
	for _, s := range d.Services {
		dir := s.Directory
		if dir == "" {
			dir = "."
		}
		typeCell := ui.PrimaryStyle.Render(s.Type)
		if s.Type == "frontend" {
			typeCell = ui.HighlightStyle.Render(s.Type)
		}
		t.Rows = append(t.Rows, []string{s.ID, typeCell, dir, s.Framework, fmt.Sprintf("%d", s.Port), ui.ConfidenceBar(s.Confidence)})
	}
	fmt.Println(t.Render())
}

var portKeyRe = regexp.MustCompile(`(?m)^\s*(PORT|HTTP_PORT|SERVER_PORT|APP_PORT)\s*=\s*['"]?(\d{2,5})['"]?\s*$`)

// reconcilePortsFromEnv copies any PORT declared in a service's env content
// back onto the service. Static-served SPAs always sit behind nginx on port
// 80, so PORT is ignored for them. Returns true when a change was made.
func reconcilePortsFromEnv(services []detector.Service, envResults []envvar.Result) bool {
	if len(services) == 0 || len(envResults) == 0 {
		return false
	}
	byID := map[string]*detector.Service{}
	for i := range services {
		byID[services[i].ID] = &services[i]
	}
	changed := false
	for _, r := range envResults {
		if r.EnvContent == "" {
			continue
		}
		svc, ok := byID[r.ServiceID]
		if !ok {
			continue
		}
		switch svc.Framework {
		case "react", "vite", "cra":
			continue
		}
		m := portKeyRe.FindStringSubmatch(r.EnvContent)
		if m == nil {
			continue
		}
		p, err := strconv.Atoi(m[2])
		if err != nil || p < 80 || p > 65535 {
			continue
		}
		if svc.Port != p {
			svc.Port = p
			changed = true
		}
	}
	return changed
}

const generatedGitignore = `# Managed by yoink.
*
!.gitignore
!README.md
!Dockerfile.*
!docker-compose.yml
!quick_start.md
!env-vars/
!env-vars/*/
!env-vars/*/.env.example
`

func writeOutputs(outputDir string, out *generator.Output, envResults []envvar.Result, repoName string, services []detector.Service, outputSubdir string, io *initIO) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	for _, name := range generator.SortedFilenames(out.Files) {
		if err := os.WriteFile(filepath.Join(outputDir, name), []byte(out.Files[name]), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(outputDir, ".gitignore"), []byte(generatedGitignore), 0644); err != nil {
		return fmt.Errorf("failed to write .gitignore: %w", err)
	}
	for _, r := range envResults {
		envDir := filepath.Join(outputDir, "env-vars", r.ServiceID)
		if err := os.MkdirAll(envDir, 0755); err != nil {
			return fmt.Errorf("failed to create env-vars directory: %w", err)
		}
		content := r.EnvContent
		if strings.TrimSpace(content) == "" {
			content = envvar.GenerateEnvExample(r.Vars, r.Technology)
		}
		examplePath := filepath.Join(envDir, ".env.example")
		envPath := filepath.Join(envDir, ".env")
		if err := os.WriteFile(examplePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write .env.example: %w", err)
		}
		// Seed .env from the example so docker compose up works immediately.
		if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
			return fmt.Errorf("failed to write .env: %w", err)
		}
		io.success(fmt.Sprintf("  %s/env-vars/%s/.env.example", outputSubdir, r.ServiceID))
	}
	if err := os.WriteFile(filepath.Join(outputDir, "quick_start.md"), []byte(quickStart(repoName, services, outputSubdir)), 0644); err != nil {
		return fmt.Errorf("failed to write quick_start.md: %w", err)
	}
	io.success("  " + outputSubdir + "/quick_start.md")
	return nil
}

// parseEnvContent parses KEY=VALUE lines from a .env string into a map,
// skipping comments and blank lines. Used to populate BuildEnv for the
// generator's ENV directives.
func parseEnvContent(content string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

func renderCompletionSummary(repoName, outputDir, targetDir, outputSubdir string, elapsed time.Duration, hres *healer.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Init complete for %s in %.1fs\n\n", ui.HighlightStyle.Render(repoName), elapsed.Seconds())
	b.WriteString("Files written to:\n  " + ui.HighlightStyle.Render(outputDir))

	if hres != nil {
		fmt.Fprintf(&b, "\n\nBuild/heal: %d attempt(s)\n", len(hres.Attempts))
		for _, a := range hres.Attempts {
			fmt.Fprintf(&b, "  - %s (%s)", ui.HighlightStyle.Render(string(a.Status)), a.Duration.Truncate(time.Second))
			if a.Service != "" {
				fmt.Fprintf(&b, " · %s", a.Service)
			}
			if a.Summary != "" {
				fmt.Fprintf(&b, " · %s", a.Summary)
			}
			b.WriteString("\n")
		}
		if hres.Success {
			// The stack is running (Phase 6). Show the live URLs + HTTP
			// status rather than telling the user to run `yoink up`.
			if hres.FinalOutput != "" {
				b.WriteString("\n" + hres.FinalOutput)
			} else {
				b.WriteString("\nBuild succeeded — services running")
			}
			return b.String()
		}
		// configuration_required / blocked: the agent path already rendered
		// the actionable report box; show the remaining next-steps otherwise.
		if hres.FinalOutput != "" && (hres.Summary == "blocked" || hres.Summary == "failed") {
			b.WriteString("\n" + hres.FinalOutput)
			return b.String()
		}
	}

	b.WriteString("\n\nNext steps:\n")
	fmt.Fprintf(&b, "  1. Edit  %s/env-vars/<service>/.env\n", outputSubdir)
	fmt.Fprintf(&b, "  2. yoink up %s\n", repoName)
	fmt.Fprintf(&b, "  3. yoink dash %s\n", repoName)
	return b.String()
}

// runningSummary renders the live URLs + HTTP result for a successfully
// running stack, shown by `yoink init` on success (Phase 6).
func runningSummary(checks []httpcheck.Result) string {
	if len(checks) == 0 {
		return ui.SuccessStyle.Render("● Running") + "\n\n  (no HTTP services published)"
	}
	var b strings.Builder
	b.WriteString(ui.SuccessStyle.Render("● Running") + "\n")
	for _, c := range checks {
		sym := ui.SymRun
		line := c.URL
		if c.Code != 0 {
			line = fmt.Sprintf("%s  (HTTP %d)", c.URL, c.Code)
		}
		fmt.Fprintf(&b, "\n  %s  %s", ui.SuccessStyle.Render(sym), ui.HighlightStyle.Render(line))
	}
	return b.String()
}

// blockedHTTPSummary explains why the stack could not be left running: which
// service's HTTP probe failed and how (application error / unreachable /
// timeout). Container-healthy-but-5xx is reported here, not as success.
func blockedHTTPSummary(checks []httpcheck.Result) string {
	var b strings.Builder
	b.WriteString(ui.ErrorStyle.Render("× runtime verification failed") + "\n")
	for _, c := range checks {
		if c.Healthy() {
			continue
		}
		fmt.Fprintf(&b, "\n  %s  %s  %s", ui.ErrorStyle.Render(ui.SymFail), c.URL, c.Status)
		if c.Code != 0 {
			fmt.Fprintf(&b, " (HTTP %d)", c.Code)
		}
		if c.Err != "" {
			b.WriteString("  " + c.Err)
		}
	}
	return b.String()
}
