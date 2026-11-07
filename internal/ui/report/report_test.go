package report_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

	"lo0.ai/lo0/internal/claudecode"
	"lo0.ai/lo0/internal/claudecode/install"
	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/ui/theme"
	"lo0.ai/lo0/internal/usage"
)

var update = flag.Bool("update", false, "rewrite golden files")

func home() string { return filepath.FromSlash("/Users/dev") }

func under(p string) string { return filepath.Join(home(), filepath.FromSlash(p)) }

func sampleResult() scan.Result {
	at := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 12, 0, 0, 0, time.Local) }
	res := scan.Result{
		GeneratedAt: time.Date(2026, 11, 4, 18, 52, 0, 0, time.Local),
		DurationMS:  612,
		Layout:      claudecode.Layout{ConfigDir: under(".claude")},
		Machine: machine.Profile{
			OS:     machine.OS{GOOS: "darwin", Arch: "arm64", Platform: "darwin", Version: "27.0.1"},
			CPU:    machine.CPU{Model: "Apple M5 Pro", PhysicalCores: 18, LogicalCores: 18},
			Memory: machine.Memory{TotalBytes: 64 << 30, Unified: true},
			GPUs:   []machine.GPU{{Name: "Apple M5 Pro", Vendor: machine.VendorApple, Cores: 20, API: "Metal 4", Unified: true}},
			Disk:   machine.Disk{TotalBytes: 926 << 30, FreeBytes: 412 << 30},
		},
		Runtimes: []runtimes.Runtime{
			{
				ID: runtimes.Ollama, Name: "Ollama", Installed: true, Service: true, Running: true, Version: "0.34.4", Endpoint: "http://127.0.0.1:11434",
				Models: []runtimes.Model{
					{Name: "qwen3.6:latest", Parameters: "36.0B", Quantization: "Q4_K_M", SizeBytes: 23_700_000_000, ContextLength: 262144, Capabilities: []runtimes.Capability{"vision", "tools", "thinking"}},
					{Name: "qwen3-coder:30b", Parameters: "30.5B", Quantization: "Q4_K_M", SizeBytes: 18_556_700_000, ContextLength: 262144, Capabilities: []runtimes.Capability{"tools"}},
					{Name: "hf.co/acme/gemma-chat:2b", Parameters: "2.6B", Quantization: "Q8_0", SizeBytes: 2_800_000_000, ContextLength: 8192},
				},
			},
			{ID: runtimes.LMStudio, Name: "LM Studio", Service: true},
			{ID: runtimes.LlamaCpp, Name: "llama.cpp", Installed: true, Version: "b6123"},
		},
		Setup: setup.Inventory{
			Projects: []string{under("Code/app"), under("Code/site")},
			Instructions: []setup.InstructionFile{
				{Origin: setup.Origin{Scope: setup.ScopeUser}, Kind: setup.InstructionClaudeMD, Lines: 12},
				{Origin: setup.Origin{Scope: setup.ScopeProject}, Kind: setup.InstructionClaudeMD, Lines: 140},
				{Origin: setup.Origin{Scope: setup.ScopeProject}, Kind: setup.InstructionAgentsMD, Lines: 60},
				{Origin: setup.Origin{Scope: setup.ScopeProject}, Kind: setup.InstructionRule, Lines: 30},
			},
			MCPServers: []setup.MCPServer{
				{Origin: setup.Origin{Scope: setup.ScopeLocal, Project: under("Code/app")}, Name: "railway", Transport: setup.TransportStdio},
				{Origin: setup.Origin{Scope: setup.ScopeUser}, Name: "stitch", Transport: setup.TransportHTTP},
				{Origin: setup.Origin{Scope: setup.ScopePlugin, Plugin: "cloudflare"}, Name: "cloudflare", Transport: setup.TransportHTTP},
				{Origin: setup.Origin{Scope: setup.ScopeUser}, Name: "chrome-devtools", Transport: setup.TransportStdio},
			},
			Hooks: []setup.Hook{
				{Event: "SessionStart", Type: "command"},
				{Event: "Stop", Type: "command"},
				{Event: "UserPromptSubmit", Type: "command"},
				{Event: "PostToolUse", Matcher: "Edit", Type: "command"},
			},
			Extensions: []setup.Extension{
				{Kind: setup.ExtensionSkill, Name: "translations"},
				{Kind: setup.ExtensionSkill, Name: "e2e"},
				{Kind: setup.ExtensionCommand, Name: "review"},
			},
			Plugins: []setup.Plugin{{Name: "cloudflare", Enabled: true}, {Name: "lint"}},
			Permissions: []setup.Permissions{
				{Allow: 14, Deny: 2}, {Allow: 3, Ask: 1},
			},
		},
		Usage: usage.Profile{
			Sessions: 427, Turns: 51041, First: at(3, 8), Last: at(11, 4),
			Context: usage.Context{
				P50: 194641, P90: 573947, P99: 961248, Max: 965618,
				Fit: []usage.Fit{
					{Tokens: 32 << 10, Sessions: 1, Share: 0.0023},
					{Tokens: 64 << 10, Sessions: 5, Share: 0.0117},
					{Tokens: 128 << 10, Sessions: 140, Share: 0.3279},
					{Tokens: 256 << 10, Sessions: 262, Share: 0.6136},
					{Tokens: 1 << 20, Sessions: 427, Share: 1},
				},
			},
			Compaction: usage.Compaction{Sessions: 14, Share: 0.0328, Total: 16, Auto: 13},
			Subagents:  usage.Subagents{Sessions: 131, Share: 0.3068, Calls: 789, Transcripts: 1727},
			Tools: []usage.Tally{
				{Name: "Bash", Count: 35081},
				{Name: "Edit", Count: 3790},
				{Name: "Read", Count: 3040},
				{Name: "Write", Count: 2021},
				{Name: "Agent", Count: 789},
				{Name: "ToolSearch", Count: 448},
				{Name: "TaskUpdate", Count: 302},
				{Name: "TaskCreate", Count: 166},
				{Name: "Grep", Count: 90},
			},
			MCPServers: []usage.Tally{{Name: "Claude_Browser", Count: 4741}, {Name: "claude-in-chrome", Count: 2941}, {Name: "github", Count: 65}},
			Languages:  []usage.Tally{{Name: "TypeScript", Count: 3141}, {Name: "PHP", Count: 1896}, {Name: "Markdown", Count: 1145}, {Name: "CSS", Count: 563}},
			Models:     []usage.Tally{{Name: "claude-opus-5-5", Count: 26491}, {Name: "claude-fable-5", Count: 8970}},
			Repos: []usage.Repo{
				{Path: under("Code/app"), Sessions: 336, ContextP90: 573947},
				{Path: under("Code/site"), Sessions: 61, ContextP90: 552374},
				{Path: filepath.FromSlash("/srv/shared/tools"), Sessions: 3, ContextP90: 42000},
			},
			Requests: usage.Requests{
				Count: 3014, ToolCalls: usage.Spread{P50: 5, P90: 44, Max: 370},
				Output: usage.Spread{P50: 3877, P90: 34121}, Seconds: usage.Spread{P50: 58, P90: 659},
			},
			Interruptions: usage.Interruptions{LimitHits: 6, LimitSessions: 3, FailedResponses: 46, Retries: 781, NetworkRetries: 751, Sessions: 125},
			Reasoning:     usage.Reasoning{ThinkingTokens: 11e6, ThinkingShare: 0.26, HighEffortShare: 0.75},
			Images:        usage.Images{Attached: 39, AttachedSessions: 20, FromTools: 4359, ToolSessions: 190, Sessions: 196},
			Changes:       usage.Changes{LinesAdded: 265620, LinesRemoved: 35510, Files: 2442, PullRequests: 93, PRSessions: 74},
			ToolErrors:    usage.ToolErrors{Results: 57071, Failed: 1416, Share: 0.0248},
			Skills:        []usage.Tally{{Name: "translations", Count: 89}, {Name: "code-review", Count: 17}},
			HookRuns:      []usage.Tally{{Name: "PostToolUse", Count: 1174}, {Name: "Stop", Count: 108}},
			Format: usage.Format{Responses: 109555, Versions: []usage.Tally{
				{Name: "2.1.317", Sessions: 16}, {Name: "2.1.300", Sessions: 200}, {Name: "2.1.161", Sessions: 3},
			}},
		},
		Claude:   install.Installs{CLI: "2.1.221", App: "2.1.317", Latest: "2.1.318", Stable: "2.1.311"},
		Warnings: []scan.Warning{{Stage: scan.StageSetup, Message: "reading ~/.claude/settings.json: unexpected end of JSON input"}},
	}
	res.Usage.Harnesses = []usage.HarnessProfile{{Name: claudecode.Harness, Profile: res.Usage}}
	return res
}

