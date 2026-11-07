package codex

import (
	"slices"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/runtimes"
)

func testServed() migrate.Served {
	return migrate.Served{
		Runtime: runtimes.Ollama, RuntimeName: "Ollama", BaseURL: "http://127.0.0.1:11434",
		Model: "lo0/qwen3-coder-30b-a3b:64k", DisplayName: "Qwen3-Coder-30B-A3B", Context: 65536, Token: "ollama",
	}
}

func testEdit() edit {
	return edit{provider: "lo0-ollama", served: testServed()}
}

// userConfig has the shapes a hand-edited config.toml has: comments, a
// multi-line array and string whose lines look like keys and headers, a
// quoted table name and a key that is already set.
const userConfig = `# My Codex settings
model = "gpt-5.5" # the default
approval_policy = "on-request"
notify = [
  "say",
  "done # not a comment",
]
instructions_note = """
model = "not a key"
[not.a.table]
"""

[model_providers."corp"]
name = "Corp"
base_url = "https://llm.corp.example/v1"

# Servers I use everywhere.
[mcp_servers.github]
command = "github-mcp"

[projects."/home/u/code/app"]
trust_level = "trusted"
`

func decode(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if _, err := toml.Decode(string(data), &out); err != nil {
		t.Fatalf("not TOML: %v\n%s", err, data)
	}
	return out
}

func TestMergeConfigKeepsTheRest(t *testing.T) {
	e := testEdit()
	e.fallback = true
	e.servers = map[string][]string{
		"github":   {`command = "replaced"`},
		"postgres": {`command = "npx"`, `args = ["-y", "@pg/mcp"]`},
	}
	got, err := mergeConfig([]byte(userConfig), e)
	if err != nil {
		t.Fatal(err)
	}
	cfg := decode(t, got)
	if cfg["model"] != "lo0/qwen3-coder-30b-a3b:64k" || cfg["model_provider"] != "lo0-ollama" || cfg["model_context_window"] != int64(65536) {
		t.Errorf("model keys = %v, %v, %v", cfg["model"], cfg["model_provider"], cfg["model_context_window"])
	}
	providers := cfg["model_providers"].(map[string]any)
	lo0 := providers["lo0-ollama"].(map[string]any)
	if lo0["name"] != "Ollama (local)" || lo0["base_url"] != "http://127.0.0.1:11434/v1" || providers["corp"] == nil {
		t.Errorf("model_providers = %v", providers)
	}
	servers := cfg["mcp_servers"].(map[string]any)
	if servers["github"].(map[string]any)["command"] != "github-mcp" {
		t.Error("an MCP server the user already had was replaced")
	}
	if servers["postgres"].(map[string]any)["command"] != "npx" {
		t.Errorf("postgres was not added: %v", servers)
	}
	if fb := cfg["project_doc_fallback_filenames"]; !slices.Equal(fb.([]any), []any{"CLAUDE.md"}) {
		t.Errorf("project_doc_fallback_filenames = %v", fb)
	}
	for _, kept := range []string{"# My Codex settings", "# Servers I use everywhere.", `"done # not a comment",`, `[not.a.table]`, `approval_policy = "on-request"`} {
		if !strings.Contains(string(got), kept) {
			t.Errorf("lost %q:\n%s", kept, got)
		}
	}
	if strings.Contains(string(got), "gpt-5.5") {
		t.Errorf("the old model is still set:\n%s", got)
	}
	again, err := mergeConfig(got, e)
	if err != nil || string(again) != string(got) {
		t.Errorf("merging again changed the file (%v):\n%s", err, again)
	}
}

func TestMergeConfigStartsAFile(t *testing.T) {
	got, err := mergeConfig(nil, testEdit())
	if err != nil {
		t.Fatal(err)
	}
	want := `model = "lo0/qwen3-coder-30b-a3b:64k"
model_provider = "lo0-ollama"
model_context_window = 65536

[model_providers.lo0-ollama]
name = "Ollama (local)"
base_url = "http://127.0.0.1:11434/v1"
`
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestMergeConfigReplacesItsOwnProvider(t *testing.T) {
	old := "model = \"x\"\r\n\r\n[model_providers.lo0-ollama]\r\nname = \"Ollama (local)\"\r\nbase_url = \"http://127.0.0.1:9999/v1\"\r\n\r\n[tui]\r\nanimations = false\r\n"
	got, err := mergeConfig([]byte(old), testEdit())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "9999") || !strings.Contains(string(got), "[tui]\r\nanimations = false\r\n") {
		t.Errorf("got:\n%q", got)
	}
	if strings.Count(string(got), "\n") != strings.Count(string(got), "\r\n") {
		t.Errorf("line endings were mixed:\n%q", got)
	}
}

