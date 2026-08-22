// Package state stores per-repository persistent data under
// ~/.yoink/state/<repo>/. Three files live there:
//
//   - yoink.lock         — repo metadata, detection hash, and the canonical
//     port map so `yoink up` and `yoink dash` can act without re-detecting.
//   - env-overrides.json — user-edited env values, applied at `yoink up`
//     time on top of the generated .env.example.
//   - settings.json      — preferences (last flags, autostart toggles).
//
// All files are JSON. The lock file is the authoritative record of what
// Yoink generated; the others are user-mutable.
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

	"yoink/internal/config"
	"yoink/internal/detector"
)

// Manager is bound to a single repository's state directory.
type Manager struct {
	Dir  string // absolute path to ~/.yoink/state/<repo>/
	Repo string
}

// For returns a Manager for the named repo. The state directory is created
// on disk (0700) so callers can write immediately.
// For returns a Manager for the named repo. The name is canonicalised so
// "Sevatra" and "sevatra" share one state directory. The directory is
// created on disk (0700) so callers can write immediately.
func For(repo string) (*Manager, error) {
	home, err := config.YoinkHome()
	if err != nil {
		return nil, err
	}
	id := Canonicalize(repo)
	if id == "" {
		return nil, fmt.Errorf("project name cannot be empty")
	}
	dir := filepath.Join(home, "state", id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create state dir: %w", err)
	}
	return &Manager{Dir: dir, Repo: id}, nil
}

// Lock captures everything `yoink up` / `yoink dash` need to know about a
// previous init. When detection diverges from the recorded Hash the user
// is prompted to re-init.
type Lock struct {
	// Project is the user-facing project name (defaults to Repo when init
	// was run without --name). It is the key under ~/.yoink/state/ and the
	// identifier every project-centric command operates on.
	Project      string             `json:"project,omitempty"`
	Repo         string             `json:"repo"`
	RepoURL      string             `json:"repo_url"`
	RepoPath     string             `json:"repo_path"`     // local clone root
	OutputSubdir string             `json:"output_subdir"` // typically "yoink-outputs"
	Services     []detector.Service `json:"services"`
	Infra        []string           `json:"infra,omitempty"`
	// InfraDetails is the compact projection of inferred infra services
	// (name/kind/mode/provider/port/reason) so `yoink explain` and the
	// ServiceGraph can render infrastructure without re-running detection.
	InfraDetails []InfraDetail `json:"infra_details,omitempty"`
	// Links is the compact app->infra dependency graph (keyed by app service
	// ID) persisted for `yoink explain` and the ServiceGraph rebuild.
	Links map[string][]LinkRef `json:"links,omitempty"`
	// PortMap is the canonical host-port assignment for the app services
	// (keyed by service ID). Infra ports are not pinned here.
	PortMap  map[string]int `json:"port_map,omitempty"`
	Hash     string         `json:"detection_hash"`
	LastInit time.Time      `json:"last_init"`
	LastUp   time.Time      `json:"last_up,omitempty"`
	Version  string         `json:"yoink_version"`
}

// Overrides is the env-overrides.json shape: a per-service map of variable
// names to user-specified values.
type Overrides map[string]map[string]string

// Settings holds preferences that survive across runs. Fields left empty fall
// back to the global config (~/.yoink/config.json), so a project only
// overrides what it explicitly sets.
type Settings struct {
	LastFlags map[string]string `json:"last_flags,omitempty"`
	Autostart bool              `json:"autostart"`
	// HealTries overrides the heal-loop attempt cap for this project.
	HealTries int `json:"heal_tries,omitempty"`
	// AutoHeal runs the heal loop automatically after a failed build.
	AutoHeal bool `json:"auto_heal,omitempty"`
	// DefaultService is the service `open`/`status` highlight by default.
	DefaultService string `json:"default_service,omitempty"`
	// LLMProvider/LLMModel let a project pin a different model than global.
	LLMProvider string `json:"llm_provider,omitempty"`
	LLMModel    string `json:"llm_model,omitempty"`
}

// SaveLock writes yoink.lock atomically (write to .tmp, rename).
func (m *Manager) SaveLock(l *Lock) error {
	return writeJSON(filepath.Join(m.Dir, "yoink.lock"), l)
}

// LoadLock reads yoink.lock. Returns (nil, nil) when the file is absent.
func (m *Manager) LoadLock() (*Lock, error) {
	var l Lock
	ok, err := readJSON(filepath.Join(m.Dir, "yoink.lock"), &l)
	if err != nil || !ok {
		return nil, err
	}
	return &l, nil
}

// SaveOverrides writes env-overrides.json atomically.
func (m *Manager) SaveOverrides(o Overrides) error {
	return writeJSON(filepath.Join(m.Dir, "env-overrides.json"), o)
}

