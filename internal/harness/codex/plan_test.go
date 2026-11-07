package codex_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"lo0.ai/lo0/internal/claudecode"
	"lo0.ai/lo0/internal/claudecode/setup"
	codexsetup "lo0.ai/lo0/internal/codex/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/harness/codex"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/migrate/migratetest"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/system/systemtest"
	"lo0.ai/lo0/internal/usage"
)

const home = "/home/u"

func p(rel string) string { return filepath.Join(home, filepath.FromSlash(rel)) }

func served() migrate.Served {
	return migrate.Served{
		Runtime: runtimes.Ollama, RuntimeName: "Ollama", BaseURL: "http://127.0.0.1:11434",
		Model: "lo0/qwen3-coder-30b-a3b:64k", DisplayName: "Qwen3-Coder-30B-A3B", Context: 65536, Token: "ollama",
	}
}

func inventory() setup.Inventory {
	user := func(rel string) setup.Origin { return setup.Origin{Scope: setup.ScopeUser, Path: p(rel)} }
	project := func(path string) setup.Origin {
		return setup.Origin{Scope: setup.ScopeProject, Project: "/code/app", Path: filepath.FromSlash(path)}
	}
	return setup.Inventory{
		Instructions: []setup.InstructionFile{
			{Origin: user(".claude/CLAUDE.md"), Kind: setup.InstructionClaudeMD},
			{Origin: project("/code/app/CLAUDE.md"), Kind: setup.InstructionClaudeMD},
			{Origin: project("/code/app/.claude/rules/x.md"), Kind: setup.InstructionRule},
			{Origin: setup.Origin{Scope: setup.ScopeProject, Project: "/code/both", Path: filepath.FromSlash("/code/both/CLAUDE.md")}, Kind: setup.InstructionClaudeMD},
			{Origin: setup.Origin{Scope: setup.ScopeProject, Project: "/code/both", Path: filepath.FromSlash("/code/both/AGENTS.md")}, Kind: setup.InstructionAgentsMD},
		},
		MCPServers: []setup.MCPServer{
			{Origin: user(".claude.json"), Name: "postgres", Transport: setup.TransportStdio},
			{Origin: user(".claude.json"), Name: "linear", Transport: setup.TransportHTTP},
			{Origin: user(".claude.json"), Name: "old", Transport: setup.TransportSSE},
			{Origin: project("/code/app/.mcp.json"), Name: "local-only", Transport: setup.TransportStdio},
		},
		Hooks: []setup.Hook{
			{Origin: user(".claude/settings.json"), Event: "PreToolUse", Type: "command"},
			{Origin: user(".claude/settings.json"), Event: "Notification", Type: "command"},
			{Origin: user(".claude/settings.json"), Event: "Stop", Type: "prompt"},
			{Origin: user(".claude/settings.json"), Event: "PostToolUse", Type: "http"},
		},
		Extensions: []setup.Extension{
			{Origin: user(".claude/commands/review.md"), Kind: setup.ExtensionCommand, Name: "review"},
			{Origin: user(".claude/agents/tester.md"), Kind: setup.ExtensionAgent, Name: "tester"},
			{Origin: setup.Origin{Scope: setup.ScopePlugin, Path: p(".claude/plugins/x/skills/deploy/SKILL.md")}, Kind: setup.ExtensionSkill, Name: "deploy"},
			{Origin: user(".claude/skills/own/SKILL.md"), Kind: setup.ExtensionSkill, Name: "own"},
			{Origin: project("/code/app/.claude/skills/p/SKILL.md"), Kind: setup.ExtensionSkill, Name: "p"},
		},
		Plugins:     []setup.Plugin{{Name: "on", Enabled: true}, {Name: "off"}},
		Permissions: []setup.Permissions{{Origin: user(".claude/settings.json"), Allow: 2}},
		Settings:    []setup.SettingsFile{{Origin: user(".claude/settings.json"), Keys: []string{"env", "model", "statusLine"}}},
	}
}

func request() migrate.Request {
	req := migrate.Request{
		GOOS: "linux", Home: home, Getenv: func(string) string { return "" },
		LookPath: (&systemtest.Runner{Paths: map[string]string{"codex": "/usr/bin/codex"}}).LookPath,
		Layout:   claudecode.Layout{ConfigDir: p(".claude"), GlobalConfig: p(".claude.json")},
		Setup:    inventory(),
	}
	req.Features = codex.Harness{}.Features(req.Setup, usage.Profile{}, harness.Installation{})
	return req
}

