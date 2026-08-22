package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yoink/internal/config"
	"yoink/internal/detector"
	"yoink/internal/state"
)

// makeLegacyProject writes a lock into a NON-canonical state directory (e.g.
// "Sevatra"), simulating state created by an older Yoink before canonical
// naming. It bypasses state.For (which would canonicalise the dir).
func makeLegacyProject(t *testing.T, dirName, display string) {
	t.Helper()
	home, err := config.YoinkHome()
	if err != nil {
		t.Fatalf("YoinkHome: %v", err)
	}
	dir := filepath.Join(home, "state", dirName)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mgr := &state.Manager{Dir: dir, Repo: dirName}
	lock := &state.Lock{
		Project:      display,
		Repo:         dirName,
		RepoURL:      "https://github.com/u/" + display + ".git",
		RepoPath:     filepath.Join(t.TempDir(), "repo"),
		OutputSubdir: "yoink-outputs",
		Services:     []detector.Service{{ID: "service-1", Framework: "next", Port: 3000}},
		PortMap:      map[string]int{"service-1": 3000},
	}
	if err := mgr.SaveLock(lock); err != nil {
		t.Fatalf("SaveLock: %v", err)
	}
}

// resolveOK resolves name and fails the test on error.
func resolveOK(t *testing.T, name string) *Project {
	t.Helper()
	p, err := Resolve(name)
	if err != nil {
		t.Fatalf("Resolve(%q): %v", name, err)
	}
	return p
}

// TestResolveCanonicalMixedCase: init --name Sevatra, then every casing resolves.
func TestResolveCanonicalMixedCase(t *testing.T) {
	defer withTempHome(t)()
	makeProject(t, "Sevatra", "https://github.com/u/Sevatra.git")

	for _, query := range []string{"sevatra", "Sevatra", "SEVATRA", "sevAtRa"} {
		p := resolveOK(t, query)
		if p.Name != "Sevatra" {
			t.Errorf("Resolve(%q).Name: want display %q, got %q", query, "Sevatra", p.Name)
		}
	}
}

// TestResolveCanonicalSpacesAndHyphens: "My Project" -> "my-project".
func TestResolveCanonicalSpacesAndHyphens(t *testing.T) {
	defer withTempHome(t)()
	makeProject(t, "My Project", "https://github.com/u/mp.git")

	for _, query := range []string{"my-project", "My Project", "my_project", "MY PROJECT"} {
		p := resolveOK(t, query)
		if p.Name != "My Project" {
			t.Errorf("Resolve(%q).Name: want %q, got %q", query, "My Project", p.Name)
		}
	}
}

// TestResolveLegacyNonCanonicalDir: state created as "Sevatra" (old behaviour)
// still resolves via canonical matching, without migration.
func TestResolveLegacyNonCanonicalDir(t *testing.T) {
	defer withTempHome(t)()
	makeLegacyProject(t, "Sevatra", "Sevatra")

	p := resolveOK(t, "sevatra")
	if p.Name != "Sevatra" {
		t.Errorf("display name not preserved: want Sevatra, got %s", p.Name)
	}
	// Canonical id used for the compose project name must be stable.
	if got := state.Canonicalize(p.Name); got != "sevatra" {
		t.Errorf("canonical id: want sevatra, got %s", got)
	}
}

// TestResolveCanonicalUnknownStillSuggests: unknown name lists available
// projects (by display/canonical name) and suggests `yoink list`.
func TestResolveCanonicalUnknownStillSuggests(t *testing.T) {
	defer withTempHome(t)()
	makeProject(t, "Sevatra", "https://github.com/u/Sevatra.git")
	makeProject(t, "My Project", "https://github.com/u/mp.git")

	_, err := Resolve("nope")
	if err == nil {
		t.Fatal("expected UnknownProjectError")
	}
	msg := err.Error()
	if !strings.Contains(msg, "nope") || !strings.Contains(msg, "yoink list") {
		t.Errorf("error should name the project and suggest `yoink list`: %s", msg)
	}
}

// TestCanonicalIsStableAcrossCalls: re-resolving never creates duplicate state.
func TestCanonicalIsStableAcrossCalls(t *testing.T) {
	defer withTempHome(t)()
	makeProject(t, "Sevatra", "https://github.com/u/Sevatra.git")

	_ = resolveOK(t, "sevatra")
	_ = resolveOK(t, "Sevatra")
	_ = resolveOK(t, "SEVATRA")

	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Errorf("canonical resolution must not duplicate state: got %d projects", len(all))
	}
}
