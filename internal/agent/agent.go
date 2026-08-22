// Package agent implements Yoink's bounded, tool-using AI engineering agent.
//
// The agent is NOT the authority. It reasons about repository failures and
// proposes minimal, evidence-backed repairs. Yoink's deterministic systems
// validate every proposal, apply patches, run builds, and verify runtime
// health. The agent decides WHAT to investigate and WHAT to change;
// Yoink decides whether it's safe and whether it worked.
//
// The agent loop:
//
//	OBSERVE (structured context: detection, failure, evidence)
//	  ↓
//	REASON (LLM call with tools available)
//	  ↓
//	TOOL CALL (read_file, search, build, apply_patch, etc.)
//	  ↓
//	OBSERVE RESULT (tool output fed back)
//	  ↓
//	REASON AGAIN
//	  ↓
//	VERIFY (Yoink independently validates)
//	  ↓
//	COMPLETE / CONTINUE / BLOCK
//
// The agent is bounded by max iterations, max tool calls, max bytes read,
// max builds, and max runtime. If it cannot solve the issue within budget,
// it produces a structured diagnostic report.
package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"yoink/internal/detector"
	"yoink/internal/docker"
	"yoink/internal/generator"
	"yoink/internal/healer"
	"yoink/internal/llm"
	"yoink/internal/state"
)

