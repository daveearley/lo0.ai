// Package setup inventories a Codex installation: config.toml with its
// model providers and MCP servers, profile files, AGENTS.md files, skills,
// custom prompts and agents, hooks, rules and plugins, for the user and
// every project config.toml names.
//
// The inventory records names, counts and shapes only. It never keeps file
// contents, command arguments, environment values or header values, because
// those routinely hold secrets.
package setup

// Scope is where a piece of configuration comes from.
type Scope string

// Scopes.
const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

// Origin locates a configuration item.
type Origin struct {
	Scope Scope `json:"scope"`
	// Project is the project directory for project scope.
	Project string `json:"project,omitzero"`
	// Path is the file the item was read from.
	Path string `json:"path"`
}

// InstructionFile is an AGENTS.md or AGENTS.override.md file.
type InstructionFile struct {
	Origin
	// Override is true for AGENTS.override.md, which Codex reads instead of
	// AGENTS.md in the same folder.
	Override bool  `json:"override,omitzero"`
	Bytes    int64 `json:"bytes"`
	Lines    int   `json:"lines"`
}

// Transport is how Codex talks to an MCP server.
type Transport string

// MCP transports.
const (
	TransportStdio Transport = "stdio"
	TransportHTTP  Transport = "http"
)

// MCPServer is one configured MCP server.
type MCPServer struct {
	Origin
	Name      string    `json:"name"`
	Transport Transport `json:"transport"`
	// Command is the executable's base name for stdio servers, such as "npx".
	Command string `json:"command,omitzero"`
	// EnvKeys are the names set in env and passed through by env_vars.
	EnvKeys []string `json:"envKeys,omitzero"`
	// HeaderKeys are the names set in http_headers and env_http_headers.
	HeaderKeys []string `json:"headerKeys,omitzero"`
	// Cwd is true when a stdio server sets the folder it starts in.
	Cwd bool `json:"cwd,omitzero"`
	// Disabled is true when the server has enabled = false.
	Disabled bool `json:"disabled,omitzero"`
	// App is true for a server the ChatGPT or Codex app adds for itself,
	// which works only inside that app.
	App bool `json:"app,omitzero"`
}

// Provider is a model provider defined in config.toml.
type Provider struct {
	ID   string `json:"id"`
	Name string `json:"name,omitzero"`
	// Local is true when its base_url is on this machine.
	Local bool `json:"local,omitzero"`
}

// Profile is a <name>.config.toml file in the Codex home, which codex
// --profile <name> lays over config.toml.
type Profile struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Model    string `json:"model,omitzero"`
	Provider string `json:"provider,omitzero"`
}

// Hook is one hook handler bound to an event.
type Hook struct {
	Origin
	Event   string `json:"event"`
	Matcher string `json:"matcher,omitzero"`
	// Type is the handler type, such as "command".
	Type string `json:"type"`
}

// ExtensionKind is the kind of a user-defined extension.
type ExtensionKind string

// Extension kinds.
const (
	// ExtensionSkill is a folder with a SKILL.md.
	ExtensionSkill ExtensionKind = "skill"
	// ExtensionPrompt is a custom prompt in prompts/, run as /prompts:name.
	ExtensionPrompt ExtensionKind = "prompt"
	// ExtensionAgent is a custom agent defined in a TOML file in agents/.
	ExtensionAgent ExtensionKind = "agent"
)

// Extension is a skill, custom prompt or custom agent.
type Extension struct {
	Origin
	Kind ExtensionKind `json:"kind"`
	Name string        `json:"name"`
}

// RuleFile is a .rules file of command rules.
type RuleFile struct {
	Origin
	// Rules is how many prefix_rule calls the file makes.
	Rules int `json:"rules"`
}

// Plugin is a Codex plugin named in config.toml.
type Plugin struct {
	Name        string `json:"name"`
	Marketplace string `json:"marketplace,omitzero"`
	Enabled     bool   `json:"enabled"`
}

// Inventory is everything found in a Codex installation.
type Inventory struct {
	// Found is true when the Codex home directory has a config.toml.
	Found bool `json:"found,omitzero"`
	// Path is config.toml's path.
	Path string `json:"path,omitzero"`
	// Model and Provider are the top-level model and model_provider keys.
	Model    string `json:"model,omitzero"`
	Provider string `json:"provider,omitzero"`
	// Keys are the top-level keys config.toml sets, tables included.
	Keys         []string          `json:"keys,omitzero"`
	Providers    []Provider        `json:"providers,omitzero"`
	Profiles     []Profile         `json:"profiles,omitzero"`
	Projects     []string          `json:"projects,omitzero"`
	Instructions []InstructionFile `json:"instructions,omitzero"`
	MCPServers   []MCPServer       `json:"mcpServers,omitzero"`
	Hooks        []Hook            `json:"hooks,omitzero"`
	Extensions   []Extension       `json:"extensions,omitzero"`
	Rules        []RuleFile        `json:"rules,omitzero"`
	Plugins      []Plugin          `json:"plugins,omitzero"`
}

// ExtensionsOf returns the extensions of kind k.
func (inv Inventory) ExtensionsOf(k ExtensionKind) []Extension {
	var out []Extension
	for _, e := range inv.Extensions {
		if e.Kind == k {
			out = append(out, e)
		}
	}
	return out
}

// Items counts what the setup holds that could carry over to another
// harness: instruction files, MCP servers, hooks, extensions, rule files
// and enabled plugins.
func (inv Inventory) Items() int {
	n := len(inv.Instructions) + len(inv.MCPServers) + len(inv.Hooks) + len(inv.Extensions) + len(inv.Rules)
	for _, p := range inv.Plugins {
		if p.Enabled {
			n++
		}
	}
	return n
}
