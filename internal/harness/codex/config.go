package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"lo0.ai/lo0/internal/migrate"
)

// fallbackFile is what lo0 adds to project_doc_fallback_filenames, so Codex
// reads a project's CLAUDE.md where it has no AGENTS.md.
const fallbackFile = "CLAUDE.md"

// edit is what lo0 sets in config.toml.
type edit struct {
	provider string
	served   migrate.Served
	// servers are MCP servers to add, by name, as TOML table bodies.
	servers  map[string][]string
	fallback bool
}

// codexConfig is the part of config.toml lo0 changes, read back to check
// an edit did what it should.
type codexConfig struct {
	Model              string `toml:"model"`
	ModelProvider      string `toml:"model_provider"`
	ModelContextWindow int64  `toml:"model_context_window"`
	ModelProviders     map[string]struct {
		Name    string `toml:"name"`
		BaseURL string `toml:"base_url"`
	} `toml:"model_providers"`
	Fallback   []string                  `toml:"project_doc_fallback_filenames"`
	MCPServers map[string]toml.Primitive `toml:"mcp_servers"`
}

// mergeConfig makes lo0's changes to a config.toml and leaves everything
// else in it as the user wrote it. MCP servers the file already defines are
// kept, not replaced. The result is parsed again and compared with what
// was asked, and an edit the file's layout would make unsafe is refused.
func mergeConfig(old []byte, e edit) ([]byte, error) {
	var before map[string]any
	var have codexConfig
	if _, err := toml.Decode(string(old), &before); err != nil {
		return nil, fmt.Errorf("lo0 can only update a config.toml Codex can read: %w", err)
	}
	if _, err := toml.Decode(string(old), &have); err != nil {
		return nil, fmt.Errorf("lo0 can only update a config.toml Codex can read: %w", err)
	}
	d := parseDocument(string(old))
	d.setRoot("model", tomlString(e.served.Model))
	d.setRoot("model_provider", tomlString(e.provider))
	d.setRoot("model_context_window", strconv.Itoa(e.served.Context))
	fallback := have.Fallback
	if e.fallback && !slices.Contains(fallback, fallbackFile) {
		fallback = append(slices.Clone(fallback), fallbackFile)
		d.setRoot("project_doc_fallback_filenames", tomlArray(fallback))
	}
	d.setTable([]string{"model_providers", e.provider}, providerBody(e.served))
	for _, name := range slices.Sorted(maps.Keys(e.servers)) {
		if _, ok := have.MCPServers[name]; !ok {
			d.setTable([]string{"mcp_servers", name}, e.servers[name])
		}
	}
	data := []byte(d.String())

	var after map[string]any
	var got codexConfig
	_, errAny := toml.Decode(string(data), &after)
	_, errGot := toml.Decode(string(data), &got)
	p := got.ModelProviders[e.provider]
	switch {
	case errors.Join(errAny, errGot) != nil,
		got.Model != e.served.Model, got.ModelProvider != e.provider, got.ModelContextWindow != int64(e.served.Context),
		p.Name != providerName(e.served), p.BaseURL != baseURL(e.served),
		!slices.Equal(got.Fallback, fallback),
		slices.ContainsFunc(slices.Collect(maps.Keys(before)), func(k string) bool { _, ok := after[k]; return !ok }):
		return nil, fmt.Errorf("lo0 could not update config.toml without changing what else it says; set model_provider = %q and add [model_providers.%s] by hand", e.provider, e.provider)
	}
	return data, nil
}

func providerBody(s migrate.Served) []string {
	return []string{
		"name = " + tomlString(providerName(s)),
		"base_url = " + tomlString(baseURL(s)),
	}
}

func providerName(s migrate.Served) string { return s.RuntimeName + " (local)" }

func baseURL(s migrate.Served) string { return s.BaseURL + "/v1" }

// claudeServer is an MCP server as Claude Code configures it.
type claudeServer struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// userServers reads the named MCP servers from Claude Code's global
// configuration and converts them to config.toml tables. Values, including
// secrets in env and headers, move from one local file to the other and are
// never shown.
func userServers(files migrate.Files, path string, names []string) (map[string][]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
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
	out := map[string][]string{}
	for name, s := range global.MCPServers {
		if slices.Contains(names, name) {
			out[name] = s.codex()
		}
	}
	return out, nil
}

// envRef matches a value that is only a reference to one environment
// variable, ${VAR}, which Claude Code expands and Codex does not.
var envRef = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)

// bearerRef matches an Authorization header that sends a token from the
// environment.
var bearerRef = regexp.MustCompile(`^Bearer \$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)

// codex converts the server to the body of its [mcp_servers] table. A
// stdio server keeps its command, args and env; an env value that only
// passes on the variable of the same name becomes env_vars, which Codex
// fills from its own environment. An HTTP server keeps its url; headers
// that send environment variables become bearer_token_env_var and
// env_http_headers. Other values are copied as they are.
func (s claudeServer) codex() []string {
	if s.URL != "" && s.Command == "" {
		body := []string{"url = " + tomlString(s.URL)}
		headers, fromEnv := map[string]string{}, map[string]string{}
		for k, v := range s.Headers {
			switch m, b := envRef.FindStringSubmatch(v), bearerRef.FindStringSubmatch(v); {
			case b != nil && strings.EqualFold(k, "Authorization"):
				body = append(body, "bearer_token_env_var = "+tomlString(b[1]))
			case m != nil:
				fromEnv[k] = m[1]
			default:
				headers[k] = v
			}
		}
		if len(headers) > 0 {
			body = append(body, "http_headers = "+tomlInline(headers))
		}
		if len(fromEnv) > 0 {
			body = append(body, "env_http_headers = "+tomlInline(fromEnv))
		}
		return body
	}
	body := []string{"command = " + tomlString(s.Command)}
	if len(s.Args) > 0 {
		body = append(body, "args = "+tomlArray(s.Args))
	}
	env := map[string]string{}
	var pass []string
	for k, v := range s.Env {
		if m := envRef.FindStringSubmatch(v); m != nil && m[1] == k {
			pass = append(pass, k)
			continue
		}
		env[k] = v
	}
	if len(env) > 0 {
		body = append(body, "env = "+tomlInline(env))
	}
	if len(pass) > 0 {
		slices.Sort(pass)
		body = append(body, "env_vars = "+tomlArray(pass))
	}
	return body
}