// LLMClient is the minimal interface the agent needs from the LLM.
type LLMClient interface {
	Call(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// Budget limits the agent's resource consumption.
type Budget struct {
	MaxIterations int
	MaxToolCalls  int
	MaxBytesRead  int
	MaxBuilds     int
	MaxDuration   time.Duration
}

// DefaultBudget returns sensible limits for the MVP.
func DefaultBudget() Budget {
	return Budget{
		MaxIterations: 10,
		MaxToolCalls:  20,
		MaxBytesRead:  256 * 1024,
		MaxBuilds:     4,
		MaxDuration:   5 * time.Minute,
	}
}

// AgentState is the structured memory maintained across iterations.
type AgentState struct {
	ProjectName string
	RepoRoot    string
	OutputDir   string
	Services    []detector.Service
	Infra       []string
	Budget      Budget
	// Detection results (from the deterministic detector).
	Detection *detector.Result
	// Generated artifacts (from the deterministic generator).
	Generated *generator.Output
	// The current failure being investigated (nil if none).
	CurrentFailure *healer.Failure
	// Previous failures and their fingerprints (for progression).
	PrevFailure *healer.Failure
	// All tool calls and results (for audit/debugging).
	ToolHistory []ToolCall
	// All patches applied (for provenance).
	Patches []PatchRecord
	// Environment requirements discovered.
	EnvReqs []EnvRequirement
	// Iteration counter.
	Iteration int
	// Bytes read so far.
	BytesRead int
	// Builds run so far.
	BuildsRun int
	// Start time.
	StartedAt time.Time
	// Tee for progress output.
	Tee func(string)
}

// EnvRequirement captures a discovered environment variable requirement.
type EnvRequirement struct {
	Name            string `json:"name"`
	Phase           string `json:"phase"` // build | runtime | unknown
	Secret          bool   `json:"secret"`
	Provider        string `json:"provider,omitempty"`
	Source          string `json:"source"`
	SafePlaceholder bool   `json:"safe_placeholder"`
}

// PatchRecord records an applied patch for provenance.
type PatchRecord struct {
	File      string
	Operation string
	Reason    string
	Timestamp time.Time
}

// ToolCall records one tool invocation for audit.
type ToolCall struct {
	Tool    string
	Args    map[string]any
	Result  string
	Success bool
}

// Agent is the bounded AI engineering agent.
type Agent struct {
	LLM     LLMClient
	Compose *docker.Compose
	Reader  llm.FileReader
	State   *AgentState
	Manager *state.Manager // for repair provenance
}

// New creates an agent with the given LLM client and Docker handle.
func New(llmClient LLMClient, compose *docker.Compose, repoRoot, outputDir string, manager *state.Manager, tee func(string)) *Agent {
	return &Agent{
		LLM:     llmClient,
		Compose: compose,
		Reader:  nil, // set after safefs init
		Manager: manager,
		State: &AgentState{
			RepoRoot:  repoRoot,
			OutputDir: outputDir,
			Budget:    DefaultBudget(),
			StartedAt: time.Now(),
			Tee:       tee,
		},
	}
}

// SetProjectName sets the project name for the final report.
func (a *Agent) SetProjectName(name string) {
	a.State.ProjectName = name
}

// SetDetection feeds the deterministic detection results to the agent.
func (a *Agent) SetDetection(detection *detector.Result, services []detector.Service, infra []string) {
	a.State.Detection = detection
	a.State.Services = services
	a.State.Infra = infra
}

// SetGenerated feeds the deterministic generation output to the agent.
func (a *Agent) SetGenerated(out *generator.Output) {
	a.State.Generated = out
}

// SetReader sets the sandboxed file reader.
func (a *Agent) SetReader(reader llm.FileReader) {
	a.Reader = reader
}

// RunHealLoop is the main agent entry point when a build fails. It replaces
// the old healer.Loop.Run() with a multi-tool-call agent that can inspect
// files, apply patches, run builds, and verify runtime health — all within
// bounded budgets.
func (a *Agent) RunHealLoop(ctx context.Context, buildOut string, maxTries int) (*healer.Result, error) {
	if maxTries <= 0 {
		maxTries = healer.DefaultMaxTries
	}
	a.State.Budget.MaxIterations = maxTries

	res := &healer.Result{}

	// Parse the initial failure.
	failure := healer.AnalyzeFailure(buildOut, "")
	a.State.CurrentFailure = &failure
	a.log(fmt.Sprintf("→ Agent analyzing failure: %s (%s)", failure.Category, failure.Progression))

	for i := 1; i <= a.State.Budget.MaxIterations; i++ {
		a.State.Iteration = i
		a.log(fmt.Sprintf("→ Agent iteration %d/%d", i, a.State.Budget.MaxIterations))

		// Check budget limits.
		if a.State.BuildsRun >= a.State.Budget.MaxBuilds {
			a.log("  ↳ build budget exhausted")
			break
		}
		if time.Since(a.State.StartedAt) > a.State.Budget.MaxDuration {
			a.log("  ↳ time budget exhausted")
			break
		}

		// Phase 1: Try deterministic fixers FIRST (no LLM needed).
		df := a.readDockerfile()
		if df != "" {
			if fixed, summary, ok := healer.DeterministicFix(a.State.CurrentFailure.RelevantLog, df, ""); ok {
				a.log(fmt.Sprintf("  ↳ deterministic fix: %s", summary))
				a.applyDockerfileFix(fixed, summary)
				// Rebuild and check.
				if a.rebuildAndCheck(ctx, res) {
					return res, nil
				}
				continue
			}
		}

		// Phase 2: Agent reasoning — call LLM with tools available.
		done, err := a.agentIteration(ctx)
		if err != nil {
			// LLM error (API timeout, 503, etc.) — don't just bail.
			// Still produce the final state report so the user gets
			// an actionable summary instead of a raw error.
			a.log("  ↳ agent LLM error: " + err.Error())
			break
		}
		if done {
			// Agent declared it's done — verify independently.
			if a.rebuildAndCheck(ctx, res) {
				return res, nil
			}
			// Build still fails — continue if budget allows.
			if a.State.CurrentFailure != nil {
				a.log(fmt.Sprintf("  ↳ build still failing after agent repair: %s", a.State.CurrentFailure.Category))
			}
			continue
		}

		// Phase 3: If the agent applied patches, rebuild and verify.
		if len(a.State.Patches) > 0 && (i < a.State.Budget.MaxIterations) {
			if a.rebuildAndCheck(ctx, res) {
				return res, nil
			}
		}
	}

	// Final verification build.
	a.log("  ↳ final verification build")
	buildOut2, buildErr := a.Compose.Build(ctx)
	a.State.BuildsRun++
	if buildErr == nil {
		// Runtime verification.
		if a.verifyRuntime(ctx) {
			res.Success = true
			res.FinalOutput = "verified green + runtime healthy"
			return res, nil
		}
	}

	// Build failed — determine the final state (SUCCESS vs CONFIGURATION_REQUIRED vs BLOCKED).
	finalFailure := healer.AnalyzeFailure(buildOut2, "")
	a.State.CurrentFailure = &finalFailure
	res.FinalOutput = finalFailure.Error

	// Collect environment requirements from the failure's env refs.
	a.collectEnvRequirements()

	// Determine the final state.
	state := determineFinalState(false, &finalFailure, a.State.EnvReqs)
	res.Summary = string(state)
	if state == StateConfigRequired {
		// Build the actionable report.
		report := a.buildFinalReport(state)
		res.FinalOutput = report.Render()
	}
	return res, nil
}

// collectEnvRequirements extracts env var requirements from the current
// failure's EnvRefs AND from the BuildEnv map (env vars injected into the
// Dockerfile). For nextjs-build failures where the error doesn't name a
// specific variable, we fall back to the BuildEnv to identify which
// secrets are likely needed.
func (a *Agent) collectEnvRequirements() {
	if a.State.CurrentFailure == nil {
		return
	}
	seen := map[string]bool{}

	// From failure EnvRefs (high confidence — the build error named them).
	for _, name := range a.State.CurrentFailure.EnvRefs {
		if seen[name] {
			continue
		}
		seen[name] = true
		req := EnvRequirement{
			Name:   name,
			Phase:  "build",
			Source: "build error: " + a.State.CurrentFailure.Error,
		}
		upper := strings.ToUpper(name)
		req.Secret = isSecretName(upper)
		req.SafePlaceholder = !req.Secret
		a.State.EnvReqs = append(a.State.EnvReqs, req)
	}

	// From BuildEnv (the env vars injected into the Dockerfile as placeholders).
	// For nextjs-build failures, these are the likely culprits.
	if a.State.CurrentFailure.Category == "nextjs-build" || a.State.CurrentFailure.Category == "missing-environment" {
		for _, svc := range a.State.Services {
			for name, value := range svc.BuildEnv {
				if seen[name] {
					continue
				}
				seen[name] = true
				upper := strings.ToUpper(name)
				isSecret := isSecretName(upper)
				// Only report env vars that are secrets or have empty values
				// (those are the ones the user needs to configure).
				if !isSecret && value != "" && value != "yoink-build-placeholder" {
					continue
				}
				req := EnvRequirement{
					Name:            name,
					Phase:           "build",
					Secret:          isSecret,
					SafePlaceholder: !isSecret,
					Source:          "build-time env injection",
				}
				a.State.EnvReqs = append(a.State.EnvReqs, req)
			}
		}
	}
}

// buildFinalReport constructs the structured terminal report.
func (a *Agent) buildFinalReport(state FinalState) *FinalReport {
	report := &FinalReport{
		State:           state,
		ProjectName:     a.State.ProjectName,
		ServicesTotal:   len(a.State.Services),
		StartedAt:       a.State.StartedAt,
		Duration:        time.Since(a.State.StartedAt),
		AgentIterations: a.State.Iteration,
		RequiredEnvVars: a.State.EnvReqs,
	}
	// Frameworks detected.
	for _, svc := range a.State.Services {
		report.DetectedFrameworks = append(report.DetectedFrameworks, svc.Framework)
	}
	// Deterministic fixes (from patches marked "deterministic").
	for _, p := range a.State.Patches {
		if p.Operation == "deterministic" {
			report.DeterministicFixes = append(report.DeterministicFixes, p.Reason)
		} else {
			report.AgentPatches = append(report.AgentPatches, p)
		}
	}
	// Build log tail.
	if a.State.CurrentFailure != nil {
		report.BuildLogTail = a.State.CurrentFailure.RelevantLog
	}
	return report
}

// isSecretName checks if the variable name looks like a secret.
func isSecretName(upper string) bool {
	for _, marker := range []string{"SECRET", "PASSWORD", "TOKEN", "CREDENTIAL", "PRIVATE_KEY", "API_KEY"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

// agentIteration runs one LLM call with tool-use support. The LLM can:
// 1. Return a repair plan (patches to apply)
// 2. Request file reads
// 3. Request a build
// 4. Declare completion
// 5. Declare blocked (unfixable)
func (a *Agent) agentIteration(ctx context.Context) (bool, error) {
	// Build the structured context for the LLM — including relevant files
	// selected by the existing selectRelevantFiles logic.
	userPrompt := a.buildContextPrompt()

	// Call the LLM.
	raw, err := a.LLM.Call(ctx, agentSystemPrompt, userPrompt)
	if err != nil {
		return false, err
	}

	// Parse the response.
	jsonStr := llm.ExtractJSON(raw)
	var resp agentResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return false, fmt.Errorf("agent response not parseable: %w\nlast: %s", err, truncate(raw, 400))
	}

	// Handle tool calls (the agent wants to inspect files before patching).
	for _, tc := range resp.ToolCalls {
		if a.State.BytesRead >= a.State.Budget.MaxBytesRead {
			a.log("  ↳ read budget exhausted")
			break
		}
		result := a.executeToolCall(ctx, tc)
		_ = result // fed back in the next iteration
	}

	// Handle patch proposals.
	if len(resp.Changes) > 0 {
		applied := a.applyAgentPatches(resp.Changes)
		if !applied {
			a.log("  ↳ patches rejected by invariants or validation")
		}
		return false, nil
	}

	// Handle full-file replacement fallback.
	if resp.Dockerfile != "" || resp.Compose != "" {
		a.log("  ↳ agent returned full-file replacement (fallback)")
		// Write via the existing applyFix path — invariant-checked.
		fix := &llm.BuildFixResponse{
			Service: resp.Service, Dockerfile: resp.Dockerfile, Compose: resp.Compose,
			Summary: resp.Summary,
		}
		dfKey := "Dockerfile." + a.resolveService()
		original := a.State.Generated.Files[dfKey]
		cleaned := llm.CleanContent(fix.Dockerfile)
		violations := healer.CheckInvariants([]healer.Change{{File: dfKey, Operation: "create_file", Content: cleaned}}, map[string]string{dfKey: original})
		if healer.HasRejections(violations) {
			a.log("  ↳ rejected by invariants:\n" + healer.FormatViolations(violations))
			return false, nil
		}
		a.State.Generated.Files[dfKey] = cleaned
		_ = os.WriteFile(filepath.Join(a.State.OutputDir, dfKey), []byte(cleaned), 0644)
		a.log(fmt.Sprintf("  ↳ applied: %s", resp.Summary))
		return false, nil
	}

	if resp.Unfixable {
		a.log("  ↳ agent declared unfixable: " + resp.Summary)
		return true, nil
	}

	if resp.Done {
		return true, nil
	}

	return false, nil
}

// agentResponse is the JSON schema the LLM returns.
type agentResponse struct {
	Thinking   string          `json:"thinking,omitempty"`
	ToolCalls  []toolCallReq   `json:"tool_calls,omitempty"`
	Changes    []healer.Change `json:"changes,omitempty"`
	Dockerfile string          `json:"dockerfile,omitempty"`
	Compose    string          `json:"compose,omitempty"`
	Service    string          `json:"service,omitempty"`
	Summary    string          `json:"summary,omitempty"`
	Done       bool            `json:"done,omitempty"`
	Unfixable  bool            `json:"unfixable,omitempty"`
}

// toolCallReq is a tool call request from the LLM.
type toolCallReq struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args"`
}

// executeToolCall dispatches a single tool call and returns the result string.
func (a *Agent) executeToolCall(ctx context.Context, tc toolCallReq) string {
	switch tc.Tool {
	case "read_file":
		path, _ := tc.Args["path"].(string)
		offset, _ := tc.Args["offset"].(float64)
		limit, _ := tc.Args["limit"].(float64)
		return a.toolReadFile(path, int(offset), int(limit))
	case "search":
		pattern, _ := tc.Args["pattern"].(string)
		path, _ := tc.Args["path"].(string)
		return a.toolSearch(pattern, path)
	case "list_tree":
		path, _ := tc.Args["path"].(string)
		depth, _ := tc.Args["depth"].(float64)
		return a.toolListTree(path, int(depth))
	case "build":
		return a.toolBuild(ctx)
	case "get_logs":
		service, _ := tc.Args["service"].(string)
		tail, _ := tc.Args["tail"].(float64)
		return a.toolGetLogs(ctx, service, int(tail))
	case "check_health":
		return a.toolCheckHealth(ctx)
	default:
		return fmt.Sprintf("unknown tool: %s", tc.Tool)
	}
}

// --- Tool implementations ---

func (a *Agent) toolReadFile(path string, offset, limit int) string {
	fullPath := filepath.Join(a.State.RepoRoot, path)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Sprintf("[error reading %s: %v]", path, err)
	}
	content := string(data)
	// Apply offset/limit for bounded reads.
	if offset > 0 {
		lines := strings.Split(content, "\n")
		if offset < len(lines) {
			end := offset + limit
			if limit <= 0 || end > len(lines) {
				end = len(lines)
			}
			content = strings.Join(lines[offset:end], "\n")
		} else {
			content = ""
		}
	} else if limit > 0 {
		lines := strings.Split(content, "\n")
		if limit < len(lines) {
			content = strings.Join(lines[:limit], "\n") + "\n[...truncated...]"
		}
	}
	// Cap at 16KB per read.
	if len(content) > 16*1024 {
		content = content[:16*1024] + "\n[...truncated at 16KB...]"
	}
	a.State.BytesRead += len(content)
	a.State.ToolHistory = append(a.State.ToolHistory, ToolCall{
		Tool: "read_file", Args: map[string]any{"path": path, "offset": offset, "limit": limit},
		Result: content, Success: true,
	})
	a.log(fmt.Sprintf("  → read_file(%s, offset=%d, limit=%d) → %d bytes", path, offset, limit, len(content)))
	return content
}

func (a *Agent) toolSearch(pattern, path string) string {
	searchDir := a.State.RepoRoot
	if path != "" {
		searchDir = filepath.Join(a.State.RepoRoot, path)
	}
	var matches []string
	_ = filepath.WalkDir(searchDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.Contains(filepath.Base(p), pattern) {
			rel, _ := filepath.Rel(a.State.RepoRoot, p)
			matches = append(matches, rel)
		}
		return nil
	})
	result := strings.Join(matches, "\n")
	if result == "" {
		result = "[no matches]"
	}
	a.State.ToolHistory = append(a.State.ToolHistory, ToolCall{
		Tool: "search", Args: map[string]any{"pattern": pattern, "path": path},
		Result: result, Success: true,
	})
	a.log(fmt.Sprintf("  → search(%s, %s) → %d matches", pattern, path, len(matches)))
	return result
}

