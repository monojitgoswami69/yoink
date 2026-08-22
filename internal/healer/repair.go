// Package healer repair.go — patch-based repair model, patch validator,
// patch applier, and repair invariants. This is the control-model change:
// the LLM proposes minimal structured patches, Yoink validates and applies
// them. The LLM never directly overwrites files.
package healer

import (
	"fmt"
	"strings"
)

// RepairPlan is the structured repair proposal the LLM returns instead of a
// full file replacement. It contains a diagnosis, a list of minimal changes
// (patches), and a validation plan. Yoink validates the plan before applying
// anything.
type RepairPlan struct {
	Diagnosis  Diagnosis `json:"diagnosis"`
	Changes    []Change  `json:"changes"`
	Validation []string  `json:"validation,omitempty"`
	// Summary is a 1-line human-readable explanation.
	Summary string `json:"summary,omitempty"`
	// Unfixable is true when the model determines the failure is outside its
	// scope (e.g. Docker Hub is down). When true, Changes should be empty.
	Unfixable bool `json:"unfixable,omitempty"`
	// NeedsFiles requests the caller to read repo files and resubmit.
	NeedsFiles []string `json:"needs_files,omitempty"`
}

// Diagnosis is the structured root-cause analysis the model must provide
// before proposing any change.
type Diagnosis struct {
	Category      string   `json:"category"`                  // dependency | compilation | configuration | environment | runtime | unknown
	RootCause     string   `json:"root_cause"`                // the actual cause, not the symptom
	Confidence    float64  `json:"confidence"`                // 0.0–1.0
	Evidence      []string `json:"evidence"`                  // references to context items that support the diagnosis
	SourceOfTruth string   `json:"source_of_truth,omitempty"` // repository | generator | detector | infrastructure
	Risk          string   `json:"risk,omitempty"`            // low | medium | high
}

// Change is one minimal patch to one file. The model proposes the smallest
// change necessary; Yoink validates the anchor, hash, and invariants before
// applying.
type Change struct {
	File      string `json:"file"`             // e.g. "Dockerfile.service-2"
	Operation string `json:"operation"`        // insert_after | insert_before | replace_line | replace_exact | create_file
	Anchor    string `json:"anchor,omitempty"` // the line or string to match (for insert/replace ops)
	Content   string `json:"content"`          // the new content to insert/replace with
	Reason    string `json:"reason,omitempty"` // why this change is needed
}

// ContextSnapshot captures the state of all relevant files at the time the
// LLM was invoked. It's immutable for that request. Any filesystem mutation
// invalidates the snapshot, and stale patches are rejected.
type ContextSnapshot struct {
	Files map[string]string // path → SHA-256 hex digest
}

// NewSnapshot computes hashes for the given file contents.
func NewSnapshot(files map[string]string) *ContextSnapshot {
	s := &ContextSnapshot{Files: make(map[string]string, len(files))}
	for path, content := range files {
		s.Files[path] = hashString(content)
	}
	return s
}

// IsStale reports whether any file in the snapshot has changed on disk.
// currentFiles is a map of path → current content (freshly read).
func (s *ContextSnapshot) IsStale(currentFiles map[string]string) (string, bool) {
	for path, expectedHash := range s.Files {
		current, ok := currentFiles[path]
		if !ok {
			return path, true // file was deleted
		}
		if hashString(current) != expectedHash {
			return path, true // content changed
		}
	}
	return "", false
}

// --- Patch operations ---

// ApplyPatch applies a single change to the file content and returns the
// modified content. It validates that the anchor exists and is unambiguous.
func ApplyPatch(content string, change Change) (string, error) {
	switch change.Operation {
	case "create_file":
		return change.Content, nil

	case "insert_after":
		return applyInsertAfter(content, change.Anchor, change.Content)

	case "insert_before":
		return applyInsertBefore(content, change.Anchor, change.Content)

	case "replace_line":
		return applyReplaceLine(content, change.Anchor, change.Content)

	case "replace_exact":
		return applyReplaceExact(content, change.Anchor, change.Content)

	default:
		return "", fmt.Errorf("unsupported operation: %s", change.Operation)
	}
}

