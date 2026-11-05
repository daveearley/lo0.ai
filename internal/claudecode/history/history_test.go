package history_test

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"lo0.ai/lo0/internal/claudecode/history"
	"lo0.ai/lo0/internal/claudecode/transcript"
	"lo0.ai/lo0/internal/usage"
)

type identityRepos struct{}

func (identityRepos) Repo(dir string) string { return dir }

func fixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "session.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSummarize(t *testing.T) {
	s, err := history.Summarize(transcript.Records(strings.NewReader(string(fixture(t)))))
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	checks := []struct {
		name      string
		got, want any
	}{
		{"ID", s.ID, "s1"},
		{"CWD", s.CWD, "/home/u/code/app"},
		{"Version", s.Version, "2.4.1"},
		{"Entrypoint", s.Entrypoint, "cli"},
		{"Turns", s.Turns, 3},
		{"PeakContext", s.PeakContext, 184003},
		{"Compactions", s.Compactions, 1},
		{"AutoCompactions", s.AutoCompactions, 1},
		{"SubagentCalls", s.SubagentCalls, 1},
		{"DecodeErrors", s.DecodeErrors, 1},
		{"Tools[Bash]", s.Tools["Bash"], 1},
		{"MCP[github]", s.MCP[usage.MCPTool{Server: "github", Tool: "create_pull_request"}], 1},
		{"Languages[Go]", s.Languages["Go"], 1},
		{"Languages[TypeScript]", s.Languages["TypeScript"], 1},
		{"Models[opus]", s.Models["claude-opus-5-5"], 2},
		{"Models[sonnet]", s.Models["claude-sonnet-5-5"], 1},
		{"Start", s.Start, time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)},
		{"End", s.End, time.Date(2026, 10, 1, 9, 40, 0, 0, time.UTC)},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestSummarizeIgnoresSidechainAndSyntheticTurns(t *testing.T) {
	in := strings.Join([]string{
		`{"type":"assistant","isSidechain":true,"message":{"id":"a","model":"m","usage":{"input_tokens":900000}}}`,
		`{"type":"assistant","message":{"id":"b","model":"<synthetic>","usage":{"input_tokens":10}}}`,
	}, "\n")
	s, err := history.Summarize(transcript.Records(strings.NewReader(in)))
	if err != nil {
		t.Fatal(err)
	}
	if s.PeakContext != 10 || s.Turns != 1 || len(s.Models) != 0 {
		t.Errorf("Summarize = %+v, want sidechain ignored and synthetic model uncounted", s)
	}
}

func TestSummarizeSplitsMCPTools(t *testing.T) {
	in := `{"type":"assistant","message":{"id":"a","model":"m","content":[` +
		`{"type":"tool_use","name":"mcp__github__create_pull_request"},` +
		`{"type":"tool_use","name":"mcp__plugin_marketing_ahrefs__doc"},` +
		`{"type":"tool_use","name":"mcp__broken"},` +
		`{"type":"tool_use","name":"Bash"}]}}`
	s, err := history.Summarize(transcript.Records(strings.NewReader(in)))
	if err != nil {
		t.Fatal(err)
	}
	wantMCP := map[usage.MCPTool]int{{Server: "github", Tool: "create_pull_request"}: 1, {Server: "plugin_marketing_ahrefs", Tool: "doc"}: 1}
	if !maps.Equal(s.MCP, wantMCP) || s.Tools["Bash"] != 1 || s.Tools["mcp__broken"] != 1 {
		t.Errorf("MCP = %v, Tools = %v", s.MCP, s.Tools)
	}
}