func (a *Agent) toolListTree(path string, depth int) string {
	searchDir := a.State.RepoRoot
	if path != "" {
		searchDir = filepath.Join(a.State.RepoRoot, path)
	}
	var b strings.Builder
	_ = filepath.WalkDir(searchDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(searchDir, p)
		if rel == "." {
			return nil
		}
		dep := strings.Count(rel, string(filepath.Separator))
		if depth > 0 && dep > depth {
			if dep == depth+1 {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip noise.
		base := d.Name()
		if d.IsDir() && (base == "node_modules" || base == ".git" || base == ".next" || base == "dist") {
			return filepath.SkipDir
		}
		b.WriteString(rel)
		b.WriteString("\n")
		return nil
	})
	result := b.String()
	if len(result) > 8*1024 {
		result = result[:8*1024] + "\n[...truncated...]"
	}
	a.State.ToolHistory = append(a.State.ToolHistory, ToolCall{
		Tool: "list_tree", Args: map[string]any{"path": path, "depth": depth},
		Result: result, Success: true,
	})
	a.log(fmt.Sprintf("  → list_tree(%s, depth=%d)", path, depth))
	return result
}

func (a *Agent) toolBuild(ctx context.Context) string {
	if a.State.BuildsRun >= a.State.Budget.MaxBuilds {
		return "[build budget exhausted]"
	}
	out, err := a.Compose.Build(ctx)
	a.State.BuildsRun++
	if err != nil {
		f := healer.AnalyzeFailure(out, "")
		a.State.CurrentFailure = &f
		a.State.PrevFailure = &f
		return fmt.Sprintf("BUILD FAILED\n%s", f.RelevantLog)
	}
	return "BUILD SUCCEEDED"
}

func (a *Agent) toolGetLogs(ctx context.Context, service string, tail int) string {
	if tail <= 0 {
		tail = 50
	}
	out, err := a.Compose.Logs(ctx, service, tail)
	if err != nil {
		return fmt.Sprintf("[error: %v]", err)
	}
	return out
}

func (a *Agent) toolCheckHealth(ctx context.Context) string {
	ps, err := a.Compose.Ps(ctx)
	if err != nil {
		return fmt.Sprintf("[error: %v]", err)
	}
	var b strings.Builder
	for _, c := range ps {
		fmt.Fprintf(&b, "%s: state=%s health=%s\n", c.Service, c.State, c.Health)
	}
	return b.String()
}

// --- Patch application ---

func (a *Agent) applyAgentPatches(changes []healer.Change) bool {
	// Normalize file paths FIRST (strip yoink-outputs/ prefix) so the
	// originals map uses the same keys as the in-memory Output.Files.
	normalized := make([]healer.Change, len(changes))
	for i, c := range changes {
		file := strings.TrimPrefix(c.File, "yoink-outputs/")
		normalized[i] = c
		normalized[i].File = file
	}
	changes = normalized

	originals := map[string]string{}
	for _, c := range changes {
		if _, ok := originals[c.File]; !ok {
			full := filepath.Join(a.State.OutputDir, c.File)
			if data, err := os.ReadFile(full); err == nil {
				originals[c.File] = string(data)
			} else if f, ok := a.State.Generated.Files[c.File]; ok {
				originals[c.File] = f
			}
		}
	}
	// Invariant check.
	violations := healer.CheckInvariants(changes, originals)
	if healer.HasRejections(violations) {
		a.log("  ↳ rejected by invariants:\n" + healer.FormatViolations(violations))
		return false
	}
	// Scope check.
	scope := healer.EvaluateScope(changes, originals, "")
	if scope.ShouldReject() {
		a.log("  ↳ rejected by scope: " + scope.Reason)
		return false
	}
	// Apply each patch.
	applied := false
	for _, change := range changes {
		file := change.File
		diskPath := filepath.Join(a.State.OutputDir, file)
		original := originals[file]
		if original == "" {
			original = a.State.Generated.Files[file]
		}
		patched, err := healer.ApplyPatch(original, change)
		if err != nil {
			a.log(fmt.Sprintf("  ↳ patch failed for %s: %v", file, err))
			continue
		}
		cleaned := llm.CleanContent(patched)
		a.State.Generated.Files[file] = cleaned
		_ = os.WriteFile(diskPath, []byte(cleaned), 0644)
		applied = true
		a.State.Patches = append(a.State.Patches, PatchRecord{
			File: file, Operation: change.Operation, Reason: change.Reason, Timestamp: time.Now(),
		})
		a.log(fmt.Sprintf("  ↳ patched %s (%s)", file, change.Operation))
		// Record provenance.
		if a.Manager != nil {
			_ = a.Manager.RecordRepair(state.RepairRecord{
				Timestamp: time.Now().UTC(), File: file,
				OriginalHash: hashString(original), ResultingHash: hashString(cleaned),
				Diagnosis: change.Reason, Operation: "agent-patch",
			})
		}
	}
	return applied
}

// --- Helpers ---

func (a *Agent) readDockerfile() string {
	dfKey := "Dockerfile." + a.resolveService()
	if df, ok := a.State.Generated.Files[dfKey]; ok {
		return df
	}
	return ""
}

func (a *Agent) resolveService() string {
	if len(a.State.Services) == 1 {
		return a.State.Services[0].ID
	}
	if a.State.CurrentFailure != nil && a.State.CurrentFailure.Service != "" {
		return a.State.CurrentFailure.Service
	}
	if len(a.State.Services) > 0 {
		return a.State.Services[0].ID
	}
	return ""
}

func (a *Agent) applyDockerfileFix(fixed, summary string) {
	dfKey := "Dockerfile." + a.resolveService()
	cleaned := llm.CleanContent(fixed)
	a.State.Generated.Files[dfKey] = cleaned
	_ = os.WriteFile(filepath.Join(a.State.OutputDir, dfKey), []byte(cleaned), 0644)
	if a.Manager != nil {
		_ = a.Manager.RecordRepair(state.RepairRecord{
			Timestamp: time.Now().UTC(), File: dfKey,
			Operation: "deterministic", Summary: summary,
		})
	}
}

func (a *Agent) rebuildAndCheck(ctx context.Context, res *healer.Result) bool {
	buildOut, buildErr := a.Compose.Build(ctx)
	a.State.BuildsRun++
	res.Attempts = append(res.Attempts, healer.Attempt{
		N: a.State.Iteration, Status: healer.StatusFixed, Summary: "agent repair",
	})

	if buildErr != nil {
		// Build failed — analyze the new failure.
		newFailure := healer.AnalyzeFailure(buildOut, "")
		newFailure.Progression = healer.ClassifyProgression(&newFailure, a.State.PrevFailure)
		a.State.CurrentFailure = &newFailure
		a.State.PrevFailure = &newFailure
		a.log(fmt.Sprintf("  ↳ build failed: %s (%s)", newFailure.Category, newFailure.Progression))
		return false
	}

	// Build succeeded — runtime verification.
	if a.verifyRuntime(ctx) {
		res.Success = true
		res.FinalOutput = "verified green + runtime healthy"
		a.log("  ↳ build succeeded + runtime healthy")
		return true
	}

	a.log("  ↳ build succeeded but runtime unhealthy")
	return false
}

func (a *Agent) verifyRuntime(ctx context.Context) bool {
	if a.Compose == nil {
		return true // no Docker — can't verify
	}
	upOut, upErr := a.Compose.Up(ctx, "-d")
	if upErr != nil {
		a.log("  ↳ containers failed to start: " + upOut)
		return false
	}
	healthy := a.waitForHealth(ctx)
	_, _ = a.Compose.Down(ctx, false)
	return healthy
}

func (a *Agent) waitForHealth(ctx context.Context) bool {
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		ps, err := a.Compose.Ps(ctx)
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

func (a *Agent) log(msg string) {
	if a.State.Tee != nil {
		a.State.Tee(msg)
	}
}

func (a *Agent) buildContextPrompt() string {
	var b strings.Builder

	// Service metadata.
	if len(a.State.Services) > 0 {
		svc := a.State.Services[0]
		fmt.Fprintf(&b, "SERVICE: %s\nFramework: %s\nLanguage: %s\nPM: %s\nPort: %d\n", svc.ID, svc.Framework, svc.Language, svc.PackageManager, svc.Port)
		if len(svc.Evidence) > 0 {
			b.WriteString("\nDETECTION EVIDENCE:\n")
			for _, e := range svc.Evidence {
				fmt.Fprintf(&b, "  %s (%s, source: %s)\n", e.Fact, e.Weight, e.Source)
			}
		}
	}

	// Current failure.
	if a.State.CurrentFailure != nil {
		f := a.State.CurrentFailure
		fmt.Fprintf(&b, "\nFAILURE:\n  Category: %s\n  Error: %s\n", f.Category, f.Error)
		if len(f.FileRefs) > 0 {
			fmt.Fprintf(&b, "  Files: %s\n", strings.Join(f.FileRefs, ", "))
		}
		if len(f.EnvRefs) > 0 {
			fmt.Fprintf(&b, "  EnvVars: %s\n", strings.Join(f.EnvRefs, ", "))
		}
		fmt.Fprintf(&b, "  Progression: %s\n", f.Progression)
		fmt.Fprintf(&b, "\nRELEVANT LOG:\n%s\n", f.RelevantLog)
	}

	// Current Dockerfile.
	dfKey := "Dockerfile." + a.resolveService()
	if df, ok := a.State.Generated.Files[dfKey]; ok {
		fmt.Fprintf(&b, "\nCURRENT DOCKERFILE:\n%s\n", df)
	}

	// Current compose.
	if compose, ok := a.State.Generated.Files["docker-compose.yml"]; ok {
		fmt.Fprintf(&b, "\nCURRENT COMPOSE:\n%s\n", compose)
	}

	// Relevant files — selected deterministically based on failure type.
	// This gives the agent immediate access to package.json, tsconfig,
	// source files, workspace configs, etc. without wasting iterations.
	if a.State.CurrentFailure != nil {
		svc := detector.Service{}
		if len(a.State.Services) > 0 {
			svc = a.State.Services[0]
		}
		pack := healer.BuildContextPack(svc, *a.State.CurrentFailure,
			a.State.Generated.Files[dfKey], a.State.Generated.Files["docker-compose.yml"],
			a.State.RepoRoot, a.State.OutputDir, nil)
		if len(pack.RelevantFiles) > 0 {
			b.WriteString("\nRELEVANT FILES (pre-selected):\n")
			for _, f := range pack.RelevantFiles {
				fmt.Fprintf(&b, "\n=== %s ===\n(%s)\n%s\n", f.Path, f.Reason, f.Content)
			}
		}
	}

	// Previous patches.
	if len(a.State.Patches) > 0 {
		b.WriteString("\nPREVIOUS PATCHES:\n")
		for _, p := range a.State.Patches {
			fmt.Fprintf(&b, "  %s (%s): %s\n", p.File, p.Operation, p.Reason)
		}
		b.WriteString("\nDo NOT repeat a strategy that already failed.\n")
	}

	// Tool history (last few).
	if len(a.State.ToolHistory) > 0 {
		b.WriteString("\nRECENT TOOL CALLS:\n")
		start := len(a.State.ToolHistory) - 3
		if start < 0 {
			start = 0
		}
		for _, tc := range a.State.ToolHistory[start:] {
			fmt.Fprintf(&b, "  %s → %s\n", tc.Tool, truncate(tc.Result, 200))
		}
	}

	b.WriteString("\nAvailable tools: read_file(path, offset, limit), search(pattern, path), list_tree(path, depth), build(), get_logs(service, tail), check_health()\n")
	b.WriteString("\nRespond with JSON: {\"thinking\": \"...\", \"tool_calls\": [...], \"changes\": [...], \"summary\": \"...\", \"done\": true/false, \"unfixable\": true/false}\n")
	return b.String()
}

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// agentSystemPrompt is the system prompt for the agent.
const agentSystemPrompt = `You are Yoink's local software deployment engineer.

Your objective is to make the repository runnable locally with the smallest safe set of changes.

RULES:
1. Inspect before modifying.
2. Never guess when evidence can be obtained.
3. Prefer deterministic evidence (already provided in context).
4. Read only relevant file ranges (use offset/limit).
5. Do not overwrite entire files unnecessarily — use patch operations.
6. Never bypass validation or disable type checking.
7. Never remove healthchecks or add sleep commands.
8. Never expose or bake secrets into Dockerfiles.
9. Never modify unrelated source code without strong evidence.
10. Never declare success yourself — Yoink verifies independently.
11. Do not repeat a failed strategy without new evidence.
12. When blocked by real secrets, report exactly what the user must configure.

RESPONSE FORMAT (JSON only, no markdown):
{
  "thinking": "your reasoning",
  "tool_calls": [{"tool": "read_file", "args": {"path": "...", "offset": 0, "limit": 50}}],
  "changes": [{"file": "Dockerfile.service-1", "operation": "insert_after", "anchor": "...", "content": "...", "reason": "..."}],
  "summary": "one-line explanation",
  "done": false,
  "unfixable": false
}

Use tool_calls to inspect files BEFORE proposing changes. When you have enough evidence, propose changes. Set done=true when you believe the repair is complete (Yoink will verify). Set unfixable=true when the failure is outside your scope (e.g. missing secrets).`
