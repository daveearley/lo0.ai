package usage_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"lo0.ai/lo0/internal/usage"
)

func TestLanguage(t *testing.T) {
	tests := map[string]string{
		"/a/b/main.go":                     "Go",
		`C:\src\app\Login.TSX`:             "TypeScript",
		"/srv/resources/views/x.blade.php": "PHP",
		"/repo/Dockerfile":                 "Docker",
		"/repo/Dockerfile.dev":             "Docker",
		"/repo/Makefile":                   "Make",
		"/repo/LICENSE":                    "",
		"/repo/image.png":                  "",
		"":                                 "",
	}
	for in, want := range tests {
		if got := usage.Language(in); got != want {
			t.Errorf("Language(%q) = %q, want %q", in, got, want)
		}
	}
}

type identityRepos struct{}

func (identityRepos) Repo(dir string) string { return dir }

func TestAggregate(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 9, d, 12, 0, 0, 0, time.UTC) }
	sessions := []usage.Session{
		{
			ID: "a", CWD: "/r/app", Entrypoint: "cli", Turns: 10, PeakContext: 20_000, Start: day(3), End: day(3),
			Tools:     map[string]int{"Bash": 5},
			MCP:       map[usage.MCPTool]int{{Server: "github", Tool: "get_issue"}: 2, {Server: "github", Tool: "create_pr"}: 1},
			Languages: map[string]int{"Go": 4},
			Models:    map[string]int{"opus": 10}, Plan: "plus",
		},
		{
			ID: "b", CWD: "/r/app", Entrypoint: "cli", Turns: 50, PeakContext: 300_000, Start: day(1), End: day(2),
			Compactions: 2, AutoCompactions: 1, SubagentCalls: 3,
			Tools:     map[string]int{"Bash": 1, "Read": 7},
			Languages: map[string]int{"Go": 1, "TypeScript": 6},
			Models:    map[string]int{"opus": 50}, Plan: "free",
		},
		{
			ID: "c", CWD: "/r/lib", Turns: 4, PeakContext: 100_000, Start: day(9), End: day(10),
			Tools: map[string]int{"Edit": 2}, DecodeErrors: 2,
		},
		{ID: "empty", CWD: "/r/app"},
	}
	p := usage.Aggregate(sessions, 7, identityRepos{})

	if p.Sessions != 3 || p.EmptySessions != 1 || p.Turns != 64 || p.DecodeErrors != 2 {
		t.Errorf("counts = sessions %d, empty %d, turns %d, decode errors %d", p.Sessions, p.EmptySessions, p.Turns, p.DecodeErrors)
	}
	if !p.First.Equal(day(1)) || !p.Last.Equal(day(10)) {
		t.Errorf("period = %v to %v", p.First, p.Last)
	}
	if p.Context.P50 != 100_000 || p.Context.P90 != 300_000 || p.Context.Max != 300_000 {
		t.Errorf("Context = %+v", p.Context)
	}
	if !slices.Equal(p.Context.Peaks, []int{20_000, 100_000, 300_000}) {
		t.Errorf("Peaks = %v, want sorted session peaks", p.Context.Peaks)
	}
	for tokens, want := range map[int]float64{0: 0, 20_000: 1.0 / 3, 99_999: 1.0 / 3, 100_000: 2.0 / 3, 1 << 20: 1} {
		if got := p.Context.Within(tokens); got != want {
			t.Errorf("Within(%d) = %v, want %v", tokens, got, want)
		}
	}
	wantFit := map[int]int{32 << 10: 1, 64 << 10: 1, 128 << 10: 2, 256 << 10: 2, 1 << 20: 3}
	for _, f := range p.Context.Fit {
		if f.Sessions != wantFit[f.Tokens] {
			t.Errorf("Fit at %d = %d sessions, want %d", f.Tokens, f.Sessions, wantFit[f.Tokens])
		}
	}
	if p.Compaction != (usage.Compaction{Sessions: 1, Share: 1.0 / 3, Total: 2, Auto: 1}) {
		t.Errorf("Compaction = %+v", p.Compaction)
	}
	if p.Subagents != (usage.Subagents{Sessions: 1, Share: 1.0 / 3, Calls: 3, Transcripts: 7}) {
		t.Errorf("Subagents = %+v", p.Subagents)
	}
	if got := p.Tools[0]; !sameTally(got, usage.Tally{Name: "Read", Count: 7, Sessions: 1}) {
		t.Errorf("top tool = %+v", got)
	}
	if len(p.MCPServers) != 1 || !sameTally(p.MCPServers[0], usage.Tally{Name: "github", Count: 3, Sessions: 1}) {
		t.Errorf("MCPServers = %+v, want github grouped across its tools", p.MCPServers)
	}
	if !sameTally(p.Languages[0], usage.Tally{Name: "TypeScript", Count: 6, Sessions: 1}) || !sameTally(p.Languages[1], usage.Tally{Name: "Go", Count: 5, Sessions: 2}) {
		t.Errorf("Languages = %+v", p.Languages)
	}
	if len(p.Repos) != 2 || p.Repos[0] != (usage.Repo{Path: "/r/app", Sessions: 2, Turns: 60, ContextP90: 300_000, ContextMax: 300_000}) {
		t.Errorf("Repos = %+v", p.Repos)
	}
	if !sameTally(p.Entrypoints[0], usage.Tally{Name: "cli", Count: 2, Sessions: 2}) {
		t.Errorf("Entrypoints = %+v", p.Entrypoints)
	}
	if p.Plan != "plus" {
		t.Errorf("Plan = %q, want the latest session's that names one", p.Plan)
	}
}