func applyInsertAfter(content, anchor, newContent string) (string, error) {
	lines := strings.Split(content, "\n")
	matches := 0
	insertIdx := -1
	for i, line := range lines {
		if strings.Contains(line, anchor) {
			matches++
			insertIdx = i
		}
	}
	if matches == 0 {
		return "", fmt.Errorf("anchor not found: %s", anchor)
	}
	if matches > 1 {
		return "", fmt.Errorf("anchor matches %d locations (must be unambiguous): %s", matches, anchor)
	}
	// Insert newContent after the matched line.
	newLines := strings.Split(newContent, "\n")
	result := make([]string, 0, len(lines)+len(newLines))
	result = append(result, lines[:insertIdx+1]...)
	result = append(result, newLines...)
	result = append(result, lines[insertIdx+1:]...)
	return strings.Join(result, "\n"), nil
}

func applyInsertBefore(content, anchor, newContent string) (string, error) {
	lines := strings.Split(content, "\n")
	matches := 0
	insertIdx := -1
	for i, line := range lines {
		if strings.Contains(line, anchor) {
			matches++
			insertIdx = i
		}
	}
	if matches == 0 {
		return "", fmt.Errorf("anchor not found: %s", anchor)
	}
	if matches > 1 {
		return "", fmt.Errorf("anchor matches %d locations: %s", matches, anchor)
	}
	newLines := strings.Split(newContent, "\n")
	result := make([]string, 0, len(lines)+len(newLines))
	result = append(result, lines[:insertIdx]...)
	result = append(result, newLines...)
	result = append(result, lines[insertIdx:]...)
	return strings.Join(result, "\n"), nil
}

func applyReplaceLine(content, anchor, newContent string) (string, error) {
	lines := strings.Split(content, "\n")
	matches := 0
	replaceIdx := -1
	for i, line := range lines {
		if strings.Contains(line, anchor) {
			matches++
			replaceIdx = i
		}
	}
	if matches == 0 {
		return "", fmt.Errorf("anchor not found: %s", anchor)
	}
	if matches > 1 {
		return "", fmt.Errorf("anchor matches %d locations: %s", matches, anchor)
	}
	lines[replaceIdx] = newContent
	return strings.Join(lines, "\n"), nil
}

func applyReplaceExact(content, anchor, newContent string) (string, error) {
	count := strings.Count(content, anchor)
	if count == 0 {
		return "", fmt.Errorf("exact anchor not found: %s", anchor)
	}
	if count > 1 {
		return "", fmt.Errorf("exact anchor matches %d locations: %s", count, anchor)
	}
	return strings.Replace(content, anchor, newContent, 1), nil
}

// --- Repair invariants ---

// InvariantViolation describes a repair that weakens correctness guarantees.
type InvariantViolation struct {
	Change    Change
	Violation string
	Severity  string // "reject" | "warn"
}

// CheckInvariants inspects the proposed changes and the resulting file
// content for forbidden patterns. A repair that hides a failure is NOT
// equivalent to a repair that fixes the cause.
func CheckInvariants(changes []Change, originalContent map[string]string) []InvariantViolation {
	var violations []InvariantViolation

	for _, change := range changes {
		// Determine the content after the change (for content-level checks).
		var after string
		if before, ok := originalContent[change.File]; ok {
			after, _ = ApplyPatch(before, change)
		} else {
			after = change.Content // new file
		}
		lowContent := strings.ToLower(change.Content)

		// 1. Disabling TypeScript checking / type validation.
		if strings.Contains(lowContent, "ignorebuilderrors") ||
			strings.Contains(lowContent, "ignore_build_errors") ||
			strings.Contains(lowContent, "--no-type-check") ||
			strings.Contains(lowContent, "typescript.ignorebuilderrors") {
			violations = append(violations, InvariantViolation{
				Change: change, Violation: "disables type checking — this hides the symptom, not the root cause", Severity: "reject",
			})
		}

		// 2. Removing healthcheck from compose.
		if strings.Contains(change.File, "docker-compose") {
			if strings.Contains(strings.ToLower(originalContent[change.File]), "healthcheck") {
				beforeLines := strings.Split(originalContent[change.File], "\n")
				afterLines := strings.Split(after, "\n")
				beforeHC := countLinesContaining(beforeLines, "healthcheck")
				afterHC := countLinesContaining(afterLines, "healthcheck")
				if afterHC < beforeHC {
					violations = append(violations, InvariantViolation{
						Change: change, Violation: "removes a healthcheck — a failing healthcheck must be diagnosed, not removed", Severity: "reject",
					})
				}
			}
		}

		// 3. Replacing CMD with sleep/echo (hiding a crash).
		if strings.Contains(lowContent, "cmd") && (strings.Contains(lowContent, "sleep") || strings.Contains(lowContent, "echo no start")) {
			if !strings.Contains(strings.ToLower(originalContent[change.File]), "sleep") {
				violations = append(violations, InvariantViolation{
					Change: change, Violation: "replaces the start command with sleep — this hides a startup failure", Severity: "reject",
				})
			}
		}

		// 4. Removing dependency installation.
		if change.Operation == "replace_line" || change.Operation == "replace_exact" {
			if strings.Contains(strings.ToLower(change.Anchor), "npm install") ||
				strings.Contains(strings.ToLower(change.Anchor), "npm ci") ||
				strings.Contains(strings.ToLower(change.Anchor), "pip install") ||
				strings.Contains(strings.ToLower(change.Anchor), "poetry install") {
				if !strings.Contains(lowContent, "install") {
					violations = append(violations, InvariantViolation{
						Change: change, Violation: "removes dependency installation — this hides dependency errors", Severity: "reject",
					})
				}
			}
		}

		// 5. Modifying user source files (warn only).
		if !strings.HasPrefix(change.File, "Dockerfile.") &&
			!strings.HasPrefix(change.File, "docker-compose") &&
			change.Operation != "create_file" {
			violations = append(violations, InvariantViolation{
				Change: change, Violation: "modifies a non-generated file (" + change.File + ") — source files should only be changed with strong evidence", Severity: "warn",
			})
		}
	}

	return violations
}

