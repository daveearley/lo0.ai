package opencode_test

import (
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/harness/opencode"
	"lo0.ai/lo0/internal/usage"
)

func TestFeatures(t *testing.T) {
	app := filepath.FromSlash("/code/app")
	both := filepath.FromSlash("/code/both")
	inv := setup.Inventory{
		Instructions: []setup.InstructionFile{
			{Origin: setup.Origin{Scope: setup.ScopeUser, Path: filepath.FromSlash("/home/u/.claude/CLAUDE.md")}, Kind: setup.InstructionClaudeMD},
			{Origin: setup.Origin{Scope: setup.ScopeProject, Project: app, Path: filepath.Join(app, "CLAUDE.md")}, Kind: setup.InstructionClaudeMD},
			{Origin: setup.Origin{Scope: setup.ScopeProject, Project: app, Path: filepath.Join(app, ".claude", "CLAUDE.md")}, Kind: setup.InstructionClaudeMD},
			{Origin: setup.Origin{Scope: setup.ScopeProject, Project: both, Path: filepath.Join(both, "CLAUDE.md")}, Kind: setup.InstructionClaudeMD},
			{Origin: setup.Origin{Scope: setup.ScopeProject, Project: both, Path: filepath.Join(both, "AGENTS.md")}, Kind: setup.InstructionAgentsMD},
			{Origin: setup.Origin{Scope: setup.ScopeProject, Project: app, Path: filepath.Join(app, ".claude", "rules", "go.md")}, Kind: setup.InstructionRule},
			{Origin: setup.Origin{Scope: setup.ScopeLocal, Project: app, Path: filepath.Join(app, "CLAUDE.local.md")}, Kind: setup.InstructionLocalMD},
		},
		MCPServers: []setup.MCPServer{
			{Name: "postgres", Transport: setup.TransportStdio},
			{Name: "stitch", Transport: setup.TransportHTTP},
			{Name: "legacy", Transport: setup.TransportSSE},
			{Origin: setup.Origin{Scope: setup.ScopePlugin, Plugin: "marketing"}, Name: "ahrefs", Transport: setup.TransportHTTP},
		},
		Hooks: []setup.Hook{
			{Event: "PreToolUse", Type: "command"},
			{Event: "PermissionRequest", Type: "command"},
			{Event: "UserPromptSubmit", Type: "command"},
			{Event: "Stop", Type: "prompt"},
			{Event: "Notification", Type: "http"},
		},
		Extensions: []setup.Extension{
			{Kind: setup.ExtensionCommand, Name: "review"},
			{Kind: setup.ExtensionAgent, Name: "tester"},
			{Origin: setup.Origin{Scope: setup.ScopeProject}, Kind: setup.ExtensionSkill, Name: "release"},
			{Origin: setup.Origin{Scope: setup.ScopePlugin}, Kind: setup.ExtensionSkill, Name: "deploy"},
		},
		Plugins:     []setup.Plugin{{Name: "marketing", Enabled: true}, {Name: "off"}},
		Permissions: []setup.Permissions{{Origin: setup.Origin{Path: "settings.json"}, Allow: 3}, {Origin: setup.Origin{Path: "empty.json"}}},
		Settings: []setup.SettingsFile{
			{Keys: []string{"env", "hooks", "statusLine"}},
			{Keys: []string{"env", "outputStyle"}},
		},
	}
	history := usage.Profile{MCPServers: []usage.Tally{
		{Name: "postgres", Count: 10},
		{Name: "plugin_marketing_ahrefs", Count: 5},
		{Name: "Claude_Browser", Count: 400},
		{Name: "ccd_session", Count: 30},
		{Name: "917d60b0-8a5b-4ceb-8448-c4cd013721a8", Count: 12, Tools: []string{"get_thread", "search_threads"}},
	}}

	same := map[string]harness.Status{
		"instructions:/code/both/AGENTS.md":          harness.StatusNative,
		"instructions:/code/app/.claude/CLAUDE.md":   harness.StatusTranslate,
		"instructions:/code/both/CLAUDE.md":          harness.StatusTranslate,
		"instructions:/code/app/.claude/rules/go.md": harness.StatusTranslate,
		"instructions:/code/app/CLAUDE.local.md":     harness.StatusTranslate,
		"mcp:postgres":                               harness.StatusTranslate,
		"mcp:stitch":                                 harness.StatusTranslate,
		"mcp:legacy":                                 harness.StatusPartial,
		"mcp:ahrefs":                                 harness.StatusTranslate,
		"app-tools:Claude_Browser":                   harness.StatusPartial,
		"app-tools:Gmail connector":                  harness.StatusLost,
		"hooks:PreToolUse":                           harness.StatusTranslate,
		"hooks:Stop":                                 harness.StatusLost,
		"hooks:Notification":                         harness.StatusLost,
		"commands:review":                            harness.StatusTranslate,
		"subagents:tester":                           harness.StatusTranslate,
		"skills:release":                             harness.StatusNative,
		"skills:deploy":                              harness.StatusTranslate,
		"plugins:marketing":                          harness.StatusPartial,
		"permissions:settings.json":                  harness.StatusTranslate,
		"settings:env":                               harness.StatusTranslate,
		"settings:statusLine":                        harness.StatusLost,
		"settings:outputStyle":                       harness.StatusLost,
	}
	for _, tt := range []struct {
		name      string
		installed harness.Installation
		want      map[string]harness.Status
	}{
		{
			// OpenCode 1 reads CLAUDE.md when there is no AGENTS.md beside it,
			// and its plugin events can watch a permission but not decide it.
			"OpenCode 1",
			harness.Installation{Installed: true, Version: "1.19.5"},
			map[string]harness.Status{
				"instructions:/home/u/.claude/CLAUDE.md": harness.StatusNative,
				"instructions:/code/app/CLAUDE.md":       harness.StatusNative,
				"hooks:PermissionRequest":                harness.StatusPartial,
				"hooks:UserPromptSubmit":                 harness.StatusLost,
			},
		},
		{
			// OpenCode 2 reads AGENTS.md only, and its plugins can decide a
			// permission and add to a prompt.
			"OpenCode 2",
			harness.Installation{Installed: true, Version: "2.2.5"},
			map[string]harness.Status{
				"instructions:/home/u/.claude/CLAUDE.md": harness.StatusTranslate,
				"instructions:/code/app/CLAUDE.md":       harness.StatusTranslate,
				"hooks:PermissionRequest":                harness.StatusTranslate,
				"hooks:UserPromptSubmit":                 harness.StatusPartial,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			want := maps.Clone(same)
			maps.Copy(want, tt.want)
			checkFeatures(t, (opencode.Harness{}).Features(inv, history, tt.installed), want)
		})
	}
}

