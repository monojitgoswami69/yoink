package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Color palette as per specification
var (
	ColourPrimary   = lipgloss.Color("#7C3AED")
	ColourSuccess   = lipgloss.Color("#10B981")
	ColourWarning   = lipgloss.Color("#F59E0B")
	ColourError     = lipgloss.Color("#EF4444")
	ColourMuted     = lipgloss.Color("#6B7280")
	ColourHighlight = lipgloss.Color("#60A5FA")
	ColourDim       = lipgloss.Color("#374151")
)

// Styles
var (
	PrimaryStyle   = lipgloss.NewStyle().Foreground(ColourPrimary)
	SuccessStyle   = lipgloss.NewStyle().Foreground(ColourSuccess)
	WarningStyle   = lipgloss.NewStyle().Foreground(ColourWarning)
	ErrorStyle     = lipgloss.NewStyle().Foreground(ColourError)
	MutedStyle     = lipgloss.NewStyle().Foreground(ColourMuted)
	HighlightStyle = lipgloss.NewStyle().Foreground(ColourHighlight)
	DimStyle       = lipgloss.NewStyle().Foreground(ColourDim)
	BoldStyle      = lipgloss.NewStyle().Bold(true)
)

// Box styles
var (
	SuccessBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColourSuccess).
			Padding(0, 1)

	ErrorBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColourError).
			Padding(0, 1)

	WarningBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColourWarning).
			Padding(0, 1)
)

// SetNoColor turns off all ANSI styling globally for this process.
// Call before printing any styled content. Honours NO_COLOR convention.
func SetNoColor() {
	lipgloss.SetColorProfile(termenv.Ascii) // no color, 1-bit
	plain := lipgloss.NewStyle()
	PrimaryStyle = plain
	SuccessStyle = plain
	WarningStyle = plain
	ErrorStyle = plain
	MutedStyle = plain
	HighlightStyle = plain
	DimStyle = plain
	BoldStyle = lipgloss.NewStyle().Bold(true)
	SuccessBox = lipgloss.NewStyle().Padding(0, 1)
	ErrorBox = lipgloss.NewStyle().Padding(0, 1)
	WarningBox = lipgloss.NewStyle().Padding(0, 1)
}
