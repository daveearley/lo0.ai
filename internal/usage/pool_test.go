package usage_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"lo0.ai/lo0/internal/usage"
)

type fakeSource struct {
	name     string
	sessions usage.Sessions
	err      error
}

func (f fakeSource) Harness() string { return f.name }

func (f fakeSource) Sessions(ctx context.Context, progress usage.Progress) (usage.Sessions, error) {
	if err := ctx.Err(); err != nil {
		return usage.Sessions{}, err
	}
	n := len(f.sessions.List)
	progress(0, n)
	progress(n, n)
	return f.sessions, f.err
}

func session(id string, peak int, version string) usage.Session {
	return usage.Session{
		ID: id, CWD: "/r/app", Turns: 2, PeakContext: peak, Version: version,
		Start: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC),
		MCP:   map[usage.MCPTool]int{{Server: id + "-server", Tool: "t"}: 1},
	}
}

func TestPoolProfilesTogetherAndApart(t *testing.T) {
	claude := fakeSource{name: "Claude Code", sessions: usage.Sessions{List: []usage.Session{session("c1", 400_000, "2.1.317"), session("c2", 100_000, "2.1.300")}, Subagents: 3}}
	codexWindow := session("x1", 200_000, "0.153.4")
	codexWindow.ContextWindow = 258_400
	codex := fakeSource{name: "Codex", sessions: usage.Sessions{List: []usage.Session{codexWindow}}, err: errors.New("reading one log: truncated")}

	var mu sync.Mutex
	var last [2]int
	p, err := usage.Pool{Sources: []usage.Source{claude, codex}, Repos: identityRepos{}}.Analyze(t.Context(), func(done, total int) {
		mu.Lock()
		defer mu.Unlock()
		last = [2]int{done, total}
	})
	if err == nil {
		t.Error("a source's error was dropped")
	}
	if p.Sessions != 3 || p.Subagents.Transcripts != 3 || p.Context.Max != 400_000 {
		t.Errorf("pooled = %d sessions, %d subagent logs, max %d", p.Sessions, p.Subagents.Transcripts, p.Context.Max)
	}
	if last != [2]int{3, 3} {
		t.Errorf("progress ended at %v, want the sum of both sources", last)
	}
	if len(p.Format.Versions) != 0 {
		t.Errorf("pooled versions = %+v, want none: versions only compare within a harness", p.Format.Versions)
	}
	for _, r := range p.Records {
		if r.Harness == "" {
			t.Errorf("record %+v has no harness", r)
		}
	}

	c, x := p.Of("Claude Code"), p.Of("Codex")
	if c.Sessions != 2 || len(c.MCPServers) != 2 || c.Format.Newest() != "2.1.317" || c.ContextWindow != 0 {
		t.Errorf("Claude Code profile = %d sessions, %d servers, newest %q, window %d", c.Sessions, len(c.MCPServers), c.Format.Newest(), c.ContextWindow)
	}
	if x.Sessions != 1 || x.ContextWindow != 258_400 || x.Records[0].Harness != "Codex" {
		t.Errorf("Codex profile = %+v", x)
	}
	if got := p.Of("Gemini"); got.Sessions != 0 {
		t.Errorf("Of an unscanned harness = %d sessions, want none", got.Sessions)
	}
}

func TestOfWithoutPooling(t *testing.T) {
	p := usage.Profile{Sessions: 4}
	if p.Of("Claude Code").Sessions != 4 {
		t.Error("a profile that was not pooled should stand for any harness")
	}
}

func TestPoolCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := usage.Pool{Sources: []usage.Source{fakeSource{name: "a"}}, Repos: identityRepos{}}.Analyze(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Analyze error = %v, want context.Canceled", err)
	}
}
