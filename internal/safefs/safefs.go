// Package safefs constrains filesystem reads to a known root directory.
package safefs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Reader reads files under Root. Paths supplied to Read may be relative or
// absolute, but the resolved location must stay inside Root.
type Reader struct {
	Root    string
	MaxSize int64 // 0 means 1 MiB
}

// New creates a reader rooted at root. root is resolved to an absolute path.
func New(root string) (*Reader, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	return &Reader{Root: abs, MaxSize: 1 << 20}, nil
}

// Resolve returns the absolute path for p, or an error if p escapes the root.
func (r *Reader) Resolve(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	p = filepath.Clean(p)
	if filepath.IsAbs(p) {
		// Reject absolute paths outright — callers should always pass repo-relative paths.
		return "", fmt.Errorf("absolute paths are not permitted: %s", p)
	}
	joined := filepath.Join(r.Root, p)
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	// Defend against symlinks: resolve and re-check containment.
	resolved := abs
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		resolved = real
	}
	rootWithSep := r.Root + string(os.PathSeparator)
	if resolved != r.Root && !strings.HasPrefix(resolved, rootWithSep) {
		return "", fmt.Errorf("path escapes repository root: %s", p)
	}
	return resolved, nil
}

// Read returns the file contents, capped at MaxSize bytes.
func (r *Reader) Read(p string) (string, error) {
	abs, err := r.Resolve(p)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory: %s", p)
	}
	limit := r.MaxSize
	if limit <= 0 {
		limit = 1 << 20
	}
	f, err := os.Open(abs)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, limit)
	n, _ := f.Read(buf)
	if int64(n) == limit {
		return string(buf[:n]) + "\n[...truncated...]\n", nil
	}
	return string(buf[:n]), nil
}
