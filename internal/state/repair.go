// Package state repair.go — repair provenance for generated artifacts.
// Tracks which generated files have been modified by the healer so that
// `yoink update` can detect conflicts and preserve healed changes rather
// than silently overwriting them.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// RepairRecord captures a single healer modification to a generated artifact.
type RepairRecord struct {
	ID            string    `json:"id"` // unique repair ID
	Timestamp     time.Time `json:"timestamp"`
	Service       string    `json:"service,omitempty"`
	File          string    `json:"file"`           // e.g. "Dockerfile.service-1"
	OriginalHash  string    `json:"original_hash"`  // hash before repair
	ResultingHash string    `json:"resulting_hash"` // hash after repair
	Diagnosis     string    `json:"diagnosis,omitempty"`
	Summary       string    `json:"summary,omitempty"`
	FailureCat    string    `json:"failure_category,omitempty"`
	Operation     string    `json:"operation"` // "deterministic" | "llm-patch" | "llm-replace"
}

// RepairHistory is the provenance log for all healer modifications to a
// project's generated artifacts. Persisted as repairs.json alongside
// yoink.lock in the project state directory.
type RepairHistory struct {
	Repairs []RepairRecord `json:"repairs"`
}

// ArtifactState describes the current state of a generated artifact relative
// to what the generator would produce.
type ArtifactState string

const (
	ArtifactClean    ArtifactState = "clean"    // matches generator output
	ArtifactHealed   ArtifactState = "healed"   // modified by healer, recorded in provenance
	ArtifactDiverged ArtifactState = "diverged" // modified, NOT in provenance (user edit or unknown)
)

// LoadRepairHistory reads repairs.json. Returns an empty history if absent.
func (m *Manager) LoadRepairHistory() (*RepairHistory, error) {
	path := filepath.Join(m.Dir, "repairs.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &RepairHistory{}, nil
		}
		return nil, err
	}
	var rh RepairHistory
	if err := json.Unmarshal(data, &rh); err != nil {
		return nil, fmt.Errorf("repairs.json: %w", err)
	}
	return &rh, nil
}

// SaveRepairHistory writes repairs.json atomically.
func (m *Manager) SaveRepairHistory(rh *RepairHistory) error {
	return writeJSON(filepath.Join(m.Dir, "repairs.json"), rh)
}

// RecordRepair appends a repair record and persists.
func (m *Manager) RecordRepair(rec RepairRecord) error {
	rh, err := m.LoadRepairHistory()
	if err != nil {
		return err
	}
	if rec.ID == "" {
		rec.ID = fmt.Sprintf("repair-%d", len(rh.Repairs)+1)
	}
	rh.Repairs = append(rh.Repairs, rec)
	return m.SaveRepairHistory(rh)
}

// IsHealedFile reports whether the given file has been modified by the healer.
func (m *Manager) IsHealedFile(filename string) bool {
	rh, _ := m.LoadRepairHistory()
	for _, r := range rh.Repairs {
		if r.File == filename {
			return true
		}
	}
	return false
}

// HealedFiles returns the set of filenames modified by the healer.
func (m *Manager) HealedFiles() []string {
	rh, _ := m.LoadRepairHistory()
	seen := map[string]bool{}
	var out []string
	for _, r := range rh.Repairs {
		if !seen[r.File] {
			seen[r.File] = true
			out = append(out, r.File)
		}
	}
	sort.Strings(out)
	return out
}

// ClassifyArtifact determines whether a generated artifact is clean, healed,
// or diverged by comparing its current hash against the generator's output
// hash and the repair provenance.
func (m *Manager) ClassifyArtifact(filename, generatorOutput string) ArtifactState {
	// Read current file from disk.
	current, err := os.ReadFile(filepath.Join(m.Dir, filename))
	if err != nil {
		return ArtifactDiverged // can't read — treat as diverged
	}
	currentHash := hashContent(string(current))
	genHash := hashContent(generatorOutput)

	// If current matches generator output → clean.
	if currentHash == genHash {
		return ArtifactClean
	}

	// If the file is in repair provenance → healed.
	rh, _ := m.LoadRepairHistory()
	for _, r := range rh.Repairs {
		if r.File == filename && r.ResultingHash == currentHash {
			return ArtifactHealed
		}
	}

	// Modified but not in provenance → diverged.
	return ArtifactDiverged
}

// hashContent returns a SHA-256 hex digest of the content. Used for
// artifact provenance comparison.
func hashContent(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
