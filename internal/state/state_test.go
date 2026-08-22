package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"yoink/internal/detector"
)

func withTempHome(t *testing.T) func() {
	t.Helper()
	tmp := t.TempDir()
	prev := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	return func() { os.Setenv("HOME", prev) }
}

func TestSaveAndLoadLockRoundtrip(t *testing.T) {
	defer withTempHome(t)()

	m, err := For("acme-app")
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	want := &Lock{
		Repo: "acme-app", RepoURL: "https://github.com/acme/app.git",
		RepoPath: "/tmp/x", OutputSubdir: "yoink-outputs",
		Services: []detector.Service{{ID: "service-1", Framework: "fastapi", Port: 8000}},
		Infra:    []string{"postgres"},
		PortMap:  map[string]int{"service-1": 8000},
		Hash:     "abc",
		LastInit: time.Now().UTC().Truncate(time.Second),
	}
	if err := m.SaveLock(want); err != nil {
		t.Fatalf("SaveLock: %v", err)
	}

	got, err := m.LoadLock()
	if err != nil {
		t.Fatalf("LoadLock: %v", err)
	}
	if got.Repo != want.Repo || got.RepoURL != want.RepoURL {
		t.Errorf("lock mismatched: got=%+v want=%+v", got, want)
	}
	if got.Services[0].Framework != "fastapi" {
		t.Errorf("services not persisted: %+v", got.Services)
	}
}

func TestLockFilePerms(t *testing.T) {
	defer withTempHome(t)()
	m, _ := For("perm-check")
	if err := m.SaveLock(&Lock{Repo: "perm-check"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(m.Dir, "yoink.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected 0600 perms, got %v", info.Mode().Perm())
	}
}

func TestOverridesRoundtripAndSet(t *testing.T) {
	defer withTempHome(t)()
	m, _ := For("over")
	if err := m.SetOverride("service-1", "DATABASE_URL", "postgres://x"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetOverride("service-2", "PORT", "9000"); err != nil {
		t.Fatal(err)
	}
	got, err := m.LoadOverrides()
	if err != nil {
		t.Fatal(err)
	}
	if got["service-1"]["DATABASE_URL"] != "postgres://x" {
		t.Errorf("missing service-1 override: %+v", got)
	}
	if got["service-2"]["PORT"] != "9000" {
		t.Errorf("missing service-2 override: %+v", got)
	}
}

func TestHashDetectionStableAcrossOrder(t *testing.T) {
	a := []detector.Service{
		{ID: "service-1", Framework: "next", Directory: "web"},
		{ID: "service-2", Framework: "fastapi", Directory: "api"},
	}
	b := []detector.Service{
		{ID: "service-2", Framework: "fastapi", Directory: "api"},
		{ID: "service-1", Framework: "next", Directory: "web"},
	}
	if HashDetection(a) != HashDetection(b) {
		t.Errorf("hash should be order-independent")
	}
}

func TestHashDetectionChangesOnFramework(t *testing.T) {
	a := []detector.Service{{ID: "s", Framework: "fastapi"}}
	b := []detector.Service{{ID: "s", Framework: "django"}}
	if HashDetection(a) == HashDetection(b) {
		t.Errorf("framework change should alter hash")
	}
}

func TestMergedEnvOverridesAndAppends(t *testing.T) {
	example := `# Comment
DATABASE_URL=postgres://default/x
PORT=8000
`
	merged := MergedEnv(example, map[string]string{"DATABASE_URL": "postgres://prod/y", "SECRET": "shh"})
	if !strings.Contains(merged, "DATABASE_URL=postgres://prod/y") {
		t.Errorf("override should replace value:\n%s", merged)
	}
	if !strings.Contains(merged, "PORT=8000") {
		t.Errorf("non-overridden line should remain:\n%s", merged)
	}
	if !strings.Contains(merged, "SECRET=shh") {
		t.Errorf("unknown override should be appended:\n%s", merged)
	}
	if !strings.Contains(merged, "# Comment") {
		t.Errorf("comments should be preserved:\n%s", merged)
	}
}

func TestMostRecentReturnsNilWhenEmpty(t *testing.T) {
	defer withTempHome(t)()
	m, err := MostRecent()
	if err != nil {
		t.Fatal(err)
	}
	if m != nil {
		t.Errorf("expected nil for empty state, got %+v", m)
	}
}
