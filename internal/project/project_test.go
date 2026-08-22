package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"yoink/internal/detector"
	"yoink/internal/state"
)

// withTempHome isolates state to a temp HOME so tests don't touch the real
// ~/.yoink. Mirrors internal/state/state_test.go.
func withTempHome(t *testing.T) func() {
	t.Helper()
	tmp := t.TempDir()
	prev := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	return func() { os.Setenv("HOME", prev) }
}

// makeProject writes a lock under <name> so Resolve/All can find it.
func makeProject(t *testing.T, name, repoURL string) {
	t.Helper()
	m, err := state.For(name)
	if err != nil {
		t.Fatalf("state.For: %v", err)
	}
	lock := &state.Lock{
		Project:      name,
		Repo:         name,
		RepoURL:      repoURL,
		RepoPath:     filepath.Join(t.TempDir(), "repo"),
		OutputSubdir: "yoink-outputs",
		Services: []detector.Service{
			{ID: "service-1", Framework: "fastapi", Port: 8000},
			{ID: "service-2", Framework: "next", Port: 3000},
		},
		PortMap:  map[string]int{"service-1": 8000, "service-2": 3000},
		LastInit: time.Now().UTC(),
	}
	if err := m.SaveLock(lock); err != nil {
		t.Fatalf("SaveLock: %v", err)
	}
}

func TestResolveKnownProject(t *testing.T) {
	defer withTempHome(t)()
	makeProject(t, "my-app", "https://github.com/user/my-app.git")

	p, err := Resolve("my-app")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.Name != "my-app" {
		t.Errorf("Name: want my-app, got %s", p.Name)
	}
	if p.Lock.RepoURL != "https://github.com/user/my-app.git" {
		t.Errorf("RepoURL: %s", p.Lock.RepoURL)
	}
	if p.Compose == nil {
		t.Error("Compose handle must be wired")
	}
}

func TestResolveFallsBackToRepoWhenNoProjectField(t *testing.T) {
	defer withTempHome(t)()
	// A lock with no Project field (pre-existing state from older Yoink).
	m, _ := state.For("legacy-app")
	if err := m.SaveLock(&state.Lock{
		Repo: "legacy-app", RepoURL: "https://github.com/u/legacy.git",
		RepoPath: "/tmp/x", OutputSubdir: "yoink-outputs",
		Services: []detector.Service{{ID: "service-1", Framework: "node", Port: 3000}},
	}); err != nil {
		t.Fatalf("SaveLock: %v", err)
	}
	p, err := Resolve("legacy-app")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.Name != "legacy-app" {
		t.Errorf("Name should fall back to Repo, got %s", p.Name)
	}
}

func TestResolveUnknownListsAvailable(t *testing.T) {
	defer withTempHome(t)()
	makeProject(t, "alpha", "https://github.com/u/alpha.git")
	makeProject(t, "beta", "https://github.com/u/beta.git")

	_, err := Resolve("nope")
	if err == nil {
		t.Fatal("expected UnknownProjectError")
	}
	msg := err.Error()
	if !strings.Contains(msg, "nope") {
		t.Errorf("error should name the missing project: %s", msg)
	}
	if !strings.Contains(msg, "alpha") || !strings.Contains(msg, "beta") {
		t.Errorf("error should list available projects: %s", msg)
	}
}

func TestResolveEmptyUsesMostRecent(t *testing.T) {
	defer withTempHome(t)()
	makeProject(t, "first", "https://github.com/u/first.git")
	time.Sleep(10 * time.Millisecond)
	makeProject(t, "second", "https://github.com/u/second.git")

	// AllManagers orders by last-init time, most recent first.
	p, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// "second" was initialised last.
	if p.Name != "second" {
		t.Errorf("most-recent: want second, got %s", p.Name)
	}
}

func TestResolveNoProjects(t *testing.T) {
	defer withTempHome(t)()
	if _, err := Resolve(""); err != ErrNoProjects {
		t.Errorf("expected ErrNoProjects, got %v", err)
	}
}

func TestAllSkipsStatelessDirs(t *testing.T) {
	defer withTempHome(t)()
	makeProject(t, "has-lock", "https://github.com/u/has.git")
	// An empty state dir (no lock) — e.g. created by a failed init.
	if _, err := state.For("no-lock"); err != nil {
		t.Fatal(err)
	}
	all, err := All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 1 || all[0].Name != "has-lock" {
		t.Errorf("All should only return projects with a lock, got %v", names(all))
	}
}

func TestNames(t *testing.T) {
	defer withTempHome(t)()
	makeProject(t, "a", "https://github.com/u/a.git")
	makeProject(t, "b", "https://github.com/u/b.git")
	names, err := Names()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %v", names)
	}
}

func TestConfiguredURLsFromPortMap(t *testing.T) {
	defer withTempHome(t)()
	makeProject(t, "urls", "https://github.com/u/urls.git")
	p, err := Resolve("urls")
	if err != nil {
		t.Fatal(err)
	}
	urls := ConfiguredURLs(p)
	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs (service-1 + service-2), got %v", urls)
	}
	want8000 := "http://localhost:8000"
	want3000 := "http://localhost:3000"
	if urls[0].URL != want8000 || urls[1].URL != want3000 {
		t.Errorf("URLs: want %s then %s, got %v", want8000, want3000, urls)
	}
}

func names(ps []*Project) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
}