func TestRenderGolden(t *testing.T) {
	for _, width := range []int{80, 60} {
		t.Run(filepath.Join("width", strconv.Itoa(width)), func(t *testing.T) {
			got := filepath.ToSlash(report.Render(sampleResult(), report.Options{Width: width, Theme: theme.Plain(), Home: home()}))
			golden := filepath.Join("testdata", "report_"+strconv.Itoa(width)+".golden")
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("reading golden file (run go test -update): %v", err)
			}
			if got != string(want) {
				t.Errorf("report differs from %s; run go test ./internal/ui/report -update and review the diff\n\ngot:\n%s", golden, got)
			}
		})
	}
}

func TestRenderFitsWidth(t *testing.T) {
	for _, width := range []int{60, 80, 100, 160} {
		out := report.Render(sampleResult(), report.Options{Width: width, Theme: theme.New(true), Home: home()})
		limit := min(width, report.MaxWidth)
		for i, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
			if w := lipgloss.Width(line); w > limit {
				t.Errorf("width %d: line %d is %d columns: %q", width, i+1, w, line)
			}
		}
	}
}

func TestRenderStripsColourForPlainWriters(t *testing.T) {
	out := report.Render(sampleResult(), report.Options{Width: 80, Theme: theme.New(true), Home: home()})
	var buf bytes.Buffer
	w := &colorprofile.Writer{Forward: &buf, Profile: colorprofile.NoTTY}
	if _, err := w.WriteString(out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "\x1b[") {
		t.Error("NoTTY output still contains escape sequences")
	}
	plain := report.Render(sampleResult(), report.Options{Width: 80, Theme: theme.Plain(), Home: home()})
	if buf.String() != plain {
		t.Error("stripped coloured output differs from the plain rendering")
	}
}

