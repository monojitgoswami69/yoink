package git

import "testing"

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
