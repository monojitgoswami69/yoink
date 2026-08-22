// Package ui holds Yoink's terminal presentation layer.
//
// This file is the single source of truth for the Yoink visual language:
// the symbol vocabulary, spacing scale, status colours, and the shared
// renderers (header, steps, status badges, URLs, errors, columns, footer).
// Every command and the dashboard draw from these primitives so the look is
// coherent and a theme change touches only a handful of files.
//
// Design principles: calm, minimal, dense-but-not-cramped, restrained colour.
// Colour communicates meaning (green=healthy, amber=starting, red=unhealthy);
// the majority of the UI stays neutral. Symbols are ASCII/common-Unicode so
// they render in any terminal without a Nerd Font, and stay distinguishable
// with --no-color.
package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// ─── Symbol vocabulary (§31) ──────────────────────────────────────────────
// Chosen for broad terminal compatibility. With --no-color the symbol alone
// still distinguishes the state.
var (
	SymDone  = "✓" // success / complete
	SymFail  = "×" // error / failed
	SymRun   = "●" // healthy / running
	SymStop  = "○" // stopped / inactive
	SymWork  = "◌" // working / in-progress
	SymArrow = "→" // url / action
	SymSel   = "›" // selection cursor
	SymSep   = "·" // muted separator
	SymBar   = "─" // thin rule
)

// ─── Spacing scale (§5) ───────────────────────────────────────────────────
// 1 tight (within a group), 2 normal, 3 section, 4 major section.
const (
	SP1 = 1
	SP2 = 2
	SP3 = 4
	SP4 = 6
)

// V returns n blank lines.
func V(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("\n", n)
}

// Indent prefixes each line of s with n spaces. Used for nested groups.
func Indent(s string, n int) string {
	if n <= 0 {
		return s
	}
	pad := strings.Repeat(" ", n)
	parts := strings.Split(s, "\n")
	for i, p := range parts {
		parts[i] = pad + p
	}
	return strings.Join(parts, "\n")
}

// Section renders a small, uppercase, muted section label — the only place
// uppercase is used deliberately (§4).
func Section(title string) string {
	return MutedStyle.Bold(true).Render(strings.ToUpper(title))
}

// Rule renders a thin horizontal rule of the given width (or term width).
func Rule(width int) string {
	if width <= 0 {
		width = 40
	}
	return DimStyle.Render(strings.Repeat(SymBar, width))
}

// ─── Status (§3, §31) ─────────────────────────────────────────────────────
// These are the ONLY status renderers; commands and the dashboard must call
// them instead of hand-rolling symbols + colours.

// OverallStatus renders a project-level status badge, e.g. "● Running".
func OverallStatus(overall string) string {
	switch overall {
	case "running":
		return SuccessStyle.Render(SymRun + " Running")
	case "starting":
		return WarningStyle.Render(SymWork + " Starting")
	case "unhealthy":
		return ErrorStyle.Render(SymRun + " Unhealthy")
	case "failed":
		return ErrorStyle.Render(SymFail + " Failed")
	case "stopped":
		return MutedStyle.Render(SymStop + " Stopped")
	}
	return MutedStyle.Render(SymStop + " " + titleCase(overall))
}

