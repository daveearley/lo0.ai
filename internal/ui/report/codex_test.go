package report_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/claudecode/setup"
	codexsetup "lo0.ai/lo0/internal/codex/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/harness/claude"
	codexharness "lo0.ai/lo0/internal/harness/codex"
	"lo0.ai/lo0/internal/harness/opencode"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/ui/theme"
	"lo0.ai/lo0/internal/usage"
)

func sampleCodexSetup() codexsetup.Inventory {
	app := under("code/app")
	user := func(rel string) codexsetup.Origin {
		return codexsetup.Origin{Scope: codexsetup.ScopeUser, Path: under(".codex/" + rel)}
	}
	project := codexsetup.Origin{Scope: codexsetup.ScopeProject, Project: app, Path: filepath.Join(app, ".codex", "config.toml")}
	return codexsetup.Inventory{
		Found: true, Path: under(".codex/config.toml"), Model: "gpt-5.5", Provider: "openai",
		Providers: []codexsetup.Provider{{ID: "corp", Name: "Corp"}, {ID: "box", Name: "Box", Local: true}},
		Profiles:  []codexsetup.Profile{{Name: "fast"}, {Name: "deep"}},
		Projects:  []string{app},
		Instructions: []codexsetup.InstructionFile{
			{Origin: user("AGENTS.md"), Lines: 40},
			{Origin: codexsetup.Origin{Scope: codexsetup.ScopeProject, Project: app, Path: filepath.Join(app, "AGENTS.md")}, Lines: 12},
		},
		MCPServers: []codexsetup.MCPServer{
			{Origin: user("config.toml"), Name: "github", Transport: codexsetup.TransportStdio},
			{Origin: project, Name: "db", Transport: codexsetup.TransportHTTP},
		},
		Hooks: []codexsetup.Hook{{Event: "PreToolUse", Type: "command"}, {Event: "Stop", Type: "command"}},
		Extensions: []codexsetup.Extension{
			{Kind: codexsetup.ExtensionSkill, Name: "review"}, {Kind: codexsetup.ExtensionPrompt, Name: "fix"},
		},
		Rules:   []codexsetup.RuleFile{{Rules: 3}},
		Plugins: []codexsetup.Plugin{{Name: "gmail", Enabled: true}, {Name: "pdf"}},
	}
}

func TestRenderCodexSetupGolden(t *testing.T) {
	res := sampleResult()
	res.Codex = sampleCodexSetup()
	res.Usage.Harnesses = []usage.HarnessProfile{
		{Name: "Claude Code", Profile: res.Usage},
		{Name: "Codex", Profile: usage.Profile{Sessions: 39, Format: usage.Format{Versions: []usage.Tally{{Name: "0.155.0"}}}}},
	}
	got := filepath.ToSlash(report.RenderSection(report.SectionSetup, app.Assessment{Scan: res}, report.Options{Width: 80, Theme: theme.Plain(), Home: home()}))
	golden := filepath.Join("testdata", "codex_setup_80.golden")
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
		t.Errorf("setup differs from %s; run go test ./internal/ui/report -update and review the diff\n\ngot:\n%s", golden, got)
	}
	hidden := report.RenderSection(report.SectionSetup, app.Assessment{Scan: res}, report.Options{
		Width: 80, Theme: theme.Plain(), Home: home(),
		Hide: func(s string) string { return strings.ReplaceAll(s, under("code/app"), "project 1") },
	})
	if strings.Contains(hidden, "code/app") || strings.Contains(hidden, filepath.Join("code", "app")) {
		t.Errorf("a project path survived --redact:\n%s", hidden)
	}
}

func TestNoCodexSectionWithoutCodex(t *testing.T) {
	out := report.Render(sampleResult(), report.Options{Width: 80, Theme: theme.Plain(), Home: home()})
	if strings.Contains(out, "Codex setup") {
		t.Errorf("a Codex section was shown with no Codex setup or sessions:\n%s", out)
	}
}

func TestCarryoverSaysWhatStaysBehind(t *testing.T) {
	res, _ := sampleMatch()
	res.Codex = sampleCodexSetup()
	join := func(s string) string { return strings.Join(strings.Fields(s), " ") }

	m := match.Match(res, sampleCatalog(), []match.Harness{claude.Harness{}, opencode.Harness{}, codexharness.Harness{}})
	out := report.RenderMatch(app.Assessment{Scan: res, Match: m}, report.Options{Width: 80, Theme: theme.Plain()})
	if m.TargetID == "codex" {
		t.Fatalf("recommended Codex for a Claude Code history:\n%s", out)
	}
	if strings.Contains(join(out), "stays behind") {
		t.Errorf("every harness takes the Codex setup, but the report says it stays behind:\n%s", out)
	}

	m = match.Match(res, sampleCatalog(), []match.Harness{setupOnly{}, codexharness.Harness{}})
	out = report.RenderMatch(app.Assessment{Scan: res, Match: m}, report.Options{Width: 80, Theme: theme.Plain()})
	if m.TargetID == "codex" {
		t.Fatalf("recommended Codex for a Claude Code history:\n%s", out)
	}
	want := "Your Codex setup, 10 items, stays behind: lo0 does not carry it over to Setup only. Codex on a local model keeps it."
	if !strings.Contains(join(out), want) {
		t.Errorf("the carry-over section does not say the Codex setup stays behind:\n%s", out)
	}
}

// setupOnly is a harness that reads the Claude Code setup but not the
// Codex one.
type setupOnly struct{}

func (setupOnly) ID() harness.ID { return "setup-only" }
func (setupOnly) Name() string   { return "Setup only" }
func (setupOnly) Features(inv setup.Inventory, history usage.Profile, in harness.Installation) []harness.Feature {
	return claude.Harness{}.Features(inv, history, in)
}
