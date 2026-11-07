package setup_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"lo0.ai/lo0/internal/codex/setup"
)

type missingFS struct{}

func (missingFS) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }

type tree map[string]fstest.MapFS

func (t tree) open(dir string) fs.FS {
	if fsys, ok := t[dir]; ok {
		return fsys
	}
	return missingFS{}
}

func abs(p string) string { return filepath.FromSlash(p) }

func file(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }

const configTemplate = `
model = "gpt-5.5"
model_provider = "openai"
approval_policy = "on-request"
notify = ["say", "SECRET_NOTIFY_ARG"]

[model_providers.ollama-local]
name = "Ollama"
base_url = "http://127.0.0.1:11434/v1"

[model_providers.corp]
name = "Corp"
base_url = "https://llm.corp.example/v1"
http_headers = { "X-Key" = "SECRET_PROVIDER_HEADER" }

[mcp_servers.github]
command = "/opt/homebrew/bin/github-mcp"
args = ["--token", "SECRET_MCP_ARG"]
env_vars = ["GITHUB_HOST", { name = "GH_ORG", source = "local" }]

[mcp_servers.github.env]
GITHUB_TOKEN = "SECRET_MCP_ENV"

[mcp_servers.docs]
url = "https://docs.example/mcp"
bearer_token_env_var = "DOCS_TOKEN"
http_headers = { "X-Team" = "SECRET_MCP_HEADER" }
enabled = false

[projects.APP]
trust_level = "trusted"

[projects.HOME]
trust_level = "trusted"

[projects.GONE]
trust_level = "untrusted"

[plugins."gmail@openai-curated"]
enabled = true

[plugins."pdf@openai-primary-runtime"]
enabled = false

[plugins."sites@openai-curated"]

[plugins._default]
enabled = true

[[hooks.PostToolUse]]
matcher = "apply_patch"

[[hooks.PostToolUse.hooks]]
type = "command"
command = "SECRET_INLINE_HOOK"

[hooks.state."hooks.json:stop:0:0"]
enabled = true
`

// config is the fixture config.toml with this platform's form of each
// project path, quoted as TOML reads it.
func config() string {
	return strings.NewReplacer(
		"APP", fmt.Sprintf("%q", abs("/home/u/code/app")),
		"HOME", fmt.Sprintf("%q", abs("/home/u")),
		"GONE", fmt.Sprintf("%q", abs("/home/u/code/gone")),
	).Replace(configTemplate)
}

func fixture() tree {
	return tree{
		abs("/home/u/.codex"): {
			"config.toml":          file(config()),
			"local.config.toml":    file("model = \"qwen3-coder\"\nmodel_provider = \"ollama-local\"\n"),
			"deep.config.toml":     file("model_reasoning_effort = \"high\"\n"),
			"AGENTS.md":            file("# Me\nUse tabs.\n"),
			"prompts/fix.md":       file("Fix $ARGUMENTS\n"),
			"prompts/sub/x.md":     file("not read\n"),
			"agents/reviewer.toml": file("name = \"code-reviewer\"\ndescription = \"x\"\ndeveloper_instructions = \"SECRET_AGENT_PROMPT\"\n"),
			"hooks.json": file(`{"hooks": {
				"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "SECRET_HOOK_CMD", "timeout": 5}]}],
				"Stop": [{"hooks": [{"type": "command", "command": "notify"}]}]
			}}`),
			"skills/review/SKILL.md":              file("---\nname: careful-review\ndescription: x\n---\nbody\n"),
			"skills/plain/SKILL.md":               file("no frontmatter\n"),
			"skills/.system/imagegen/SKILL.md":    file("---\nname: imagegen\n---\n"),
			"skills/empty/README.md":              file("not a skill\n"),
			"rules/default.rules":                 file(`prefix_rule(pattern=["git", "status"], decision="allow")` + "\n" + `prefix_rule(pattern=["rm"], decision="forbidden")` + "\n"),
			"rules/notes.txt":                     file("ignored\n"),
			"sessions/2026/10/02/rollout-1.jsonl": file("{}\n"),
		},
		abs("/home/u/code/app"): {
			"AGENTS.md":                 file("# App\n"),
			"AGENTS.override.md":        file("# Override\nOne\nTwo\n"),
			".codex/config.toml":        file("[mcp_servers.db]\ncommand = \"db-mcp\"\n"),
			".codex/hooks.json":         file(`{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "x"}]}]}}`),
			".agents/skills/x/SKILL.md": file("---\nname: deploy\n---\n"),
		},
		abs("/home/u"): {
			".agents/skills/mine/SKILL.md": file("---\nname: mine\n---\n"),
			".codex/config.toml":           file(config()),
			".codex/hooks.json":            file(`{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "again"}]}]}}`),
		},
	}
}

