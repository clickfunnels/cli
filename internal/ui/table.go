package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// RenderTable renders a static (non-interactive) table with the shared theme.
// Use this for one-shot command output; use the bubbles table model for
// interactive TUIs.
func RenderTable(headers []string, rows [][]string) string {
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(ColorMuted)).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return HeaderRow.Padding(0, 1)
			}
			return lipgloss.NewStyle().Padding(0, 1)
		})
	return t.Render()
}

// Truncate shortens s to max runes, adding an ellipsis when cut.
func Truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return strings.TrimRight(string(r[:max-1]), " ") + "…"
}
