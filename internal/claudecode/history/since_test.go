package history_test

import (
	"testing"
	"testing/fstest"
	"time"

	"lo0.ai/lo0/internal/claudecode"
	"lo0.ai/lo0/internal/claudecode/history"
)

func TestSinceReadsOnlyRecentTranscripts(t *testing.T) {
	cut := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	fsys := fstest.MapFS{
		"-home-u-code-app/old.jsonl": {Data: fixture(t), ModTime: cut.Add(-time.Hour)},
		"-home-u-code-app/new.jsonl": {Data: fixture(t), ModTime: cut.Add(time.Minute)},
	}
	sessions, err := history.NewAnalyzer(fsys, 1).Since(t.Context(), cut)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].Harness != claudecode.Harness || sessions[0].Turns == 0 {
		t.Errorf("sessions = %d, want only the transcript changed since the cut, summarised", len(sessions))
	}
}
