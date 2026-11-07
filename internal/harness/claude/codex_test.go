package claude_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/claudecode"
	"lo0.ai/lo0/internal/claudecode/setup"
	codexsetup "lo0.ai/lo0/internal/codex/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/harness/claude"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/migrate/migratetest"
	"lo0.ai/lo0/internal/usage"
)

const home = "/home/u"

func p(rel string) string { return filepath.Join(home, filepath.FromSlash(rel)) }

func codexInventory() codexsetup.Inventory {
	user := func(rel string) codexsetup.Origin {
		return codexsetup.Origin{Scope: codexsetup.ScopeUser, Path: p(rel)}
	}
	project := func(dir, rel string) codexsetup.Origin {
		return codexsetup.Origin{Scope: codexsetup.ScopeProject, Project: dir, Path: filepath.Join(dir, filepath.FromSlash(rel))}
	}
	return codexsetup.Inventory{
		Found: true, Path: p(".codex/config.toml"), Model: "gpt-5.5",
		Instructions: []codexsetup.InstructionFile{
			{Origin: user(".codex/AGENTS.md")},
			{Origin: project("/code/app", "AGENTS.md")},
			{Origin: project("/code/both", "AGENTS.md")},
			{Origin: project("/code/over", "AGENTS.override.md"), Override: true},
			{Origin: project("/code/claude", "AGENTS.md")},
		},
		MCPServers: []codexsetup.MCPServer{
			{Origin: user(".codex/config.toml"), Name: "fs", Transport: codexsetup.TransportStdio},
			{Origin: user(".codex/config.toml"), Name: "docs", Transport: codexsetup.TransportHTTP},
			{Origin: user(".codex/config.toml"), Name: "postgres", Transport: codexsetup.TransportStdio},
			{Origin: user(".codex/config.toml"), Name: "tools", Transport: codexsetup.TransportStdio, Cwd: true},
			{Origin: user(".codex/config.toml"), Name: "off", Transport: codexsetup.TransportStdio, Disabled: true},
			{Origin: user(".codex/config.toml"), Name: "node_repl", Transport: codexsetup.TransportStdio, App: true},
			{Origin: project("/code/app", ".codex/config.toml"), Name: "local", Transport: codexsetup.TransportStdio},
		},
		Hooks: []codexsetup.Hook{
			{Origin: user(".codex/hooks.json"), Event: "PreToolUse", Matcher: "apply_patch|Bash", Type: "command"},
			{Origin: user(".codex/hooks.json"), Event: "PreModelRequest", Type: "command"},
			{Origin: user(".codex/config.toml"), Event: "Stop", Type: "prompt"},
		},
		Extensions: []codexsetup.Extension{
			{Origin: user(".agents/skills/release/SKILL.md"), Kind: codexsetup.ExtensionSkill, Name: "release"},
			{Origin: user(".agents/skills/review/SKILL.md"), Kind: codexsetup.ExtensionSkill, Name: "review"},
			{Origin: project("/code/app", ".agents/skills/app/SKILL.md"), Kind: codexsetup.ExtensionSkill, Name: "app"},
			{Origin: user(".codex/prompts/fix.md"), Kind: codexsetup.ExtensionPrompt, Name: "fix"},
			{Origin: user(".codex/agents/reviewer.toml"), Kind: codexsetup.ExtensionAgent, Name: "reviewer"},
			{Origin: user(".codex/agents/bad.toml"), Kind: codexsetup.ExtensionAgent, Name: "../bad"},
		},
		Rules:   []codexsetup.RuleFile{{Origin: user(".codex/rules/default.rules"), Rules: 2}},
		Plugins: []codexsetup.Plugin{{Name: "gmail", Enabled: true}, {Name: "pdf"}},
	}
}

func claudeInventory() setup.Inventory {
	return setup.Inventory{
		Instructions: []setup.InstructionFile{
			{Origin: setup.Origin{Scope: setup.ScopeProject, Project: "/code/both", Path: "/code/both/CLAUDE.md"}, Kind: setup.InstructionClaudeMD},
			{Origin: setup.Origin{Scope: setup.ScopeProject, Project: "/code/claude", Path: "/code/claude/AGENTS.md"}, Kind: setup.InstructionAgentsMD},
		},
		MCPServers: []setup.MCPServer{{Origin: setup.Origin{Scope: setup.ScopeUser}, Name: "postgres", Transport: setup.TransportStdio}},
		Extensions: []setup.Extension{{Origin: setup.Origin{Scope: setup.ScopeUser, Path: p(".claude/skills/review/SKILL.md")}, Kind: setup.ExtensionSkill, Name: "review"}},
	}
}

