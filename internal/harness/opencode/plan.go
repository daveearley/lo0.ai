package opencode

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/migrate"
)

// Protocol implements migrate.Harness: OpenCode reaches local runtimes
// through their OpenAI-compatible API.
func (Harness) Protocol() migrate.Protocol { return migrate.OpenAI }

// Plan implements migrate.Harness. Everything goes into OpenCode's global
// configuration directory, in the shape the installed release reads; the
// user's repositories are left alone, so project-level items are listed for
// the user to convert.
func (Harness) Plan(req migrate.Request, served migrate.Served) (migrate.Setup, error) {
	dir := configDir(req)
	r := releaseOf(req.Target)
	var steps []migrate.Step
	if req.LookPath == nil || !found(req.LookPath, Program) {
		steps = append(steps, install())
	}
	steps = append(steps, configStep(req, r, served, dir))
	if s, ok := copyStep(req, r, dir, setup.ExtensionCommand); ok {
		steps = append(steps, s)
	}
	if s, ok := copyStep(req, r, dir, setup.ExtensionAgent); ok {
		steps = append(steps, s)
	}
	if s, ok := skillStep(req, dir); ok {
		steps = append(steps, s)
	}
	steps = append(steps, codexSteps(req, dir)...)
	return migrate.Setup{
		Steps: steps,
		Then:  []string{"Run opencode in any project to use " + served.DisplayName + " through " + served.RuntimeName + "."},
	}, nil
}

// Program is the OpenCode command.
const Program = "opencode"

func install() migrate.Step {
	return migrate.Step{
		Kind:   migrate.KindManual,
		Title:  "Install OpenCode",
		Detail: []string{"Run: " + installScript, "Or: npm install -g " + npmPackage},
		Run: func(_ context.Context, fx migrate.Effects, _ func(done, total int64)) error {
			_, err := fx.Runner.LookPath(Program)
			return err
		},
	}
}

// configDir is OpenCode's global configuration directory. OpenCode 2 roots
// it at XDG_CONFIG_HOME, or ~/.config, on every platform
// (packages/util/src/global-roots.ts), in the place OpenCode 1 used.
func configDir(req migrate.Request) string {
	base := filepath.Join(req.Home, ".config")
	if req.Getenv != nil {
		base = cmp.Or(req.Getenv("XDG_CONFIG_HOME"), base)
	}
	return filepath.Join(base, "opencode")
}

func found(look func(string) (string, error), program string) bool {
	_, err := look(program)
	return err == nil
}

// configStep merges the provider, the default model, the user's MCP servers
// and their user-level instructions, from Claude Code and Codex, into
// opencode.json, keeping everything else in the file as it is. OpenCode 2
// does not read ~/.claude/CLAUDE.md, and no release reads the Codex home's
// AGENTS.md, so those files are added to the instructions.
func configStep(req migrate.Request, r release, served migrate.Served, dir string) migrate.Step {
	path := filepath.Join(dir, "opencode.json")
	provider := string(served.Runtime)
	var servers, rules []string
	for _, s := range req.Setup.MCPServers {
		if s.Scope == setup.ScopeUser {
			servers = append(servers, s.Name)
		}
	}
	for _, f := range req.Setup.Instructions {
		own := f.Kind == setup.InstructionRule || (r == v2 && f.Kind == setup.InstructionClaudeMD)
		if own && f.Scope == setup.ScopeUser {
			rules = append(rules, f.Path)
		}
	}
	codex := codexServerNames(req)
	servers = append(servers, codex...)
	rules = append(rules, codexInstructionPaths(req)...)
	detail := []string{
		path, `"model": "` + provider + "/" + served.Model + `"`,
		`"` + r.providerKey() + "." + provider + `" at ` + served.BaseURL + "/v1",
	}
	if len(servers) > 0 {
		detail = append(detail, `"`+r.mcpKey()+`": `+strings.Join(servers, ", "))
	}
	if len(rules) > 0 {
		detail = append(detail, `"instructions": `+fileCount(len(rules)))
	}
	return migrate.Step{
		Kind:   migrate.KindWrite,
		Title:  "Add " + served.DisplayName + " to OpenCode's configuration",
		Detail: detail,
		Run: func(ctx context.Context, fx migrate.Effects, progress func(done, total int64)) error {
			mcp, err := userServers(fx.Files, req.Layout.GlobalConfig, r)
			if err != nil {
				return err
			}
			if mcp, err = addCodexServers(fx.Files, req, codex, mcp, r); err != nil {
				return err
			}
			build := func(old []byte) ([]byte, error) { return mergeConfig(old, r, provider, served, mcp, rules) }
			// It may hold MCP env and header values, so only the user reads it.
			return migrate.WriteFile(path, 0o600, build)(ctx, fx, progress)
		},
	}
}