func sameTally(a, b usage.Tally) bool {
	return a.Name == b.Name && a.Count == b.Count && a.Sessions == b.Sessions
}

func TestAggregateCollectsMCPToolNames(t *testing.T) {
	p := usage.Aggregate([]usage.Session{{ID: "a", Turns: 1, Tools: map[string]int{"Bash": 3}, MCP: map[usage.MCPTool]int{
		{Server: "github", Tool: "get_issue"}: 2, {Server: "github", Tool: "create_pr"}: 1,
	}}}, 0, identityRepos{})
	if len(p.MCPServers) != 1 || !slices.Equal(p.MCPServers[0].Tools, []string{"create_pr", "get_issue"}) {
		t.Errorf("MCPServers = %+v", p.MCPServers)
	}
}

func TestAggregateEmpty(t *testing.T) {
	p := usage.Aggregate(nil, 0, identityRepos{})
	if p.Sessions != 0 || p.Context.Max != 0 || len(p.Context.Fit) != len(usage.ContextWindows) {
		t.Errorf("Aggregate(nil) = %+v", p)
	}
}

func TestGitRepos(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "app")
	worktree := filepath.Join(root, "app-wt")
	for _, d := range []string{
		filepath.Join(main, ".git", "worktrees", "wt"),
		filepath.Join(main, "web", "src"),
		filepath.Join(worktree, "pkg"),
		filepath.Join(root, "plain"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	gitdir := "gitdir: " + filepath.Join(main, ".git", "worktrees", "wt") + "\n"
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte(gitdir), 0o644); err != nil {
		t.Fatal(err)
	}

	repos := usage.NewGitRepos(".claude/worktrees")
	tests := map[string]string{
		filepath.Join(main, "web", "src"):                          main,
		filepath.Join(worktree, "pkg"):                             main,
		filepath.Join(main, ".claude", "worktrees", "deleted-one"): main,
		filepath.Join(root, "plain"):                               filepath.Join(root, "plain"),
		"":                                                         "",
	}
	for dir, want := range tests {
		if got := repos.Repo(dir); got != want {
			t.Errorf("Repo(%q) = %q, want %q", dir, got, want)
		}
	}
}

func TestAggregateSignals(t *testing.T) {
	base := usage.Session{
		ID: "s2", Turns: 4, Version: "2.1.317",
		Requests:       []usage.Request{{ToolCalls: 2, Output: 160, Duration: time.Minute}},
		Effort:         map[string]int{"high": 1, "medium": 1},
		ImagesAttached: 1, ToolImages: 1,
		LimitHits: 1, FailedResponses: 1, Retries: 1, NetworkRetries: 1,
		Hooks:      map[string]int{"PreToolUse": 1},
		Files:      map[string]bool{"/r/x.go": true},
		LinesAdded: 2, LinesRemoved: 1,
		PullRequests: map[string]bool{"https://example.com/pull/7": true},
		Responses:    4, WithoutUsage: 1,
	}
	other := usage.Session{
		ID: "o", Turns: 3, Version: "2.1.161",
		Requests:  []usage.Request{{ToolCalls: 40, Output: 9000, Duration: 10 * time.Minute}},
		Effort:    map[string]int{"low": 2},
		Files:     map[string]bool{"/r/x.go": true, "/r/y.go": true},
		Hooks:     map[string]int{"PreToolUse": 4, "Stop": 1},
		Responses: 3,
	}
	p := usage.Aggregate([]usage.Session{base, other, {ID: "empty", Version: "2.1.400", Responses: 2, WithoutUsage: 2}}, 0, identityRepos{})

	if p.Requests.Count != 2 || p.Requests.ToolCalls.Max != 40 || p.Requests.Seconds.Max != 600 {
		t.Errorf("Requests = %+v", p.Requests)
	}
	if p.Changes.Files != 2 || p.Changes.LinesAdded != 2 || p.Changes.PullRequests != 1 || p.Changes.PRSessions != 1 {
		t.Errorf("Changes = %+v, want files counted once across sessions", p.Changes)
	}
	if p.Interruptions != (usage.Interruptions{LimitHits: 1, LimitSessions: 1, FailedResponses: 1, Retries: 1, NetworkRetries: 1, Sessions: 1}) {
		t.Errorf("Interruptions = %+v", p.Interruptions)
	}
	if got := p.Reasoning.HighEffortShare; got != 0.25 {
		t.Errorf("HighEffortShare = %v, want 1 of 4 turns", got)
	}
	if p.Images != (usage.Images{Attached: 1, AttachedSessions: 1, FromTools: 1, ToolSessions: 1, Sessions: 1}) {
		t.Errorf("Images = %+v", p.Images)
	}
	if !sameTally(p.HookRuns[0], usage.Tally{Name: "PreToolUse", Count: 5, Sessions: 2}) {
		t.Errorf("HookRuns = %+v", p.HookRuns)
	}
	f := p.Format
	if f.Newest() != "2.1.400" || f.Oldest() != "2.1.161" || f.WithoutUsage != 3 || f.Responses != 9 {
		t.Errorf("Format = %+v, want empty sessions counted too and versions newest first", f)
	}
}
