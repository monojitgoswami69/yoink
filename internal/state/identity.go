package state

import "strings"

// Canonicalize normalises a project name into a stable, filesystem-safe
// canonical identifier. It is the key under ~/.yoink/state/ and the suffix
// of the Docker Compose project name (yoink-<id>), so that "Sevatra" and
// "sevatra" resolve to the same project. The original human-readable name is
// preserved in Lock.Project for display.
//
//	Canonicalize("Sevatra")       == "sevatra"
//	Canonicalize("My Project")   == "my-project"
//	Canonicalize("my__project")  == "my-project"
//	Canonicalize("  Mixed  Case ") == "mixed-case"
//
// Resolution compares Canonicalize(m.Repo) == Canonicalize(name) so legacy
// state directories created with mixed-case names still resolve.
func Canonicalize(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}
