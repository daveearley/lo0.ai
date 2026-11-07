package claude_test

import (
	"path/filepath"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/harness/claude"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/migrate/migratetest"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/usage"
)

func TestFeaturesKeepTheSetup(t *testing.T) {
	inv := setup.Inventory{
		Instructions: []setup.InstructionFile{{Origin: setup.Origin{Path: "/code/app/CLAUDE.md"}, Kind: setup.InstructionClaudeMD}},
		MCPServers:   []setup.MCPServer{{Name: "postgres", Transport: setup.TransportStdio}, {Name: "legacy", Transport: setup.TransportSSE}},
		Hooks:        []setup.Hook{{Event: "UserPromptSubmit", Type: "command"}, {Event: "Stop", Type: "prompt"}, {Event: "Notification", Type: "http"}},
		Extensions: []setup.Extension{
			{Kind: setup.ExtensionCommand, Name: "review"},
			{Kind: setup.ExtensionAgent, Name: "tester"},
			{Origin: setup.Origin{Scope: setup.ScopePlugin}, Kind: setup.ExtensionSkill, Name: "deploy"},
		},
		Plugins:     []setup.Plugin{{Name: "marketing", Enabled: true}, {Name: "off"}},
		Permissions: []setup.Permissions{{Origin: setup.Origin{Path: "settings.json"}, Allow: 3}},
		Settings:    []setup.SettingsFile{{Keys: []string{"statusLine", "model", "hooks"}}},
	}
	history := usage.Profile{
		MCPServers: []usage.Tally{
			{Name: "postgres", Count: 10},
			{Name: "claude-in-chrome", Count: 40},
			{Name: "917d60b0-8a5b-4ceb-8448-c4cd013721a8", Count: 12, Tools: []string{"search_threads"}},
		},
		HookRuns: []usage.Tally{{Name: "Stop", Count: 7}},
	}
	got := map[string]harness.Feature{}
	for _, f := range (claude.Harness{}).Features(inv, history, harness.Installation{}) {
		got[string(f.Area)+":"+filepath.ToSlash(f.Name)] = f
	}
	want := map[string]harness.Status{
		"instructions:/code/app/CLAUDE.md": harness.StatusNative,
		"mcp:postgres":                     harness.StatusNative,
		"mcp:legacy":                       harness.StatusNative,
		"app-tools:claude-in-chrome":       harness.StatusPartial,
		"app-tools:Gmail connector":        harness.StatusLost,
		"hooks:UserPromptSubmit":           harness.StatusNative,
		"hooks:Stop":                       harness.StatusPartial,
		"commands:review":                  harness.StatusNative,
		"subagents:tester":                 harness.StatusNative,
		"skills:deploy":                    harness.StatusNative,
		"plugins:marketing":                harness.StatusNative,
		"permissions:settings.json":        harness.StatusNative,
		"settings:statusLine":              harness.StatusNative,
		"settings:model":                   harness.StatusTranslate,
		"hooks:Notification":               harness.StatusNative,
	}
	for key, status := range want {
		if got[key].Status != status {
			t.Errorf("%s = %q, want %q", key, got[key].Status, status)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d features, want %d: %v", len(got), len(want), got)
	}
	if f := got["hooks:Stop"]; f.Calls != 7 || !strings.Contains(f.How, "local model judges prompt hooks") {
		t.Errorf("prompt hook = %+v", f)
	}
	if !got["settings:model"].Migrated {
		t.Error("the model setting is replaced by claude-local, so migrate handles it")
	}
}

func served() migrate.Served {
	return migrate.Served{
		Runtime: runtimes.Ollama, RuntimeName: "Ollama", BaseURL: "http://127.0.0.1:11434",
		Model: "lo0/qwen3-coder-30b-a3b:64k", DisplayName: "Qwen3-Coder-30B-A3B", Context: 65536, Token: "ollama",
	}
}

func TestPlanWritesALauncher(t *testing.T) {
	req := migrate.Request{GOOS: "darwin", Home: "/Users/u", Getenv: func(k string) string {
		if k == "PATH" {
			return "/usr/bin:/Users/u/.local/bin"
		}
		return ""
	}}
	set, err := claude.Harness{}.Plan(req, served())
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Steps) != 1 || set.Steps[0].Kind != migrate.KindWrite {
		t.Fatalf("steps = %+v, want one file write", set.Steps)
	}
	files := migratetest.NewFiles(nil)
	fx := migrate.Effects{Files: files, Now: migratetest.Clock}
	if err := set.Steps[0].Run(t.Context(), fx, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	script, ok := files.Get("/Users/u/.local/bin/claude-local")
	if !ok || files.Perm("/Users/u/.local/bin/claude-local") != 0o755 {
		t.Fatalf("launcher not written as an executable: %v", files.Names())
	}
	for _, want := range []string{
		"#!/bin/sh\n",
		"unset ANTHROPIC_API_KEY\n",
		"export ANTHROPIC_BASE_URL='http://127.0.0.1:11434'\n",
		"export ANTHROPIC_AUTH_TOKEN='ollama'\n",
		"export ANTHROPIC_DEFAULT_HAIKU_MODEL='lo0/qwen3-coder-30b-a3b:64k'\n",
		"export CLAUDE_CODE_MAX_CONTEXT_TOKENS='65536'\n",
		"export CLAUDE_CODE_MAX_OUTPUT_TOKENS='16384'\n",
		"export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC='1'\n",
		"exec claude \"$@\"\n",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("launcher lacks %q:\n%s", want, script)
		}
	}
	if len(set.Then) != 2 || !strings.HasPrefix(set.Then[0], "Run claude-local in any project") {
		t.Errorf("then = %q", set.Then)
	}
}

