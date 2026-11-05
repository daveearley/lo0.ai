package opencode_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/claudecode"
	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/harness/opencode"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/migrate/migratetest"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/system/systemtest"
)

const home = "/home/u"

func p(rel string) string { return filepath.Join(home, filepath.FromSlash(rel)) }

func served() migrate.Served {
	return migrate.Served{
		Runtime: runtimes.Ollama, RuntimeName: "Ollama", BaseURL: "http://127.0.0.1:11434",
		Model: "lo0/qwen3-coder-30b-a3b:64k", DisplayName: "Qwen3-Coder-30B-A3B", Context: 65536, Token: "ollama",
	}
}

func request() migrate.Request {
	user := func(rel string) setup.Origin { return setup.Origin{Scope: setup.ScopeUser, Path: p(rel)} }
	return migrate.Request{
		GOOS: "linux", Home: home, Getenv: func(string) string { return "" },
		LookPath: (&systemtest.Runner{Paths: map[string]string{"opencode": "/usr/bin/opencode"}}).LookPath,
		Layout:   claudecode.Layout{ConfigDir: p(".claude"), GlobalConfig: p(".claude.json")},
		Setup: setup.Inventory{
			Instructions: []setup.InstructionFile{
				{Origin: user(".claude/CLAUDE.md"), Kind: setup.InstructionClaudeMD},
				{Origin: user(".claude/rules/go.md"), Kind: setup.InstructionRule},
				{Origin: setup.Origin{Scope: setup.ScopeProject, Path: "/code/app/.claude/rules/x.md"}, Kind: setup.InstructionRule},
			},
			MCPServers: []setup.MCPServer{
				{Origin: user(".claude.json"), Name: "postgres", Transport: setup.TransportStdio},
				{Origin: user(".claude.json"), Name: "linear", Transport: setup.TransportHTTP},
				{Origin: setup.Origin{Scope: setup.ScopeProject, Path: "/code/app/.mcp.json"}, Name: "local-only", Transport: setup.TransportStdio},
			},
			Extensions: []setup.Extension{
				{Origin: user(".claude/commands/review.md"), Kind: setup.ExtensionCommand, Name: "review"},
				{Origin: user(".claude/commands/git/ship.md"), Kind: setup.ExtensionCommand, Name: "git:ship"},
				{Origin: user(".claude/agents/tester.md"), Kind: setup.ExtensionAgent, Name: "tester"},
				{Origin: setup.Origin{Scope: setup.ScopeProject, Path: "/code/app/.claude/commands/p.md"}, Kind: setup.ExtensionCommand, Name: "p"},
				{Origin: setup.Origin{Scope: setup.ScopePlugin, Path: p(".claude/plugins/x/skills/deploy/SKILL.md")}, Kind: setup.ExtensionSkill, Name: "deploy"},
				{Origin: user(".claude/skills/own/SKILL.md"), Kind: setup.ExtensionSkill, Name: "own"},
			},
		},
	}
}

// opencode1 is the request with OpenCode 1 installed. A plain request() has
// no installation recorded, so it plans for OpenCode 2.
func opencode1(req migrate.Request) migrate.Request {
	req.Target = harness.Installation{Installed: true, Version: "1.19.5"}
	return req
}

func files() *migratetest.Files {
	return migratetest.NewFiles(map[string]string{
		p(".claude.json"): `{"mcpServers":{
			"postgres":{"command":"npx","args":["-y","@pg/mcp","${DATABASE_URL}"],"env":{"PGPASSWORD":"hunter2"}},
			"linear":{"type":"http","url":"https://mcp.linear.app/mcp","headers":{"Authorization":"Bearer ${LINEAR_TOKEN}"}}
		},"oauthAccount":{"emailAddress":"u@example.com"}}`,
		p(".claude/commands/review.md"):                    "---\ndescription: Review the diff\nallowed-tools: Bash(git diff:*), Read\nmodel: haiku\n---\nReview $ARGUMENTS\n",
		p(".claude/commands/git/ship.md"):                  "Ship it\n",
		p(".claude/agents/tester.md"):                      "---\nname: tester\ndescription: Runs the tests\ntools: Read, Bash, Grep\nmodel: sonnet\ncolor: green\n---\nYou run tests.\n",
		p(".claude/plugins/x/skills/deploy/SKILL.md"):      "---\nname: deploy\ndescription: Deploys\n---\nSteps\n",
		p(".claude/plugins/x/skills/deploy/scripts/go.sh"): "#!/bin/sh\n",
		p(".config/opencode/opencode.json"):                `{"theme":"tokyonight","mcp":{"linear":{"type":"remote","url":"https://mine"}},"instructions":["CONTRIBUTING.md"]}`,
		p(".config/opencode/commands/review.md"):           "an older copy\n",
		"/code/app/.claude/commands/p.md":                  "project command\n",
	})
}

