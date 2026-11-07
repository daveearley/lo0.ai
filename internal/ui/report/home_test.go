package report

import (
	"testing"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/cost"
)

func TestHomeTitle(t *testing.T) {
	both := cost.Summary{Harnesses: []cost.Report{{Harness: "Claude Code"}, {Harness: "Codex"}}}
	tests := []struct {
		name     string
		sessions int
		cost     cost.Summary
		want     string
	}{
		{"two harnesses", 50, both, "Local models for your Claude Code and Codex work"},
		{"one harness", 50, cost.Summary{Harnesses: []cost.Report{{Harness: "Codex"}}}, "Local models for your Codex work"},
		{"no sessions", 0, both, "Local models for this machine"},
		{"no harness named", 50, cost.Summary{}, "Local models for this machine"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var a app.Assessment
			a.Scan.Usage.Sessions, a.Cost = tt.sessions, tt.cost
			if got := HomeTitle(a); got != tt.want {
				t.Errorf("HomeTitle = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFitLine(t *testing.T) {
	tests := []struct {
		name       string
		fit, total int
		want       string
	}{
		{"several", 31, 50, "31 of 50 sessions fit in M’s 256k window"},
		{"one of many", 1, 50, "1 of 50 sessions fits in M’s 256k window"},
		{"one session", 1, 1, "1 of 1 session fits in M’s 256k window"},
		{"thousands", 1200, 3000, "1,200 of 3,000 sessions fit in M’s 256k window"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FitLine(tt.fit, tt.total, "M", 262_144); got != tt.want {
				t.Errorf("FitLine = %q, want %q", got, tt.want)
			}
		})
	}
}
