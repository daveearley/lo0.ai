// Package opencode maps a Claude Code setup to OpenCode and plans the move:
// the local model as an OpenCode provider, and the user-level MCP servers,
// commands, subagents, rules and plugin skills converted to OpenCode's
// formats. OpenCode 1 and 2 read different files and keys, so the mapping
// and plan follow the release that is installed. Files in the user's own
// repositories are never written.
package opencode

import (
	"path/filepath"
	"slices"

	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/usage"
)

// Harness is OpenCode as a migration target. The zero value is ready to use.
type Harness struct{}

// ID implements the harness contract.
func (Harness) ID() harness.ID { return harness.OpenCode }

// Name implements the harness contract.
func (Harness) Name() string { return "OpenCode" }

// Product implements app.Harness: the harness's own name, for short labels.
func (Harness) Product() string { return "OpenCode" }

type hookTarget struct {
	status harness.Status
	how    string
}

// hookTargets maps Claude Code hook events to OpenCode plugin hooks in each
// release. Events that are absent have no equivalent. OpenCode 2 plugins
// use a new API, and OpenCode 1 plugins do not run in it
// (https://opencode.ai/v2/docs/build/plugins/migrate-v1/).
var hookTargets = map[release]map[string]hookTarget{
	v1: {
		"PreToolUse":        {harness.StatusTranslate, `plugin hook "tool.execute.before"`},
		"PostToolUse":       {harness.StatusTranslate, `plugin hook "tool.execute.after"`},
		"SessionStart":      {harness.StatusTranslate, `plugin event "session.created"`},
		"Stop":              {harness.StatusTranslate, `plugin event "session.idle"`},
		"PreCompact":        {harness.StatusTranslate, `plugin hook "experimental.session.compacting"`},
		"PermissionRequest": {harness.StatusPartial, `plugin event "permission.asked" can observe but not decide`},
	},
	v2: {
		"PreToolUse":        {harness.StatusTranslate, `plugin hook tool "execute.before"`},
		"PostToolUse":       {harness.StatusTranslate, `plugin hook tool "execute.after"`},
		"Stop":              {harness.StatusTranslate, `plugin event "session.idle"`},
		"PreCompact":        {harness.StatusTranslate, `plugin hook session "compaction"`},
		"PermissionRequest": {harness.StatusTranslate, `plugin hook permission "evaluate", which can allow, ask or deny`},
		"UserPromptSubmit":  {harness.StatusPartial, `plugin hook session "prompt" can add files to the prompt`},
	},
}

// Features maps the user's setup to the OpenCode release that is installed,
// or to OpenCode 2 when none is. History is used to find tools the user
// relies on that come from the Claude apps rather than from their
// configuration.
func (Harness) Features(inv setup.Inventory, history usage.Profile, installed harness.Installation) []harness.Feature {
	r := releaseOf(installed)
	return slices.Concat(
		instructions(inv, r),
		mcpServers(inv, r),
		appTools(inv, history),
		hooks(inv, history, r),
		extensions(inv, history, r),
		plugins(inv),
		permissions(inv, r),
		settings(inv, r),
	)
}

func instructions(inv setup.Inventory, r release) []harness.Feature {
	if r == v2 {
		return instructions2(inv)
	}
	agentsIn := map[string]bool{}
	for _, f := range inv.Instructions {
		if f.Kind == setup.InstructionAgentsMD {
			agentsIn[f.Project] = true
		}
	}
	out := make([]harness.Feature, 0, len(inv.Instructions))
	for _, f := range inv.Instructions {
		feat := harness.Feature{Area: harness.AreaInstructions, Name: f.Path}
		switch {
		case f.Kind == setup.InstructionAgentsMD:
			feat.Status, feat.How = harness.StatusNative, "read as is"
		case f.Kind == setup.InstructionClaudeMD && f.Scope == setup.ScopeUser:
			feat.Status, feat.How = harness.StatusNative, "read from ~/.claude when there is no global AGENTS.md"
		case f.Kind == setup.InstructionClaudeMD && filepath.Base(filepath.Dir(f.Path)) != ".claude" && !agentsIn[f.Project]:
			feat.Status, feat.How = harness.StatusNative, "read as is when the project has no AGENTS.md"
		case f.Kind == setup.InstructionClaudeMD:
			feat.Status, feat.How = harness.StatusTranslate, `add to "instructions"; OpenCode reads AGENTS.md instead`
		case f.Kind == setup.InstructionRule:
			feat.Status, feat.How = harness.StatusTranslate, `add to "instructions"; loaded in every session`
			feat.Migrated = f.Scope == setup.ScopeUser
		default:
			feat.Status, feat.How = harness.StatusTranslate, `add to "instructions"`
		}
		out = append(out, feat)
	}
	return out
}

