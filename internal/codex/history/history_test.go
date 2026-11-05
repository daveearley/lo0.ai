package history_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"lo0.ai/lo0/internal/codex/history"
	"lo0.ai/lo0/internal/codex/rollout"
	"lo0.ai/lo0/internal/usage"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "session.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func summarize(t *testing.T, data string) history.Summary {
	t.Helper()
	s, err := history.Summarize(rollout.Records(strings.NewReader(data)))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSummarize(t *testing.T) {
	sum := summarize(t, string(fixture(t)))
	s := sum.Session
	checks := []struct {
		name      string
		got, want any
	}{
		{"Parent", sum.Parent, ""},
		{"ID", s.ID, "main-1"},
		{"CWD", s.CWD, "/r/app"},
		{"Version", s.Version, "0.153.4"},
		{"Entrypoint", s.Entrypoint, "Codex Desktop"},
		{"Start", s.Start, time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)},
		{"Turns, with the repeated count seen once", s.Turns, 2},
		{"Models", s.Models["gpt-5.5"], 2},
		{"Effort", s.Effort["high"], 2},
		{"PeakContext", s.PeakContext, 120_000},
		{"ThinkingTokens", s.ThinkingTokens, int64(120)},
		{"OutputTokens", s.OutputTokens, int64(350)},
		{"Compactions", s.Compactions, 1},
		{"Tools[exec_command]", s.Tools["exec_command"], 1},
		{"Tools[apply_patch]", s.Tools["apply_patch"], 1},
		{"MCP", s.MCP[usage.MCPTool{Server: "cua_repl", Tool: "js"}], 1},
		{"Tools[js] stays out of built-in tools", s.Tools["js"], 0},
		{"SubagentCalls", s.SubagentCalls, 1},
		{"ImagesAttached", s.ImagesAttached, 1},
		{"ToolImages", s.ToolImages, 1},
		{"ToolResults", s.ToolResults, 1},
		{"ToolErrors", s.ToolErrors, 1},
		{"Files", len(s.Files), 2},
		{"LinesAdded", s.LinesAdded, 3},
		{"LinesRemoved", s.LinesRemoved, 1},
		{"Languages[TypeScript]", s.Languages["TypeScript"], 1},
		{"Languages[Markdown]", s.Languages["Markdown"], 1},
		{"LimitHits: one reached limit and one failed task", s.LimitHits, 2},
		{"UnknownTypes", s.UnknownTypes["hologram"], 1},
		{"DecodeErrors", s.DecodeErrors, 1},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	wantRequests := []usage.Request{{ToolCalls: 4, Output: 350, Duration: time.Minute}, {Duration: 5 * time.Second}}
	if !slices.Equal(s.Requests, wantRequests) {
		t.Errorf("Requests = %+v, want %+v", s.Requests, wantRequests)
	}
	wantUsage := []usage.ModelUsage{
		{Model: "gpt-5.5", Tokens: usage.Tokens{Input: 300, CacheWrite5m: 100, CacheRead: 600, Output: 50}},
		{Model: "gpt-5.5", LongPrompt: true, Tokens: usage.Tokens{Input: 119_000, CacheRead: 1000, Output: 300}},
	}
	if !slices.Equal(s.Usage, wantUsage) {
		t.Errorf("Usage = %+v, want %+v", s.Usage, wantUsage)
	}
}

func TestFileChangeItemsReplacePatchCounts(t *testing.T) {
	log := string(fixture(t)) + "\n" +
		`{"timestamp":"2026-10-02T10:05:00.000Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"FileChange","status":"completed","changes":{"/r/app/main.go":{"type":"update","unified_diff":"--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n"}}}}}`
	s := summarize(t, log).Session
	if len(s.Files) != 1 || !s.Files["/r/app/main.go"] || s.LinesAdded != 1 || s.LinesRemoved != 1 || s.Languages["Go"] != 1 {
		t.Errorf("files %v, +%d -%d, languages %v; want only the FileChange item counted", s.Files, s.LinesAdded, s.LinesRemoved, s.Languages)
	}
}

func TestProfileHoldsNoContent(t *testing.T) {
	p := usage.Aggregate([]usage.Session{summarize(t, string(fixture(t))).Session}, 0, identityRepos{})
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "SECRET") {
		t.Errorf("profile contains session content: %s", data)
	}
}

// replayed is a session log's first lines as a fork or subagent writes
// them: a copy of the history it started from, then its own work.
func replayed(id, meta string) string {
	return strings.Join([]string{
		`{"timestamp":"2026-10-02T11:00:00Z","type":"session_meta","payload":{"id":"` + id + `",` + meta + `}}`,
		`{"timestamp":"2026-10-02T11:00:00Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1000,"output_tokens":50,"total_tokens":1050},"last_token_usage":{"input_tokens":1000,"output_tokens":50,"total_tokens":1050}}}}`,
		`{"timestamp":"2026-10-02T11:00:00Z","type":"response_item","payload":{"type":"function_call","name":"exec_command"}}`,
		`{"timestamp":"2026-10-02T11:00:01Z","type":"turn_context","payload":{"model":"gpt-5.5"}}`,
		`{"timestamp":"2026-10-02T11:00:02Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1500,"output_tokens":70,"total_tokens":1570},"last_token_usage":{"input_tokens":500,"output_tokens":20,"total_tokens":520}}}}`,
	}, "\n")
}