func files() *migratetest.Files {
	return migratetest.NewFiles(map[string]string{
		p(".claude.json"): `{"mcpServers":{
			"postgres":{"command":"npx","args":["-y","@pg/mcp"],"env":{"PGPASSWORD":"hunter2","DATABASE_URL":"${DATABASE_URL}"}},
			"linear":{"type":"http","url":"https://mcp.linear.app/mcp","headers":{"Authorization":"Bearer ${LINEAR_TOKEN}"}},
			"old":{"type":"sse","url":"https://old.example/sse"}
		},"oauthAccount":{"emailAddress":"u@example.com"}}`,
		p(".codex/config.toml"):                            "# mine\nmodel = \"gpt-5.5\"\n\n[mcp_servers.linear]\nurl = \"https://mine\"\n",
		p(".claude/agents/tester.md"):                      "---\nname: tester\ndescription: Runs the tests\ntools: Read, Bash\nmodel: sonnet\n---\nYou run tests.\n",
		p(".claude/skills/own/SKILL.md"):                   "---\nname: own\ndescription: Mine\n---\nSteps\n",
		p(".claude/plugins/x/skills/deploy/SKILL.md"):      "---\nname: deploy\ndescription: Deploys\n---\nSteps\n",
		p(".claude/plugins/x/skills/deploy/scripts/go.sh"): "#!/bin/sh\n",
	})
}

func apply(t *testing.T, req migrate.Request, f *migratetest.Files) migrate.Setup {
	t.Helper()
	set, err := codex.Harness{}.Plan(req, served())
	if err != nil {
		t.Fatal(err)
	}
	fx := migrate.Effects{Files: f, Open: f.Open, Now: migratetest.Clock}
	for _, s := range set.Steps {
		if s.Run == nil || s.Kind == migrate.KindManual {
			continue
		}
		if err := s.Run(t.Context(), fx, func(int64, int64) {}); err != nil {
			t.Fatalf("%s: %v", s.Title, err)
		}
	}
	return set
}

func TestPlanRewritesConfigToml(t *testing.T) {
	f := files()
	set := apply(t, request(), f)
	data, _ := f.Get(p(".codex/config.toml"))
	var cfg struct {
		Model          string `toml:"model"`
		ModelProvider  string `toml:"model_provider"`
		Window         int    `toml:"model_context_window"`
		Fallback       []string
		ModelProviders map[string]map[string]string `toml:"model_providers"`
		MCPServers     map[string]map[string]any    `toml:"mcp_servers"`
	}
	md, err := toml.Decode(data, &cfg)
	if err != nil {
		t.Fatalf("config.toml is not TOML: %v\n%s", err, data)
	}
	if cfg.Model != "lo0/qwen3-coder-30b-a3b:64k" || cfg.ModelProvider != "lo0-ollama" || cfg.Window != 65536 {
		t.Errorf("config = %+v", cfg)
	}
	if got := cfg.ModelProviders["lo0-ollama"]; got["base_url"] != "http://127.0.0.1:11434/v1" || got["name"] != "Ollama (local)" {
		t.Errorf("provider = %v", got)
	}
	if !md.IsDefined("project_doc_fallback_filenames") || !strings.Contains(data, `project_doc_fallback_filenames = ["CLAUDE.md"]`) {
		t.Errorf("no CLAUDE.md fallback:\n%s", data)
	}
	pg := cfg.MCPServers["postgres"]
	if pg["command"] != "npx" || pg["env"].(map[string]any)["PGPASSWORD"] != "hunter2" || !slices.Equal(pg["env_vars"].([]any), []any{"DATABASE_URL"}) {
		t.Errorf("postgres = %v", pg)
	}
	if cfg.MCPServers["linear"]["url"] != "https://mine" {
		t.Errorf("an MCP server the user already had was replaced: %v", cfg.MCPServers["linear"])
	}
	if cfg.MCPServers["old"]["url"] != "https://old.example/sse" {
		t.Errorf("old = %v", cfg.MCPServers["old"])
	}
	if _, ok := cfg.MCPServers["local-only"]; ok {
		t.Error("a project MCP server was added to the user's config")
	}
	if !strings.HasPrefix(data, "# mine\n") {
		t.Errorf("the user's comment was lost:\n%s", data)
	}
	if _, ok := f.Get(p(".codex/config.toml.lo0-backup-20261104-183000")); !ok {
		t.Error("the existing config.toml was not backed up")
	}
	for _, want := range []string{".agents/skills/own/SKILL.md", ".agents/skills/deploy/SKILL.md", ".agents/skills/deploy/scripts/go.sh"} {
		if _, ok := f.Get(p(want)); !ok {
			t.Errorf("%s was not copied", want)
		}
	}
	if _, ok := f.Get(p(".agents/skills/p/SKILL.md")); ok {
		t.Error("a project skill was copied out of the project")
	}
	agent, _ := f.Get(p(".codex/agents/tester.toml"))
	if !strings.Contains(agent, `description = "Runs the tests"`) || !strings.Contains(agent, "You run tests.") {
		t.Errorf("agent:\n%s", agent)
	}
	if !strings.HasPrefix(set.Then[0], "Run codex") {
		t.Errorf("then = %q", set.Then)
	}
}