func TestScan(t *testing.T) {
	inv, err := setup.NewScanner(abs("/home/u/.codex"), abs("/home/u"), fixture().open).Scan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !inv.Found || inv.Path != abs("/home/u/.codex/config.toml") {
		t.Errorf("Found, Path = %v, %q", inv.Found, inv.Path)
	}
	if inv.Model != "gpt-5.5" || inv.Provider != "openai" {
		t.Errorf("model, provider = %q, %q", inv.Model, inv.Provider)
	}
	if want := []string{"approval_policy", "hooks", "mcp_servers", "model", "model_provider", "model_providers", "notify", "plugins", "projects"}; !slices.Equal(inv.Keys, want) {
		t.Errorf("Keys = %v, want %v", inv.Keys, want)
	}
	if want := []setup.Provider{{ID: "corp", Name: "Corp"}, {ID: "ollama-local", Name: "Ollama", Local: true}}; !slices.Equal(inv.Providers, want) {
		t.Errorf("Providers = %+v", inv.Providers)
	}
	if want := []setup.Profile{
		{Name: "deep", Path: abs("/home/u/.codex/deep.config.toml")},
		{Name: "local", Path: abs("/home/u/.codex/local.config.toml"), Model: "qwen3-coder", Provider: "ollama-local"},
	}; !slices.Equal(inv.Profiles, want) {
		t.Errorf("Profiles = %+v", inv.Profiles)
	}
	if want := []string{abs("/home/u"), abs("/home/u/code/app")}; !slices.Equal(inv.Projects, want) {
		t.Errorf("Projects = %v, want %v", inv.Projects, want)
	}
	if want := []setup.Plugin{
		{Name: "gmail", Marketplace: "openai-curated", Enabled: true},
		{Name: "pdf", Marketplace: "openai-primary-runtime"},
		{Name: "sites", Marketplace: "openai-curated", Enabled: true},
	}; !slices.Equal(inv.Plugins, want) {
		t.Errorf("Plugins = %+v", inv.Plugins)
	}

	var files []string
	for _, f := range inv.Instructions {
		files = append(files, f.Path)
	}
	if want := []string{abs("/home/u/.codex/AGENTS.md"), abs("/home/u/code/app/AGENTS.override.md")}; !slices.Equal(files, want) {
		t.Errorf("Instructions = %v, want %v", files, want)
	}
	if f := inv.Instructions[1]; !f.Override || f.Lines != 3 || f.Scope != setup.ScopeProject {
		t.Errorf("override = %+v", f)
	}

	byName := map[string]setup.MCPServer{}
	for _, s := range inv.MCPServers {
		byName[s.Name] = s
	}
	if gh := byName["github"]; gh.Transport != setup.TransportStdio || gh.Command != "github-mcp" || !slices.Equal(gh.EnvKeys, []string{"GH_ORG", "GITHUB_HOST", "GITHUB_TOKEN"}) {
		t.Errorf("github = %+v", gh)
	}
	if docs := byName["docs"]; docs.Transport != setup.TransportHTTP || !docs.Disabled || !slices.Equal(docs.HeaderKeys, []string{"Authorization", "X-Team"}) {
		t.Errorf("docs = %+v", docs)
	}
	if db := byName["db"]; db.Scope != setup.ScopeProject || db.Path != abs("/home/u/code/app/.codex/config.toml") {
		t.Errorf("db = %+v", db)
	}

	var events []string
	for _, h := range inv.Hooks {
		events = append(events, h.Event)
	}
	if want := []string{"PostToolUse", "PreToolUse", "Stop", "SessionStart"}; !slices.Equal(events, want) {
		t.Errorf("hook events = %v, want %v", events, want)
	}
	var extensions []string
	for _, e := range inv.Extensions {
		extensions = append(extensions, string(e.Kind)+" "+e.Name)
	}
	if want := []string{
		"skill mine", "skill plain", "skill careful-review", "prompt fix", "agent code-reviewer", "skill deploy",
	}; !slices.Equal(extensions, want) {
		t.Errorf("extensions = %v, want %v", extensions, want)
	}
	if len(inv.Rules) != 1 || inv.Rules[0].Rules != 2 {
		t.Errorf("Rules = %+v", inv.Rules)
	}
	if got := inv.Items(); got != 2+3+4+6+1+2 {
		t.Errorf("Items = %d", got)
	}
}