// HasRejections returns true if any violation has severity "reject".
func HasRejections(violations []InvariantViolation) bool {
	for _, v := range violations {
		if v.Severity == "reject" {
			return true
		}
	}
	return false
}

// FormatViolations renders violations as a human-readable string.
func FormatViolations(violations []InvariantViolation) string {
	if len(violations) == 0 {
		return ""
	}
	var b strings.Builder
	for _, v := range violations {
		label := "⚠"
		if v.Severity == "reject" {
			label = "×"
		}
		fmt.Fprintf(&b, "  %s %s: %s\n", label, v.Change.File, v.Violation)
	}
	return b.String()
}

// --- Patch scope evaluation ---

// Scope classes for enforcement.
const (
	ScopeSafe      = "safe"
	ScopeBroad     = "broad"
	ScopeHighRisk  = "high_risk"
	ScopeForbidden = "forbidden"
)

// ScopeAssessment classifies the breadth and risk of a proposed repair.
type ScopeAssessment struct {
	Class        string // ScopeSafe | ScopeBroad | ScopeHighRisk | ScopeForbidden
	Reason       string
	LinesChanged int
}

// EvaluateScope assesses whether the proposed changes are suspiciously broad
// or high-risk. The class determines whether the repair is applied, rejected,
// or requires stronger evidence.
func EvaluateScope(changes []Change, originalContent map[string]string, failureCategory string) ScopeAssessment {
	totalLines := 0
	for _, change := range changes {
		totalLines += len(strings.Split(change.Content, "\n"))
	}
	// Check for forbidden patterns first (handled by CheckInvariants, but
	// also flagged here for observability).
	for _, c := range changes {
		low := strings.ToLower(c.Content)
		if strings.Contains(low, "sleep") && strings.Contains(low, "cmd") {
			return ScopeAssessment{Class: ScopeForbidden, Reason: "replaces CMD with sleep — hides startup failure", LinesChanged: totalLines}
		}
	}
	// Broad: >50 lines in a single repair.
	if totalLines > 50 {
		return ScopeAssessment{Class: ScopeBroad, Reason: fmt.Sprintf("%d lines changed — suspiciously broad", totalLines), LinesChanged: totalLines}
	}
	return ScopeAssessment{Class: ScopeSafe, Reason: "", LinesChanged: totalLines}
}

// ShouldReject reports whether the scope class requires rejection.
func (s ScopeAssessment) ShouldReject() bool {
	return s.Class == ScopeBroad || s.Class == ScopeForbidden
}

// --- Helpers ---

// (hashString is defined in healer.go — shared across the package.)

func countLinesContaining(lines []string, substr string) int {
	count := 0
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), strings.ToLower(substr)) {
			count++
		}
	}
	return count
}
