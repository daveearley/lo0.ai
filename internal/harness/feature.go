// Package harness describes the coding harnesses lo0 can move a Claude Code
// or Codex setup to, and what happens to each piece of that setup in them. Each
// harness lives in its own subpackage, which maps the setup and plans the
// migration; this package holds what they share.
package harness

import "lo0.ai/lo0/internal/stats"

// ID identifies a target harness.
type ID string

// Target harnesses.
const (
	// ClaudeCode is Claude Code itself, pointed at a local model.
	ClaudeCode ID = "claude-code"
	// OpenCode is the open-source OpenCode harness.
	OpenCode ID = "opencode"
	// Codex is OpenAI's Codex, pointed at a local model.
	Codex ID = "codex"
)

// Status says what happens to a feature in the target harness.
type Status string

// Feature statuses.
const (
	// StatusNative works as it is: the target reads the Claude Code file.
	StatusNative Status = "native"
	// StatusTranslate needs converting, but nothing is lost.
	StatusTranslate Status = "translate"
	// StatusPartial converts with reduced behaviour.
	StatusPartial Status = "partial"
	// StatusLost has no equivalent.
	StatusLost Status = "lost"
)

// Area groups features for reporting.
type Area string

// Feature areas, in report order.
const (
	AreaInstructions Area = "instructions"
	AreaMCP          Area = "mcp"
	AreaAppTools     Area = "app-tools"
	AreaHooks        Area = "hooks"
	AreaCommands     Area = "commands"
	AreaSubagents    Area = "subagents"
	AreaSkills       Area = "skills"
	AreaPlugins      Area = "plugins"
	AreaPermissions  Area = "permissions"
	AreaSettings     Area = "settings"
)

// Areas lists every area in report order.
var Areas = []Area{
	AreaInstructions, AreaMCP, AreaAppTools, AreaHooks, AreaCommands,
	AreaSubagents, AreaSkills, AreaPlugins, AreaPermissions, AreaSettings,
}

// Feature is one piece of the user's setup and its fate in the target.
type Feature struct {
	Area   Area   `json:"area"`
	Name   string `json:"name"`
	Status Status `json:"status"`
	// How says where the feature goes in the target, or why it cannot.
	How string `json:"how"`
	// Migrated is true when lo0 migrate converts the feature itself. Native
	// features need nothing, so it is only set on the others.
	Migrated bool `json:"migrated,omitzero"`
	// Calls is how often the session history used it, where that applies:
	// tool calls for app tools, runs of the event for hooks and loads for
	// skills.
	Calls int `json:"calls,omitzero"`
	// From names the harness whose setup the feature comes from, when it
	// is not Claude Code's.
	From string `json:"from,omitzero"`
}

// Carryover summarises features by status.
type Carryover struct {
	Native, Translate, Partial, Lost int
}

// Summarize counts features by status, optionally only in one area.
func Summarize(features []Feature, area Area) Carryover {
	var c Carryover
	for _, f := range features {
		if area != "" && f.Area != area {
			continue
		}
		switch f.Status {
		case StatusNative:
			c.Native++
		case StatusTranslate:
			c.Translate++
		case StatusPartial:
			c.Partial++
		case StatusLost:
			c.Lost++
		}
	}
	return c
}

// Total is the number of features counted.
func (c Carryover) Total() int {
	return c.Native + c.Translate + c.Partial + c.Lost
}

// CarriedShare is the share of features that survive in some form.
func (c Carryover) CarriedShare() float64 {
	return stats.Share(c.Native+c.Translate+c.Partial, c.Total())
}

// AppCalls sums history calls to app tools with the given status and names
// them in order.
func AppCalls(features []Feature, status Status) (int, []string) {
	n := 0
	var names []string
	for _, f := range features {
		if f.Area == AreaAppTools && f.Status == status {
			n += f.Calls
			names = append(names, f.Name)
		}
	}
	return n, names
}
