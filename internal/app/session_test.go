package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/migrate/migratetest"
	"lo0.ai/lo0/internal/usage"
)

type fakeHistory struct {
	sessions []usage.Session
	since    *time.Time
	err      error
}

func (f fakeHistory) Since(_ context.Context, t time.Time) ([]usage.Session, error) {
	*f.since = t
	return f.sessions, f.err
}

func TestSessionReportFindsTheTriedSession(t *testing.T) {
	started := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	reply := func(output int, seconds int) usage.Reply {
		return usage.Reply{Model: "lo0/local:64k", At: started.Add(time.Minute), Output: output, Duration: time.Duration(seconds) * time.Second}
	}
	tried := usage.Session{
		Start: started.Add(2 * time.Second), End: started.Add(14 * time.Minute),
		Models:   map[string]int{"lo0/local:64k": 30},
		Requests: make([]usage.Request, 4), PeakContext: 61_000, Compactions: 1, ToolResults: 50, ToolErrors: 3,
		Replies: []usage.Reply{reply(600, 30), reply(400, 20), reply(500, 25)},
	}
	other := usage.Session{
		Start: started.Add(time.Minute), End: started.Add(5 * time.Minute),
		Models:   map[string]int{"claude-opus-5-5": 10},
		Requests: make([]usage.Request, 9), ToolResults: 90,
	}
	var since time.Time
	mig := migrator(migratetest.NewFiles(nil))
	mig.Histories = []app.RecentHistory{fakeHistory{sessions: []usage.Session{other, tried}, since: &since, err: errors.New("one transcript unreadable")}}
	svc, a := assess(t, result(), mig)
	p := migrate.Plan{HarnessName: "Claude Code on a local model", Model: "Local", RuntimeName: "Ollama", Context: 65_536, Served: migrate.Served{Model: "lo0/local:64k"}}

	r, err := svc.SessionReport(t.Context(), a, p, started, started.Add(15*time.Minute), 0)
	if err == nil {
		t.Error("an unreadable transcript was not reported")
	}
	if !since.Equal(started) {
		t.Errorf("read sessions since %v, want the start of the try", since)
	}
	if !r.Found || r.Requests != 4 || r.PeakContext != 61_000 || r.ToolResults != 50 || r.ToolErrors != 3 || r.Compactions != 1 {
		t.Errorf("report = %+v, want only the session that used lo0's model", r)
	}
	if r.Speed != 20 || r.Replies != 3 {
		t.Errorf("speed = %v over %d replies, want a median 20 tok/s over 3", r.Speed, r.Replies)
	}
}

func TestSessionReportWithoutLogs(t *testing.T) {
	svc, a := assess(t, result(), migrator(migratetest.NewFiles(nil)))
	started := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	r, err := svc.SessionReport(t.Context(), a, migrate.Plan{Model: "Local"}, started, started.Add(time.Minute), 2)
	if err != nil || r.Found || r.ExitCode != 2 || !r.Ended.Equal(started.Add(time.Minute)) {
		t.Errorf("report = %+v, err %v; want how long it ran and its exit code only", r, err)
	}
}
