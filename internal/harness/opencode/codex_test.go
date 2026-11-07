package opencode_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	codexsetup "lo0.ai/lo0/internal/codex/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/harness/opencode"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/usage"
)

func codexInventory() codexsetup.Inventory {
	user := func(rel string) codexsetup.Origin {
		return codexsetup.Origin{Scope: codexsetup.ScopeUser, Path: p(rel)}
	}
	project := func(rel string) codexsetup.Origin {
		return codexsetup.Origin{Scope: codexsetup.ScopeProject, Project: "/code/app", Path: filepath.Join("/code/app", filepath.FromSlash(rel))}
	}
	return codexsetup.Inventory{
		Found: true, Path: p(".codex/config.toml"), Model: "gpt-5.5",
		Instructions: []codexsetup.InstructionFile{
			{Origin: user(".codex/AGENTS.md")},
			{Origin: project("AGENTS.md")},
			{Origin: codexsetup.Origin{Scope: codexsetup.ScopeProject, Project: "/code/x", Path: "/code/x/AGENTS.override.md"}, Override: true},
		},
		MCPServers: []codexsetup.MCPServer{
			{Origin: user(".codex/config.toml"), Name: "fs", Transport: codexsetup.TransportStdio},
			{Origin: user(".codex/config.toml"), Name: "docs", Transport: codexsetup.TransportHTTP},
			{Origin: user(".codex/config.toml"), Name: "postgres", Transport: codexsetup.TransportStdio},
			{Origin: user(".codex/config.toml"), Name: "off", Transport: codexsetup.TransportStdio, Disabled: true},
			{Origin: user(".codex/config.toml"), Name: "node_repl", Transport: codexsetup.TransportStdio, App: true},
			{Origin: project(".codex/config.toml"), Name: "local", Transport: codexsetup.TransportStdio},
		},
		Hooks: []codexsetup.Hook{
			{Origin: user(".codex/hooks.json"), Event: "PreToolUse", Type: "command"},
			{Origin: user(".codex/hooks.json"), Event: "SessionEnd", Type: "command"},
			{Origin: user(".codex/hooks.json"), Event: "Stop", Type: "mcp_tool"},
		},
		Extensions: []codexsetup.Extension{
			{Origin: user(".agents/skills/release/SKILL.md"), Kind: codexsetup.ExtensionSkill, Name: "release"},
			{Origin: user(".codex/skills/old/SKILL.md"), Kind: codexsetup.ExtensionSkill, Name: "old"},
			{Origin: project(".codex/skills/proj/SKILL.md"), Kind: codexsetup.ExtensionSkill, Name: "proj"},
			{Origin: user(".codex/prompts/fix.md"), Kind: codexsetup.ExtensionPrompt, Name: "fix"},
			{Origin: user(".codex/prompts/review.md"), Kind: codexsetup.ExtensionPrompt, Name: "review"},
			{Origin: user(".codex/agents/reviewer.toml"), Kind: codexsetup.ExtensionAgent, Name: "reviewer"},
		},
		Rules:   []codexsetup.RuleFile{{Origin: user(".codex/rules/default.rules"), Rules: 1}},
		Plugins: []codexsetup.Plugin{{Name: "gmail", Enabled: true}},
	}
}

