package setup

import (
	"fmt"
	"maps"
	"net/url"
	"path"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// configFile is the part of config.toml the inventory reads. Values that
// can hold secrets, such as env and header values, are decoded only to be
// counted by key.
type configFile struct {
	Model          string                    `toml:"model"`
	ModelProvider  string                    `toml:"model_provider"`
	ModelProviders map[string]providerConfig `toml:"model_providers"`
	MCPServers     map[string]MCPConfig      `toml:"mcp_servers"`
	Projects       map[string]struct{}       `toml:"projects"`
	Plugins        map[string]struct {
		Enabled *bool `toml:"enabled"`
	} `toml:"plugins"`
	// Hooks holds an array of matcher groups per event, and a state table
	// that is not an event.
	Hooks map[string]any `toml:"hooks"`
}

type providerConfig struct {
	Name    string `toml:"name"`
	BaseURL string `toml:"base_url"`
}

// MCPConfig is an MCP server as config.toml defines it: a stdio server
// with command, or a Streamable HTTP server with url.
type MCPConfig struct {
	Command           string            `toml:"command"`
	Args              []string          `toml:"args"`
	Env               map[string]string `toml:"env"`
	EnvVars           []EnvVar          `toml:"env_vars"`
	Cwd               string            `toml:"cwd"`
	URL               string            `toml:"url"`
	BearerTokenEnvVar string            `toml:"bearer_token_env_var"`
	HTTPHeaders       map[string]string `toml:"http_headers"`
	EnvHTTPHeaders    map[string]string `toml:"env_http_headers"`
	Enabled           *bool             `toml:"enabled"`
}

// EnvVar is one env_vars entry: a variable's name, written as a string or
// as a table with a name and a source.
type EnvVar string

// UnmarshalTOML implements toml.Unmarshaler.
func (e *EnvVar) UnmarshalTOML(v any) error {
	switch v := v.(type) {
	case string:
		*e = EnvVar(v)
	case map[string]any:
		name, _ := v["name"].(string)
		*e = EnvVar(name)
	default:
		return fmt.Errorf("env_vars entry is a %T, not a name or a table", v)
	}
	return nil
}

func (c MCPConfig) envVarNames() []string {
	out := make([]string, 0, len(c.EnvVars))
	for _, e := range c.EnvVars {
		if e != "" {
			out = append(out, string(e))
		}
	}
	return out
}

// parsedConfig is what one config.toml contributes to the inventory.
type parsedConfig struct {
	file configFile
	keys []string
}

func parseConfig(data []byte) (parsedConfig, error) {
	var keys map[string]any
	if _, err := toml.Decode(string(data), &keys); err != nil {
		return parsedConfig{}, err
	}
	var f configFile
	if _, err := toml.Decode(string(data), &f); err != nil {
		return parsedConfig{}, err
	}
	return parsedConfig{file: f, keys: slices.Sorted(maps.Keys(keys))}, nil
}

func mcpServers(origin Origin, configs map[string]MCPConfig) []MCPServer {
	out := make([]MCPServer, 0, len(configs))
	for _, name := range slices.Sorted(maps.Keys(configs)) {
		c := configs[name]
		s := MCPServer{Origin: origin, Name: name, Transport: TransportStdio, Disabled: c.Enabled != nil && !*c.Enabled, App: c.app()}
		if c.Command == "" && c.URL != "" {
			s.Transport = TransportHTTP
			headers := slices.Collect(maps.Keys(c.HTTPHeaders))
			headers = append(headers, slices.Collect(maps.Keys(c.EnvHTTPHeaders))...)
			if c.BearerTokenEnvVar != "" {
				headers = append(headers, "Authorization")
			}
			s.HeaderKeys = sortedUnique(headers)
		} else {
			s.Command, s.Cwd = programName(c.Command), c.Cwd != ""
			s.EnvKeys = sortedUnique(append(slices.Collect(maps.Keys(c.Env)), c.envVarNames()...))
		}
		out = append(out, s)
	}
	return out
}

// appEnv are variables the Codex app sets for the servers it adds to
// config.toml, and that a server the user adds has no reason to set.
var appEnv = []string{"CODEX_CLI_PATH", "CODEX_HOME"}

// app reports whether the ChatGPT or Codex app added the server for
// itself: its program is inside the app's bundle, as with node_repl in
// ChatGPT.app, or its env holds variables only the app sets.
func (c MCPConfig) app() bool {
	if slices.ContainsFunc(appEnv, func(k string) bool { _, ok := c.Env[k]; return ok }) {
		return true
	}
	parts := strings.Split(c.Command, "/")
	for i, p := range parts[:max(len(parts)-1, 0)] {
		if strings.HasSuffix(p, ".app") && (strings.HasPrefix(p, "ChatGPT") || strings.HasPrefix(p, "Codex")) && parts[i+1] == "Contents" {
			return true
		}
	}
	return false
}

func sortedUnique(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	slices.Sort(s)
	return slices.Compact(s)
}

func providers(configs map[string]providerConfig) []Provider {
	out := make([]Provider, 0, len(configs))
	for _, id := range slices.Sorted(maps.Keys(configs)) {
		c := configs[id]
		out = append(out, Provider{ID: id, Name: c.Name, Local: IsLocal(c.BaseURL)})
	}
	return out
}

// IsLocal reports whether a base URL points at this machine.
func IsLocal(base string) bool {
	u, err := url.Parse(base)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

// pluginDefaults is the key in [plugins] that sets every plugin's default
// rather than naming one.
const pluginDefaults = "_default"

// plugins lists the plugins config.toml names. One without enabled inherits
// the installed default, which is on.
func plugins(f configFile) []Plugin {
	out := make([]Plugin, 0, len(f.Plugins))
	for _, id := range slices.Sorted(maps.Keys(f.Plugins)) {
		if id == pluginDefaults {
			continue
		}
		name, marketplace, _ := strings.Cut(id, "@")
		p := f.Plugins[id]
		out = append(out, Plugin{Name: name, Marketplace: marketplace, Enabled: p.Enabled == nil || *p.Enabled})
	}
	return out
}

// programName is the base name of the program a stdio server runs, such as
// "npx". Only the name is kept, and one with anything but letters, digits
// and ._+- is dropped, since a hand-written command can carry arguments.
func programName(command string) string {
	fields := strings.Fields(strings.ReplaceAll(command, `\`, "/"))
	if len(fields) == 0 || strings.Contains(fields[0], "=") {
		return ""
	}
	name := path.Base(fields[0])
	if strings.ContainsFunc(name, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && !strings.ContainsRune("._+-", r)
	}) {
		return ""
	}
	return name
}

// inlineHooks lists the hooks a config.toml sets in its [hooks] table, in
// the shape of hooks.json: [[hooks.Event]] groups with a matcher and
// [[hooks.Event.hooks]] handlers.
func inlineHooks(origin Origin, table map[string]any) []Hook {
	var out []Hook
	for _, event := range slices.Sorted(maps.Keys(table)) {
		groups, ok := table[event].([]map[string]any)
		if !ok {
			continue
		}
		for _, g := range groups {
			matcher, _ := g["matcher"].(string)
			handlers, _ := g["hooks"].([]map[string]any)
			for _, h := range handlers {
				kind, _ := h["type"].(string)
				out = append(out, Hook{Origin: origin, Event: event, Matcher: matcher, Type: kind})
			}
		}
	}
	return out
}

// MCPConfigs decodes the [mcp_servers] tables of a config.toml, values
// included, for moving them to another harness's file on this machine.
func MCPConfigs(data []byte) (map[string]MCPConfig, error) {
	var f struct {
		MCPServers map[string]MCPConfig `toml:"mcp_servers"`
	}
	if _, err := toml.Decode(string(data), &f); err != nil {
		return nil, err
	}
	return f.MCPServers, nil
}

// HTTP reports whether the server is a Streamable HTTP one.
func (c MCPConfig) HTTP() bool { return c.Command == "" && c.URL != "" }

// Environment is what a stdio server is given beyond Codex's own
// environment: env as written, and each env_vars name passed on as
// ref(name).
func (c MCPConfig) Environment(ref func(name string) string) map[string]string {
	out := make(map[string]string, len(c.Env)+len(c.EnvVars))
	maps.Copy(out, c.Env)
	for _, name := range c.envVarNames() {
		if _, ok := out[name]; !ok {
			out[name] = ref(name)
		}
	}
	return out
}

// Headers is what an HTTP server is sent: http_headers as written, each
// env_http_headers entry as ref(variable), and bearer_token_env_var as an
// Authorization header.
func (c MCPConfig) Headers(ref func(name string) string) map[string]string {
	out := make(map[string]string, len(c.HTTPHeaders)+len(c.EnvHTTPHeaders)+1)
	maps.Copy(out, c.HTTPHeaders)
	for header, name := range c.EnvHTTPHeaders {
		out[header] = ref(name)
	}
	if c.BearerTokenEnvVar != "" {
		out["Authorization"] = "Bearer " + ref(c.BearerTokenEnvVar)
	}
	return out
}

// AgentConfig is the part of a custom agent's TOML file another harness
// can use.
type AgentConfig struct {
	Name                  string `toml:"name"`
	Description           string `toml:"description"`
	DeveloperInstructions string `toml:"developer_instructions"`
}

// DecodeAgent reads a custom agent's TOML file.
func DecodeAgent(data []byte) (AgentConfig, error) {
	var a AgentConfig
	_, err := toml.Decode(string(data), &a)
	return a, err
}
