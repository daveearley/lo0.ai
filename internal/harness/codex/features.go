// Package codex maps a Claude Code or Codex setup to OpenAI's Codex running
// against a local model, and plans the move: the local runtime as a model
// provider in ~/.codex/config.toml, the default model, and the user-level
// MCP servers, skills and subagents converted to Codex's formats. A Codex
// setup carries over as it is. Files in the user's own repositories are
// never written.
package codex

import (
	"path/filepath"
	"slices"

	"lo0.ai/lo0/internal/claudecode/setup"
	codexhome "lo0.ai/lo0/internal/codex"
	codexsetup "lo0.ai/lo0/internal/codex/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/usage"
)

// Harness is Codex on a local model as a migration target. The zero value
// is ready to use.
type Harness struct{}

// ID implements the harness contract.
func (Harness) ID() harness.ID { return harness.Codex }

// Name implements the harness contract.
func (Harness) Name() string { return "Codex on a local model" }

// Product implements app.Harness: the harness's own name, for short labels.
func (Harness) Product() string { return "Codex" }

// Features maps a Claude Code setup to Codex. History is used to find tools
// the user relies on that come from the Claude apps rather than from their
// configuration.
func (Harness) Features(inv setup.Inventory, history usage.Profile, _ harness.Installation) []harness.Feature {
	return slices.Concat(
		instructions(inv),
		mcpServers(inv),
		appTools(inv, history),
		hooks(inv, history),
		extensions(inv, history),
		plugins(inv),
		permissions(inv),
		harness.SettingFeatures(inv, settingTargets),
	)
}

// instructions maps instruction files. Codex reads AGENTS.md, one file per
// folder, and other names only when they are listed in
// project_doc_fallback_filenames, which lo0 sets to CLAUDE.md. Outside
// projects it reads only its own AGENTS.md (developers.openai.com/codex/
// guides/agents-md).
func instructions(inv setup.Inventory) []harness.Feature {
	agentsIn := map[string]bool{}
	for _, f := range inv.Instructions {
		if f.Kind == setup.InstructionAgentsMD {
			agentsIn[f.Project] = true
		}
	}
	out := make([]harness.Feature, 0, len(inv.Instructions))
	for _, f := range inv.Instructions {
		feat := harness.Feature{Area: harness.AreaInstructions, Name: f.Path, Status: harness.StatusTranslate}
		switch {
		case f.Kind == setup.InstructionAgentsMD:
			feat.Status, feat.How = harness.StatusNative, "read as is"
		case f.Kind == setup.InstructionClaudeMD && f.Scope == setup.ScopeUser:
			feat.How = "copy into ~/.codex/AGENTS.md; Codex reads no CLAUDE.md outside a project"
		case f.Kind == setup.InstructionClaudeMD && filepath.Base(filepath.Dir(f.Path)) != ".claude" && !agentsIn[f.Project]:
			feat.How, feat.Migrated = `read once "CLAUDE.md" is in project_doc_fallback_filenames`, true
		case f.Kind == setup.InstructionClaudeMD:
			feat.How = "merge into the project's AGENTS.md; Codex reads one instruction file per folder"
		default:
			feat.How = "merge into AGENTS.md"
		}
		out = append(out, feat)
	}
	return out
}

// needsFallback reports whether a project's CLAUDE.md would be read once
// Codex falls back to it.
func needsFallback(features []harness.Feature) bool {
	return slices.ContainsFunc(features, func(f harness.Feature) bool {
		return f.Area == harness.AreaInstructions && f.Migrated
	})
}