func checkFeatures(t *testing.T, features []harness.Feature, want map[string]harness.Status) {
	t.Helper()
	got := map[string]harness.Status{}
	for _, f := range features {
		got[string(f.Area)+":"+filepath.ToSlash(f.Name)] = f.Status
		if f.How == "" {
			t.Errorf("%s %s has no explanation", f.Area, f.Name)
		}
	}
	for key, status := range want {
		if got[key] != status {
			t.Errorf("%s = %q, want %q", key, got[key], status)
		}
	}
	if len(got) != len(want) {
		for key := range got {
			if _, ok := want[key]; !ok {
				t.Errorf("unexpected feature %s = %s", key, got[key])
			}
		}
	}
}

func TestSummarize(t *testing.T) {
	features := []harness.Feature{
		{Area: harness.AreaHooks, Status: harness.StatusTranslate},
		{Area: harness.AreaHooks, Status: harness.StatusLost},
		{Area: harness.AreaSkills, Status: harness.StatusNative},
		{Area: harness.AreaMCP, Status: harness.StatusPartial},
	}
	all := harness.Summarize(features, "")
	if all != (harness.Carryover{Native: 1, Translate: 1, Partial: 1, Lost: 1}) || all.CarriedShare() != 0.75 {
		t.Errorf("Summarize(all) = %+v, share %v", all, all.CarriedShare())
	}
	if hooks := harness.Summarize(features, harness.AreaHooks); hooks.Translate != 1 || hooks.Lost != 1 || hooks.Native != 0 {
		t.Errorf("Summarize(hooks) = %+v", hooks)
	}
}