func TestProfileNeverContainsTranscriptText(t *testing.T) {
	fsys := fstest.MapFS{"-home-u-code-app/s1.jsonl": {Data: fixture(t)}}
	p, err := analyze(t.Context(), history.NewAnalyzer(fsys, 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Fix the failing test", "secret reasoning", "find callers", "package auth", "go test"} {
		if strings.Contains(string(data), text) {
			t.Errorf("profile JSON contains transcript text %q", text)
		}
	}
}

func TestAnalyzer(t *testing.T) {
	fsys := fstest.MapFS{
		"-home-u-code-app/s1.jsonl":                   {Data: fixture(t)},
		"-home-u-code-app/s1/subagents/agent-a.jsonl": {Data: []byte(`{"type":"assistant"}`)},
		"-home-u-code-app/s2.jsonl":                   {Data: fixture(t)},
		"-home-u-code-lib/s3.jsonl":                   {Data: []byte(`{"type":"user","message":{"content":"hi"}}`)},
	}
	var calls, last int
	progress := func(done, total int) {
		calls++
		if total != 3 {
			t.Errorf("progress total = %d, want 3", total)
		}
		last = done
	}
	p, err := analyze(t.Context(), history.NewAnalyzer(fsys, 1), progress)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if p.Sessions != 2 || p.EmptySessions != 1 || p.Subagents.Transcripts != 1 {
		t.Errorf("profile = sessions %d, empty %d, subagent transcripts %d", p.Sessions, p.EmptySessions, p.Subagents.Transcripts)
	}
	if calls != 4 || last != 3 {
		t.Errorf("progress called %d times ending at %d, want 4 ending at 3", calls, last)
	}
}

func TestAnalyzerMissingProjectsDir(t *testing.T) {
	p, err := analyze(t.Context(), history.NewAnalyzer(os.DirFS(filepath.Join(t.TempDir(), "missing")), 2), nil)
	if err != nil || p.Sessions != 0 {
		t.Fatalf("Analyze = %+v, %v; want an empty profile", p, err)
	}
}

func TestAnalyzerCancelled(t *testing.T) {
	fsys := fstest.MapFS{"d/s.jsonl": {Data: fixture(t)}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := analyze(ctx, history.NewAnalyzer(fsys, 1), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Analyze error = %v, want context.Canceled", err)
	}
}

func signals(t *testing.T) usage.Session {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "signals.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := history.Summarize(transcript.Records(strings.NewReader(string(data))))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSummarizeSignals(t *testing.T) {
	s := signals(t)
	if want := []usage.Request{{ToolCalls: 2, Output: 160, Duration: 60 * time.Second}}; !slices.Equal(s.Requests, want) {
		t.Errorf("Requests = %+v, want %+v; the unanswered second prompt is dropped", s.Requests, want)
	}
	checks := []struct {
		name      string
		got, want any
	}{
		{"Version", s.Version, "2.1.317"},
		{"Turns", s.Turns, 4},
		{"Effort[high]", s.Effort["high"], 1},
		{"Effort[medium]", s.Effort["medium"], 1},
		{"ThinkingTokens", s.ThinkingTokens, int64(40)},
		{"OutputTokens", s.OutputTokens, int64(160)},
		{"ImagesAttached", s.ImagesAttached, 1},
		{"ToolImages", s.ToolImages, 1},
		{"LimitHits", s.LimitHits, 1},
		{"FailedResponses", s.FailedResponses, 1},
		{"Retries", s.Retries, 1},
		{"NetworkRetries", s.NetworkRetries, 1},
		{"ToolResults", s.ToolResults, 2},
		{"ToolErrors", s.ToolErrors, 1},
		{"Skills", s.Skills["plugin:review"], 1},
		{"Hooks", s.Hooks["PreToolUse"], 1},
		{"Files", len(s.Files), 1},
		{"LinesAdded", s.LinesAdded, 2},
		{"LinesRemoved", s.LinesRemoved, 1},
		{"PullRequests", len(s.PullRequests), 1},
		{"Responses", s.Responses, 4},
		{"WithoutUsage", s.WithoutUsage, 1},
		{"UnknownTypes", s.UnknownTypes["hologram"], 1},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestProfileHoldsNoContent(t *testing.T) {
	p := usage.Aggregate([]usage.Session{signals(t)}, 0, identityRepos{})
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "SECRET") {
		t.Errorf("profile contains transcript content: %s", data)
	}
}

// analyze reads every session the way the scan's pool does for one source.
func analyze(ctx context.Context, a *history.Analyzer, progress usage.Progress) (usage.Profile, error) {
	s, err := a.Sessions(ctx, progress)
	return usage.Aggregate(s.List, s.Subagents, identityRepos{}), err
}

// A resumed session's transcript repeats the responses of the session it
// came from, with the same IDs; each is billed once, to the older session.
func TestAnalyzerBillsARepeatedResponseOnce(t *testing.T) {
	line := func(at, id string, out int) string {
		return `{"type":"assistant","timestamp":"` + at + `","message":{"id":"` + id + `","model":"claude-opus-5-5","usage":{"output_tokens":` + strconv.Itoa(out) + `}}}` + "\n"
	}
	fsys := fstest.MapFS{
		"-home-u-app/first.jsonl":   {Data: []byte(line("2026-10-01T09:00:00Z", "msg-1", 100))},
		"-home-u-app/resumed.jsonl": {Data: []byte(line("2026-10-01T09:00:00Z", "msg-1", 100) + line("2026-10-02T09:00:00Z", "msg-2", 7))},
	}
	s, err := history.NewAnalyzer(fsys, 2).Sessions(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var out int64
	for _, session := range s.List {
		for _, u := range session.Usage {
			out += u.Tokens.Output
		}
	}
	if len(s.List) != 2 || out != 107 {
		t.Errorf("%d sessions billing %d output tokens, want 2 sessions and 107", len(s.List), out)
	}
}
