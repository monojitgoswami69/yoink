package ui

// HeaderArgs is retained for call-site compatibility. The giant ASCII banner
// is gone (§30); a header is now intentionally minimal — the command's own
// content provides the identity.
type HeaderArgs struct {
	Command string
	Version string
}

// Header renders a minimal, muted context line. Most commands pair this with
// a ProjectHeader or a Section label for their actual content. Returns "" for
// no command, so callers may print it unconditionally.
func Header(args HeaderArgs) string {
	if args.Command == "" {
		return ""
	}
	return MutedStyle.Render("yoink " + args.Command)
}

// Banner is retained as an empty shim so legacy callers compile; the ASCII
// art was removed in favour of typographic identity.
func Banner() string { return "" }

// StepLine renders a minimal step title for legacy callers. New code should
// use the symbol-based Steps renderer in theme.go.
func StepLine(n, total int, title string) string {
	return "  " + BoldStyle.Render(title)
}