func TestPlanFollowsCodexHome(t *testing.T) {
	req := request()
	req.Getenv = func(k string) string {
		if k == "CODEX_HOME" {
			return "/srv/codex"
		}
		return ""
	}
	f := files()
	apply(t, req, f)
	if _, ok := f.Get("/srv/codex/config.toml"); !ok {
		t.Errorf("config.toml was not written in CODEX_HOME: %v", f.Names())
	}
	if perm := f.Perm("/srv/codex/config.toml"); perm&0o077 != 0 {
		t.Errorf("config.toml holds MCP secrets but is mode %o", perm)
	}
	if _, ok := f.Get("/srv/codex/agents/tester.toml"); !ok {
		t.Errorf("agents were not written in CODEX_HOME: %v", f.Names())
	}
}

func TestPlanNeverShowsSecretValues(t *testing.T) {
	set, err := codex.Harness{}.Plan(request(), served())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range set.Steps {
		for _, d := range append([]string{s.Title}, s.Detail...) {
			if strings.Contains(d, "hunter2") || strings.Contains(d, "LINEAR_TOKEN") || strings.Contains(d, "example.com") {
				t.Errorf("step %q shows a secret: %q", s.Title, d)
			}
		}
	}
}

func TestPlanInstallsCodexFirstWhenMissing(t *testing.T) {
	req := request()
	req.LookPath = (&systemtest.Runner{}).LookPath
	set, err := codex.Harness{}.Plan(req, served())
	if err != nil {
		t.Fatal(err)
	}
	if set.Steps[0].Kind != migrate.KindManual || set.Steps[0].Title != "Install Codex" {
		t.Errorf("first step = %+v", set.Steps[0])
	}
}

func TestPlanIsIdempotent(t *testing.T) {
	f := files()
	apply(t, request(), f)
	before := map[string]string{}
	for _, n := range f.Names() {
		before[n], _ = f.Get(n)
	}
	set, _ := codex.Harness{}.Plan(request(), served())
	plan := migrate.Settle(migrate.Plan{Steps: set.Steps}, migrate.Effects{Files: f, Open: f.Open, Now: migratetest.Clock})
	for _, s := range plan.Steps {
		if !s.Done {
			t.Errorf("step %q is not done after it ran", s.Title)
		}
	}
	for _, n := range f.Names() {
		if got, _ := f.Get(n); got != before[n] {
			t.Errorf("%s changed when planning again", n)
		}
	}
}

func TestSession(t *testing.T) {
	s, steps, err := codex.Harness{}.Session(request(), served())
	if err != nil || len(steps) != 0 {
		t.Fatalf("Session = %v, %v", steps, err)
	}
	want := []string{
		"-c", `model_providers.lo0-ollama.name="Ollama (local)"`,
		"-c", `model_providers.lo0-ollama.base_url="http://127.0.0.1:11434/v1"`,
		"-c", `model_provider="lo0-ollama"`,
		"-c", `model="lo0/qwen3-coder-30b-a3b:64k"`,
		"-c", "model_context_window=65536",
	}
	if s.Program != "codex" || !slices.Equal(s.Args, want) || len(s.Env) != 0 {
		t.Errorf("session = %+v", s)
	}
	// Codex reads each -c value as TOML.
	for i := 1; i < len(s.Args); i += 2 {
		if _, err := toml.Decode(s.Args[i], new(map[string]any)); err != nil {
			t.Errorf("%s is not a TOML key/value: %v", s.Args[i], err)
		}
	}
}

