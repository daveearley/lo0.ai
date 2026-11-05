package tui

import (
	"charm.land/lipgloss/v2"

	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/ui/theme"
)

func levelStyle(t theme.Theme, l match.Level) lipgloss.Style {
	switch l {
	case match.LevelReady:
		return t.Good
	case match.LevelNotYet:
		return t.Bad
	default:
		return t.Warn
	}
}

func coverageStyle(t theme.Theme, share float64) lipgloss.Style {
	switch match.FitOf(share) {
	case match.FitGood:
		return t.Good
	case match.FitPartial:
		return t.Warn
	default:
		return t.Bad
	}
}
