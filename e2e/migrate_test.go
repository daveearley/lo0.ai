package e2e_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// TestMigrateAndUndo applies a real migration against the fake Ollama,
// checks what it set up, runs it again to see it change nothing, then
// undoes it and checks the machine is back as it was.
func TestMigrateAndUndo(t *testing.T) {
	tests := []struct {
		to string
		// json applies with --json --yes instead of the printed report.
		json bool
		// check looks at what the migration left in the home directory.
		check func(t *testing.T, m *machine, served string)
	}{
		{to: "claude-code", check: checkClaudeLocal},
		{to: "opencode", json: true, check: checkOpenCodeConfig},
		{to: "codex", check: checkCodexConfig},
	}
	for _, tt := range tests {
		t.Run(tt.to, func(t *testing.T) {
			t.Parallel()
			m := newMachine(t)
			o := m.startOllama("qwen3:4b")
			m.install("claude", "opencode", "codex")
			before := snapshot(t, m.home)
			args := []string{"migrate", "--yes", "--to", tt.to, "--runtime", "ollama", "--model", "qwen3.5-2b"}

			var r result
			if tt.json {
				r = m.run(append(args, "--json")...).ok(t)
				checkOutcome(t, r.stdout)
			} else {
				r = m.run(args...).ok(t)
				plainText(t, r.stdout)
			}
			noSecrets(t, r.output())

			served := createdModel(t, o)
			for _, want := range []string{"pull hf.co/", "create " + served} {
				if !slices.ContainsFunc(o.changed(), func(c string) bool { return strings.HasPrefix(c, want) }) {
					t.Errorf("Ollama was not asked to %s: %v", want, o.changed())
				}
			}
			tt.check(t, m, served)

			applied, calls := snapshot(t, m.home), len(o.changed())
			again := m.run(args...).ok(t)
			if !strings.Contains(again.stderr, "Everything is already set up") {
				t.Errorf("a second migrate did not say everything is set up:\n%s", again.output())
			}
			if diff := changes(applied, snapshot(t, m.home)); len(diff) > 0 {
				t.Errorf("a second migrate changed files or the journal:\n%s", strings.Join(diff, "\n"))
			}
			if len(o.changed()) != calls {
				t.Errorf("a second migrate changed Ollama: %v", o.changed()[calls:])
			}

			undo := m.run("migrate", "--undo", "--yes").ok(t)
			plainText(t, undo.stdout)
			if got := o.has(); !slices.Equal(got, []string{"qwen3:4b"}) {
				t.Errorf("after undo Ollama holds %v, want only the model it started with", got)
			}
			// Undo removes what migrate wrote but not the directories it had
			// to create for it, which may hold the user's own files by then.
			if diff := changes(files(before), files(snapshot(t, m.home)), stateDir()); len(diff) > 0 {
				t.Errorf("undo left the home directory changed:\n%s", strings.Join(diff, "\n"))
			}
			if r := m.run("migrate", "--undo").ok(t); !strings.Contains(r.stdout, "nothing to undo") {
				t.Errorf("after undo, lo0 migrate --undo said:\n%s", r.output())
			}
		})
	}
}

// createdModel is the one model lo0 created in Ollama, named as lo0 names
// them.
func createdModel(t *testing.T, o *fakeOllama) string {
	t.Helper()
	for _, c := range o.changed() {
		if name, ok := strings.CutPrefix(c, "create "); ok && strings.HasPrefix(name, "lo0/qwen3.5-2b:") {
			return name
		}
	}
	t.Fatalf("lo0 did not create lo0/qwen3.5-2b in Ollama: %v", o.changed())
	return ""
}

// checkClaudeLocal checks the launcher migrate writes beside claude points
// Claude Code at the fake Ollama and the model lo0 created.
func checkClaudeLocal(t *testing.T, m *machine, served string) {
	t.Helper()
	// Windows keeps per-user programs under LOCALAPPDATA, which environ
	// points inside the home directory.
	path := filepath.Join(m.home, ".local", "bin", "claude-local")
	if runtime.GOOS == "windows" {
		path = filepath.Join(m.home, "AppData", "Local", "lo0", "bin", "claude-local.cmd")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("migrate did not write the launcher: %v", err)
	}
	mcp := filepath.Join(m.home, filepath.FromSlash(stateDir()), "claude-local", "mcp.json")
	for _, want := range []string{"ANTHROPIC_BASE_URL", m.ollama.host(), "ANTHROPIC_MODEL", served, "claude", "--mcp-config=" + mcp} {
		if !strings.Contains(string(data), want) {
			t.Errorf("claude-local lacks %q:\n%s", want, data)
		}
	}
	// The Codex MCP servers move with their values, from one file on this
	// machine to another.
	servers, err := os.ReadFile(mcp)
	if err != nil {
		t.Fatalf("migrate did not write the Codex MCP servers: %v", err)
	}
	for _, want := range []string{`"fs"`, `"linear"`, "SECRET_CODEX_ENV_VALUE", "SECRET_CODEX_HEADER_VALUE"} {
		if !strings.Contains(string(servers), want) {
			t.Errorf("mcp.json lacks %q:\n%s", want, servers)
		}
	}
	if strings.Contains(string(servers), "node_repl") || strings.Contains(string(servers), "SECRET_CODEX_APP") {
		t.Errorf("mcp.json holds the Codex app's own server:\n%s", servers)
	}
	for rel, want := range map[string]string{
		".claude/skills/release/SKILL.md": "SECRET_CODEX_SKILL_TEXT",
		".claude/commands/fix.md":         "Fix $0.",
		".claude/agents/reviewer.md":      "SECRET_CODEX_AGENT_TEXT",
	} {
		if got, err := os.ReadFile(filepath.Join(m.home, filepath.FromSlash(rel))); err != nil || !strings.Contains(string(got), want) {
			t.Errorf("%s lacks %q: %v\n%s", rel, want, err, got)
		}
	}
}

