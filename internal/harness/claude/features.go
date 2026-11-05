// Package claude maps a Claude Code setup to Claude Code itself running
// against a local model, and plans the move: a claude-local command that
// starts the user's own Claude Code with the local runtime as its API.
// Nothing in the user's Claude Code configuration is changed, so the setup
// carries over as it is and the normal claude command keeps working.
package claude

import (
	"slices"

	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/usage"
)

// Harness is Claude Code on a local model as a migration target. The zero
// value is ready to use.
type Harness struct{}

// ID implements the harness contract.
func (Harness) ID() harness.ID { return harness.ClaudeCode }

// Name implements the harness contract.
func (Harness) Name() string { return "Claude Code on a local model" }

// Product implements app.Harness: the harness's own name, for short labels.
func (Harness) Product() string { return "Claude Code" }

// Features maps the user's setup to Claude Code with a local backend. Only
// what depends on Anthropic's servers or the Claude apps changes.
func (Harness) Features(inv setup.Inventory, history usage.Profile, _ harness.Installation) []harness.Feature {
	var out []harness.Feature
	asIs := func(area harness.Area, name, how string, calls int) {
		out = append(out, harness.Feature{Area: area, Name: name, Status: harness.StatusNative, How: how, Calls: calls})
	}
	for _, f := range inv.Instructions {
		asIs(harness.AreaInstructions, f.Path, "read as is", 0)
	}
	for _, s := range inv.MCPServers {
		asIs(harness.AreaMCP, s.Name, "read as is", 0)
	}
	out = append(out, appTools(inv, history)...)
	runs := harness.HookRuns(history)
	for _, h := range inv.Hooks {
		// Only prompt and agent hooks ask a model; command, http and
		// mcp_tool hooks run without one.
		if h.Type != "prompt" && h.Type != "agent" {
			asIs(harness.AreaHooks, h.Event, "runs as is", runs[h.Event])
			continue
		}
		out = append(out, harness.Feature{
			Area: harness.AreaHooks, Name: h.Event, Status: harness.StatusPartial, Calls: runs[h.Event],
			How: "runs, but the local model judges " + h.Type + " hooks instead of Claude",
		})
	}
	loads := harness.SkillLoads(history)
	for _, e := range inv.Extensions {
		switch e.Kind {
		case setup.ExtensionCommand:
			asIs(harness.AreaCommands, e.Name, "read as is", 0)
		case setup.ExtensionAgent:
			asIs(harness.AreaSubagents, e.Name, "read as is; opus, sonnet, haiku and fable resolve to local models", 0)
		case setup.ExtensionSkill:
			asIs(harness.AreaSkills, e.Name, "read as is", loads[e.Name])
		}
	}
	for _, p := range inv.Plugins {
		if p.Enabled {
			asIs(harness.AreaPlugins, p.Name, "loaded as is", 0)
		}
	}
	for _, p := range inv.Permissions {
		if p.Allow+p.Deny+p.Ask > 0 || p.DefaultMode != "" {
			asIs(harness.AreaPermissions, p.Path, "applied as is", 0)
		}
	}
	return slices.Concat(out, harness.SettingFeatures(inv, settingTargets))
}

// appTools describes the tools the Claude apps and claude.ai provide. The
// claude-local command signs in to the local runtime instead of claude.ai,
// so connectors do not load, and the apps' own tools stay in the apps.
func appTools(inv setup.Inventory, history usage.Profile) []harness.Feature {
	tools := harness.AppTools(inv, history)
	out := make([]harness.Feature, 0, len(tools))
	for _, t := range tools {
		f := harness.Feature{
			Area: harness.AreaAppTools, Name: t.Name, Calls: t.Calls, Status: harness.StatusLost,
			How: "provided by the Claude app, which keeps using Claude",
		}
		if t.Connector {
			f.How = "a claude.ai connector (" + t.Server + "); connectors need the claude.ai sign-in that the local setup replaces, " +
				"so add the service's own MCP server if it has one"
		}
		if t.Browser {
			f.Status = harness.StatusPartial
			f.How = "replace with " + harness.BrowserReplacement(inv) + "; local models are less reliable over long browser sessions, " +
				"and page snapshots use a lot of context"
		}
		out = append(out, f)
	}
	return out
}

var settingTargets = map[string]harness.Feature{
	"statusLine":  {Status: harness.StatusNative, How: "runs as is"},
	"outputStyle": {Status: harness.StatusNative, How: "applied as is"},
	"env":         {Status: harness.StatusNative, How: "applied as is"},
	"model":       {Status: harness.StatusTranslate, How: "the claude-local command sets the local model", Migrated: true},
}