func fileCount(n int) string {
	if n == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", n)
}

// mergeConfig adds lo0's entries to an opencode.json. Keys the user already
// set are kept: an MCP server of the same name or a rule already listed is
// not added twice, but the provider entry and default model are lo0's.
//
// OpenCode 2 reads a file that mixes OpenCode 1 and 2 keys, merging the two
// forms entry by entry (packages/core/src/config/normalize.ts). lo0 writes
// its entries in the installed release's form, and drops its own provider
// from the OpenCode 1 map so the two forms never disagree.
func mergeConfig(old []byte, r release, provider string, served migrate.Served, mcp map[string]any, rules []string) ([]byte, error) {
	cfg := map[string]any{}
	if len(old) > 0 {
		if err := json.Unmarshal(old, &cfg); err != nil {
			return nil, fmt.Errorf("lo0 can only update an opencode.json without comments: %w", err)
		}
	}
	if _, ok := cfg["$schema"]; !ok {
		cfg["$schema"] = "https://opencode.ai/config.json"
	}
	if r == v2 {
		if legacy, ok := cfg["provider"].(map[string]any); ok {
			delete(legacy, provider)
			if len(legacy) == 0 {
				delete(cfg, "provider")
			}
		}
	}
	object(cfg, r.providerKey())[provider] = providerEntry(served, r)
	cfg["model"] = provider + "/" + served.Model
	if len(mcp) > 0 {
		legacy := object(cfg, "mcp")
		servers := legacy
		if r == v2 {
			servers = object(legacy, "servers")
		}
		for name, s := range mcp {
			_, had := servers[name]
			_, hadLegacy := legacy[name]
			if !had && !hadLegacy {
				servers[name] = s
			}
		}
	}
	if len(rules) > 0 {
		var list []any
		if have, ok := cfg["instructions"].([]any); ok {
			list = have
		}
		for _, r := range rules {
			if !slices.Contains(list, any(r)) {
				list = append(list, r)
			}
		}
		cfg["instructions"] = list
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// providerEntry is the OpenCode provider that reaches served through the
// runtime's OpenAI-compatible API. OpenCode 2 names the package and its
// settings differently and ships its own OpenAI-compatible package
// (https://opencode.ai/v2/docs/providers/).
func providerEntry(served migrate.Served, r release) map[string]any {
	entry := map[string]any{
		"name": served.RuntimeName + " (local)",
		"models": map[string]any{served.Model: map[string]any{
			"name":  served.DisplayName,
			"limit": map[string]any{"context": served.Context, "output": served.OutputTokens()},
		}},
	}
	connection := map[string]any{"baseURL": served.BaseURL + "/v1"}
	if r == v1 {
		entry["npm"], entry["options"] = "@ai-sdk/openai-compatible", connection
	} else {
		entry["package"], entry["settings"] = "@opencode/ai/providers/openai-compatible", connection
	}
	return entry
}

// object returns cfg[key] as an object, creating it if it is missing or not
// an object.
func object(cfg map[string]any, key string) map[string]any {
	if m, ok := cfg[key].(map[string]any); ok {
		return m
	}
	m := map[string]any{}
	cfg[key] = m
	return m
}

// userServers reads the user-level MCP servers from Claude Code's global
// configuration and converts them to OpenCode entries. Values, including
// secrets in env and headers, move from one local file to the other and are
// never shown.
func userServers(files migrate.Files, path string, r release) (map[string]any, error) {
	data, err := files.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var global struct {
		MCPServers map[string]claudeServer `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &global); err != nil {
		return nil, fmt.Errorf("reading MCP servers from %s: %w", path, err)
	}
	out := make(map[string]any, len(global.MCPServers))
	for name, s := range global.MCPServers {
		out[name] = s.openCode(r)
	}
	return out, nil
}

// claudeServer is an MCP server as Claude Code configures it.
type claudeServer struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// openCode converts the server. Claude Code expands ${VAR} in these values
// and OpenCode expands {env:VAR}, so references to the environment keep
// working. OpenCode 1 servers are switched on with "enabled"; OpenCode 2
// has "disabled" instead and connects to every server without it.
func (s claudeServer) openCode(r release) map[string]any {
	out := map[string]any{}
	if r == v1 {
		out["enabled"] = true
	}
	if s.URL != "" && s.Command == "" {
		out["type"], out["url"] = "remote", envRefs(s.URL)
		if len(s.Headers) > 0 {
			out["headers"] = envMap(s.Headers)
		}
		return out
	}
	command := []string{envRefs(s.Command)}
	for _, a := range s.Args {
		command = append(command, envRefs(a))
	}
	out["type"], out["command"] = "local", command
	if len(s.Env) > 0 {
		out["environment"] = envMap(s.Env)
	}
	return out
}

func envMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = envRefs(v)
	}
	return out
}