func apply(t *testing.T, req migrate.Request, f *migratetest.Files) migrate.Setup {
	t.Helper()
	set, err := opencode.Harness{}.Plan(req, served())
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

func TestPlanMergesTheConfigurationForOpenCode1(t *testing.T) {
	f := files()
	set := apply(t, opencode1(request()), f)
	data, _ := f.Get(p(".config/opencode/opencode.json"))
	var cfg struct {
		Schema   string `json:"$schema"`
		Theme    string `json:"theme"`
		Model    string `json:"model"`
		Provider map[string]struct {
			NPM     string            `json:"npm"`
			Options map[string]string `json:"options"`
			Models  map[string]struct {
				Limit struct{ Context, Output int } `json:"limit"`
			} `json:"models"`
		} `json:"provider"`
		MCP          map[string]map[string]any `json:"mcp"`
		Instructions []string                  `json:"instructions"`
	}
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatalf("opencode.json is not JSON: %v\n%s", err, data)
	}
	if cfg.Theme != "tokyonight" || cfg.Schema == "" || cfg.Model != "ollama/lo0/qwen3-coder-30b-a3b:64k" {
		t.Errorf("config = %+v", cfg)
	}
	prov := cfg.Provider["ollama"]
	if prov.NPM != "@ai-sdk/openai-compatible" || prov.Options["baseURL"] != "http://127.0.0.1:11434/v1" ||
		prov.Models["lo0/qwen3-coder-30b-a3b:64k"].Limit.Context != 65536 {
		t.Errorf("provider = %+v", prov)
	}
	pg := cfg.MCP["postgres"]
	cmd, _ := json.Marshal(pg["command"])
	env, _ := json.Marshal(pg["environment"])
	if pg["type"] != "local" || string(cmd) != `["npx","-y","@pg/mcp","{env:DATABASE_URL}"]` || string(env) != `{"PGPASSWORD":"hunter2"}` {
		t.Errorf("postgres = %v", pg)
	}
	if cfg.MCP["linear"]["url"] != "https://mine" {
		t.Errorf("an MCP server the user already had was replaced: %v", cfg.MCP["linear"])
	}
	if _, ok := cfg.MCP["local-only"]; ok {
		t.Error("a project MCP server was added to the global config")
	}
	if pg["enabled"] != true {
		t.Errorf("postgres = %v, want it enabled, as OpenCode 1 expects", pg)
	}
	// OpenCode 1 reads ~/.claude/CLAUDE.md itself.
	if !slices.Equal(cfg.Instructions, []string{"CONTRIBUTING.md", p(".claude/rules/go.md")}) {
		t.Errorf("instructions = %q", cfg.Instructions)
	}
	if _, ok := f.Get(p(".config/opencode/opencode.json.lo0-backup-20261104-183000")); !ok {
		t.Error("the existing opencode.json was not backed up")
	}
	if !strings.HasPrefix(set.Then[0], "Run opencode") {
		t.Errorf("then = %q", set.Then)
	}
}