func TestBrowserToolsPointAtAConfiguredReplacement(t *testing.T) {
	history := usage.Profile{MCPServers: []usage.Tally{{Name: "claude-in-chrome", Count: 10}}}
	without := (opencode.Harness{}).Features(setup.Inventory{}, history, harness.Installation{})
	with := (opencode.Harness{}).Features(setup.Inventory{MCPServers: []setup.MCPServer{{Name: "chrome-devtools", Transport: setup.TransportStdio}}}, history, harness.Installation{})
	if len(without) != 1 || !strings.Contains(without[0].How, "@playwright/mcp") {
		t.Errorf("without a browser server: %+v", without)
	}
	var app harness.Feature
	for _, f := range with {
		if f.Area == harness.AreaAppTools {
			app = f
		}
	}
	if app.Status != harness.StatusPartial || !strings.Contains(app.How, "chrome-devtools MCP server you already have") {
		t.Errorf("with chrome-devtools configured: %+v", app)
	}
}

func TestFeaturesFollowTheInstalledRelease(t *testing.T) {
	user := setup.Inventory{Instructions: []setup.InstructionFile{
		{Origin: setup.Origin{Scope: setup.ScopeUser, Path: "/home/u/.claude/CLAUDE.md"}, Kind: setup.InstructionClaudeMD},
	}}
	for _, tt := range []struct {
		name      string
		installed harness.Installation
		want      harness.Status
	}{
		{"not installed, so lo0 installs OpenCode 2", harness.Installation{}, harness.StatusTranslate},
		{"OpenCode 1", harness.Installation{Installed: true, Version: "1.14.2"}, harness.StatusNative},
		{"an OpenCode 1 prerelease", harness.Installation{Installed: true, Version: "1.20.0-beta.1"}, harness.StatusNative},
		{"OpenCode 2", harness.Installation{Installed: true, Version: "2.0.0"}, harness.StatusTranslate},
		{"a later major release", harness.Installation{Installed: true, Version: "3.0.1"}, harness.StatusTranslate},
		{"a version that could not be read", harness.Installation{Installed: true}, harness.StatusTranslate},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := (opencode.Harness{}).Features(user, usage.Profile{}, tt.installed)
			if len(f) != 1 || f[0].Status != tt.want {
				t.Errorf("features = %+v, want CLAUDE.md %s", f, tt.want)
			}
		})
	}
}

func TestFeaturesNameOpenCode2Keys(t *testing.T) {
	inv := setup.Inventory{
		Instructions: []setup.InstructionFile{{Origin: setup.Origin{Scope: setup.ScopeUser, Path: "/home/u/.claude/CLAUDE.md"}, Kind: setup.InstructionClaudeMD}},
		MCPServers:   []setup.MCPServer{{Origin: setup.Origin{Scope: setup.ScopeUser}, Name: "postgres", Transport: setup.TransportStdio}},
		Hooks:        []setup.Hook{{Event: "PreToolUse", Type: "command"}},
		Extensions:   []setup.Extension{{Kind: setup.ExtensionAgent, Name: "tester"}},
		Permissions:  []setup.Permissions{{Origin: setup.Origin{Path: "settings.json"}, Deny: 1}},
		Settings:     []setup.SettingsFile{{Keys: []string{"env"}}},
	}
	how := map[harness.Area]string{}
	migrated := map[harness.Area]bool{}
	for _, f := range (opencode.Harness{}).Features(inv, usage.Profile{}, harness.Installation{Installed: true, Version: "2.2.5"}) {
		how[f.Area], migrated[f.Area] = f.How, f.Migrated
	}
	for area, want := range map[harness.Area]string{
		harness.AreaInstructions: "OpenCode 2 reads AGENTS.md, not CLAUDE.md",
		harness.AreaMCP:          `"mcp.servers" entry of type local`,
		harness.AreaHooks:        `tool "execute.before"`,
		harness.AreaSubagents:    "permissions rules",
		harness.AreaPermissions:  `"permissions" rules`,
		harness.AreaSettings:     `shell "create.before"`,
	} {
		if !strings.Contains(how[area], want) {
			t.Errorf("%s: how = %q, want it to mention %q", area, how[area], want)
		}
	}
	if !migrated[harness.AreaInstructions] {
		t.Error("the user's CLAUDE.md is not marked as moved by lo0, although the plan adds it to the instructions")
	}
}
