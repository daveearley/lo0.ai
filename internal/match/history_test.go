package match_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/harness/opencode"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/usage"
)

func TestProjectsAndMonthsUseTheBestWindow(t *testing.T) {
	res := appleResult(20_000, 40_000, 60_000, 600_000)
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 12, 0, 0, 0, time.Local) }
	res.Usage.Records = []usage.SessionRecord{
		{Start: day(9, 1), PeakContext: 20_000, Repo: "/r/app"},
		{Start: day(9, 2), PeakContext: 600_000, Repo: "/r/app"},
		{Start: day(10, 1), PeakContext: 40_000, Repo: "/r/lib"},
		{Start: day(10, 2), PeakContext: 60_000},
	}
	m := match.Match(res, testCatalog(), harnesses())
	if len(m.Projects) != 2 || m.Projects[0] != (match.ProjectFit{Repo: "/r/app", Sessions: 2, Fits: 0.5}) || m.Projects[1].Fits != 1 {
		t.Errorf("Projects = %+v", m.Projects)
	}
	want := []match.MonthFit{
		{Month: "2026-09", Sessions: 2, MedianPeak: 20_000, Fits: 0.5},
		{Month: "2026-10", Sessions: 2, MedianPeak: 40_000, Fits: 1},
	}
	if len(m.Months) != 2 || m.Months[0] != want[0] || m.Months[1] != want[1] {
		t.Errorf("Months = %+v, want %+v", m.Months, want)
	}
}

func TestInsightsFromHistory(t *testing.T) {
	res := appleResult(20_000, 40_000, 60_000)
	res.Usage.Images = usage.Images{Sessions: 3, FromTools: 9}
	res.Usage.Reasoning = usage.Reasoning{HighEffortShare: 0.8}
	res.Usage.Requests = usage.Requests{ToolCalls: usage.Spread{P50: 5, P90: 44}}
	res.Runtimes = []runtimes.Runtime{{ID: runtimes.Ollama, Installed: true, Models: []runtimes.Model{
		{Name: "coder:30b", Quantization: "Q4_K_M", SizeBytes: uint64(17 * gib), Capabilities: []runtimes.Capability{runtimes.CapabilityTools}},
	}}}
	cat := testCatalog()
	cat.Models[1].Vision = true
	m := match.Match(res, cat, harnesses())

	byID := map[string]match.Candidate{}
	for _, c := range m.Candidates {
		byID[c.Model.ID] = c
	}
	has := func(lines []string, want string) bool {
		for _, l := range lines {
			if strings.Contains(l, want) {
				return true
			}
		}
		return false
	}
	coder, general := byID["coder-a"], byID["general-b"]
	if !has(coder.Insights.Concerns, "Cannot read images: 3 of your sessions") {
		t.Errorf("coder concerns = %q, want the missing vision named", coder.Insights.Concerns)
	}
	if !has(coder.Insights.Concerns, "No thinking mode, and 80% of your turns") {
		t.Errorf("coder concerns = %q, want the installed build's missing thinking mode named", coder.Insights.Concerns)
	}
	if has(coder.Insights.Concerns, "chains") {
		t.Errorf("coder concerns = %q, tier A models need no long-chain warning", coder.Insights.Concerns)
	}
	if !has(general.Insights.Strengths, "Reads images") || !has(general.Insights.Concerns, "chains 44 or more tool calls; tier B") {
		t.Errorf("general insights = %+v", general.Insights)
	}
	if has(general.Insights.Concerns, "thinking") || has(general.Insights.Strengths, "thinking") {
		t.Errorf("general insights = %+v, thinking is unknown for a model that is not installed", general.Insights)
	}
	if !has(m.Verdict.Reasons, "One in ten of your requests chains 44 or more tool calls") {
		t.Errorf("Reasons = %q, want the long chains named", m.Verdict.Reasons)
	}
	if !has(m.Verdict.Reasons, "3 of your sessions had images") {
		t.Errorf("Reasons = %q, want the best model's missing vision named", m.Verdict.Reasons)
	}
}

