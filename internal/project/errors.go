package project

import (
	"fmt"
	"strings"
)

// ErrNoProjects means nothing has been initialised yet.
var ErrNoProjects = fmt.Errorf("no projects initialised — run `yoink init <github-url>` first")

// UnknownProjectError names a project that wasn't found and lists the known
// ones so the CLI can render an actionable message.
type UnknownProjectError struct {
	Name      string
	Available []string
}

func (e *UnknownProjectError) Error() string {
	if len(e.Available) == 0 {
		return fmt.Sprintf("project %q was not found", e.Name)
	}
	return fmt.Sprintf("project %q was not found\n\nAvailable projects:\n  %s\n\nRun:\n  yoink list", e.Name, strings.Join(e.Available, "\n  "))
}

// MissingLockError means the state directory exists but has no yoink.lock
// (interrupted init or hand-edited state).
type MissingLockError struct {
	Project string
}

func (e *MissingLockError) Error() string {
	return fmt.Sprintf("project %q has no yoink.lock — has init been run?", e.Project)
}