// instructions2 maps instruction files to OpenCode 2, which reads
// AGENTS.md only (https://opencode.ai/v2/docs/instructions/). The user's own
// CLAUDE.md is added to the global "instructions"; a project's is the
// user's to add, since lo0 does not write into their repositories.
func instructions2(inv setup.Inventory) []harness.Feature {
	out := make([]harness.Feature, 0, len(inv.Instructions))
	for _, f := range inv.Instructions {
		feat := harness.Feature{Area: harness.AreaInstructions, Name: f.Path, Status: harness.StatusTranslate}
		switch f.Kind {
		case setup.InstructionAgentsMD:
			feat.Status, feat.How = harness.StatusNative, "read as is"
		case setup.InstructionClaudeMD:
			feat.How = `add to "instructions"; OpenCode 2 reads AGENTS.md, not CLAUDE.md`
			feat.Migrated = f.Scope == setup.ScopeUser
		case setup.InstructionRule:
			feat.How = `add to "instructions"; loaded in every session`
			feat.Migrated = f.Scope == setup.ScopeUser
		default:
			feat.How = `add to "instructions"`
		}
		out = append(out, feat)
	}
	return out
}

func mcpServers(inv setup.Inventory, r release) []harness.Feature {
	key := `"` + r.mcpKey() + `" entry`
	out := make([]harness.Feature, 0, len(inv.MCPServers))
	for _, s := range inv.MCPServers {
		feat := harness.Feature{Area: harness.AreaMCP, Name: s.Name, Migrated: s.Scope == setup.ScopeUser}
		switch {
		case s.Transport == setup.TransportStdio:
			feat.Status, feat.How = harness.StatusTranslate, key+" of type local; command becomes an array, env becomes environment"
		case s.Transport == setup.TransportSSE && r == v1:
			feat.Status, feat.How = harness.StatusPartial, key+" of type remote; OpenCode does not document SSE support, so check the server"
		case s.Transport == setup.TransportSSE:
			feat.Status, feat.How = harness.StatusPartial, key+" of type remote; OpenCode 2 connects over Streamable HTTP only, so check the server"
		default:
			feat.Status, feat.How = harness.StatusTranslate, key+" of type remote with the same URL and headers"
		}
		out = append(out, feat)
	}
	return out
}

