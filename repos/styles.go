package main

import "github.com/charmbracelet/lipgloss"

type styles struct {
	accent, faint, bold, yellow, green, red, matched, border lipgloss.Style
	tabActive, tabIdle                                       lipgloss.Style
}

var (
	colAccent = lipgloss.AdaptiveColor{Light: "#6D28D9", Dark: "#A78BFA"}
	colFaint  = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#7C808A"}
	colBorder = lipgloss.AdaptiveColor{Light: "#D1D5DB", Dark: "#3F434C"}
	colYellow = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FBBF24"}
	colGreen  = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"}
	colRed    = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
)

func newStyles(r *lipgloss.Renderer) styles {
	return styles{
		accent:    r.NewStyle().Foreground(colAccent),
		faint:     r.NewStyle().Foreground(colFaint),
		bold:      r.NewStyle().Bold(true),
		yellow:    r.NewStyle().Foreground(colYellow),
		green:     r.NewStyle().Foreground(colGreen),
		red:       r.NewStyle().Foreground(colRed),
		matched:   r.NewStyle().Foreground(colAccent).Bold(true).Underline(true),
		border:    r.NewStyle().Foreground(colBorder),
		tabActive: r.NewStyle().Foreground(colAccent).Bold(true),
		tabIdle:   r.NewStyle().Foreground(colFaint),
	}
}

// newANSIStyles uses the terminal's own 16-colour palette, which never needs
// a background-colour query.
func newANSIStyles(r *lipgloss.Renderer) styles {
	type c = lipgloss.Color
	return styles{
		accent:    r.NewStyle().Foreground(c("5")),
		faint:     r.NewStyle().Faint(true),
		bold:      r.NewStyle().Bold(true),
		yellow:    r.NewStyle().Foreground(c("3")),
		green:     r.NewStyle().Foreground(c("2")),
		red:       r.NewStyle().Foreground(c("1")),
		matched:   r.NewStyle().Foreground(c("5")).Bold(true).Underline(true),
		border:    r.NewStyle().Faint(true),
		tabActive: r.NewStyle().Foreground(c("5")).Bold(true),
		tabIdle:   r.NewStyle().Faint(true),
	}
}