func TestCarryoverCountsHookRunsAndSkillLoads(t *testing.T) {
	inv := setup.Inventory{
		Hooks: []setup.Hook{
			{Event: "Elicitation", Type: "command"},
			{Event: "PreToolUse", Type: "command"},
			{Event: "PreToolUse", Type: "command"},
		},
		Extensions: []setup.Extension{
			{Kind: setup.ExtensionSkill, Name: "review", Origin: setup.Origin{Scope: setup.ScopePlugin}},
			{Kind: setup.ExtensionSkill, Name: "unused"},
		},
	}
	history := usage.Profile{
		HookRuns: []usage.Tally{{Name: "PreToolUse", Count: 12}},
		Skills:   []usage.Tally{{Name: "tools:review", Count: 3}, {Name: "review", Count: 1}},
	}
	calls := map[string]int{}
	for _, f := range (opencode.Harness{}).Features(inv, history, harness.Installation{}) {
		calls[string(f.Area)+"/"+f.Name] = f.Calls
	}
	if calls["hooks/PreToolUse"] != 12 || calls["hooks/Elicitation"] != 0 || calls["skills/review"] != 4 || calls["skills/unused"] != 0 {
		t.Errorf("calls = %v", calls)
	}
}

func TestHookReasonSaysHowOftenLostHooksRan(t *testing.T) {
	res := appleResult(20_000, 40_000)
	res.Setup.Hooks = []setup.Hook{{Event: "Elicitation", Type: "command"}, {Event: "Elicitation", Type: "command"}, {Event: "PreToolUse", Type: "command"}}
	reason := func() string {
		for _, r := range match.Match(res, testCatalog(), []match.Harness{opencode.Harness{}}).Verdict.Reasons {
			if strings.Contains(r, "hook handlers") {
				return r
			}
		}
		return ""
	}
	if got := reason(); strings.Contains(got, "ran") {
		t.Errorf("reason = %q, want no claim about runs when the history records none", got)
	}
	res.Usage.HookRuns = []usage.Tally{{Name: "PreToolUse", Count: 5}}
	if got := reason(); !strings.Contains(got, "none of them ran in your history") {
		t.Errorf("reason = %q", got)
	}
	res.Usage.HookRuns = []usage.Tally{{Name: "Elicitation", Count: 2}}
	if got := reason(); !strings.Contains(got, "they ran 2 times") {
		t.Errorf("reason = %q, want each lost event's runs counted once", got)
	}
}

func TestPooledHistoryFitsTogetherButCarriesOverOnlyClaudeCode(t *testing.T) {
	res := appleResult(20_000, 40_000, 600_000)
	claude := usage.Profile{Sessions: 1, MCPServers: []usage.Tally{{Name: "Claude_Browser", Count: 50}}}
	codexOnly := usage.Profile{Sessions: 2, MCPServers: []usage.Tally{{Name: "cua_repl", Count: 900}}, HookRuns: []usage.Tally{{Name: "Stop", Count: 3}}}
	res.Usage.MCPServers = slices.Concat(claude.MCPServers, codexOnly.MCPServers)
	res.Usage.HookRuns = codexOnly.HookRuns
	res.Usage.Harnesses = []usage.HarnessProfile{{Name: "Claude Code", Profile: claude}, {Name: "Codex", Profile: codexOnly}}
	res.Setup.Hooks = []setup.Hook{{Event: "Elicitation", Type: "command"}}

	m := match.Match(res, testCatalog(), harnesses())
	for _, f := range m.Features {
		if f.Name == "cua_repl" {
			t.Errorf("Codex's MCP server became a Claude Code feature: %+v", f)
		}
	}
	if best, _ := m.Best(); best.Best.Coverage < 0.6 {
		t.Errorf("coverage = %v, want fit measured on every pooled session", best.Best.Coverage)
	}
	for _, r := range m.Verdict.Reasons {
		if strings.Contains(r, "hook handlers") && strings.Contains(r, "ran") {
			t.Errorf("reason %q counts Codex's hook runs against Claude Code's hooks", r)
		}
	}
}