func TestCodexFeatures(t *testing.T) {
	type want struct {
		status   harness.Status
		migrated bool
		how      string
	}
	tests := []struct {
		name      string
		installed harness.Installation
		want      map[string]want
	}{
		{
			name:      "a Claude Code that reads AGENTS.md",
			installed: harness.Installation{Installed: true, Version: "2.1.300"},
			want: map[string]want{
				"instructions /home/u/.codex/AGENTS.md":      {harness.StatusTranslate, false, "~/.claude/CLAUDE.md"},
				"instructions /code/app/AGENTS.md":           {harness.StatusNative, false, "no CLAUDE.md"},
				"instructions /code/both/AGENTS.md":          {harness.StatusTranslate, false, "add @AGENTS.md to the project's CLAUDE.md"},
				"instructions /code/over/AGENTS.override.md": {harness.StatusTranslate, false, "does not read AGENTS.override.md"},
				"mcp fs":                {harness.StatusTranslate, true, "--mcp-config"},
				"mcp docs":              {harness.StatusTranslate, true, "${VAR}"},
				"mcp postgres":          {harness.StatusTranslate, false, "same name"},
				"mcp tools":             {harness.StatusPartial, true, "no cwd setting"},
				"mcp local":             {harness.StatusTranslate, false, ".mcp.json"},
				"app-tools node_repl":   {harness.StatusLost, false, "works only inside it"},
				"hooks PreToolUse":      {harness.StatusTranslate, false, "Edit and Write, not apply_patch"},
				"hooks PreModelRequest": {harness.StatusLost, false, "no PreModelRequest event"},
				"hooks Stop":            {harness.StatusPartial, false, "Codex skips prompt hooks"},
				"skills release":        {harness.StatusTranslate, true, "~/.claude/skills"},
				"skills review":         {harness.StatusNative, false, "same name"},
				"skills app":            {harness.StatusTranslate, false, "project's .claude/skills"},
				"commands fix":          {harness.StatusTranslate, true, "$1 to $9 become $0 to $8"},
				"subagents reviewer":    {harness.StatusTranslate, true, "~/.claude/agents"},
				"subagents ../bad":      {harness.StatusTranslate, false, "~/.claude/agents"},
				"permissions /home/u/.codex/rules/default.rules": {harness.StatusTranslate, false, "Bash(git push *)"},
				"plugins gmail":  {harness.StatusLost, false, "does not load Codex plugins"},
				"settings model": {harness.StatusTranslate, true, "claude-local"},
			},
		},
		{
			name:      "a Claude Code older than AGENTS.md support",
			installed: harness.Installation{Installed: true, Version: "2.1.221"},
			want: map[string]want{
				"instructions /code/app/AGENTS.md": {harness.StatusTranslate, false, "reads AGENTS.md itself from 2.1.277"},
			},
		},
		{
			name: "no Claude Code, so the newest is installed",
			want: map[string]want{
				"instructions /code/app/AGENTS.md": {harness.StatusNative, false, "no CLAUDE.md"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history := usage.Profile{MCPServers: []usage.Tally{{Name: "node_repl", Count: 12}}}
			features := claude.Harness{}.CodexFeatures(codexInventory(), claudeInventory(), history, tt.installed)
			got := map[string]harness.Feature{}
			for _, f := range features {
				if f.From != "Codex" {
					t.Errorf("%s has From %q", f.Name, f.From)
				}
				got[string(f.Area)+" "+filepath.ToSlash(f.Name)] = f
			}
			for key, w := range tt.want {
				f, ok := got[key]
				if !ok {
					t.Errorf("%s is missing", key)
					continue
				}
				if f.Status != w.status || f.Migrated != w.migrated || !strings.Contains(f.How, w.how) {
					t.Errorf("%s = %s migrated=%v %q, want %s migrated=%v mentioning %q", key, f.Status, f.Migrated, f.How, w.status, w.migrated, w.how)
				}
			}
			if _, ok := got["instructions /code/claude/AGENTS.md"]; ok {
				t.Error("an AGENTS.md the Claude Code setup lists too was counted twice")
			}
			if _, ok := got["mcp off"]; ok {
				t.Error("a disabled MCP server was mapped")
			}
			if _, ok := got["mcp node_repl"]; ok {
				t.Error("the Codex app's own server was mapped as the user's")
			}
			if f := got["app-tools node_repl"]; f.Calls != 12 {
				t.Errorf("node_repl calls = %d, want 12", f.Calls)
			}
			if len(tt.want) > 5 && len(got) != len(tt.want) {
				t.Errorf("got %d features, want %d", len(got), len(tt.want))
			}
		})
	}
}

func TestFeaturesReadAgentsMDByVersion(t *testing.T) {
	inv := claudeInventory()
	for _, tt := range []struct {
		version string
		want    harness.Status
	}{
		{"2.1.221", harness.StatusTranslate},
		{"2.1.277", harness.StatusNative},
		{"", harness.StatusTranslate},
	} {
		var agents harness.Feature
		for _, f := range (claude.Harness{}).Features(inv, usage.Profile{}, harness.Installation{Installed: true, Version: tt.version}) {
			if filepath.Base(f.Name) == "AGENTS.md" {
				agents = f
			}
		}
		if agents.Status != tt.want {
			t.Errorf("Claude Code %q: AGENTS.md = %+v, want %s", tt.version, agents, tt.want)
		}
	}
}

func codexRequest() migrate.Request {
	return migrate.Request{
		GOOS: "linux", Home: home, State: p(".local/state/lo0"), Getenv: func(string) string { return "" },
		Layout: claudecode.Layout{ConfigDir: p(".claude"), GlobalConfig: p(".claude.json")},
		Setup:  claudeInventory(),
		Codex:  codexInventory(),
	}
}

func codexFiles() *migratetest.Files {
	return migratetest.NewFiles(map[string]string{
		p(".codex/config.toml"): `model = "gpt-5.5"
[mcp_servers.fs]
command = "npx"
args = ["-y", "fs-mcp"]
env = { FS_TOKEN = "SECRET_FS" }
env_vars = ["HOME_EXTRA"]
[mcp_servers.docs]
url = "https://docs.example/mcp"
bearer_token_env_var = "DOCS_TOKEN"
http_headers = { "X-Key" = "SECRET_HEADER" }
[mcp_servers.postgres]
command = "pg-mcp"
[mcp_servers.tools]
command = "tools-mcp"
cwd = "/srv/tools"
[mcp_servers.off]
command = "off"
enabled = false
[mcp_servers.node_repl]
command = "/Applications/ChatGPT.app/Contents/Resources/cua_node/bin/node_repl"
env = { CODEX_HOME = "SECRET_APP_HOME", CODEX_CLI_PATH = "SECRET_APP_CLI" }
`,
		p(".agents/skills/release/SKILL.md"):       "---\nname: release\ndescription: Releases\n---\nSteps\n",
		p(".agents/skills/release/scripts/tag.sh"): "#!/bin/sh\n",
		p(".agents/skills/review/SKILL.md"):        "codex review\n",
		p(".claude/skills/review/SKILL.md"):        "claude review\n",
		p(".codex/prompts/fix.md"):                 "---\ndescription: Fix an issue\nargument-hint: ISSUE\n---\nFix $1 in $2, costs $$5, keep $$HOME and $FILE. All: $ARGUMENTS\n",
		p(".codex/agents/reviewer.toml"):           "name = \"reviewer\"\ndescription = \"Reviews: diffs\"\ndeveloper_instructions = \"Look for bugs.\"\nmodel = \"gpt-5.5\"\n",
	})
}

func applyPlan(t *testing.T, req migrate.Request, f *migratetest.Files) migrate.Setup {
	t.Helper()
	set, err := claude.Harness{}.Plan(req, served())
	if err != nil {
		t.Fatal(err)
	}
	fx := migrate.Effects{Files: f, Open: f.Open, Now: migratetest.Clock}
	for _, s := range set.Steps {
		if err := s.Run(t.Context(), fx, func(int64, int64) {}); err != nil {
			t.Fatalf("%s: %v", s.Title, err)
		}
	}
	return set
}

func TestPlanCarriesTheCodexSetupOver(t *testing.T) {
	f := codexFiles()
	set := applyPlan(t, codexRequest(), f)

	mcpPath := p(".local/state/lo0/claude-local/mcp.json")
	data, ok := f.Get(mcpPath)
	if !ok || f.Perm(mcpPath) != 0o600 {
		t.Fatalf("mcp.json not written for the user only: %v", f.Names())
	}
	var cfg struct {
		MCPServers map[string]struct {
			Type    string            `json:"type"`
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatal(err)
	}
	fs, docs := cfg.MCPServers["fs"], cfg.MCPServers["docs"]
	if fs.Type != "stdio" || fs.Command != "npx" || !slices.Equal(fs.Args, []string{"-y", "fs-mcp"}) || fs.Env["FS_TOKEN"] != "SECRET_FS" || len(fs.Env) != 1 {
		t.Errorf("fs = %+v", fs)
	}
	if strings.Contains(data, "SECRET_APP") {
		t.Errorf("mcp.json holds the Codex app's environment:\n%s", data)
	}
	if docs.Type != "http" || docs.URL != "https://docs.example/mcp" || docs.Headers["Authorization"] != "Bearer ${DOCS_TOKEN}" || docs.Headers["X-Key"] != "SECRET_HEADER" {
		t.Errorf("docs = %+v", docs)
	}
	for _, name := range []string{"postgres", "off", "node_repl"} {
		if _, ok := cfg.MCPServers[name]; ok {
			t.Errorf("%s was written, though Claude Code has its own, it is switched off or the Codex app owns it", name)
		}
	}

	script, _ := f.Get(p(".local/bin/claude-local"))
	if !strings.Contains(script, "exec claude '--mcp-config="+mcpPath+"' \"$@\"\n") {
		t.Errorf("launcher does not load the Codex servers:\n%s", script)
	}
	if !slices.ContainsFunc(set.Then, func(s string) bool { return strings.Contains(s, "Codex MCP servers load in claude-local only") }) {
		t.Errorf("then = %q", set.Then)
	}

	for path, want := range map[string]string{
		".claude/skills/release/SKILL.md":       "---\nname: release\ndescription: Releases\n---\nSteps\n",
		".claude/skills/release/scripts/tag.sh": "#!/bin/sh\n",
		".claude/skills/review/SKILL.md":        "claude review\n",
		".claude/commands/fix.md":               "---\ndescription: Fix an issue\nargument-hint: ISSUE\n---\nFix $0 in $1, costs \\$5, keep $HOME and $FILE. All: $ARGUMENTS\n",
		".claude/agents/reviewer.md":            "---\nname: \"reviewer\"\ndescription: \"Reviews: diffs\"\n---\nLook for bugs.\n",
	} {
		if got, _ := f.Get(p(path)); got != want {
			t.Errorf("%s =\n%q\nwant\n%q", path, got, want)
		}
	}
	for _, name := range f.Names() {
		if strings.Contains(name, "bad") {
			t.Errorf("an agent with an unsafe name was written to %s", name)
		}
	}
}

func TestPlanNeverShowsCodexSecrets(t *testing.T) {
	set, err := claude.Harness{}.Plan(codexRequest(), served())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range set.Steps {
		text := s.Title + strings.Join(s.Detail, "\n")
		if strings.Contains(text, "SECRET") {
			t.Errorf("step shows a secret: %s", text)
		}
	}
}

func TestPlanWithoutACodexSetupWritesOnlyTheLauncher(t *testing.T) {
	req := codexRequest()
	req.Codex = codexsetup.Inventory{}
	set, _ := claude.Harness{}.Plan(req, served())
	if len(set.Steps) != 1 || strings.Contains(strings.Join(set.Steps[0].Detail, " "), "--mcp-config") {
		t.Errorf("steps = %+v", set.Steps)
	}
}

func TestWindowsLauncherPassesTheCodexServers(t *testing.T) {
	req := codexRequest()
	req.GOOS, req.Home, req.State = "windows", `C:\Users\u`, `C:\Users\u\AppData\Local\lo0`
	set, _ := claude.Harness{}.Plan(req, served())
	f := migratetest.NewFiles(nil)
	launcher := set.Steps[len(set.Steps)-1]
	if err := launcher.Run(t.Context(), migrate.Effects{Files: f, Now: migratetest.Clock}, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	script, _ := f.Get(launcher.Detail[0])
	want := `claude "--mcp-config=` + filepath.Join(`C:\Users\u\AppData\Local\lo0`, "claude-local", "mcp.json") + `" %*` + "\r\n"
	if !strings.HasSuffix(script, want) {
		t.Errorf("script ends\n%q\nwant\n%q", script[max(0, len(script)-80):], want)
	}
}