// TestScanKeepsNoSecrets checks no value that could be a secret reaches the
// inventory or its JSON.
func TestScanKeepsNoSecrets(t *testing.T) {
	inv, err := setup.NewScanner(abs("/home/u/.codex"), abs("/home/u"), fixture().open).Scan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(inv)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "SECRET_") || strings.Contains(string(data), "llm.corp.example") {
		t.Errorf("inventory holds a secret:\n%s", data)
	}
}

func TestScanDegrades(t *testing.T) {
	t.Run("nothing there", func(t *testing.T) {
		inv, err := setup.NewScanner(abs("/home/u/.codex"), abs("/home/u"), tree{}.open).Scan(t.Context())
		if err != nil || inv.Found || inv.Items() != 0 {
			t.Errorf("Scan = %+v, %v", inv, err)
		}
	})
	t.Run("broken files are warnings", func(t *testing.T) {
		broken := tree{abs("/home/u/.codex"): {
			"config.toml": file("model = \n"),
			"hooks.json":  file("{"),
			"AGENTS.md":   file("still read\n"),
		}}
		inv, err := setup.NewScanner(abs("/home/u/.codex"), abs("/home/u"), broken.open).Scan(t.Context())
		if err == nil || !strings.Contains(err.Error(), "config.toml") || !strings.Contains(err.Error(), "hooks.json") {
			t.Errorf("err = %v, want both files named", err)
		}
		if len(inv.Instructions) != 1 {
			t.Errorf("the rest was not read: %+v", inv)
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := setup.NewScanner(abs("/home/u/.codex"), abs("/home/u"), fixture().open).Scan(ctx); err == nil {
			t.Error("a cancelled scan succeeded")
		}
	})
}

func TestScanMarksAppServers(t *testing.T) {
	config := `
[mcp_servers.node_repl]
command = "/Applications/ChatGPT.app/Contents/Resources/cua_node/bin/node_repl"
env = { NODE_REPL_TRUSTED_SERVICES = "SECRET_APP_ENV" }

[mcp_servers.computer-use]
command = "./Codex Computer Use.app/Contents/SharedSupport/Client.app/Contents/MacOS/Client"
args = ["mcp"]

[mcp_servers.helper]
command = "node"
env = { CODEX_CLI_PATH = "SECRET_APP_CLI", SKY_CUA_SERVICE_PATH = "SECRET_APP_SKY" }

[mcp_servers.home]
command = "codex-tool"
env = { CODEX_HOME = "SECRET_APP_HOME" }

[mcp_servers.fs]
command = "npx"
args = ["-y", "fs-mcp"]
env = { FS_TOKEN = "SECRET_FS", NOTE = "CODEX_HOME" }

[mcp_servers.tools]
command = "/Applications/Codex Tools/bin/mcp"

[mcp_servers.bundle]
command = "/Applications/MyCodex.app/Contents/MacOS/mcp"

[mcp_servers.docs]
url = "https://docs.example/mcp"
`
	fsys := tree{abs("/home/u/.codex"): {"config.toml": file(config)}}
	inv, err := setup.NewScanner(abs("/home/u/.codex"), abs("/home/u"), fsys.open).Scan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"node_repl": true, "computer-use": true, "helper": true, "home": true,
		"fs": false, "tools": false, "bundle": false, "docs": false,
	}
	for _, s := range inv.MCPServers {
		if app, ok := want[s.Name]; !ok || s.App != app {
			t.Errorf("%s: App = %v, want %v", s.Name, s.App, app)
		}
	}
	if len(inv.MCPServers) != len(want) {
		t.Errorf("got %d servers, want %d", len(inv.MCPServers), len(want))
	}
	data, err := json.Marshal(inv)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "SECRET_") {
		t.Errorf("inventory holds a secret:\n%s", data)
	}
}