func TestPlanNeverShowsSecretValues(t *testing.T) {
	set, err := opencode.Harness{}.Plan(request(), served())
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

func TestPlanMergesTheConfigurationForOpenCode2(t *testing.T) {
	f := files()
	_ = f.WriteFile(p(".config/opencode/opencode.json"), []byte(`{"theme":"tokyonight",
		"provider":{"ollama":{"npm":"@ai-sdk/openai-compatible"},"openrouter":{"name":"OpenRouter"}},
		"mcp":{"linear":{"type":"remote","url":"https://mine"},"servers":{"sentry":{"type":"remote","url":"https://sentry"}}},
		"instructions":["CONTRIBUTING.md"]}`), 0o644)
	set := apply(t, request(), f)
	data, _ := f.Get(p(".config/opencode/opencode.json"))
	var cfg struct {
		Theme     string                    `json:"theme"`
		Model     string                    `json:"model"`
		Provider  map[string]map[string]any `json:"provider"`
		Providers map[string]struct {
			Package  string            `json:"package"`
			Settings map[string]string `json:"settings"`
			Models   map[string]struct {
				Limit struct{ Context, Output int } `json:"limit"`
			} `json:"models"`
		} `json:"providers"`
		MCP struct {
			Servers map[string]map[string]any `json:"servers"`
			Linear  map[string]any            `json:"linear"`
		} `json:"mcp"`
		Instructions []string `json:"instructions"`
	}
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatalf("opencode.json is not JSON: %v\n%s", err, data)
	}
	if cfg.Theme != "tokyonight" || cfg.Model != "ollama/lo0/qwen3-coder-30b-a3b:64k" {
		t.Errorf("config = %+v", cfg)
	}
	prov := cfg.Providers["ollama"]
	if prov.Package != "@opencode/ai/providers/openai-compatible" || prov.Settings["baseURL"] != "http://127.0.0.1:11434/v1" ||
		prov.Models["lo0/qwen3-coder-30b-a3b:64k"].Limit.Context != 65536 {
		t.Errorf("providers.ollama = %+v", prov)
	}
	if _, ok := cfg.Provider["ollama"]; ok || cfg.Provider["openrouter"] == nil {
		t.Errorf("provider = %v, want lo0's OpenCode 1 entry gone and the user's kept", cfg.Provider)
	}
	pg := cfg.MCP.Servers["postgres"]
	cmd, _ := json.Marshal(pg["command"])
	if pg["type"] != "local" || string(cmd) != `["npx","-y","@pg/mcp","{env:DATABASE_URL}"]` {
		t.Errorf("mcp.servers.postgres = %v", pg)
	}
	if _, ok := pg["enabled"]; ok {
		t.Errorf("mcp.servers.postgres = %v; OpenCode 2 has no enabled field", pg)
	}
	if cfg.MCP.Linear["url"] != "https://mine" || cfg.MCP.Servers["linear"] != nil || cfg.MCP.Servers["sentry"] == nil {
		t.Errorf("mcp = %+v, want the user's servers kept and linear not added twice", cfg.MCP)
	}
	// OpenCode 2 reads AGENTS.md only, so the user's CLAUDE.md is listed.
	if !slices.Equal(cfg.Instructions, []string{"CONTRIBUTING.md", p(".claude/CLAUDE.md"), p(".claude/rules/go.md")}) {
		t.Errorf("instructions = %q", cfg.Instructions)
	}
	detail := strings.Join(set.Steps[0].Detail, "\n")
	for _, want := range []string{`"providers.ollama"`, `"mcp.servers": postgres, linear`, `"instructions": 2 files`} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail = %q, want %q", detail, want)
		}
	}
}

func TestPlanConvertsCommandsAndSubagents(t *testing.T) {
	f := files()
	apply(t, opencode1(request()), f)
	review, _ := f.Get(p(".config/opencode/commands/review.md"))
	if review != "---\ndescription: Review the diff\n---\nReview $ARGUMENTS\n" {
		t.Errorf("review command =\n%s", review)
	}
	if _, ok := f.Get(p(".config/opencode/commands/review.md.lo0-backup-20261104-183000")); !ok {
		t.Error("the replaced command was not backed up")
	}
	if ship, _ := f.Get(p(".config/opencode/commands/git/ship.md")); ship != "Ship it\n" {
		t.Errorf("nested command = %q", ship)
	}
	if _, ok := f.Get(p(".config/opencode/commands/p.md")); ok {
		t.Error("a project command was copied to the global directory")
	}
	tester, _ := f.Get(p(".config/opencode/agents/tester.md"))
	want := "---\ndescription: Runs the tests\nmode: subagent\npermission:\n  edit: deny\n  webfetch: deny\n---\nYou run tests.\n"
	if tester != want {
		t.Errorf("subagent =\n%s\nwant\n%s", tester, want)
	}
}

