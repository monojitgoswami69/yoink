// Package healer runs the build-heal loop: invoke `docker compose build`,
// when the build fails extract the affected service and the tail of the
// build output, ask the LLM for a corrected Dockerfile (or compose), apply
// it, and retry. Loop is capped at a small number of attempts so a
// genuinely broken project doesn't burn LLM credit forever.
package healer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"yoink/internal/detector"
	"yoink/internal/docker"
	"yoink/internal/generator"
	"yoink/internal/llm"
	"yoink/internal/state"
)

// DefaultMaxTries is the cap on heal attempts when none is provided.
const DefaultMaxTries = 3

// LLMClient is the minimal interface the healer needs from the LLM. The
// concrete *llm.Client satisfies this interface, and tests can provide a
// mock to exercise the full heal loop without a real API call.
type LLMClient interface {
	Call(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// Loop holds everything needed to drive one build/heal session.
type Loop struct {
	Output    *generator.Output
	Services  []detector.Service
	Compose   *docker.Compose
	LLM       LLMClient
	Reader    llm.FileReader
	OutputDir string // absolute path: <repo>/yoink-outputs
	Tree      string
	MaxTries  int
	// Tee receives one short progress line per state change. Optional.
	Tee func(line string)
	// Manager is the project state manager for repair provenance.
	// When set, healer modifications are recorded as RepairRecords.
	Manager *state.Manager
}

// AttemptStatus is the outcome of a single iteration.
type AttemptStatus string

const (
	StatusBuilt    AttemptStatus = "built"   // build succeeded
	StatusFixed    AttemptStatus = "fixed"   // build failed, LLM proposed a fix
	StatusGaveUp   AttemptStatus = "gave_up" // LLM declined to fix
	StatusNoFix    AttemptStatus = "no_fix"  // LLM returned no usable fix
	StatusBuildErr AttemptStatus = "build_err"
)

// Attempt records one iteration's input and outcome.
type Attempt struct {
	N        int           `json:"n"`
	Status   AttemptStatus `json:"status"`
	Service  string        `json:"service,omitempty"`
	Summary  string        `json:"summary,omitempty"`
	LogTail  string        `json:"log_tail,omitempty"`
	RawLog   string        `json:"raw_log,omitempty"`
	Duration time.Duration `json:"duration"`
}

// Result is the overall outcome of Run.
type Result struct {
	Success  bool
	Attempts []Attempt
	// FinalOutput is the trailing build log of the last attempt — useful when
	// the build still fails after MaxTries so the user can copy/paste.
	FinalOutput string
	// Summary carries the terminal state when the agent determines the
	// project can't be completed (e.g. "configuration_required", "blocked").
	Summary string
}

// Run executes the loop. Output files are expected to already be on disk;
// fixes are written back to the same paths.
func (l *Loop) Run(ctx context.Context) (*Result, error) {
	if l.MaxTries <= 0 {
		l.MaxTries = DefaultMaxTries
	}
	res := &Result{}

	// Track previous fix attempts for diff-aware context.
	var previousAttempts []AttemptContext
	var prevFailure *Failure

	for i := 1; i <= l.MaxTries; i++ {
		l.log(fmt.Sprintf("→ Build attempt %d/%d", i, l.MaxTries))
		start := time.Now()
		buildOut, buildErr := l.Compose.Build(ctx)
		attempt := Attempt{N: i, Duration: time.Since(start)}

		if buildErr == nil {
			attempt.Status = StatusBuilt
			res.Attempts = append(res.Attempts, attempt)
			// Runtime verification: build success ≠ runtime success.
			// Start containers, wait for health, verify stability.
			if l.Compose != nil {
				l.log("  ↳ runtime verification")
				upOut, upErr := l.Compose.Up(ctx, "-d")
				if upErr != nil {
					l.log("  ↳ containers failed to start: " + docker.TailLines(upOut, 20))
					res.Success = false
					res.FinalOutput = docker.TailLines(upOut, 30)
					continue // not a real success
				}
				healthy := l.waitForRuntimeHealth(ctx)
				if !healthy {
					l.log("  ↳ healthcheck failed — build passed but runtime is unhealthy")
					_, _ = l.Compose.Down(ctx, false)
					res.Success = false
					continue
				}
				_, _ = l.Compose.Down(ctx, false)
			}
			res.Success = true
			res.FinalOutput = docker.TailLines(buildOut, 30)
			return res, nil
		}

		attempt.Status = StatusBuildErr
		attempt.RawLog = buildOut
		attempt.LogTail = docker.TailLines(buildOut, 80)
		attempt.Service = docker.ExtractFailedService(buildOut)

		// Structured failure analysis — replaces raw log as the primary
		// diagnostic signal. The LLM receives the structured Failure,
		// not the raw 80-line tail.
		failure := AnalyzeFailure(buildOut, attempt.Service)
		failure.Progression = ClassifyProgression(&failure, prevFailure)
		l.log(fmt.Sprintf("  ↳ failure: %s (%s)", failure.Category, failure.Progression))

		// Resolve the canonical service ID ONCE and use it everywhere.
		// When ExtractFailedService returns "" (common for single-service
		// repos), default to the first/only service. This prevents the
		// "Dockerfile." key mismatch between snapshot and staleness check.
		resolvedService := resolveServiceID(l.Services, attempt.Service)
		if resolvedService != "" {
			attempt.Service = resolvedService
		}
		dfKey := "Dockerfile." + resolvedService

		// Re-read the Dockerfile from DISK (not in-memory) to guarantee
		// current-state freshness.
		diskDockerfile := l.readDockerfileFromDisk(attempt.Service)
		if diskDockerfile != "" {
			l.Output.Files[dfKey] = diskDockerfile
		}
		// Use the resolved key so snapshot and staleness check read the
		// SAME file (not the concatenated blob from dockerfileForService).
		failingDockerfile := l.Output.Files[dfKey]
		if failingDockerfile == "" {
			failingDockerfile = l.dockerfileForService(attempt.Service)
		}
		compose := l.Output.Files["docker-compose.yml"]

		// Deterministic pre-fix for known failure patterns. These run
		// REGARDLESS of whether an LLM is configured — they need no model
		// and are the highest-confidence repairs (Phase 11 boundary:
		// deterministic before agentic). If one applies, rebuild and verify;
		// only when no deterministic fix applies do we need the LLM.
		if fixed, summary, ok := deterministicFix(buildOut, failingDockerfile, attempt.Service); ok {
			attempt.Status = StatusFixed
			attempt.Summary = summary
			cleaned := llm.CleanContent(fixed)
			filename := "Dockerfile." + resolveServiceID(l.Services, attempt.Service)
			l.Output.Files[filename] = cleaned
			if err := os.WriteFile(filepath.Join(l.OutputDir, filename), []byte(cleaned), 0644); err != nil {
				attempt.Summary = "could not apply deterministic fix: " + err.Error()
				res.Attempts = append(res.Attempts, attempt)
				res.FinalOutput = attempt.LogTail
				return res, nil
			}
			l.log(fmt.Sprintf("  ↳ applied: %s", attempt.Summary))
			res.Attempts = append(res.Attempts, attempt)
			previousAttempts = append(previousAttempts, AttemptContext{
				N: i, Diagnosis: summary, ChangedFiles: []string{filename},
				Result: "deterministic fix applied", Progression: "progressed",
			})
			prevFailure = &failure
			continue
		}

		// No LLM configured and no deterministic fix applied — surface the
		// failure. (Deterministic fixes above already ran.)
		if l.LLM == nil {
			attempt.Summary = "build failed; no LLM configured to heal"
			res.Attempts = append(res.Attempts, attempt)
			res.FinalOutput = attempt.LogTail
			return res, nil
		}

		// Build the structured context pack — includes service metadata,
		// structured failure, relevant files, and diff-aware iteration.
		var svc detector.Service
		for _, s := range l.Services {
			if s.ID == attempt.Service || strings.EqualFold(s.ID, attempt.Service) {
				svc = s
				break
			}
		}
		pack := BuildContextPack(svc, failure, failingDockerfile, compose, l.Compose.Dir, l.OutputDir, previousAttempts)

		// WIRE: ContextSnapshot now hashes EVERY file supplied to the LLM
		// (Dockerfile, compose, AND all relevant files from pack.RelevantFiles).
		// dfKey was already resolved above — reuse it.
		// NOTE: We hash the FULL file content from disk (not the truncated
		// 16KB version in pack.RelevantFiles[].Content which is for the LLM).
		// This prevents false stale detection on large files like package-lock.json.
		snapshotFiles := map[string]string{
			dfKey:                failingDockerfile,
			"docker-compose.yml": compose,
		}
		for _, f := range pack.RelevantFiles {
			if fullContent, err := os.ReadFile(filepath.Join(l.Compose.Dir, f.Path)); err == nil {
				snapshotFiles[f.Path] = string(fullContent)
			} else {
				snapshotFiles[f.Path] = f.Content
			}
		}
		snapshot := NewSnapshot(snapshotFiles)

		// WIRE: callLLMForRepair uses pack.Render() as the structured user
		// prompt and parses a hybrid response (RepairPlan changes or
		// full-file replacement fallback). This replaces the old
		// FixBuildFailure call that sent raw Dockerfile + 80-line error tail.
		l.log(fmt.Sprintf("  ↳ asking LLM to repair %s (category=%s, %d relevant files)", attempt.Service, failure.Category, len(pack.RelevantFiles)))
		resp, err := l.callLLMForRepair(ctx, pack, l.Reader, previousFixesFromAttempts(previousAttempts))
		if err != nil {
			attempt.Summary = "LLM error: " + err.Error()
			res.Attempts = append(res.Attempts, attempt)
			res.FinalOutput = attempt.LogTail
			return res, nil
		}

		if resp.Unfixable {
			attempt.Status = StatusGaveUp
			attempt.Summary = nonEmpty(resp.Summary, "LLM declined to fix")
			res.Attempts = append(res.Attempts, attempt)
			res.FinalOutput = attempt.LogTail
			return res, nil
		}

		// Stale-context protection: if ANY file supplied to the LLM (Dockerfile,
		// compose, OR relevant source files like package.json/tsconfig) changed
		// on disk during the LLM call, reject the fix and refresh context.
		currentFiles := map[string]string{
			dfKey:                l.readDockerfileFromDisk(attempt.Service),
			"docker-compose.yml": readFileFromDisk(l.OutputDir, "docker-compose.yml"),
		}
		// Re-read ALL relevant files that were in the snapshot.
		for _, f := range pack.RelevantFiles {
			if content, err := os.ReadFile(filepath.Join(l.Compose.Dir, f.Path)); err == nil {
				currentFiles[f.Path] = string(content)
			}
		}
		if staleFile, isStale := snapshot.IsStale(currentFiles); isStale {
			l.log("  ↳ stale context: " + staleFile + " changed during LLM call, rejecting fix")
			if df := l.readDockerfileFromDisk(attempt.Service); df != "" {
				l.Output.Files[dfKey] = df
			}
			res.Attempts = append(res.Attempts, Attempt{
				N: i, Status: StatusNoFix, Summary: "stale context: " + staleFile,
			})
			prevFailure = &failure
			continue
		}

		// WIRE: Dispatch based on response type.
		// If the LLM returned a RepairPlan with changes → use ApplyPatch
		// (patch-based, anchor-validated, minimal).
		// If the LLM returned a full-file replacement → use applyFix
		// (existing path with invariant checks).
		var applied bool
		var changedFiles []string
		var applyErr error
		if resp.hasChanges() {
			// WIRE: ApplyPatch — the patch-based path.
			applied, changedFiles, applyErr = l.applyPatches(resp.Changes, attempt.Service)
		} else if resp.hasFullReplacement() {
			// Fallback: full-file replacement via applyFix (with invariants).
			applied, applyErr = l.applyFix(resp.toBuildFixResponse(), attempt.Service)
			if applied {
				changedFiles = []string{attempt.Service}
			}
		}
		if applyErr != nil {
			attempt.Summary = "could not apply fix: " + applyErr.Error()
			res.Attempts = append(res.Attempts, attempt)
			res.FinalOutput = attempt.LogTail
			return res, nil
		}
		if !applied {
			// Patch failed or no usable fix — CONTINUE to next attempt
			// (don't exit the loop). The LLM gets fresh context + the
			// failure info on the next iteration.
			attempt.Status = StatusNoFix
			attempt.Summary = nonEmpty(resp.Summary, "LLM patch failed or returned no usable fix")
			res.Attempts = append(res.Attempts, attempt)
			previousAttempts = append(previousAttempts, AttemptContext{
				N: i, Diagnosis: attempt.Summary,
				Result: failure.Progression, Progression: failure.Progression,
			})
			prevFailure = &failure
			continue
		}

		attempt.Status = StatusFixed
		attempt.Summary = nonEmpty(resp.Summary, "applied LLM fix")
		l.log(fmt.Sprintf("  ↳ applied: %s", attempt.Summary))

		// Re-run the deterministic fixer on the LLM's output to ensure
		// known patterns survive every LLM rewrite.
		filename := "Dockerfile." + resolveServiceID(l.Services, attempt.Service)
		if df, ok := l.Output.Files[filename]; ok {
			if fixed, summ, dok := deterministicFix(attempt.LogTail, df, attempt.Service); dok {
				cleaned := llm.CleanContent(fixed)
				l.Output.Files[filename] = cleaned
				_ = os.WriteFile(filepath.Join(l.OutputDir, filename), []byte(cleaned), 0644)
				attempt.Summary += " + " + summ
				l.log(fmt.Sprintf("  ↳ re-applied deterministic: %s", summ))
			}
		}

		res.Attempts = append(res.Attempts, attempt)
		previousAttempts = append(previousAttempts, AttemptContext{
			N: i, Diagnosis: attempt.Summary,
			ChangedFiles: changedFiles,
			Result:       failure.Progression,
			Progression:  failure.Progression,
		})
		prevFailure = &failure
	}

	// The loop only builds at the START of each iteration, so a fix applied
	// on the final attempt is never verified. Do one last build + runtime
	// verification to confirm — build success is NOT runtime success.
	l.log("  ↳ final verification build")
	start := time.Now()
	finalOut, finalErr := l.Compose.Build(ctx)
	if finalErr == nil {
		// Runtime verification: start containers, wait for health, stop.
		if l.Compose != nil {
			l.log("  ↳ final runtime verification")
			_, _ = l.Compose.Up(ctx, "-d")
			healthy := l.waitForRuntimeHealth(ctx)
			_, _ = l.Compose.Down(ctx, false)
			if !healthy {
				l.log("  ↳ final runtime verification failed")
				res.FinalOutput = docker.TailLines(finalOut, 30)
				return res, nil
			}
		}
		res.Success = true
		res.FinalOutput = docker.TailLines(finalOut, 30)
		res.Attempts = append(res.Attempts, Attempt{
			N: l.MaxTries + 1, Status: StatusBuilt, Summary: "verified green + runtime healthy", Duration: time.Since(start),
		})
		return res, nil
	}
	res.FinalOutput = docker.TailLines(finalOut, 80)
	return res, nil
}

// applyPatches applies a list of Changes to the generated files using
// ApplyPatch (anchor-validated, minimal patches). This is the WIRED
// patch-based path that replaces full-file replacement when the LLM returns
// a RepairPlan with changes. Returns (applied, changedFiles, error).
func (l *Loop) applyPatches(changes []Change, failingService string) (bool, []string, error) {
	// Normalize and validate every path before reading or mutating anything.
	// The LLM may use compose-relative yoink-outputs/ paths, but it may only
	// modify artifacts already owned by this generated output.
	normalized := make([]Change, len(changes))
	for i, change := range changes {
		change.File = strings.TrimPrefix(change.File, "yoink-outputs/")
		if !validGeneratedPath(change.File, l.Services, l.Output) {
			return false, nil, fmt.Errorf("repair path is outside generated artifacts: %s", change.File)
		}
		normalized[i] = change
	}
	changes = normalized
	// Invariant check: reject plans that weaken validation.
	originals := map[string]string{}
	for _, c := range changes {
		if _, ok := originals[c.File]; !ok {
			if content, err := os.ReadFile(filepath.Join(l.OutputDir, c.File)); err == nil {
				originals[c.File] = string(content)
			} else if f, ok := l.Output.Files[c.File]; ok {
				originals[c.File] = f
			}
		}
	}
	violations := CheckInvariants(changes, originals)
	if HasRejections(violations) {
		l.log("  ↳ rejected by invariants:\n" + FormatViolations(violations))
		return false, nil, nil
	}

	// Scope check: flag suspiciously broad repairs.
	scope := EvaluateScope(changes, originals, "")
	if scope.ShouldReject() {
		l.log("  ↳ rejected by scope (" + scope.Class + "): " + scope.Reason)
		return false, nil, nil
	}

	applied := false
	var changedFiles []string
	for _, change := range changes {
		// Normalize: strip yoink-outputs/ prefix if the LLM used the full
		// path from the compose reference (e.g. "yoink-outputs/Dockerfile.service-1").
		file := strings.TrimPrefix(change.File, "yoink-outputs/")
		// Read current content from disk (fresh state).
		diskPath := filepath.Join(l.OutputDir, file)
		content, err := os.ReadFile(diskPath)
		var original string
		if err == nil {
			original = string(content)
		} else if f, ok := l.Output.Files[file]; ok {
			original = f
		}

		// WIRE: ApplyPatch — the patch-based application with anchor validation.
		patched, err := ApplyPatch(original, change)
		if err != nil {
			l.log(fmt.Sprintf("  ↳ patch failed for %s: %v", file, err))
			continue
		}

		cleaned := llm.CleanContent(patched)
		l.Output.Files[file] = cleaned
		if err := os.WriteFile(diskPath, []byte(cleaned), 0644); err != nil {
			return false, changedFiles, err
		}
		changedFiles = append(changedFiles, file)
		applied = true
		l.log(fmt.Sprintf("  ↳ patched %s (%s: %s)", file, change.Operation, change.Reason))

		// Record repair provenance.
		if l.Manager != nil {
			_ = l.Manager.RecordRepair(state.RepairRecord{
				Timestamp:     time.Now().UTC(),
				Service:       failingService,
				File:          file,
				OriginalHash:  hashString(original),
				ResultingHash: hashString(cleaned),
				Diagnosis:     change.Reason,
				Operation:     "llm-patch",
			})
		}
	}
	return applied, changedFiles, nil
}

// applyFix mutates the on-disk + in-memory Output to reflect the model's
// proposed change. Returns (true, nil) when a usable change was applied,
// (false, nil) when the model returned nothing actionable or the fix was
// rejected by invariant checks.
func (l *Loop) applyFix(fix *llm.BuildFixResponse, failingService string) (bool, error) {
	wrote := false

	if fix.Dockerfile != "" {
		service := fix.Service
		if service == "" {
			service = failingService
		}
		if service == "" {
			return false, fmt.Errorf("LLM returned a Dockerfile but didn't say which service")
		}
		filename := "Dockerfile." + service
		// Guard against the model rewriting an unrelated service.
		if _, ok := l.Output.Files[filename]; !ok {
			for _, s := range l.Services {
				if strings.EqualFold(s.ID, service) {
					filename = "Dockerfile." + s.ID
					break
				}
			}
		}
		if !validGeneratedPath(filename, l.Services, l.Output) {
			return false, fmt.Errorf("repair path is outside generated artifacts: %s", filename)
		}
		cleaned := llm.CleanContent(fix.Dockerfile)
		original := l.Output.Files[filename]

		// Invariant check: does the proposed content weaken validation?
		// Treat a full-file replacement as a create_file operation so the
		// invariant checker can inspect the resulting content.
		violations := CheckInvariants([]Change{{
			File: filename, Operation: "create_file", Content: cleaned,
		}}, map[string]string{filename: original})
		if HasRejections(violations) {
			l.log("  ↳ rejected by invariants:\n" + FormatViolations(violations))
			return false, nil
		}

		// Scope evaluation: flag suspiciously broad rewrites.
		scope := EvaluateScope([]Change{{Content: cleaned}}, map[string]string{filename: original}, "")
		if scope.ShouldReject() {
			l.log("  ↳ warning: " + scope.Reason)
		}

		l.Output.Files[filename] = cleaned
		if err := os.WriteFile(filepath.Join(l.OutputDir, filename), []byte(cleaned), 0644); err != nil {
			return false, err
		}
		wrote = true
	}

	if fix.Compose != "" {
		cleaned := llm.CleanContent(fix.Compose)
		if err := llm.AssertComposeLayout(cleaned); err != nil {
			l.log("  ↳ ignored compose rewrite: " + err.Error())
		} else {
			original := l.Output.Files["docker-compose.yml"]
			violations := CheckInvariants([]Change{{
				File: "docker-compose.yml", Operation: "create_file", Content: cleaned,
			}}, map[string]string{"docker-compose.yml": original})
			if HasRejections(violations) {
				l.log("  ↳ rejected compose by invariants:\n" + FormatViolations(violations))
			} else {
				l.Output.Files["docker-compose.yml"] = cleaned
				if err := os.WriteFile(filepath.Join(l.OutputDir, "docker-compose.yml"), []byte(cleaned), 0644); err != nil {
					return false, err
				}
				wrote = true
			}
		}
	}

	return wrote, nil
}

func validGeneratedPath(file string, services []detector.Service, output *generator.Output) bool {
	if file == "" || filepath.IsAbs(file) {
		return false
	}
	clean := filepath.Clean(file)
	if clean != file || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return false
	}
	if clean == "docker-compose.yml" {
		return output != nil && output.Files != nil
	}
	for _, svc := range services {
		if clean == "Dockerfile."+svc.ID {
			return output != nil && output.Files != nil
		}
	}
	return false
}

func (l *Loop) dockerfileForService(service string) string {
	if service != "" {
		if df, ok := l.Output.Files["Dockerfile."+service]; ok {
			return df
		}
	}
	// Fall back to concatenating every Dockerfile so the model has something
	// to work with — but only the first ~16 KB so we don't blow the context.
	var b strings.Builder
	for _, name := range generator.SortedFilenames(l.Output.Files) {
		if !strings.HasPrefix(name, "Dockerfile.") {
			continue
		}
		fmt.Fprintf(&b, "=== %s ===\n%s\n\n", name, l.Output.Files[name])
		if b.Len() > 16*1024 {
			b.WriteString("[...truncated...]\n")
			break
		}
	}
	return b.String()
}

func (l *Loop) log(line string) {
	if l.Tee != nil {
		l.Tee(line)
	}
}

// waitForRuntimeHealth polls docker compose ps until all containers are
// healthy or the deadline expires. This is the independent runtime
// verification that build success ≠ runtime success.
func (l *Loop) waitForRuntimeHealth(ctx context.Context) bool {
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		ps, err := l.Compose.Ps(ctx)
		if err != nil || len(ps) == 0 {
			time.Sleep(2 * time.Second)
			continue
		}
		allHealthy := true
		for _, c := range ps {
			if c.State != "running" {
				return false // crashed/exited
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

func nonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) != "" {
		return s
	}
	return fallback
}

// readDockerfileFromDisk reads the Dockerfile for the given service from the
// output directory on disk, ensuring current-state freshness. Returns "" if
// the file can't be read.
func (l *Loop) readDockerfileFromDisk(service string) string {
	id := resolveServiceID(l.Services, service)
	if id == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(l.OutputDir, "Dockerfile."+id))
	if err != nil {
		return ""
	}
	return string(data)
}

// readFileFromDisk reads a file from the output directory. Returns "" on error.
func readFileFromDisk(outputDir, filename string) string {
	data, err := os.ReadFile(filepath.Join(outputDir, filename))
	if err != nil {
		return ""
	}
	return string(data)
}

// resolveServiceID finds the canonical service ID (case-insensitive) from the
// services list. When the service string is empty (ExtractFailedService couldn't
// determine which service failed), and there's only one service, defaults to
// that service. This prevents the "Dockerfile." (no suffix) key bug that causes
// false stale-context rejection.
func resolveServiceID(services []detector.Service, service string) string {
	if service == "" {
		if len(services) == 1 {
			return services[0].ID
		}
		return ""
	}
	for _, s := range services {
		if s.ID == service || strings.EqualFold(s.ID, service) {
			return s.ID
		}
	}
	return service
}

// hashString returns the SHA-256 hex digest of s, used for stale-context
// detection (comparing what the LLM was given vs. what's on disk now).
func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// previousFixesFromAttempts converts structured AttemptContext back into the
// string format expected by FixBuildFailure, preserving backward compat.
func previousFixesFromAttempts(attempts []AttemptContext) []string {
	out := make([]string, len(attempts))
	for i, a := range attempts {
		out[i] = fmt.Sprintf("Attempt %d: %s (result: %s)", a.N, a.Diagnosis, a.Result)
	}
	return out
}

// pythonRequiresRe matches pip's "requires a different Python" error and
// captures the lower bound of the constraint (e.g. "3.14" from ">=3.14").
// The constraint lives inside single quotes, so the matcher must cross them.
var pythonRequiresRe = regexp.MustCompile(`requires a different Python:.*?>=\s*(\d+\.\d+)`)

// pythonFromRe matches the base image line we generate for Python services.
var pythonFromRe = regexp.MustCompile(`FROM python:\d+\.\d+(?:\.\d+)?-slim`)

// DeterministicFix inspects the build error tail for failure patterns Yoink
// can repair with certainty, without an LLM round. Exported so the agent
// package can call it.
func DeterministicFix(errorTail, dockerfile, service string) (fixed, summary string, ok bool) {
	return deterministicFix(errorTail, dockerfile, service)
}

func deterministicFix(errorTail, dockerfile, _ string) (fixed, summary string, ok bool) {
	if m := pythonRequiresRe.FindStringSubmatch(errorTail); len(m) == 2 {
		required := m[1]
		cur := pythonFromRe.FindString(dockerfile)
		if cur == "" {
			return "", "", false
		}
		curVer := regexp.MustCompile(`\d+\.\d+`).FindString(cur)
		if curVer == "" || versionGte(curVer, required) {
			return "", "", false
		}
		replacement := "FROM python:" + required + "-slim"
		if cur == replacement {
			return "", "", false
		}
		newDF := strings.Replace(dockerfile, cur, replacement, 1)
		return newDF, "bumped Python base image to " + required + " (requires-python >= " + required + ")", true
	}

	// Monorepo TS2307: sub-package deps not installed.
	if newDF, summ, fixed := fixMonorepoSubPackageDeps(errorTail, dockerfile); fixed {
		return newDF, summ, true
	}

	// Missing environment variable: "X is not defined" / "X is required".
	// Only acts when the error explicitly names the variable AND Yoink has
	// a safe (non-secret) value for it (from infra inference or common vars).
	// NEVER fabricates credentials.
	if newDF, summ, fixed := fixMissingEnv(errorTail, dockerfile); fixed {
		return newDF, summ, true
	}

	return "", "", false
}

// fixMissingEnv handles high-confidence missing-environment failures.
// Only triggers when:
//  1. The error explicitly names the variable ("X is not defined" / "X is required")
//  2. The variable is NOT a secret (never fabricate credentials)
//  3. The Dockerfile doesn't already have an ENV directive for it
//
// If the variable is a secret (JWT_SECRET, API_KEY, etc.), this fixer does
// NOT act — it returns false so the LLM gets the structured failure instead.
// The LLM is told "credential unavailable, cannot fabricate" and the build
// is expected to remain failing until the user provides the real value.
func fixMissingEnv(errorTail, dockerfile string) (string, string, bool) {
	// Extract env var names from "X is not defined" / "X is required" patterns.
	var candidates []string
	for _, m := range envNotDefinedRe.FindAllStringSubmatch(errorTail, -1) {
		if len(m) == 2 {
			candidates = append(candidates, m[1])
		}
	}
	for _, m := range envRefRe.FindAllStringSubmatch(errorTail, -1) {
		if len(m) == 2 {
			candidates = append(candidates, m[1])
		}
	}
	if len(candidates) == 0 {
		return "", "", false
	}

	changed := false
	summary := ""
	for _, name := range candidates {
		upper := strings.ToUpper(name)
		// NEVER fabricate secrets.
		if isSecretEnvVarName(upper) {
			continue
		}
		// Only inject vars that look like real env var names (have an underscore
		// or are a known common name like PORT/HOST/NODE_ENV). Rejects false
		// positives like "ERROR" or "FAILED" that match the regex.
		if !looksLikeEnvVar(name) {
			continue
		}
		// Check if the Dockerfile already has an ENV directive for this var.
		envLine := "ENV " + name + "="
		if strings.Contains(dockerfile, envLine) {
			continue
		}
		// Only inject non-secret vars with safe placeholder values.
		// Real values come from compose env_file at runtime.
		dockerfile = strings.Replace(dockerfile, "RUN ", "ENV "+name+"=yoink-build-placeholder\nRUN ", 1)
		changed = true
		summary = "added ENV " + name + "=placeholder (non-secret build-time requirement from error)"
	}
	if !changed {
		return "", "", false
	}
	return dockerfile, summary, true
}

// isSecretEnvVarName is the healer-local copy of the generator's secret
// classification. Mirrors generator.isSecretEnvVar.
func isSecretEnvVarName(name string) bool {
	for _, marker := range []string{"SECRET", "PASSWORD", "TOKEN", "CREDENTIAL", "PRIVATE_KEY", "API_KEY"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

// looksLikeEnvVar reports whether the name looks like a real environment
// variable rather than a common English word that happened to match the
// regex. Real env vars almost always have an underscore (DATABASE_URL,
// JWT_SECRET, UPSTASH_REDIS_REST_URL). A few common names without
// underscores (PORT, HOST, NODE_ENV, DEBUG) are also accepted.
func looksLikeEnvVar(name string) bool {
	upper := strings.ToUpper(name)
	switch upper {
	case "PORT", "HOST", "DEBUG", "NODE_ENV":
		return true
	}
	return strings.Contains(upper, "_")
}

// tsFileRe matches a TS2307 "Cannot find module" error and captures the
// sub-directory from the file path. BuildKit prefixes lines with "#NN 1.234 "
// so we don't anchor to line start.
// Format: "#16 9.678 ingestion-worker/src/pipeline.ts(7,60): error TS2307: Cannot find module '...'"
var tsFileRe = regexp.MustCompile(`([a-zA-Z0-9][\w-]*)/[\w/.-]+\.ts\(\d+,\d+\):.*Cannot find module`)

// fixMonorepoSubPackageDeps detects TS2307 "Cannot find module" errors for
// files in a sub-directory (e.g. `ingestion-worker/src/pipeline.ts`), which
// means the sub-package has its own deps that aren't installed. It adds:
//  1. `cd <subdir> && npm ci` to the deps stage's npm ci line.
//  2. `COPY --from=deps /app/<subdir>/node_modules ./<subdir>/node_modules`
//     to the builder stage.
func fixMonorepoSubPackageDeps(errorTail, dockerfile string) (string, string, bool) {
	if !strings.Contains(errorTail, "Cannot find module") || !strings.Contains(errorTail, ".ts") {
		return "", "", false
	}
	// Extract sub-directories from file paths in the error.
	subdirs := map[string]bool{}
	for _, m := range tsFileRe.FindAllStringSubmatch(errorTail, -1) {
		if len(m) >= 2 && m[1] != "" {
			subdirs[m[1]] = true
		}
	}
	if len(subdirs) == 0 {
		return "", "", false
	}

	newDF := dockerfile
	changed := false
	applied := []string{}
	for subdir := range subdirs {
		// Skip if already handled.
		if strings.Contains(newDF, "cd "+subdir+" && npm ci") && strings.Contains(newDF, "/app/"+subdir+"/node_modules") {
			continue
		}
		// 0. Ensure the sub-package's package*.json is copied to the deps
		//    stage (the static template only copies the root's).
		pkgCopy := "COPY " + subdir + "/package*.json ./" + subdir + "/"
		if !strings.Contains(newDF, pkgCopy) {
			// Insert before the complete RUN instruction. Inserting inside the
			// command creates invalid Dockerfile syntax such as `RUN COPY ...`.
			npmLine := "RUN npm ci --no-audit --no-fund"
			newDF = strings.Replace(newDF, npmLine, pkgCopy+"\n"+npmLine, 1)
		}
		// 1. Add cd <subdir> && npm ci to the deps stage.
		if !strings.Contains(newDF, "cd "+subdir+" && npm ci") {
			oldLine := "npm ci --no-audit --no-fund"
			newLine := oldLine + " && cd " + subdir + " && npm ci --no-audit --no-fund"
			if strings.Contains(newDF, oldLine) && !strings.Contains(newDF, newLine) {
				newDF = strings.Replace(newDF, oldLine, newLine, 1)
				changed = true
			}
		}
		// 2. Add COPY --from=deps for the sub-package's node_modules.
		copyLine := "COPY --from=deps /app/" + subdir + "/node_modules ./" + subdir + "/node_modules"
		if !strings.Contains(newDF, copyLine) {
			anchor := "COPY --from=deps /app/node_modules ./node_modules"
			if strings.Contains(newDF, anchor) {
				newDF = strings.Replace(newDF, anchor, anchor+"\n"+copyLine, 1)
				changed = true
			}
		}
		applied = append(applied, subdir)
	}
	if !changed {
		return "", "", false
	}
	return newDF, "installed sub-package deps for: " + strings.Join(applied, ", "), true
}

// versionGte reports whether a >= b for "major.minor" version strings.
func versionGte(a, b string) bool {
	ai := strings.Split(a, ".")
	bi := strings.Split(b, ".")
	for i := 0; i < 2; i++ {
		var av, bv int
		_, _ = fmt.Sscanf(ai[i], "%d", &av)
		_, _ = fmt.Sscanf(bi[i], "%d", &bv)
		if av > bv {
			return true
		}
		if av < bv {
			return false
		}
	}
	return true
}