// appTools describes the tools only the Claude apps provide. Browser control
// can be replaced by a browser MCP server; the rest has no counterpart
// outside Claude.
func appTools(inv setup.Inventory, history usage.Profile) []harness.Feature {
	tools := harness.AppTools(inv, history)
	out := make([]harness.Feature, 0, len(tools))
	for _, t := range tools {
		f := harness.Feature{
			Area: harness.AreaAppTools, Name: t.Name, Calls: t.Calls, Status: harness.StatusLost,
			How: "provided by the Claude app or a claude.ai connector, not by your configuration",
		}
		if t.Connector {
			f.How = "a claude.ai connector (" + t.Server + "); add the service's own MCP server to OpenCode if it has one"
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

func hooks(inv setup.Inventory, history usage.Profile, r release) []harness.Feature {
	runs := harness.HookRuns(history)
	out := make([]harness.Feature, 0, len(inv.Hooks))
	for _, h := range inv.Hooks {
		out = append(out, hook(h.Event, h.Type, runs[h.Event], r))
	}
	return out
}

// hook maps one hook handler, from Claude Code or Codex, which share event
// names and handler types, to an OpenCode plugin hook.
func hook(event, kind string, calls int, r release) harness.Feature {
	feat := harness.Feature{Area: harness.AreaHooks, Name: event, Status: harness.StatusLost, How: "no equivalent event in OpenCode plugins", Calls: calls}
	if target, ok := hookTargets[r][event]; ok {
		feat.Status, feat.How = target.status, target.how
	}
	switch kind {
	case "command":
	case "prompt", "agent":
		feat.Status, feat.How = harness.StatusLost, kind+" hooks ask a model to decide; OpenCode plugins run code only"
	default:
		feat.Status, feat.How = harness.StatusLost, kind+" hooks have no OpenCode equivalent; lo0 converts command hooks only"
	}
	return feat
}

func extensions(inv setup.Inventory, history usage.Profile, r release) []harness.Feature {
	loads := harness.SkillLoads(history)
	out := make([]harness.Feature, 0, len(inv.Extensions))
	for _, e := range inv.Extensions {
		feat := harness.Feature{Name: e.Name}
		switch e.Kind {
		case setup.ExtensionCommand:
			feat.Area, feat.Status, feat.How = harness.AreaCommands, harness.StatusTranslate, "copy to .opencode/commands; $ARGUMENTS works the same, allowed-tools is dropped"
			feat.Migrated = e.Scope == setup.ScopeUser
		case setup.ExtensionAgent:
			feat.Area, feat.Status, feat.How = harness.AreaSubagents, harness.StatusTranslate, "copy to .opencode/agents with mode: subagent; tools become "+r.permissionKey()+" rules"
			feat.Migrated = e.Scope == setup.ScopeUser
		case setup.ExtensionSkill:
			feat.Area = harness.AreaSkills
			feat.Calls = loads[e.Name]
			if e.Scope == setup.ScopePlugin {
				feat.Status, feat.How, feat.Migrated = harness.StatusTranslate, "copy to ~/.config/opencode/skills; plugin folders are not searched", true
			} else {
				feat.Status, feat.How = harness.StatusNative, "read from .claude/skills as is"
			}
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
			How: "the Claude plugin format is not supported; its skills, MCP servers and hooks carry over one by one",
		})
	}
	return out
}

func permissions(inv setup.Inventory, r release) []harness.Feature {
	var out []harness.Feature
	for _, p := range inv.Permissions {
		if p.Allow+p.Deny+p.Ask == 0 && p.DefaultMode == "" {
			continue
		}
		out = append(out, harness.Feature{
			Area: harness.AreaPermissions, Name: p.Path, Status: harness.StatusTranslate,
			How: `rewrite as "` + r.permissionKey() + `" rules; OpenCode applies the last matching rule`,
		})
	}
	return out
}

var settingTargets = map[release]map[string]harness.Feature{
	v1: {
		"statusLine":  {Status: harness.StatusLost, How: "OpenCode's TUI has no custom status line command"},
		"outputStyle": {Status: harness.StatusLost, How: "no output styles in OpenCode"},
		"env":         {Status: harness.StatusTranslate, How: `set in the shell or a plugin "shell.env" hook`},
		"model":       {Status: harness.StatusTranslate, How: "replaced by the local model", Migrated: true},
	},
	v2: {
		"statusLine":  {Status: harness.StatusLost, How: "OpenCode's TUI has no custom status line command"},
		"outputStyle": {Status: harness.StatusLost, How: "no output styles in OpenCode"},
		"env":         {Status: harness.StatusTranslate, How: `set in the shell or a plugin shell "create.before" hook`},
		"model":       {Status: harness.StatusTranslate, How: "replaced by the local model", Migrated: true},
	},
}

func settings(inv setup.Inventory, r release) []harness.Feature {
	return harness.SettingFeatures(inv, settingTargets[r])
}