func TestReinstallsCodexThatDoesNotStart(t *testing.T) {
	tests := []struct {
		name   string
		target harness.Installation
		want   string
	}{
		{"answers --version", harness.Installation{Installed: true, Version: "0.167.0"}, ""},
		{"fails --version", harness.Installation{Installed: true}, "Reinstall Codex"},
		{"not scanned", harness.Installation{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := request()
			req.Target = tt.target
			_, steps, err := codex.Harness{}.Session(req, served())
			if err != nil {
				t.Fatal(err)
			}
			set, err := codex.Harness{}.Plan(req, served())
			if err != nil {
				t.Fatal(err)
			}
			for _, first := range []migrate.Step{at(steps, 0), set.Steps[0]} {
				if (first.Title == "Reinstall Codex") != (tt.want != "") {
					t.Errorf("first step = %q, want %q", first.Title, tt.want)
				}
			}
		})
	}

	req := request()
	req.Target = harness.Installation{Installed: true}
	_, steps, _ := codex.Harness{}.Session(req, served())
	broken := &systemtest.Runner{Commands: map[string]systemtest.Result{
		"codex --version": {Output: "Error: spawn codex ENOENT\n", Err: errors.New("exit status 1")},
	}}
	if err := steps[0].Run(t.Context(), migrate.Effects{Runner: broken}, func(int64, int64) {}); err == nil {
		t.Error("check passed while codex --version fails")
	}
	fixed := &systemtest.Runner{Commands: map[string]systemtest.Result{"codex --version": {Output: "codex-cli 0.167.0\n"}}}
	if err := steps[0].Run(t.Context(), migrate.Effects{Runner: fixed}, func(int64, int64) {}); err != nil {
		t.Errorf("check failed once codex starts: %v", err)
	}
}

func at(steps []migrate.Step, i int) migrate.Step {
	if i < len(steps) {
		return steps[i]
	}
	return migrate.Step{}
}

func TestStatus(t *testing.T) {
	runner := &systemtest.Runner{
		Paths:    map[string]string{"codex": "/usr/bin/codex"},
		Commands: map[string]systemtest.Result{"codex --version": {Output: "codex-cli 0.167.0\n"}},
	}
	tests := []struct {
		name, config string
		want         harness.Use
	}{
		{"no config", "", harness.Use{Command: "codex", Model: "OpenAI"}},
		{"openai", "model = \"gpt-5.5\"\n", harness.Use{Command: "codex", Model: "gpt-5.5"}},
		{"built-in ollama", "model = \"gpt-oss:20b\"\nmodel_provider = \"ollama\"\n", harness.Use{Command: "codex", Model: "gpt-oss:20b through Ollama", Local: true}},
		{
			"lo0's provider", "model = \"lo0/q:64k\"\nmodel_provider = \"lo0-ollama\"\n[model_providers.lo0-ollama]\nname = \"Ollama (local)\"\nbase_url = \"http://127.0.0.1:11434/v1\"\n",
			harness.Use{Command: "codex", Model: "lo0/q:64k through Ollama", Local: true},
		},
		{
			"remote provider", "model = \"m\"\nmodel_provider = \"corp\"\n[model_providers.corp]\nname = \"Corp\"\nbase_url = \"https://llm.corp.example/v1\"\n",
			harness.Use{Command: "codex", Model: "m through Corp"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contents := map[string]string{}
			if tt.config != "" {
				contents[p(".codex/config.toml")] = tt.config
			}
			req := request()
			req.LookPath = runner.LookPath
			in := codex.Harness{}.Status(context.Background(), req, migratetest.NewFiles(contents), runner)
			if !in.Installed || in.Version != "0.167.0" || len(in.Uses) != 1 || in.Uses[0] != tt.want {
				t.Errorf("Status = %+v, want use %+v", in, tt.want)
			}
		})
	}
	req := request()
	req.LookPath = (&systemtest.Runner{}).LookPath
	if in := (codex.Harness{}).Status(t.Context(), req, migratetest.NewFiles(nil), runner); in.Installed {
		t.Errorf("Status without codex = %+v", in)
	}
}

func TestInstall(t *testing.T) {
	req := request()
	if s := (codex.Harness{}).Install(req); s.Kind != migrate.KindInstall || !strings.Contains(s.Detail[0], "chatgpt.com/codex/install.sh") {
		t.Errorf("Install = %+v", s)
	}
	req.GOOS = "windows"
	if s := (codex.Harness{}).Install(req); s.Kind != migrate.KindManual || !strings.Contains(s.Detail[0], "install.ps1") {
		t.Errorf("Install on Windows = %+v", s)
	}
}

func TestProtocol(t *testing.T) {
	if (codex.Harness{}).Protocol() != migrate.Responses {
		t.Error("Codex speaks only the Responses API")
	}
}