func TestCodexFeatures(t *testing.T) {
	claude := request().Setup
	type want struct {
		status   harness.Status
		migrated bool
		how      string
	}
	common := map[string]want{
		"instructions:/home/u/.codex/AGENTS.md":   {harness.StatusTranslate, true, `add to "instructions"`},
		"instructions:/code/app/AGENTS.md":        {harness.StatusNative, false, "read as is"},
		"instructions:/code/x/AGENTS.override.md": {harness.StatusTranslate, false, "not AGENTS.override.md"},
		"mcp:docs":            {harness.StatusTranslate, true, "{env:VAR}"},
		"mcp:postgres":        {harness.StatusTranslate, false, "same name"},
		"mcp:local":           {harness.StatusTranslate, false, "project's opencode.json"},
		"app-tools:node_repl": {harness.StatusLost, false, "works only inside it"},
		"hooks:SessionEnd":    {harness.StatusLost, false, "no equivalent"},
		"hooks:Stop":          {harness.StatusLost, false, "mcp_tool hooks"},
		"skills:release":      {harness.StatusNative, false, ".agents/skills"},
		"skills:old":          {harness.StatusTranslate, true, "~/.config/opencode/skills"},
		"skills:proj":         {harness.StatusTranslate, false, "project's .agents/skills"},
		"commands:fix":        {harness.StatusTranslate, true, "/fix"},
		"commands:review":     {harness.StatusTranslate, false, "same name"},
		"subagents:reviewer":  {harness.StatusTranslate, true, "mode: subagent"},
		"plugins:gmail":       {harness.StatusLost, false, "does not load Codex plugins"},
		"settings:model":      {harness.StatusTranslate, true, "local model"},
	}
	tests := []struct {
		name      string
		installed harness.Installation
		extra     map[string]want
	}{
		{"OpenCode 1", harness.Installation{Installed: true, Version: "1.19.5"}, map[string]want{
			"mcp:fs":           {harness.StatusTranslate, true, `"mcp" entry of type local`},
			"hooks:PreToolUse": {harness.StatusTranslate, false, `"tool.execute.before"`},
			"permissions:/home/u/.codex/rules/default.rules": {harness.StatusTranslate, false, `"permission" bash rule`},
		}},
		{"OpenCode 2", harness.Installation{Installed: true, Version: "2.2.5"}, map[string]want{
			"mcp:fs":           {harness.StatusTranslate, true, `"mcp.servers" entry of type local`},
			"hooks:PreToolUse": {harness.StatusTranslate, false, `tool "execute.before"`},
			"permissions:/home/u/.codex/rules/default.rules": {harness.StatusTranslate, false, `"permissions" shell rule`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := map[string]harness.Feature{}
			for _, f := range (opencode.Harness{}).CodexFeatures(codexInventory(), claude, usage.Profile{}, tt.installed) {
				if f.From != "Codex" {
					t.Errorf("%s has From %q", f.Name, f.From)
				}
				got[string(f.Area)+":"+filepath.ToSlash(f.Name)] = f
			}
			all := map[string]want{}
			for k, v := range common {
				all[k] = v
			}
			for k, v := range tt.extra {
				all[k] = v
			}
			for key, w := range all {
				f, ok := got[key]
				if !ok {
					t.Errorf("%s is missing", key)
					continue
				}
				if f.Status != w.status || f.Migrated != w.migrated || !strings.Contains(f.How, w.how) {
					t.Errorf("%s = %s migrated=%v %q, want %s migrated=%v mentioning %q", key, f.Status, f.Migrated, f.How, w.status, w.migrated, w.how)
				}
			}
			if len(got) != len(all) {
				t.Errorf("got %d features, want %d", len(got), len(all))
			}
		})
	}
}

func codexRequest() migrate.Request {
	req := request()
	req.Codex = codexInventory()
	return req
}

func codexFiles() map[string]string {
	return map[string]string{
		p(".codex/config.toml"): `[mcp_servers.fs]
command = "npx"
args = ["-y", "fs-mcp"]
env = { FS_TOKEN = "SECRET_FS" }
env_vars = ["HOME_EXTRA"]
cwd = "/srv/fs"
[mcp_servers.docs]
url = "https://docs.example/mcp"
env_http_headers = { "X-Key" = "DOCS_KEY" }
[mcp_servers.postgres]
command = "codex-pg"
[mcp_servers.node_repl]
command = "/Applications/ChatGPT.app/Contents/Resources/cua_node/bin/node_repl"
env = { CODEX_HOME = "SECRET_APP_HOME", CODEX_CLI_PATH = "SECRET_APP_CLI" }
`,
		p(".codex/skills/old/SKILL.md"):  "---\nname: old\n---\nOld\n",
		p(".codex/prompts/fix.md"):       "---\ndescription: Fix\nargument-hint: ISSUE\n---\nFix $1\n",
		p(".codex/prompts/review.md"):    "codex review\n",
		p(".codex/agents/reviewer.toml"): "name = \"reviewer\"\ndescription = \"Reviews\"\ndeveloper_instructions = \"Look for bugs.\"\n",
	}
}

func TestPlanCarriesTheCodexSetupOver(t *testing.T) {
	for _, tt := range []struct {
		name string
		req  migrate.Request
		key  string
	}{
		{"OpenCode 1", opencode1(codexRequest()), "mcp"},
		{"OpenCode 2", codexRequest(), "servers"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := files()
			for name, data := range codexFiles() {
				if err := f.WriteFile(name, []byte(data), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			apply(t, tt.req, f)
			data, _ := f.Get(p(".config/opencode/opencode.json"))
			var cfg struct {
				MCP          map[string]json.RawMessage `json:"mcp"`
				Instructions []string                   `json:"instructions"`
			}
			if err := json.Unmarshal([]byte(data), &cfg); err != nil {
				t.Fatal(err)
			}
			servers := cfg.MCP
			if tt.key == "servers" {
				if err := json.Unmarshal(cfg.MCP["servers"], &servers); err != nil {
					t.Fatal(err)
				}
			}
			var fs, docs, pg struct {
				Type        string            `json:"type"`
				Command     []string          `json:"command"`
				Environment map[string]string `json:"environment"`
				Cwd         string            `json:"cwd"`
				URL         string            `json:"url"`
				Headers     map[string]string `json:"headers"`
			}
			_ = json.Unmarshal(servers["fs"], &fs)
			_ = json.Unmarshal(servers["docs"], &docs)
			_ = json.Unmarshal(servers["postgres"], &pg)
			if fs.Type != "local" || !slices.Equal(fs.Command, []string{"npx", "-y", "fs-mcp"}) || fs.Environment["FS_TOKEN"] != "SECRET_FS" || len(fs.Environment) != 1 || fs.Cwd != "/srv/fs" {
				t.Errorf("fs = %+v", fs)
			}
			if docs.Type != "remote" || docs.URL != "https://docs.example/mcp" || docs.Headers["X-Key"] != "{env:DOCS_KEY}" {
				t.Errorf("docs = %+v", docs)
			}
			if slices.Contains(pg.Command, "codex-pg") {
				t.Errorf("the Codex postgres server replaced the Claude Code one: %+v", pg)
			}
			if !slices.Contains(cfg.Instructions, p(".codex/AGENTS.md")) {
				t.Errorf("instructions = %v", cfg.Instructions)
			}
			for path, want := range map[string]string{
				".config/opencode/skills/old/SKILL.md": "---\nname: old\n---\nOld\n",
				".config/opencode/commands/fix.md":     "---\ndescription: Fix\n---\nFix $1\n",
				".config/opencode/agents/reviewer.md":  "---\ndescription: \"Reviews\"\nmode: subagent\n---\nLook for bugs.\n",
			} {
				if got, _ := f.Get(p(path)); got != want {
					t.Errorf("%s =\n%q\nwant\n%q", path, got, want)
				}
			}
			if got, _ := f.Get(p(".config/opencode/commands/review.md")); strings.Contains(got, "codex review") {
				t.Error("the Codex review prompt replaced the Claude Code review command")
			}
		})
	}
}

func TestPlanNeverShowsCodexSecrets(t *testing.T) {
	set, err := opencode.Harness{}.Plan(codexRequest(), served())
	if err != nil {
		t.Fatal(err)
	}
	var text []string
	for _, s := range set.Steps {
		text = append(text, s.Title)
		text = append(text, s.Detail...)
	}
	joined := strings.Join(text, "\n")
	if strings.Contains(joined, "SECRET") {
		t.Errorf("plan shows a secret:\n%s", joined)
	}
	for _, want := range []string{"fs, docs", `"instructions": 3 files`} {
		if !strings.Contains(joined, want) {
			t.Errorf("plan lacks %q:\n%s", want, joined)
		}
	}
}