func TestPlanSaysWhenTheLauncherIsNotOnPath(t *testing.T) {
	req := migrate.Request{GOOS: "linux", Home: "/home/u", Getenv: func(string) string { return "" }}
	set, _ := claude.Harness{}.Plan(req, served())
	if !strings.HasPrefix(filepath.ToSlash(set.Then[0]), "Run /home/u/.local/bin/claude-local") || !strings.Contains(set.Then[2], "is not on your PATH") {
		t.Errorf("then = %q", set.Then)
	}
}

func TestPlanOnWindowsWritesACommandScript(t *testing.T) {
	req := migrate.Request{GOOS: "windows", Home: `C:\Users\u`, Getenv: func(k string) string {
		if k == "LOCALAPPDATA" {
			return `C:\Users\u\AppData\Local`
		}
		return ""
	}}
	set, _ := claude.Harness{}.Plan(req, served())
	path := set.Steps[0].Detail[0]
	if !strings.HasSuffix(path, "claude-local.cmd") || !strings.Contains(path, "lo0") {
		t.Errorf("path = %q", path)
	}
	files := migratetest.NewFiles(nil)
	if err := set.Steps[0].Run(t.Context(), migrate.Effects{Files: files, Now: migratetest.Clock}, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	script, _ := files.Get(path)
	if !strings.HasPrefix(script, "@echo off\r\n") || !strings.Contains(script, `set "ANTHROPIC_MODEL=lo0/qwen3-coder-30b-a3b:64k"`+"\r\n") || !strings.HasSuffix(script, "claude %*\r\n") {
		t.Errorf("script:\n%s", script)
	}
}

func TestBackgroundWorkGoesToTheBackgroundModel(t *testing.T) {
	if !(claude.Harness{}).UsesBackground() {
		t.Fatal("Claude Code sends background work to its haiku model, so it uses a background model")
	}
	s := served()
	s.Background = &migrate.ServedModel{Model: "lo0/qwen3.5-2b:64k", DisplayName: "Qwen3.5-2B", Quant: "Q8_0", Context: 65536}
	req := migrate.Request{GOOS: "darwin", Home: "/Users/u", LookPath: func(string) (string, error) { return "/usr/local/bin/claude", nil }}

	set, err := claude.Harness{}.Plan(req, s)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := claude.Harness{}.Session(req, s)
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	for _, v := range session.Env {
		env[v.Name] = v.Value
	}
	if env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "lo0/qwen3.5-2b:64k" || env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != s.Model || env["ANTHROPIC_MODEL"] != s.Model {
		t.Errorf("session env = %v, want haiku on the background model and the rest on the main one", env)
	}
	if _, ok := env["CLAUDE_CODE_SUBAGENT_MODEL"]; ok {
		t.Error("the subagent setting would send haiku subagents to the main model")
	}
	detail := strings.Join(set.Steps[0].Detail, "\n")
	if !strings.Contains(detail, "ANTHROPIC_DEFAULT_HAIKU_MODEL=lo0/qwen3.5-2b:64k") || strings.Contains(detail, "CLAUDE_CODE_SUBAGENT_MODEL") {
		t.Errorf("launcher detail:\n%s", detail)
	}
	if len(set.Then) < 2 || set.Then[1] != "Titles, summaries and haiku subagents such as Explore use Qwen3.5-2B." {
		t.Errorf("then = %q", set.Then)
	}

	files := migratetest.NewFiles(nil)
	if err := set.Steps[0].Run(t.Context(), migrate.Effects{Files: files, Now: migratetest.Clock}, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	script, _ := files.Get("/Users/u/.local/bin/claude-local")
	if !strings.Contains(script, "export ANTHROPIC_DEFAULT_HAIKU_MODEL='lo0/qwen3.5-2b:64k'\n") || !strings.Contains(script, "and Qwen3.5-2B for background work") {
		t.Errorf("launcher:\n%s", script)
	}
}

func TestWithoutABackgroundModelEveryAliasIsTheMainModel(t *testing.T) {
	session, _, err := claude.Harness{}.Session(migrate.Request{GOOS: "linux"}, served())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range session.Env {
		if strings.HasSuffix(v.Name, "_MODEL") && v.Value != served().Model {
			t.Errorf("%s = %q, want the main model", v.Name, v.Value)
		}
	}
}

func TestPlanOnWindowsFindsTheLauncherOnPathInAnyCase(t *testing.T) {
	local := `C:\Users\u\AppData\Local`
	for _, path := range []string{
		`C:\Windows;C:\Users\u\AppData\Local\lo0\bin`,
		`C:\Windows;c:\users\u\appdata\local\LO0\BIN`,
		`C:\Windows;"C:\Users\u\AppData\Local\lo0\bin"`,
	} {
		req := migrate.Request{GOOS: "windows", Home: `C:\Users\u`, Getenv: func(k string) string {
			return map[string]string{"LOCALAPPDATA": local, "PATH": path}[k]
		}}
		set, _ := claude.Harness{}.Plan(req, served())
		if !strings.HasPrefix(set.Then[0], "Run claude-local in any project") {
			t.Errorf("PATH %s: then = %q", path, set.Then)
		}
	}
}

func TestWindowsLauncherQuotesValues(t *testing.T) {
	s := served()
	s.Model = "lo0/a&b%c"
	req := migrate.Request{GOOS: "windows", Home: `C:\Users\u`, Getenv: func(string) string { return "" }}
	set, _ := claude.Harness{}.Plan(req, s)
	files := migratetest.NewFiles(nil)
	if err := set.Steps[0].Run(t.Context(), migrate.Effects{Files: files, Now: migratetest.Clock}, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	script, _ := files.Get(set.Steps[0].Detail[0])
	if !strings.Contains(script, `set "ANTHROPIC_MODEL=lo0/a&b%%c"`+"\r\n") {
		t.Errorf("script:\n%s", script)
	}
}

// A quote in a value, such as one in OLLAMA_HOST, must not end the shell
// string and run the rest as commands.
func TestLauncherQuotesValues(t *testing.T) {
	s := served()
	s.BaseURL = "http://host'; touch /tmp/pwned; '"
	req := migrate.Request{GOOS: "linux", Home: "/home/u", Getenv: func(string) string { return "" }}
	set, _ := claude.Harness{}.Plan(req, s)
	files := migratetest.NewFiles(nil)
	if err := set.Steps[0].Run(t.Context(), migrate.Effects{Files: files, Now: migratetest.Clock}, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	script, _ := files.Get(set.Steps[0].Detail[0])
	if want := `export ANTHROPIC_BASE_URL='http://host'\''; touch /tmp/pwned; '\'''` + "\n"; !strings.Contains(script, want) {
		t.Errorf("script lacks %q:\n%s", want, script)
	}
}
