package harness

import (
	"cmp"
	"slices"
	"strings"

	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/usage"
)

// AppTool is an MCP server the history called that no configuration file
// defines. It is built into the Claude apps or comes from a claude.ai
// connector, so it does not move with the configuration.
type AppTool struct {
	// Server is the name the tools were called under.
	Server string
	// Name is the readable name: a connector's product, or the server.
	Name  string
	Calls int
	// Connector is true for claude.ai connectors.
	Connector bool
	// Browser is true for tools that drive a web browser, which a browser
	// MCP server can replace.
	Browser bool
}

// AppTools lists the app tools in the history, in history order.
func AppTools(inv setup.Inventory, history usage.Profile) []AppTool {
	var out []AppTool
	for _, t := range history.MCPServers {
		if configured(inv, t.Name) || AppPlumbing(t.Name) {
			continue
		}
		name := connectorName(t.Name, t.Tools)
		out = append(out, AppTool{Server: t.Name, Name: name, Calls: t.Count, Connector: name != t.Name, Browser: isBrowser(t.Name)})
	}
	return out
}

// browserServers are MCP servers that drive a web browser.
var browserServers = []string{"chrome-devtools", "playwright", "puppeteer", "browser"}

func isBrowser(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "browser") || strings.Contains(lower, "chrome")
}

// BrowserReplacement names the browser MCP server to use instead of the
// Claude apps' browser tools, preferring one the user already configured.
func BrowserReplacement(inv setup.Inventory) string {
	for _, s := range inv.MCPServers {
		for _, b := range browserServers {
			if strings.Contains(strings.ToLower(s.Name), b) {
				return "the " + s.Name + " MCP server you already have"
			}
		}
	}
	return "a browser MCP server such as chrome-devtools-mcp or @playwright/mcp"
}

// configured reports whether a server name seen in tool calls matches a
// configured server. Plugin servers appear in tool names as
// plugin_<plugin>_<server>.
func configured(inv setup.Inventory, called string) bool {
	norm := func(s string) string { return strings.ToLower(strings.NewReplacer("-", "_", " ", "_").Replace(s)) }
	c := norm(called)
	for _, s := range inv.MCPServers {
		n := norm(s.Name)
		if c == n || strings.HasSuffix(c, "_"+n) {
			return true
		}
	}
	return false
}

// SkillLoads counts Skill tool calls by skill name. Plugin skills are called
// as "plugin:skill" and inventoried by the skill's own name, so both forms
// are counted under the bare name.
func SkillLoads(history usage.Profile) map[string]int {
	loads := map[string]int{}
	for _, t := range history.Skills {
		name := t.Name
		if _, bare, ok := strings.Cut(name, ":"); ok {
			name = bare
		}
		loads[name] += t.Count
	}
	return loads
}

// HookRuns counts hook runs by event.
func HookRuns(history usage.Profile) map[string]int {
	runs := map[string]int{}
	for _, t := range history.HookRuns {
		runs[t.Name] = t.Count
	}
	return runs
}

// SettingFeatures lists the settings keys a harness has a rule for, each
// once, in name order. targets gives each key's status and explanation.
func SettingFeatures(inv setup.Inventory, targets map[string]Feature) []Feature {
	var out []Feature
	for _, f := range inv.Settings {
		for _, key := range f.Keys {
			if t, ok := targets[key]; ok {
				t.Area, t.Name = AreaSettings, key
				out = append(out, t)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b Feature) int { return cmp.Compare(a.Name, b.Name) })
	return slices.CompactFunc(out, func(a, b Feature) bool { return a.Name == b.Name })
}

// AppPlumbing reports whether a server is part of the Claude desktop app's
// own interface, such as its task list, session cards and pull request
// status. These are the harness's plumbing rather than tools the user relies
// on, like the built-in tools, and every harness has its own.
func AppPlumbing(server string) bool {
	return strings.HasPrefix(server, "ccd_")
}