func TestMergeConfigKeepsTheUsersFallbacks(t *testing.T) {
	e := testEdit()
	e.fallback = true
	got, err := mergeConfig([]byte("project_doc_fallback_filenames = [\n  \"README.md\",\n]\n"), e)
	if err != nil {
		t.Fatal(err)
	}
	if fb := decode(t, got)["project_doc_fallback_filenames"]; !slices.Equal(fb.([]any), []any{"README.md", "CLAUDE.md"}) {
		t.Errorf("project_doc_fallback_filenames = %v\n%s", fb, got)
	}
}

func TestMergeConfigRefusesWhatItCannotDoSafely(t *testing.T) {
	tests := map[string]string{
		"not TOML":            "model = \n",
		"provider as dotted":  "model_providers.lo0-ollama.name = \"mine\"\n",
		"provider inline":     "model_providers = { lo0-ollama = { name = \"mine\" } }\n",
		"model in a table":    "[model]\nname = \"x\"\n",
		"unterminated string": "notify = \"\"\"\nopen\n",
	}
	for name, old := range tests {
		t.Run(name, func(t *testing.T) {
			if got, err := mergeConfig([]byte(old), testEdit()); err == nil {
				t.Errorf("merged:\n%s", got)
			}
		})
	}
}

func TestClaudeServerToCodex(t *testing.T) {
	tests := []struct {
		name string
		s    claudeServer
		want []string
	}{
		{
			"stdio",
			claudeServer{Command: "npx", Args: []string{"-y", "@pg/mcp"}, Env: map[string]string{"PGPASSWORD": "hunter2", "HOME_DIR": "${HOME}", "TOKEN": "${TOKEN}"}},
			[]string{`command = "npx"`, `args = ["-y", "@pg/mcp"]`, `env = { HOME_DIR = "${HOME}", PGPASSWORD = "hunter2" }`, `env_vars = ["TOKEN"]`},
		},
		{
			"http",
			claudeServer{Type: "http", URL: "https://mcp.example/mcp", Headers: map[string]string{"Authorization": "Bearer ${API_TOKEN}", "X-Team": "${TEAM}", "X-Plain": "abc"}},
			[]string{`url = "https://mcp.example/mcp"`, `bearer_token_env_var = "API_TOKEN"`, `http_headers = { X-Plain = "abc" }`, `env_http_headers = { X-Team = "TEAM" }`},
		},
		{
			"quotes and odd keys",
			claudeServer{Command: `C:\Tools\mcp.exe`, Env: map[string]string{"A.B": `say "hi"`}},
			[]string{`command = "C:\\Tools\\mcp.exe"`, `env = { "A.B" = "say \"hi\"" }`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.s.codex()
			if !slices.Equal(got, tt.want) {
				t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
			decode(t, []byte("[x]\n"+strings.Join(got, "\n")+"\n"))
		})
	}
}

func TestAgentTOML(t *testing.T) {
	src := "---\nname: tester\ndescription: \"Runs the tests\"\ntools: Read, Bash\nmodel: sonnet\n---\nYou run tests.\nNever skip one.\n"
	got := decode(t, agentTOML("tester", []byte(src)))
	if got["name"] != "tester" || got["description"] != "Runs the tests" || got["developer_instructions"] != "You run tests.\nNever skip one." {
		t.Errorf("agent = %v", got)
	}
	if _, ok := got["model"]; ok {
		t.Error("the Claude model was carried over")
	}
	odd := decode(t, agentTOML("x", []byte("Use ''' quotes\x01\n")))
	if odd["description"] != "x" || odd["developer_instructions"] != "Use ''' quotes\x01" {
		t.Errorf("agent = %v", odd)
	}
}

func TestWrapNames(t *testing.T) {
	names := make([]string, 30)
	for i := range names {
		names[i] = "skill-name"
	}
	lines := wrapNames(names)
	if len(lines) < 4 {
		t.Errorf("lines = %q", lines)
	}
	for _, l := range lines {
		if len(l) > detailWidth {
			t.Errorf("line is %d wide: %q", len(l), l)
		}
	}
	if got := strings.Join(lines, " "); strings.Count(got, "skill-name") != 30 || strings.HasSuffix(got, ",") {
		t.Errorf("joined = %q", got)
	}
}
