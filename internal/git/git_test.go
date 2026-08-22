package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseURL(t *testing.T) {
	cases := []struct {
		in     string
		ok     bool
		owner  string
		repo   string
		branch string
		subdir string
		clone  string
	}{
		{"https://github.com/foo/bar", true, "foo", "bar", "", "", "https://github.com/foo/bar.git"},
		{"https://github.com/foo/bar.git", true, "foo", "bar", "", "", "https://github.com/foo/bar.git"},
		{"https://github.com/foo/bar/", true, "foo", "bar", "", "", "https://github.com/foo/bar.git"},
		{"https://github.com/foo/bar/tree/main", true, "foo", "bar", "main", "", "https://github.com/foo/bar.git"},
		{"https://github.com/foo/bar/tree/main/packages/web", true, "foo", "bar", "main", "packages/web", "https://github.com/foo/bar.git"},
		{"git@github.com:foo/bar.git", true, "foo", "bar", "", "", "https://github.com/foo/bar.git"},
		{"git@github.com:foo/bar", true, "foo", "bar", "", "", "https://github.com/foo/bar.git"},
		{"https://gitlab.com/foo/bar", false, "", "", "", "", ""},
		{"", false, "", "", "", "", ""},
		{"not a url", false, "", "", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			p, err := ParseURL(c.in)
			if c.ok && err != nil {
				t.Fatalf("ParseURL(%q): unexpected error %v", c.in, err)
			}
			if !c.ok {
				if err == nil {
					t.Fatalf("ParseURL(%q): expected error, got nil", c.in)
				}
				return
			}
			if p.Owner != c.owner || p.Repo != c.repo || p.Branch != c.branch || p.Subdir != c.subdir || p.Clone != c.clone {
				t.Errorf("ParseURL(%q) = %+v, want owner=%s repo=%s branch=%s subdir=%s clone=%s", c.in, p, c.owner, c.repo, c.branch, c.subdir, c.clone)
			}
		})
	}
}

func TestExtractRepoName(t *testing.T) {
	if got := ExtractRepoName("https://github.com/foo/bar.git"); got != "bar" {
		t.Errorf("ExtractRepoName: got %q", got)
	}
	if got := ExtractRepoName("nonsense"); got != "" {
		t.Errorf("ExtractRepoName invalid: got %q", got)
	}
}

func TestIsLocalRef(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		in   string
		want bool
	}{
		{"", true},
		{".", true},
		{"./", true},
		{dir, true},
		{"https://github.com/foo/bar", false},
		{"git@github.com:foo/bar", false},
		{"/nonexistent/path/xyz", false},
		{"not-a-url-not-a-dir", false},
	}
	for _, c := range cases {
		if got := IsLocalRef(c.in); got != c.want {
			t.Errorf("IsLocalRef(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseLocal(t *testing.T) {
	// Empty path -> cwd. Verify it resolves to an absolute dir.
	p, err := ParseLocal("")
	if err != nil {
		t.Fatalf("ParseLocal(\"\"): %v", err)
	}
	if p.Owner != "local" {
		t.Errorf("Owner: want local, got %s", p.Owner)
	}
	if !filepath.IsAbs(p.LocalPath) {
		t.Errorf("LocalPath should be absolute, got %s", p.LocalPath)
	}
	if !strings.HasPrefix(p.Clone, "file://") {
		t.Errorf("Clone should be file:// URL, got %s", p.Clone)
	}
	if p.Repo == "" {
		t.Error("Repo should be the directory base name, got empty")
	}

	// Existing temp dir -> name = base name.
	dir := t.TempDir()
	p, err = ParseLocal(dir)
	if err != nil {
		t.Fatalf("ParseLocal(%s): %v", dir, err)
	}
	if p.Repo != filepath.Base(dir) {
		t.Errorf("Repo: want %s, got %s", filepath.Base(dir), p.Repo)
	}
	if p.LocalPath != dir {
		t.Errorf("LocalPath: want %s, got %s", dir, p.LocalPath)
	}

	// Non-existent path -> error.
	if _, err := ParseLocal("/nonexistent/xyz"); err == nil {
		t.Error("ParseLocal should error on non-existent path")
	}

	// File (not dir) -> error.
	f := filepath.Join(t.TempDir(), "file")
	os.WriteFile(f, []byte("x"), 0644)
	if _, err := ParseLocal(f); err == nil {
		t.Error("ParseLocal should error on a non-directory")
	}
}
