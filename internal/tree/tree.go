// Package tree renders a directory-tree string of a repository, skipping
// directories that are uninteresting for stack detection (node_modules, .git,
// build artefacts, etc.). The output is fed to the LLM as repo context.
package tree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var excludedDirs = map[string]bool{
	"node_modules":  true,
	".next":         true,
	".nuxt":         true,
	"dist":          true,
	"build":         true,
	".cache":        true,
	"__pycache__":   true,
	".venv":         true,
	"venv":          true,
	"env":           true,
	".git":          true,
	"coverage":      true,
	".nyc_output":   true,
	".docker":       true,
	".pytest_cache": true,
	".tox":          true,
	"vendor":        true,
	"yoink-outputs": true,
}

var excludedExtensions = map[string]bool{
	".log": true,
	".tmp": true,
}

// Generate returns a tree of rootDir, capping the result at maxLines lines.
// The returned line count is the number of lines actually emitted.
func Generate(rootDir string, maxLines int) (string, int, error) {
	w := &writer{root: rootDir, max: maxLines}
	w.walk(rootDir, "")
	return strings.TrimRight(w.buf.String(), "\n"), w.lines, nil
}

type writer struct {
	root  string
	max   int
	lines int
	buf   strings.Builder
}

func (w *writer) emit(format string, args ...any) {
	if w.lines >= w.max {
		return
	}
	fmt.Fprintf(&w.buf, format+"\n", args...)
	w.lines++
}

func (w *writer) walk(dir, prefix string) {
	if w.lines >= w.max {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	kept := make([]os.DirEntry, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if excludedDirs[name] {
			path := filepath.Join(dir, name)
			rel, _ := filepath.Rel(w.root, path)
			w.emit("%s├── %s/ [excluded · ~%d items]", prefix, rel, countItems(path))
			continue
		}
		if !e.IsDir() && excludedExtensions[filepath.Ext(name)] {
			continue
		}
		kept = append(kept, e)
	}

	for i, e := range kept {
		if w.lines >= w.max {
			return
		}
		last := i == len(kept)-1
		connector := "├──"
		nextPrefix := prefix + "│   "
		if last {
			connector = "└──"
			nextPrefix = prefix + "    "
		}
		path := filepath.Join(dir, e.Name())
		rel, _ := filepath.Rel(w.root, path)

		if e.IsDir() {
			w.emit("%s%s %s/", prefix, connector, rel)
			w.walk(path, nextPrefix)
		} else {
			w.emit("%s%s %s", prefix, connector, rel)
		}
	}
}

func countItems(dir string) int {
	count := 0
	_ = filepath.WalkDir(dir, func(_ string, _ os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		count++
		if count > 1000 {
			return filepath.SkipDir
		}
		return nil
	})
	return count
}
