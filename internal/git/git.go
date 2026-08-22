package git

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// ParsedURL describes a GitHub repository reference.
type ParsedURL struct {
	Owner  string
	Repo   string
	Branch string // optional — set when the URL pointed at a /tree/<branch>/...
	Subdir string // optional — when /tree/<branch>/<subdir> was given
	Clone  string // canonical https clone URL
}

var (
	httpsRepoRe = regexp.MustCompile(`^https?://github\.com/([a-zA-Z0-9][a-zA-Z0-9_.-]*)/([a-zA-Z0-9][a-zA-Z0-9_.-]*?)(?:\.git)?/?$`)
	httpsTreeRe = regexp.MustCompile(`^https?://github\.com/([a-zA-Z0-9][a-zA-Z0-9_.-]*)/([a-zA-Z0-9][a-zA-Z0-9_.-]*)/tree/([^/]+)(?:/(.*))?$`)
	sshRepoRe   = regexp.MustCompile(`^git@github\.com:([a-zA-Z0-9][a-zA-Z0-9_.-]*)/([a-zA-Z0-9][a-zA-Z0-9_.-]*?)(?:\.git)?$`)
)

// ParseURL accepts GitHub URLs in any common shape:
//
//	https://github.com/<owner>/<repo>
//	https://github.com/<owner>/<repo>.git
//	https://github.com/<owner>/<repo>/tree/<branch>[/<subdir>]
//	git@github.com:<owner>/<repo>[.git]
func ParseURL(s string) (*ParsedURL, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty URL")
	}
	if m := httpsTreeRe.FindStringSubmatch(s); m != nil {
		p := &ParsedURL{Owner: m[1], Repo: strings.TrimSuffix(m[2], ".git"), Branch: m[3], Subdir: m[4]}
		p.Clone = fmt.Sprintf("https://github.com/%s/%s.git", p.Owner, p.Repo)
		return p, nil
	}
	if m := httpsRepoRe.FindStringSubmatch(s); m != nil {
		p := &ParsedURL{Owner: m[1], Repo: strings.TrimSuffix(m[2], ".git")}
		p.Clone = fmt.Sprintf("https://github.com/%s/%s.git", p.Owner, p.Repo)
		return p, nil
	}
	if m := sshRepoRe.FindStringSubmatch(s); m != nil {
		p := &ParsedURL{Owner: m[1], Repo: strings.TrimSuffix(m[2], ".git")}
		p.Clone = fmt.Sprintf("https://github.com/%s/%s.git", p.Owner, p.Repo)
		return p, nil
	}
	return nil, fmt.Errorf("not a GitHub URL: %q", s)
}

// ExtractRepoName returns just the repo name, or "" on failure.
func ExtractRepoName(s string) string {
	p, err := ParseURL(s)
	if err != nil {
		return ""
	}
	return p.Repo
}

// Clone clones a repository to targetDir. PAT is supplied via http.extraheader
// so it never appears on the command line as part of the URL and never gets
// stored in .git/config.
func Clone(ctx context.Context, repoURL, targetDir, pat string) error {
	parsed, err := ParseURL(repoURL)
	if err != nil {
		return err
	}

	args := []string{}
	if pat != "" {
		auth := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + pat))
		args = append(args, "-c", "http.https://github.com/.extraheader=Authorization: Basic "+auth)
	}

	args = append(args, "clone", "--depth=1")
	if parsed.Branch != "" {
		args = append(args, "--branch", parsed.Branch, "--single-branch")
	}
	args = append(args, parsed.Clone, targetDir)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stderr = os.Stderr
	// Avoid asking the user for credentials interactively when the PAT is wrong.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	if err := cmd.Run(); err != nil {
		// Scrub the PAT from the URL just in case it crept into the error path.
		msg := err.Error()
		if pat != "" {
			msg = strings.ReplaceAll(msg, pat, "***")
		}
		return fmt.Errorf("git clone failed: %s", msg)
	}

	// Defensive: strip any extraheader that git might have persisted into .git/config.
	cleanup := exec.Command("git", "-C", targetDir, "config", "--unset-all", "http.https://github.com/.extraheader")
	_ = cleanup.Run()
	return nil
}

// CountFiles walks dir and returns (count, total-bytes). Errors are skipped.
func CountFiles(dir string) (int, int64, error) {
	var count int
	var size int64
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			count++
			size += info.Size()
		}
		return nil
	})
	return count, size, err
}