// LoadOverrides reads env-overrides.json; returns an empty map when absent.
func (m *Manager) LoadOverrides() (Overrides, error) {
	out := Overrides{}
	_, err := readJSON(filepath.Join(m.Dir, "env-overrides.json"), &out)
	return out, err
}

// SaveSettings writes settings.json atomically.
func (m *Manager) SaveSettings(s *Settings) error {
	return writeJSON(filepath.Join(m.Dir, "settings.json"), s)
}

// LoadSettings reads settings.json; returns a zero value when absent.
func (m *Manager) LoadSettings() (*Settings, error) {
	s := &Settings{}
	if _, err := readJSON(filepath.Join(m.Dir, "settings.json"), s); err != nil {
		return nil, err
	}
	return s, nil
}

// SetOverride writes a single per-service override and persists.
func (m *Manager) SetOverride(serviceID, key, value string) error {
	o, err := m.LoadOverrides()
	if err != nil {
		return err
	}
	if o[serviceID] == nil {
		o[serviceID] = map[string]string{}
	}
	o[serviceID][key] = value
	return m.SaveOverrides(o)
}

// HashDetection returns a stable hex digest of the detection result so we
// can tell when an upstream change invalidates the lock file. Only the
// fields that affect generation are hashed.
func HashDetection(services []detector.Service) string {
	type stable struct {
		ID, Type, Directory, Language, Framework, PM string
		Port                                         int
		HasLockfile                                  bool
		PythonManifest                               string
		PythonDeps                                   []string
		BuildCmd                                     []string
		StartCmd                                     []string
		InstallCmd                                   []string
	}
	snap := make([]stable, len(services))
	for i, s := range services {
		snap[i] = stable{
			s.ID, s.Type, s.Directory, s.Language, s.Framework, s.PackageManager,
			s.Port, s.HasLockfile, s.PythonManifest, s.PythonDeps,
			s.BuildCmd, s.StartCmd, s.InstallCmd,
		}
	}
	sort.Slice(snap, func(i, j int) bool { return snap[i].ID < snap[j].ID })
	data, _ := json.Marshal(snap)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// MergedEnv merges the generated .env.example content (parsed as KEY=VALUE
// lines) with the user's per-service overrides. Override values win;
// comment lines from the original are preserved verbatim. The returned
// string is suitable to write as a .env file before `docker compose up`.
func MergedEnv(envExample string, overrides map[string]string) string {
	if len(overrides) == 0 {
		return envExample
	}
	keys := make(map[string]bool, len(overrides))
	var b []byte
	for _, line := range splitLines(envExample) {
		stripped := trim(line)
		if stripped == "" || stripped[0] == '#' {
			b = append(b, line...)
			b = append(b, '\n')
			continue
		}
		key, _, ok := cut(stripped, "=")
		if !ok {
			b = append(b, line...)
			b = append(b, '\n')
			continue
		}
		if v, found := overrides[trim(key)]; found {
			b = append(b, []byte(trim(key)+"="+v)...)
			b = append(b, '\n')
			keys[trim(key)] = true
			continue
		}
		b = append(b, line...)
		b = append(b, '\n')
	}
	// Add overrides that didn't exist in the template, in sorted order.
	var extras []string
	for k := range overrides {
		if !keys[k] {
			extras = append(extras, k)
		}
	}
	sort.Strings(extras)
	if len(extras) > 0 {
		b = append(b, '\n')
		b = append(b, []byte("# User overrides (yoink dash)\n")...)
		for _, k := range extras {
			b = append(b, []byte(k+"="+overrides[k]+"\n")...)
		}
	}
	return string(b)
}

// AllManagers lists every repo that has state on disk, newest first by
// last-init time. Useful for the dashboard's "pick a recent project" UX.
func AllManagers() ([]*Manager, error) {
	home, err := config.YoinkHome()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(home, "state")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*Manager
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		out = append(out, &Manager{Dir: filepath.Join(root, e.Name()), Repo: e.Name()})
	}
	sort.Slice(out, func(i, j int) bool {
		ai, _ := os.Stat(filepath.Join(out[i].Dir, "yoink.lock"))
		aj, _ := os.Stat(filepath.Join(out[j].Dir, "yoink.lock"))
		if ai == nil || aj == nil {
			return out[i].Repo < out[j].Repo
		}
		return ai.ModTime().After(aj.ModTime())
	})
	return out, nil
}

// MostRecent returns the manager for the most recently initialised repo, or
// (nil, nil) when no state exists.
func MostRecent() (*Manager, error) {
	all, err := AllManagers()
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, nil
	}
	return all[0], nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSON(path string, v any) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return true, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return true, nil
}

// Tiny string helpers used by MergedEnv so the package has no internal
// dependencies beyond stdlib.
func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func cut(s, sep string) (string, string, bool) {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return s[:i], s[i+len(sep):], true
		}
	}
	return s, "", false
}