func TestRenderEmptyHistory(t *testing.T) {
	res := sampleResult()
	res.Usage = usage.Profile{}
	out := report.Render(res, report.Options{Width: 80, Theme: theme.Plain(), Home: home()})
	if !strings.Contains(out, "none found in "+filepath.Join("~", ".claude", "projects")) {
		t.Errorf("empty history not explained:\n%s", out)
	}
}

func TestHistoryShowsTheHarnessSplit(t *testing.T) {
	res := sampleResult()
	res.Usage.Harnesses = []usage.HarnessProfile{
		{Name: "Claude Code", Profile: usage.Profile{Sessions: 388, Format: usage.Format{Versions: []usage.Tally{{Name: "2.1.317"}, {Name: "2.1.161"}}}}},
		{Name: "Codex", Profile: usage.Profile{Sessions: 39, ContextWindow: 258_400, Format: usage.Format{Versions: []usage.Tally{{Name: "0.155.0"}}}}},
	}
	got := report.Render(res, report.Options{Width: 100, Theme: theme.Plain(), Home: home()})
	for _, want := range []string{
		"Claude Code 388 · Codex 39",
		"Codex compacts at its own 252k window",
		"Claude Code 2.1.161 to 2.1.317 (2 versions)",
		"Codex 0.155.0 (1 version)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report missing %q:\n%s", want, got)
		}
	}
}

func TestStartedFromMergesSharedNames(t *testing.T) {
	res := sampleResult()
	res.Usage.Entrypoints = []usage.Tally{
		{Name: "claude-desktop", Sessions: 40},
		{Name: "Codex Desktop", Sessions: 32},
		{Name: "codex_cli_rs", Sessions: 5},
		{Name: "codex_work_desktop", Sessions: 12},
		{Name: "codex-tui", Sessions: 2},
	}
	got := report.Render(res, report.Options{Width: 100, Theme: theme.Plain(), Home: home()})
	if want := "Codex Desktop 44 · Claude app 40 · Codex CLI 7"; !strings.Contains(got, want) {
		t.Errorf("report missing %q:\n%s", want, got)
	}
}
