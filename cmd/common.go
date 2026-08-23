package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"yoink/internal/docker"
)

// requireDocker returns an actionable error when docker isn't usable.
// It distinguishes "not installed" from "installed but daemon not running"
// so the user gets a clear next step instead of a bare exit code.
func requireDocker() error {
	if !docker.Available() {
		return fmt.Errorf("docker is not installed on PATH — install Docker Desktop or the docker engine + compose v2")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if !docker.DaemonRunning(ctx) {
		return fmt.Errorf("docker daemon is not running\n\nStart Docker and try again")
	}
	return nil
}

// plural returns "<n> <noun>" or "<n> <noun>s" for English-ish plurality.
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// readFileFromDiskSafe reads a file from the given directory. Returns the
// content and true on success, or "" and false on error. Callers must check
// the bool to avoid writing empty content over an existing file.
func readFileFromDiskSafe(dir, filename string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		return "", false
	}
	return string(data), true
}

func containsStr(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