// titleCase lowercases s then capitalises its first rune, mirroring the old
// strings.Title(strings.ToLower(s)) behaviour without the deprecation.
func titleCase(s string) string {
	s = strings.ToLower(s)
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// ServiceStatus renders one service's status: the dot (●/○) coloured by
// run-state, plus a health label. With --no-color the symbol still reads.
func ServiceStatus(state, health string) (dot, label string) {
	switch state {
	case "running":
		switch health {
		case "healthy":
			dot = SuccessStyle.Render(SymRun)
			label = SuccessStyle.Render("healthy")
		case "unhealthy":
			dot = ErrorStyle.Render(SymRun)
			label = ErrorStyle.Render("unhealthy")
		case "starting":
			dot = WarningStyle.Render(SymRun)
			label = WarningStyle.Render("starting")
		default:
			dot = PrimaryStyle.Render(SymRun)
			label = MutedStyle.Render("—")
		}
	case "exited", "dead":
		dot = ErrorStyle.Render(SymStop)
		label = ErrorStyle.Render(state)
	case "restarting", "created":
		dot = WarningStyle.Render(SymRun)
		label = WarningStyle.Render(state)
	default:
		dot = MutedStyle.Render(SymStop)
		if state == "" {
			label = MutedStyle.Render("stopped")
		} else {
			label = MutedStyle.Render(state)
		}
	}
	return
}

// ─── Step / progress model (§8) ───────────────────────────────────────────

// StepStatus is the lifecycle state of one progress step.
type StepStatus int

const (
	StepPending StepStatus = iota
	StepRunning
	StepSuccess
	StepFailed
	StepSkipped
)

// Step is one line of a multi-step operation (init/up/heal/update).
type Step struct {
	Title    string
	Status   StepStatus
	Duration time.Duration
}

// Steps is a renderable list of progress steps.
type Steps []Step

// Render renders the steps as a scannable list: a symbol, the title, and a
// muted duration on the right when known.
func (ss Steps) Render() string {
	var b strings.Builder
	for _, s := range ss {
		sym := MutedStyle.Render(SymStop)
		title := MutedStyle.Render(s.Title)
		switch s.Status {
		case StepRunning:
			sym = PrimaryStyle.Render(SymWork)
			title = BoldStyle.Render(s.Title)
		case StepSuccess:
			sym = SuccessStyle.Render(SymDone)
			title = s.Title
		case StepFailed:
			sym = ErrorStyle.Render(SymFail)
			title = ErrorStyle.Render(s.Title)
		case StepSkipped:
			sym = MutedStyle.Render("·")
			title = MutedStyle.Render(s.Title)
		}
		dur := ""
		if s.Duration > 0 {
			dur = MutedStyle.Render(fmt.Sprintf("  %s", humanStepDur(s.Duration)))
		}
		fmt.Fprintf(&b, "  %s  %s%s\n", sym, title, dur)
	}
	return strings.TrimRight(b.String(), "\n")
}

func humanStepDur(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.0fs", d.Seconds())
}

// ─── Headers (§30 — no giant ASCII) ───────────────────────────────────────
// The old ASCII banner is gone. A header is now a muted context line.

// CommandHeader renders a single muted context line for a command (used when
// a command has no project yet, e.g. `yoink doctor`, `yoink list`).
func CommandHeader(command string) string {
	return MutedStyle.Render(fmt.Sprintf("yoink %s", command))
}

// ProjectHeader renders a minimal project identity: the name (bold) and a
// muted repo line. This is the canonical top-of-output block for project
// commands (up/status/env/…), matching the spec's examples.
func ProjectHeader(name, repo string) string {
	var b strings.Builder
	b.WriteString("  " + BoldStyle.Render(name) + "\n")
	if repo != "" {
		b.WriteString("  " + MutedStyle.Render(repo) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// ─── URLs (§10) ───────────────────────────────────────────────────────────

// URL renders a single public URL line.
func URL(url string) string {
	return "  " + HighlightStyle.Render(SymArrow+" "+url)
}

// URLList renders a labelled list of URLs.
func URLList(pairs []URLPair) string {
	var b strings.Builder
	for _, p := range pairs {
		fmt.Fprintf(&b, "  %s  %s\n", PrimaryStyle.Render(p.Service), HighlightStyle.Render(p.URL))
	}
	return strings.TrimRight(b.String(), "\n")
}

// URLPair is a service → URL mapping for URLList.
type URLPair struct {
	Service string
	URL     string
}

// ─── Errors (§29) ─────────────────────────────────────────────────────────

// Error renders an actionable error block: a red × line plus an optional
// muted hint. Raw diagnostics stay available via --verbose; this is the
// friendly face.
func Error(title, hint string) string {
	var b strings.Builder
	b.WriteString("  " + ErrorStyle.Render(SymFail+" "+title))
	if hint != "" {
		b.WriteString("\n\n  " + MutedStyle.Render(hint))
	}
	return b.String()
}

// ─── Columns (§12 — alignment over borders) ───────────────────────────────
// A minimal aligned table: a bold header row, a thin rule, then rows. No
// surrounding box — whitespace separates it from surrounding content.

// Columns is a minimal aligned table.
type Columns struct {
	Headers []string
	Rows    [][]string
	Title   string
	Gap     int // column gap (default 2)
}

// Render renders the table with consistent alignment, indented 2 spaces so it
// sits inside the standard Yoink content margin.
func (c Columns) Render() string {
	if len(c.Headers) == 0 {
		return ""
	}
	gap := c.Gap
	if gap <= 0 {
		gap = 2
	}
	cols := len(c.Headers)
	widths := make([]int, cols)
	for i, h := range c.Headers {
		widths[i] = visibleWidth(h)
	}
	for _, r := range c.Rows {
		for i := 0; i < cols && i < len(r); i++ {
			if w := visibleWidth(r[i]); w > widths[i] {
				widths[i] = w
			}
		}
	}
	var b strings.Builder
	if c.Title != "" {
		b.WriteString("  " + Section(c.Title) + "\n")
	}
	// header
	b.WriteString("  ")
	for i, h := range c.Headers {
		b.WriteString(BoldStyle.Render(padCell(h, widths[i])))
		if i < cols-1 {
			b.WriteString(strings.Repeat(" ", gap))
		}
	}
	b.WriteString("\n  ")
	// rule (only as wide as the content)
	total := 0
	for i, w := range widths {
		total += w
		if i < cols-1 {
			total += gap
		}
	}
	b.WriteString(DimStyle.Render(strings.Repeat(SymBar, total)) + "\n")
	// rows
	for _, r := range c.Rows {
		b.WriteString("  ")
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(r) {
				cell = r[i]
			}
			b.WriteString(padCell(cell, widths[i]))
			if i < cols-1 {
				b.WriteString(strings.Repeat(" ", gap))
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// ─── Footer (§24) ─────────────────────────────────────────────────────────

// KeyBind is one footer keybinding.
type KeyBind struct {
	Key   string
	Label string
}

// Footer renders a subtle, single-line footer of keybindings.
func Footer(binds []KeyBind) string {
	parts := make([]string, len(binds))
	for i, b := range binds {
		parts[i] = fmt.Sprintf("%s %s", PrimaryStyle.Render(b.Key), MutedStyle.Render(b.Label))
	}
	return MutedStyle.Render(strings.Join(parts, "   "))
}

// ─── Notification (§28) ───────────────────────────────────────────────────

// Notify renders a lightweight transient success line.
func Notify(msg string) string {
	return "  " + SuccessStyle.Render(SymDone+" "+msg)
}
