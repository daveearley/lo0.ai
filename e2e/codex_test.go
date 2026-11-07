package e2e_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// codexScan is the part of the scan's JSON that holds the Codex setup.
type codexScan struct {
	Codex struct {
		Found      bool   `json:"found"`
		Model      string `json:"model"`
		MCPServers []struct {
			Name string `json:"name"`
		} `json:"mcpServers"`
		Instructions []json.RawMessage `json:"instructions"`
		Hooks        []json.RawMessage `json:"hooks"`
		Extensions   []json.RawMessage `json:"extensions"`
	} `json:"codex"`
}

func scanCodex(t *testing.T, m *machine, args ...string) codexScan {
	t.Helper()
	var res codexScan
	env := decodeOne(t, m.run(append([]string{"scan", "--json"}, args...)...).ok(t).stdout)
	if err := json.Unmarshal(env.Result, &res); err != nil {
		t.Fatal(err)
	}
	return res
}

// TestScanReadsTheCodexSetup checks the scan inventories config.toml,
// AGENTS.md, hooks and skills from the Codex home, and from where
// --codex-dir moves it.
func TestScanReadsTheCodexSetup(t *testing.T) {
	m := newMachine(t)
	check := func(res codexScan) {
		t.Helper()
		var servers []string
		for _, s := range res.Codex.MCPServers {
			servers = append(servers, s.Name)
		}
		slices.Sort(servers)
		if !res.Codex.Found || res.Codex.Model != "gpt-5.5" || !slices.Equal(servers, []string{"fs", "linear", "node_repl"}) ||
			len(res.Codex.Instructions) != 1 || len(res.Codex.Hooks) != 1 || len(res.Codex.Extensions) != 3 {
			t.Errorf("Codex setup = %+v", res.Codex)
		}
	}
	check(scanCodex(t, m))

	moved := filepath.Join(m.root, "codex-setup-elsewhere")
	if err := os.Rename(filepath.Join(m.home, ".codex"), moved); err != nil {
		t.Fatal(err)
	}
	if res := scanCodex(t, m); res.Codex.Found {
		t.Errorf("without ~/.codex, the scan found a Codex setup: %+v", res.Codex)
	}
	check(scanCodex(t, m, "--codex-dir", moved))
}

// TestCodexReports checks the scan shows the Codex setup and the plans for
// Codex follow the report rules.
func TestCodexReports(t *testing.T) {
	tests := []struct {
		args []string
		want []string
	}{
		{[]string{"scan"}, []string{"Codex setup", "Model         gpt-5.5"}},
		{
			[]string{"migrate", "--dry-run", "--to", "codex", "--runtime", "ollama", "--model", "qwen3.5-2b"},
			[]string{"Codex on a local model", "config.toml", "model_provider", "/v1/responses"},
		},
		{
			[]string{"migrate", "--dry-run", "--to", "claude-code", "--runtime", "ollama", "--model", "qwen3.5-2b"},
			[]string{"Write your Codex MCP servers for claude-local", "Copy 1 Codex skill to Claude Code", "Passes --mcp-config"},
		},
		{
			[]string{"migrate", "--dry-run", "--to", "opencode", "--runtime", "ollama", "--model", "qwen3.5-2b"},
			[]string{"docs, github, fs, linear", "Copy 1 Codex prompt to OpenCode", "Copy 1 Codex agent to OpenCode"},
		},
		{
			[]string{"try", "--dry-run", "--harness", "codex", "--runtime", "ollama", "--model", "qwen3.5-2b"},
			[]string{"codex \\", "model_provider=", "config.toml is not changed"},
		},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			t.Parallel()
			m := newMachine(t)
			m.startOllama()
			m.install("claude", "opencode", "codex")
			r := m.run(tt.args...).ok(t)
			plainText(t, r.stdout)
			noSecrets(t, r.output())
			for _, want := range tt.want {
				if !strings.Contains(r.stdout, want) {
					t.Errorf("report lacks %q:\n%s", want, r.stdout)
				}
			}
		})
	}
}

// TestTryCodexJSON checks lo0 try --json for Codex gives the session as -c
// overrides and changes nothing.
func TestTryCodexJSON(t *testing.T) {
	m := newMachine(t)
	o := m.startOllama()
	m.install("codex")
	before := snapshot(t, m.home)
	env := decodeOne(t, m.run("try", "--json", "--harness", "codex", "--runtime", "ollama", "--model", "qwen3.5-2b").ok(t).stdout)
	if env.Schema != "lo0.try/v1" {
		t.Errorf("schema = %q", env.Schema)
	}
	var res struct {
		Plan struct {
			Harness string `json:"harness"`
			Session struct {
				Program string   `json:"program"`
				Args    []string `json:"args"`
			} `json:"session"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(env.Result, &res); err != nil {
		t.Fatal(err)
	}
	if res.Plan.Harness != "codex" || res.Plan.Session.Program != "codex" ||
		!slices.Contains(res.Plan.Session.Args, `model_providers.lo0-ollama.base_url="http://`+o.host()+`/v1"`) {
		t.Errorf("try plan = %+v\n%s", res.Plan, env.Result)
	}
	if diff := changes(before, snapshot(t, m.home)); len(diff) > 0 {
		t.Errorf("try --json changed the home directory:\n%s", strings.Join(diff, "\n"))
	}
}

// TestMatchCarriesTheCodexSetup checks every target takes the Codex setup
// over, so lo0 match never says it stays behind.
func TestMatchCarriesTheCodexSetup(t *testing.T) {
	m := newMachine(t)
	m.install("claude", "opencode", "codex")
	env := decodeOne(t, m.run("match", "--json").ok(t).stdout)
	var res struct {
		Match struct {
			Targets []struct {
				ID       string   `json:"id"`
				Unread   []string `json:"unread"`
				Features []struct {
					Area     string `json:"area"`
					Name     string `json:"name"`
					From     string `json:"from"`
					Migrated bool   `json:"migrated"`
				} `json:"features"`
			} `json:"targets"`
		} `json:"match"`
	}
	if err := json.Unmarshal(env.Result, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Match.Targets) != 3 {
		t.Fatalf("targets = %+v", res.Match.Targets)
	}
	for _, target := range res.Match.Targets {
		if len(target.Unread) != 0 {
			t.Errorf("%s leaves the Codex setup behind", target.ID)
		}
		from := map[string]bool{}
		for _, f := range target.Features {
			if f.From == "Codex" {
				from[f.Area+" "+f.Name] = true
			}
		}
		for _, want := range []string{"mcp fs", "mcp linear", "hooks Stop", "skills release", "commands fix", "subagents reviewer"} {
			if !from[want] {
				t.Errorf("%s does not map the Codex %s: %v", target.ID, want, from)
			}
		}
		if target.ID != "codex" && (from["mcp node_repl"] || !from["app-tools node_repl"]) {
			t.Errorf("%s does not report the Codex app's own server as an app tool: %v", target.ID, from)
		}
	}
	r := m.run("match").ok(t)
	noSecrets(t, r.output())
	if strings.Contains(r.stdout, "stays behind") {
		t.Errorf("lo0 match says the Codex setup stays behind:\n%s", r.stdout)
	}
}
