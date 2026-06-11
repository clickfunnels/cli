// Package ui holds shared lipgloss styles and small render helpers so the
// look-and-feel is consistent across commands.
package ui

import "github.com/charmbracelet/lipgloss"

// ClickFunnels-ish palette.
var (
	ColorPrimary = lipgloss.Color("#3B82F6") // blue
	ColorAccent  = lipgloss.Color("#22D3EE") // cyan
	ColorMuted   = lipgloss.Color("#9CA3AF") // gray
	ColorSuccess = lipgloss.Color("#22C55E") // green
	ColorError   = lipgloss.Color("#EF4444") // red
)

var (
	// Title is a bold, padded heading.
	Title = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)
	// Subtle is muted secondary text.
	Subtle = lipgloss.NewStyle().Foreground(ColorMuted)
	// Success / Error / Accent inline text styles.
	Success = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)
	Error   = lipgloss.NewStyle().Foreground(ColorError).Bold(true)
	Accent  = lipgloss.NewStyle().Foreground(ColorAccent)

	// HeaderRow styles a table header.
	HeaderRow = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)

	// Box wraps content in a rounded border.
	Box = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Padding(0, 1)
)

// Checkmark and cross glyphs.
const (
	Check = "✓"
	Cross = "✗"
)
