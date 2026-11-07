package report_test

import (
	"strings"
	"testing"
	"time"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/ui/theme"
)

func TestRenderSessionWithoutRequests(t *testing.T) {
	start := time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		exit int
		want string
	}{
		{"exited normally", 0, "only how long the session ran is known"},
		{"failed to start", 1, "Codex stopped with an error before sending a request to Gemma-4-E2B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := app.SessionReport{Harness: "Codex", Model: "Gemma-4-E2B", Started: start, Ended: start.Add(300 * time.Millisecond), ExitCode: tt.exit}
			got := strings.Join(strings.Fields(report.RenderSession(r, report.Options{Width: 80, Theme: theme.Plain()})), " ")
			if !strings.Contains(got, tt.want) {
				t.Errorf("RenderSession = %q, want %q", got, tt.want)
			}
		})
	}
}