func TestForksSkipTheHistoryTheyCopied(t *testing.T) {
	for name, meta := range map[string]string{
		"subagent": `"source":{"subagent":{"thread_spawn":{"parent_thread_id":"main-1","depth":1}}}`,
		"fork":     `"forked_from_id":"main-1","source":"cli"`,
	} {
		t.Run(name, func(t *testing.T) {
			s := summarize(t, replayed("child", meta)).Session
			if s.Turns != 1 || s.OutputTokens != 20 || len(s.Tools) != 0 {
				t.Errorf("turns %d, output %d, tools %v; want only the session's own response", s.Turns, s.OutputTokens, s.Tools)
			}
		})
	}
}

type identityRepos struct{}

func (identityRepos) Repo(dir string) string { return dir }

func TestAnalyzerFoldsSubagentsIntoTheirParent(t *testing.T) {
	sub := `"source":{"subagent":{"thread_spawn":{"parent_thread_id":"main-1"}}}`
	orphan := `"source":{"subagent":{"thread_spawn":{"parent_thread_id":"gone"}}}`
	fsys := fstest.MapFS{
		"2026/10/02/rollout-2026-10-02T10-00-00-main-1.jsonl": {Data: fixture(t)},
		"2026/10/02/rollout-2026-10-02T11-00-00-child.jsonl":  {Data: []byte(replayed("child", sub))},
		"2026/10/03/rollout-2026-10-03T09-00-00-orphan.jsonl": {Data: []byte(replayed("orphan", orphan))},
		"2026/10/03/notes.txt":                                {Data: []byte("not a session")},
	}
	var calls atomic.Int64
	p, err := analyze(t.Context(), history.NewAnalyzer(fsys, 2), func(int, int) { calls.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	if p.Sessions != 1 || p.Subagents.Transcripts != 2 || calls.Load() != 4 {
		t.Errorf("sessions %d, subagent logs %d, progress calls %d", p.Sessions, p.Subagents.Transcripts, calls.Load())
	}
	output := int64(0)
	for _, u := range p.Usage {
		output += u.Tokens.Output
	}
	if output != 370 {
		t.Errorf("output tokens = %d, want the parent's 350 plus the subagent's own 20", output)
	}
	if p.Turns != 2 {
		t.Errorf("Turns = %d, want the parent's alone", p.Turns)
	}
}

func TestAnalyzerMissingSessionsDir(t *testing.T) {
	p, err := analyze(t.Context(), history.NewAnalyzer(os.DirFS(filepath.Join(t.TempDir(), "missing")), 1), nil)
	if err != nil || p.Sessions != 0 {
		t.Fatalf("Analyze = %+v, %v; want an empty profile", p, err)
	}
}

func TestAnalyzerCancelled(t *testing.T) {
	fsys := fstest.MapFS{"2026/10/02/rollout-x.jsonl": {Data: fixture(t)}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := analyze(ctx, history.NewAnalyzer(fsys, 1), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Analyze error = %v, want context.Canceled", err)
	}
}

// analyze reads every session the way the scan's pool does for one source.
func analyze(ctx context.Context, a *history.Analyzer, progress usage.Progress) (usage.Profile, error) {
	s, err := a.Sessions(ctx, progress)
	return usage.Aggregate(s.List, s.Subagents, identityRepos{}), err
}

// A fork's log replays the parent's history, its task_started and token
// counts included, then starts the fork's own task a few records before its
// first turn context. That task and the prompt it carries are the fork's.
func TestForksKeepTheirOwnFirstTask(t *testing.T) {
	data := strings.Join([]string{
		`{"timestamp":"2026-10-02T11:00:00Z","type":"session_meta","payload":{"id":"fork","forked_from_id":"main-1","source":"cli"}}`,
		`{"timestamp":"2026-10-02T11:00:00Z","type":"event_msg","payload":{"type":"task_started"}}`,
		`{"timestamp":"2026-10-02T11:00:00Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1000,"output_tokens":50,"total_tokens":1050},"last_token_usage":{"input_tokens":1000,"output_tokens":50,"total_tokens":1050}}}}`,
		`{"timestamp":"2026-10-02T11:00:01Z","type":"event_msg","payload":{"type":"task_started"}}`,
		`{"timestamp":"2026-10-02T11:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_image"}]}}`,
		`{"timestamp":"2026-10-02T11:00:01Z","type":"turn_context","payload":{"model":"gpt-5.5"}}`,
		`{"timestamp":"2026-10-02T11:00:02Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1500,"output_tokens":70,"total_tokens":1570},"last_token_usage":{"input_tokens":500,"output_tokens":20,"total_tokens":520}}}}`,
	}, "\n")
	s := summarize(t, data).Session
	if s.ImagesAttached != 1 || s.OutputTokens != 20 {
		t.Errorf("images %d, output %d; want the fork's own prompt and response only", s.ImagesAttached, s.OutputTokens)
	}
}
