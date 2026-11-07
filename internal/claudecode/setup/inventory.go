// Package setup inventories a Claude Code installation: instruction files,
// MCP servers, hooks, slash commands, subagents, skills, plugins, permission
// rules and settings, across every scope Claude Code reads.
//
// The inventory records names, counts and shapes only. It never keeps file
// contents, command arguments, environment values or header values, because
// those routinely hold secrets.
package setup

// Scope is where a piece of configuration comes from.
type Scope string

// Scopes in the order Claude Code gives them precedence, lowest first.
const (
	ScopeUser    Scope = "user"
	ScopePlugin  Scope = "plugin"
	ScopeProject Scope = "project"
	ScopeLocal   Scope = "local"
	ScopeManaged Scope = "managed"
)

// Origin locates a configuration item.
type Origin struct {
	Scope Scope `json:"scope"`
	// Project is the project directory for project and local scope.
	Project string `json:"project,omitzero"`
	// Plugin is the plugin name for plugin scope.
	Plugin string `json:"plugin,omitzero"`
	// Path is the file the item was read from.
	Path string `json:"path"`
}

// InstructionKind distinguishes the instruction files Claude Code loads.
type InstructionKind string

// Instruction file kinds.
const (
	InstructionClaudeMD InstructionKind = "CLAUDE.md"
	InstructionLocalMD  InstructionKind = "CLAUDE.local.md"
	InstructionAgentsMD InstructionKind = "AGENTS.md"
	InstructionRule     InstructionKind = "rule"
)

// InstructionFile is a memory or rules file.
type InstructionFile struct {
	Origin
	Kind  InstructionKind `json:"kind"`
	Bytes int64           `json:"bytes"`
	Lines int             `json:"lines"`
}

// Transport is how Claude Code talks to an MCP server.
type Transport string

// MCP transports.
const (
	TransportStdio Transport = "stdio"
	TransportHTTP  Transport = "http"
	TransportSSE   Transport = "sse"
)

// MCPServer is one configured MCP server.
type MCPServer struct {
	Origin
	Name      string    `json:"name"`
	Transport Transport `json:"transport"`
	// Command is the executable's base name for stdio servers, such as "npx".
	Command    string   `json:"command,omitzero"`
	EnvKeys    []string `json:"envKeys,omitzero"`
	HeaderKeys []string `json:"headerKeys,omitzero"`
}

// Hook is one hook handler bound to an event.
type Hook struct {
	Origin
	Event   string `json:"event"`
	Matcher string `json:"matcher,omitzero"`
	// Type is the handler type, such as "command" or "prompt".
	Type string `json:"type"`
}

// ExtensionKind is the kind of a user-defined extension.
type ExtensionKind string

// Extension kinds.
const (
	ExtensionCommand ExtensionKind = "command"
	ExtensionAgent   ExtensionKind = "agent"
	ExtensionSkill   ExtensionKind = "skill"
)

// Extension is a slash command, subagent or skill.
type Extension struct {
	Origin
	Kind ExtensionKind `json:"kind"`
	Name string        `json:"name"`
}

// Plugin is an installed Claude Code plugin.
type Plugin struct {
	Name        string `json:"name"`
	Marketplace string `json:"marketplace,omitzero"`
	Version     string `json:"version,omitzero"`
	Enabled     bool   `json:"enabled"`
}

// Permissions summarises the permission rules in one settings file.
type Permissions struct {
	Origin
	Allow       int    `json:"allow"`
	Deny        int    `json:"deny"`
	Ask         int    `json:"ask"`
	DefaultMode string `json:"defaultMode,omitzero"`
}

// SettingsFile lists the top-level keys set in one settings file.
type SettingsFile struct {
	Origin
	Keys []string `json:"keys"`
}

// Plan is the Claude subscription Claude Code is signed in with.
type Plan string

// Plans.
const (
	PlanUnknown    Plan = ""
	PlanPro        Plan = "pro"
	PlanMax        Plan = "max"
	PlanTeam       Plan = "team"
	PlanEnterprise Plan = "enterprise"
	// PlanAPI means Claude Code is not signed in to a subscription, so usage
	// is billed per token.
	PlanAPI Plan = "api"
)

// Account is the subscription Claude Code uses. Only the plan is read; the
// account's name, email and identifiers are never loaded.
type Account struct {
	Plan Plan `json:"plan,omitzero"`
	// Tier is the Max usage multiple, such as "5x" or "20x".
	Tier string `json:"tier,omitzero"`
	// ExtraUsage is true when usage beyond the plan is billed.
	ExtraUsage bool `json:"extraUsage,omitzero"`
}

// Inventory is everything found in a Claude Code installation.
type Inventory struct {
	Account      Account           `json:"account,omitzero"`
	Projects     []string          `json:"projects,omitzero"`
	Instructions []InstructionFile `json:"instructions,omitzero"`
	MCPServers   []MCPServer       `json:"mcpServers,omitzero"`
	Hooks        []Hook            `json:"hooks,omitzero"`
	Extensions   []Extension       `json:"extensions,omitzero"`
	Plugins      []Plugin          `json:"plugins,omitzero"`
	Permissions  []Permissions     `json:"permissions,omitzero"`
	Settings     []SettingsFile    `json:"settings,omitzero"`
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