// mcpServers maps MCP servers. Codex runs stdio servers and connects to
// Streamable HTTP ones; it has no SSE transport.
func mcpServers(inv setup.Inventory) []harness.Feature {
	out := make([]harness.Feature, 0, len(inv.MCPServers))
	for _, s := range inv.MCPServers {
		feat := harness.Feature{Area: harness.AreaMCP, Name: s.Name, Status: harness.StatusTranslate, Migrated: s.Scope == setup.ScopeUser}
		switch s.Transport {
		case setup.TransportStdio:
			feat.How = "[mcp_servers] entry with the same command, args and env"
		case setup.TransportSSE:
			feat.Status, feat.How = harness.StatusPartial, "[mcp_servers] entry with the same url; Codex connects over Streamable HTTP only, so check the server"
		default:
			feat.How = "[mcp_servers] entry with the same url and headers"
		}
		if s.Scope != setup.ScopeUser {
			feat.How += ", in the project's .codex/config.toml, which Codex reads once it trusts the project"
		}
		out = append(out, feat)
	}
	return out
}

// appTools describes the tools only the Claude apps provide.
func appTools(inv setup.Inventory, history usage.Profile) []harness.Feature {
	tools := harness.AppTools(inv, history)
	out := make([]harness.Feature, 0, len(tools))
	for _, t := range tools {
		f := harness.Feature{
			Area: harness.AreaAppTools, Name: t.Name, Calls: t.Calls, Status: harness.StatusLost,
			How: "provided by the Claude app or a claude.ai connector, not by your configuration",
		}
		if t.Connector {
			f.How = "a claude.ai connector (" + t.Server + "); add the service's own MCP server to Codex if it has one"
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

// hookEvents are the Claude Code hook events Codex also fires, under the
// same names, in hooks.json files of the same shape
// (developers.openai.com/codex/hooks).
var hookEvents = []string{
	"PreToolUse", "PermissionRequest", "PostToolUse", "PreCompact", "PostCompact", "SessionStart", "SessionEnd",
	"SubagentStart", "SubagentStop", "UserPromptSubmit", "Stop",
}

// hooks maps hook handlers. Codex runs command and mcp_tool handlers and
// skips prompt and agent ones, and it runs a hook only once the user has
// trusted it with /hooks.
func hooks(inv setup.Inventory, history usage.Profile) []harness.Feature {
	runs := harness.HookRuns(history)
	out := make([]harness.Feature, 0, len(inv.Hooks))
	for _, h := range inv.Hooks {
		feat := harness.Feature{
			Area: harness.AreaHooks, Name: h.Event, Calls: runs[h.Event], Status: harness.StatusTranslate,
			How: "copy to ~/.codex/hooks.json and trust it with /hooks; matchers on Bash, Edit, Write and mcp__ tools work as they are",
		}
		switch {
		case !slices.Contains(hookEvents, h.Event):
			feat.Status, feat.How = harness.StatusLost, "Codex has no "+h.Event+" event"
		case h.Type == "prompt" || h.Type == "agent":
			feat.Status, feat.How = harness.StatusLost, "Codex reads "+h.Type+" hooks but does not run them"
		case h.Type != "command" && h.Type != "mcp_tool":
			feat.Status, feat.How = harness.StatusLost, h.Type+" hooks have no Codex equivalent"
		}
		out = append(out, feat)
	}
	return out
}

func extensions(inv setup.Inventory, history usage.Profile) []harness.Feature {
	loads := harness.SkillLoads(history)
	out := make([]harness.Feature, 0, len(inv.Extensions))
	for _, e := range inv.Extensions {
		feat := harness.Feature{Name: e.Name, Status: harness.StatusTranslate}
		switch e.Kind {
		case setup.ExtensionCommand:
			feat.Area = harness.AreaCommands
			feat.How = "copy to ~/.codex/prompts and run it as /prompts:" + e.Name + "; Codex keeps custom prompts but prefers skills"
		case setup.ExtensionAgent:
			feat.Area, feat.Status = harness.AreaSubagents, harness.StatusPartial
			feat.How = "a custom agent in ~/.codex/agents with the same instructions; its tool list and model do not carry over"
			feat.Migrated = e.Scope == setup.ScopeUser
		case setup.ExtensionSkill:
			feat.Area, feat.Calls = harness.AreaSkills, loads[e.Name]
			feat.How = "copy to ~/.agents/skills; Codex reads the same SKILL.md format"
			if e.Scope == setup.ScopeProject || e.Scope == setup.ScopeLocal {
				feat.How = "copy to the project's .agents/skills; Codex reads the same SKILL.md format"
			}
			feat.Migrated = e.Scope == setup.ScopeUser || e.Scope == setup.ScopePlugin
		default:
			continue
		}
		out = append(out, feat)
	}
	return out
}

func plugins(inv setup.Inventory) []harness.Feature {
	var out []harness.Feature
	for _, p := range inv.Plugins {
		if !p.Enabled {
			continue
		}
		out = append(out, harness.Feature{
			Area: harness.AreaPlugins, Name: p.Name, Status: harness.StatusPartial,
			How: "Codex does not load Claude Code plugins; their skills, MCP servers and hooks carry over one by one",
		})
	}
	return out
}

func permissions(inv setup.Inventory) []harness.Feature {
	var out []harness.Feature
	for _, p := range inv.Permissions {
		if p.Allow+p.Deny+p.Ask == 0 && p.DefaultMode == "" {
			continue
		}
		out = append(out, harness.Feature{
			Area: harness.AreaPermissions, Name: p.Path, Status: harness.StatusPartial,
			How: "rewrite command rules as prefix_rule entries in ~/.codex/rules and the mode as approval_policy; Codex rules cover commands only",
		})
	}
	return out
}

var settingTargets = map[string]harness.Feature{
	"env":   {Status: harness.StatusTranslate, How: "set in the shell Codex starts from"},
	"model": {Status: harness.StatusTranslate, How: "replaced by the local model", Migrated: true},
}

// CodexFeatures implements match.CodexReader: a Codex setup carries over to
// Codex as it is, apart from the model, which becomes the local one.
func (Harness) CodexFeatures(inv codexsetup.Inventory, _ setup.Inventory, history usage.Profile, _ harness.Installation) []harness.Feature {
	var out []harness.Feature
	asIs := func(area harness.Area, name, how string, calls int) {
		out = append(out, harness.Feature{Area: area, Name: name, Status: harness.StatusNative, How: how, Calls: calls, From: codexhome.Harness})
	}
	for _, f := range inv.Instructions {
		asIs(harness.AreaInstructions, f.Path, "read as is", 0)
	}
	for _, s := range inv.MCPServers {
		if !s.Disabled {
			asIs(harness.AreaMCP, s.Name, "read as is", 0)
		}
	}
	runs := harness.HookRuns(history)
	for _, h := range inv.Hooks {
		asIs(harness.AreaHooks, h.Event, "runs as is", runs[h.Event])
	}
	loads := harness.SkillLoads(history)
	for _, e := range inv.Extensions {
		switch e.Kind {
		case codexsetup.ExtensionSkill:
			asIs(harness.AreaSkills, e.Name, "read as is", loads[e.Name])
		case codexsetup.ExtensionPrompt:
			asIs(harness.AreaCommands, e.Name, "read as is", 0)
		case codexsetup.ExtensionAgent:
			asIs(harness.AreaSubagents, e.Name, "read as is; it uses the local model unless it names another", 0)
		}
	}
	for _, p := range inv.Plugins {
		if p.Enabled {
			asIs(harness.AreaPlugins, p.Name, "loaded as is", 0)
		}
	}
	for _, r := range inv.Rules {
		asIs(harness.AreaPermissions, r.Path, "applied as is", 0)
	}
	if inv.Model != "" || inv.Provider != "" {
		out = append(out, harness.Feature{
			Area: harness.AreaSettings, Name: "model", Status: harness.StatusTranslate, Migrated: true, From: codexhome.Harness,
			How: "replaced by the local model and its provider",
		})
	}
	return out
}