func TestPlanConvertsSubagentsForOpenCode2(t *testing.T) {
	f := files()
	apply(t, request(), f)
	tester, _ := f.Get(p(".config/opencode/agents/tester.md"))
	want := "---\ndescription: Runs the tests\nmode: subagent\npermissions:\n" +
		"  - action: edit\n    resource: \"*\"\n    effect: deny\n" +
		"  - action: webfetch\n    resource: \"*\"\n    effect: deny\n" +
		"---\nYou run tests.\n"
	if tester != want {
		t.Errorf("subagent =\n%s\nwant\n%s", tester, want)
	}
}

func TestPlanCopiesPluginSkills(t *testing.T) {
	f := files()
	apply(t, request(), f)
	if skill, _ := f.Get(p(".config/opencode/skills/deploy/SKILL.md")); !strings.Contains(skill, "name: deploy") {
		t.Errorf("skill = %q; files %v", skill, f.Names())
	}
	if _, ok := f.Get(p(".config/opencode/skills/deploy/scripts/go.sh")); !ok {
		t.Error("the skill's scripts were not copied")
	}
	if _, ok := f.Get(p(".config/opencode/skills/own/SKILL.md")); ok {
		t.Error("a user skill was copied although OpenCode reads ~/.claude/skills itself")
	}
}

// A skill's name comes from the plugin's SKILL.md, so a name that is a path
// must not take the copy outside the skills folder.
func TestPlanKeepsPluginSkillsInsideTheSkillsFolder(t *testing.T) {
	req := request()
	for i, e := range req.Setup.Extensions {
		if e.Name == "deploy" {
			req.Setup.Extensions[i].Name = "../../../.local/bin"
		}
	}
	f := files()
	apply(t, req, f)
	if _, ok := f.Get(p(".config/opencode/skills/deploy/scripts/go.sh")); !ok {
		t.Errorf("the skill was not copied under its folder name; files %v", f.Names())
	}
	for _, name := range f.Names() {
		if strings.Contains(name, ".local/bin") {
			t.Errorf("wrote %s outside the skills folder", name)
		}
	}
}

func TestPlanAsksToInstallOpenCode(t *testing.T) {
	req := request()
	req.LookPath = (&systemtest.Runner{}).LookPath
	set, _ := opencode.Harness{}.Plan(req, served())
	if set.Steps[0].Kind != migrate.KindManual || set.Steps[0].Title != "Install OpenCode" {
		t.Errorf("first step = %+v", set.Steps[0])
	}
	runner := &systemtest.Runner{Paths: map[string]string{"opencode": "/usr/bin/opencode"}}
	if err := set.Steps[0].Run(t.Context(), migrate.Effects{Runner: runner}, func(int64, int64) {}); err != nil {
		t.Errorf("the install step does not notice OpenCode once installed: %v", err)
	}
}

func TestPlanRefusesToRewriteAConfigWithComments(t *testing.T) {
	f := migratetest.NewFiles(map[string]string{p(".config/opencode/opencode.json"): "{\n  // mine\n}\n"})
	set, _ := opencode.Harness{}.Plan(request(), served())
	err := set.Steps[0].Run(t.Context(), migrate.Effects{Files: f, Open: f.Open, Now: migratetest.Clock}, func(int64, int64) {})
	if err == nil || !strings.Contains(err.Error(), "without comments") {
		t.Errorf("err = %v", err)
	}
	if got, _ := f.Get(p(".config/opencode/opencode.json")); !strings.Contains(got, "// mine") {
		t.Error("the file was changed")
	}
}

func TestPlanFollowsXDGConfigHome(t *testing.T) {
	req := request()
	req.Getenv = func(k string) string {
		if k == "XDG_CONFIG_HOME" {
			return "/xdg"
		}
		return ""
	}
	set, _ := opencode.Harness{}.Plan(req, served())
	if got := set.Steps[0].Detail[0]; got != filepath.Join("/xdg", "opencode", "opencode.json") {
		t.Errorf("config path = %q", got)
	}
}
