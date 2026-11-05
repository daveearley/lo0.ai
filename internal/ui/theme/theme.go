// Package theme defines lo0's terminal styles. There is one accent colour;
// everything else is weight and contrast. Colours are full 24-bit values and
// are downsampled, or stripped, by the writer the output goes to.
package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme is the set of styles the UI renders with.
type Theme struct {
	// Title is the program name in the report header.
	Title lipgloss.Style
	// Heading starts a report section.
	Heading lipgloss.Style
	// Label names a row.
	Label lipgloss.Style
	// Value is ordinary content.
	Value lipgloss.Style
	// Muted is secondary content: units, separators, scopes.
	Muted lipgloss.Style
	// Accent draws the eye to the one thing that matters in a row.
	Accent lipgloss.Style
	Good   lipgloss.Style
	Warn   lipgloss.Style
	Bad    lipgloss.Style
}

type palette struct {
	fg, muted, faint, accent, good, warn, bad color.Color
}

var (
	dark = palette{
		fg:     lipgloss.Color("#E6E6E6"),
		muted:  lipgloss.Color("#8A8F98"),
		faint:  lipgloss.Color("#5C6068"),
		accent: lipgloss.Color("#6FC2B0"),
		good:   lipgloss.Color("#7BC47F"),
		warn:   lipgloss.Color("#E0B661"),
		bad:    lipgloss.Color("#E5786D"),
	}
	light = palette{
		fg:     lipgloss.Color("#1F2328"),
		muted:  lipgloss.Color("#656D76"),
		faint:  lipgloss.Color("#8C959F"),
		accent: lipgloss.Color("#0F766E"),
		good:   lipgloss.Color("#1A7F37"),
		warn:   lipgloss.Color("#9A6700"),
		bad:    lipgloss.Color("#CF222E"),
	}
)

// New returns the theme for a dark or light terminal background.
func New(darkBackground bool) Theme {
	p := light
	if darkBackground {
		p = dark
	}
	return Theme{
		Title:   lipgloss.NewStyle().Foreground(p.accent).Bold(true),
		Heading: lipgloss.NewStyle().Foreground(p.fg).Bold(true),
		Label:   lipgloss.NewStyle().Foreground(p.muted),
		Value:   lipgloss.NewStyle().Foreground(p.fg),
		Muted:   lipgloss.NewStyle().Foreground(p.faint),
		Accent:  lipgloss.NewStyle().Foreground(p.accent),
		Good:    lipgloss.NewStyle().Foreground(p.good),
		Warn:    lipgloss.NewStyle().Foreground(p.warn),
		Bad:     lipgloss.NewStyle().Foreground(p.bad),
	}
}

// Plain returns a theme with no styling, for tests and plain-text output.
func Plain() Theme {
	s := lipgloss.NewStyle()
	return Theme{Title: s, Heading: s, Label: s, Value: s, Muted: s, Accent: s, Good: s, Warn: s, Bad: s}
}
