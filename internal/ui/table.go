package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Table renders a simple, colour-aware table.
type Table struct {
	Headers []string
	Rows    [][]string
	Title   string
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// visibleWidth returns the printable width of s, ignoring ANSI escape codes.
func visibleWidth(s string) int {
	clean := ansiRe.ReplaceAllString(s, "")
	w := 0
	for range clean {
		w++
	}
	return w
}

func padCell(s string, width int) string {
	w := visibleWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// Render renders the table with a rounded border, padding, and a header rule.
func (t *Table) Render() string {
	if len(t.Rows) == 0 {
		return ""
	}
	cols := len(t.Headers)
	colWidths := make([]int, cols)
	for i, h := range t.Headers {
		colWidths[i] = visibleWidth(h)
	}
	for _, row := range t.Rows {
		for i := 0; i < cols && i < len(row); i++ {
			if w := visibleWidth(row[i]); w > colWidths[i] {
				colWidths[i] = w
			}
		}
	}

	var b strings.Builder
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(ColourPrimary)
	if t.Title != "" {
		b.WriteString(headerStyle.MarginBottom(1).Render(t.Title))
		b.WriteString("\n")
	}

	for i, h := range t.Headers {
		b.WriteString(headerStyle.Render(padCell(h, colWidths[i])))
		if i < cols-1 {
			b.WriteString(DimStyle.Render("  │  "))
		}
	}
	b.WriteString("\n")

	for i, w := range colWidths {
		b.WriteString(DimStyle.Render(strings.Repeat("─", w)))
		if i < cols-1 {
			b.WriteString(DimStyle.Render("──┼──"))
		}
	}
	b.WriteString("\n")

	for _, row := range t.Rows {
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			b.WriteString(padCell(cell, colWidths[i]))
			if i < cols-1 {
				b.WriteString(DimStyle.Render("  │  "))
			}
		}
		b.WriteString("\n")
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColourPrimary).
		Padding(1, 2)
	return box.Render(strings.TrimRight(b.String(), "\n"))
}

// ConfidenceBar renders a five-cell confidence bar with its label.
func ConfidenceBar(confidence string) string {
	var filled int
	switch confidence {
	case "high":
		filled = 5
	case "medium":
		filled = 3
	case "low":
		filled = 1
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", 5-filled)
	var style lipgloss.Style
	switch confidence {
	case "high":
		style = SuccessStyle
	case "medium":
		style = WarningStyle
	default:
		style = ErrorStyle
	}
	return style.Render(bar) + " " + MutedStyle.Render(confidence)
}