func TestFeatures(t *testing.T) {
	fs := codex.Harness{}.Features(inventory(), usage.Profile{}, harness.Installation{})
	got := map[string]harness.Feature{}
	for _, f := range fs {
		got[string(f.Area)+" "+filepath.ToSlash(f.Name)] = f
	}
	check := func(key string, status harness.Status, migrated bool) {
		t.Helper()
		f, ok := got[key]
		if !ok {
			t.Errorf("%s is missing", key)
			return
		}
		if f.Status != status || f.Migrated != migrated || f.How == "" {
			t.Errorf("%s = %+v, want %s migrated=%v", key, f, status, migrated)
		}
	}
	check("instructions /home/u/.claude/CLAUDE.md", harness.StatusTranslate, false)
	check("instructions /code/app/CLAUDE.md", harness.StatusTranslate, true)
	check("instructions /code/both/CLAUDE.md", harness.StatusTranslate, false)
	check("instructions /code/both/AGENTS.md", harness.StatusNative, false)
	check("instructions /code/app/.claude/rules/x.md", harness.StatusTranslate, false)
	check("mcp postgres", harness.StatusTranslate, true)
	check("mcp old", harness.StatusPartial, true)
	check("mcp local-only", harness.StatusTranslate, false)
	check("hooks PreToolUse", harness.StatusTranslate, false)
	check("hooks Notification", harness.StatusLost, false)
	check("hooks Stop", harness.StatusLost, false)
	check("hooks PostToolUse", harness.StatusLost, false)
	check("commands review", harness.StatusTranslate, false)
	check("subagents tester", harness.StatusPartial, true)
	check("skills deploy", harness.StatusTranslate, true)
	check("skills own", harness.StatusTranslate, true)
	check("skills p", harness.StatusTranslate, false)
	check("plugins on", harness.StatusPartial, false)
	check("permissions /home/u/.claude/settings.json", harness.StatusPartial, false)
	check("settings model", harness.StatusTranslate, true)
	check("settings env", harness.StatusTranslate, false)
	if _, ok := got["plugins off"]; ok {
		t.Error("a disabled plugin was mapped")
	}
	if _, ok := got["settings statusLine"]; ok {
		t.Error("statusLine was mapped though lo0 has no rule for it")
	}
}

func TestCodexFeatures(t *testing.T) {
	inv := codexsetup.Inventory{
		Found: true, Model: "gpt-5.5",
		Instructions: []codexsetup.InstructionFile{{Origin: codexsetup.Origin{Path: p(".codex/AGENTS.md")}}},
		MCPServers:   []codexsetup.MCPServer{{Name: "github"}, {Name: "off", Disabled: true}},
		Hooks:        []codexsetup.Hook{{Event: "Stop", Type: "command"}},
		Extensions: []codexsetup.Extension{
			{Kind: codexsetup.ExtensionSkill, Name: "s"}, {Kind: codexsetup.ExtensionPrompt, Name: "fix"}, {Kind: codexsetup.ExtensionAgent, Name: "a"},
		},
		Rules:   []codexsetup.RuleFile{{Origin: codexsetup.Origin{Path: p(".codex/rules/default.rules")}}},
		Plugins: []codexsetup.Plugin{{Name: "gmail", Enabled: true}, {Name: "pdf"}},
	}
	history := usage.Profile{Skills: []usage.Tally{{Name: "s", Count: 4}}, HookRuns: []usage.Tally{{Name: "Stop", Count: 9}}}
	fs := codex.Harness{}.CodexFeatures(inv, setup.Inventory{}, history, harness.Installation{})
	if len(fs) != 9 {
		t.Fatalf("features = %+v", fs)
	}
	for _, f := range fs {
		if f.From != "Codex" {
			t.Errorf("%s has From %q", f.Name, f.From)
		}
		want := harness.StatusNative
		if f.Area == harness.AreaSettings {
			want = harness.StatusTranslate
		}
		if f.Status != want {
			t.Errorf("%s = %s, want %s", f.Name, f.Status, want)
		}
		if (f.Name == "s" && f.Calls != 4) || (f.Name == "Stop" && f.Calls != 9) {
			t.Errorf("%s calls = %d", f.Name, f.Calls)
		}
	}
	if fs := (codex.Harness{}).CodexFeatures(codexsetup.Inventory{}, setup.Inventory{}, usage.Profile{}, harness.Installation{}); len(fs) != 0 {
		t.Errorf("an empty setup gave %+v", fs)
	}
}
