package report_test

import (
	"testing"

	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/ui/theme"
)

func TestPaceLabels(t *testing.T) {
	for _, tt := range []struct {
		pace            match.Pace
		label, multiple string
	}{
		{match.Pace{Band: match.BandSlower, Slower: 2.94}, "2.9× slower", "2.9×"},
		{match.Pace{Band: match.BandVerySlow, Slower: 13}, "very slow", "13×"},
		{match.Pace{Band: match.BandComparable, Slower: 1.6}, "1.6× slower", "1.6×"},
		{match.Pace{Band: match.BandComparable, Slower: 1.05}, "about as fast", "1.1×"},
		{match.Pace{Band: match.BandSlower}, "slow", "slow"},
		{match.Pace{Band: match.BandComparable}, "", ""},
	} {
		if got := report.PaceLabel(tt.pace, theme.Plain()); got != tt.label {
			t.Errorf("PaceLabel(%+v) = %q, want %q", tt.pace, got, tt.label)
		}
		if got := report.PaceMultiple(tt.pace, theme.Plain()); got != tt.multiple {
			t.Errorf("PaceMultiple(%+v) = %q, want %q", tt.pace, got, tt.multiple)
		}
	}
}

func TestSpeedComparisonNeedsAReference(t *testing.T) {
	if got := report.SpeedComparison(match.Candidate{}, nil, 0, theme.Plain(), 80); got != nil {
		t.Errorf("SpeedComparison without a reference = %q, want nothing", got)
	}
}