// checkOpenCodeConfig checks migrate added a provider for the fake Ollama
// to opencode.json and kept the settings that were already there.
func checkOpenCodeConfig(t *testing.T, m *machine, served string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(m.home, ".config", "opencode", "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("migrate left opencode.json invalid: %v\n%s", err, data)
	}
	if cfg["theme"] != "system" || cfg["share"] != "disabled" {
		t.Errorf("migrate lost the settings already in opencode.json:\n%s", data)
	}
	for _, want := range []string{m.ollama.host(), served} {
		if !strings.Contains(string(data), want) {
			t.Errorf("opencode.json lacks %q:\n%s", want, data)
		}
	}
	// MCP servers move with their env values, from one file on this
	// machine to another; only plans and reports leave the values out.
	if !strings.Contains(string(data), "SECRET_MCP_ENV_VALUE") {
		t.Errorf("migrate did not carry the github MCP server and its env over:\n%s", data)
	}
	for _, want := range []string{`"fs"`, "SECRET_CODEX_ENV_VALUE", `"linear"`, "SECRET_CODEX_HEADER_VALUE", "AGENTS.md"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("opencode.json lacks the Codex setup's %q:\n%s", want, data)
		}
	}
	if strings.Contains(string(data), "node_repl") || strings.Contains(string(data), "SECRET_CODEX_APP") {
		t.Errorf("opencode.json holds the Codex app's own server:\n%s", data)
	}
	for rel, want := range map[string]string{
		".config/opencode/skills/release/SKILL.md": "SECRET_CODEX_SKILL_TEXT",
		".config/opencode/commands/fix.md":         "Fix $1.",
		".config/opencode/agents/reviewer.md":      "mode: subagent",
	} {
		if got, err := os.ReadFile(filepath.Join(m.home, filepath.FromSlash(rel))); err != nil || !strings.Contains(string(got), want) {
			t.Errorf("%s lacks %q: %v\n%s", rel, want, err, got)
		}
	}
}

// checkOutcome checks migrate --json --yes reports the plan and an outcome
// in which every step finished.
func checkOutcome(t *testing.T, stdout string) {
	t.Helper()
	env := decodeOne(t, stdout)
	var res struct {
		Plan    json.RawMessage `json:"plan"`
		Outcome map[string]any  `json:"outcome"`
	}
	if err := json.Unmarshal(env.Result, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Plan) == 0 || res.Outcome == nil {
		t.Fatalf("migrate --json --yes printed no plan or no outcome:\n%s", stdout)
	}
	if strings.Contains(stdout, `"failed"`) || strings.Contains(stdout, `"error"`) {
		t.Errorf("the outcome reports a failure:\n%s", stdout)
	}
}

// checkCodexConfig checks migrate pointed Codex's config.toml at the fake
// Ollama and the model lo0 created, carried the Claude Code MCP servers
// over and kept what the user had written.
func checkCodexConfig(t *testing.T, m *machine, served string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(m.home, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Written by hand.", `approval_policy = "on-request"`, "SECRET_CODEX_ENV_VALUE",
		`model = "` + served + `"`, `model_provider = "lo0-ollama"`, "[model_providers.lo0-ollama]",
		`base_url = "http://` + m.ollama.host() + `/v1"`,
		// MCP servers move with their env values, from one file on this
		// machine to another; only plans and reports leave the values out.
		"[mcp_servers.github]", "SECRET_MCP_ENV_VALUE",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("config.toml lacks %q:\n%s", want, data)
		}
	}
	if strings.Contains(string(data), "gpt-5.5") {
		t.Errorf("config.toml still starts Codex with the OpenAI model:\n%s", data)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(filepath.Join(m.home, ".codex", "config.toml")); err != nil || info.Mode().Perm()&0o077 != 0 {
			t.Errorf("config.toml holds MCP secrets but is readable by others: %v %v", info.Mode(), err)
		}
	}
}
