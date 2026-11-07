package opencode

import (
	"path/filepath"
	"slices"

	"lo0.ai/lo0/internal/claudecode/setup"
	codexhome "lo0.ai/lo0/internal/codex"
	codexsetup "lo0.ai/lo0/internal/codex/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/usage"
)

// CodexFeatures implements match.CodexReader: what happens to a Codex
// setup in the OpenCode release that is installed. OpenCode reads nothing
// from the Codex home, but it reads project AGENTS.md files and skills in
// .agents/skills as Codex does.
func (Harness) CodexFeatures(inv codexsetup.Inventory, claude setup.Inventory, history usage.Profile, installed harness.Installation) []harness.Feature {
	return codexFeatures(inv, claude, history, releaseOf(installed))
}

// codexFeatures maps a Codex setup, leaving out of the plan what has the
// same name as something the Claude Code setup carries over.
func codexFeatures(inv codexsetup.Inventory, claude setup.Inventory, history usage.Profile, r release) []harness.Feature {
	out := slices.Concat(
		codexInstructions(inv, claude),
		codexServers(inv, claude, r),
		harness.CodexAppServers(inv, history),
		codexHooks(inv, history, r),
		codexExtensions(inv, claude, history),
		codexRules(inv, r),
		codexPlugins(inv),
	)
	if inv.Model != "" || inv.Provider != "" {
		out = append(out, harness.Feature{Area: harness.AreaSettings, Name: "model", Status: harness.StatusTranslate, How: "replaced by the local model", Migrated: true})
	}
	for i := range out {
		out[i].From = codexhome.Harness
	}
	return out
}

// codexInstructions maps AGENTS.md files. OpenCode reads a project's
// AGENTS.md but neither AGENTS.override.md nor anything in the Codex home,
// so the user's own file is added to "instructions".
func codexInstructions(inv codexsetup.Inventory, claude setup.Inventory) []harness.Feature {
	out := make([]harness.Feature, 0, len(inv.Instructions))
	for _, f := range harness.CodexOwnInstructions(inv, claude) {
		feat := harness.Feature{Area: harness.AreaInstructions, Name: f.Path, Status: harness.StatusTranslate}
		switch {
		case f.Scope == codexsetup.ScopeUser:
			feat.How, feat.Migrated = `add to "instructions"; OpenCode reads nothing from the Codex home`, true
		case f.Override:
			feat.How = `add to the project's "instructions"; OpenCode reads AGENTS.md, not AGENTS.override.md`
		default:
			feat.Status, feat.How = harness.StatusNative, "read as is"
		}
		out = append(out, feat)
	}
	return out
}

// codexServers maps MCP servers. OpenCode starts local servers with its own
// environment, so the variables Codex passes on with env_vars reach them
// without being listed.
func codexServers(inv codexsetup.Inventory, claude setup.Inventory, r release) []harness.Feature {
	key := `"` + r.mcpKey() + `" entry`
	var out []harness.Feature
	for _, s := range inv.MCPServers {
		if s.Disabled || s.App {
			continue
		}
		feat := harness.Feature{Area: harness.AreaMCP, Name: s.Name, Status: harness.StatusTranslate, Migrated: s.Scope == codexsetup.ScopeUser}
		if s.Transport == codexsetup.TransportHTTP {
			feat.How = key + " of type remote with the same URL; headers read from the environment become {env:VAR}"
		} else {
			feat.How = key + " of type local; command and args become one array, env becomes environment"
		}
		switch {
		case s.Scope != codexsetup.ScopeUser:
			feat.How += ", in the project's opencode.json"
		case claudeHasServer(claude, s.Name):
			feat.Migrated = false
			feat.How = "your Claude Code setup has a server of the same name, which lo0 carries over instead; rename one to keep both"
		}
		out = append(out, feat)
	}
	return out
}

func claudeHasServer(inv setup.Inventory, name string) bool {
	return slices.ContainsFunc(inv.MCPServers, func(s setup.MCPServer) bool { return s.Scope == setup.ScopeUser && s.Name == name })
}

// codexHooks maps hook handlers, which Codex writes in Claude Code's shape
// and under its event names.
func codexHooks(inv codexsetup.Inventory, history usage.Profile, r release) []harness.Feature {
	runs := harness.HookRuns(history)
	out := make([]harness.Feature, 0, len(inv.Hooks))
	for _, h := range inv.Hooks {
		out = append(out, hook(h.Event, h.Type, runs[h.Event], r))
	}
	return out
}

func codexExtensions(inv codexsetup.Inventory, claude setup.Inventory, history usage.Profile) []harness.Feature {
	loads := harness.SkillLoads(history)
	out := make([]harness.Feature, 0, len(inv.Extensions))
	for _, e := range inv.Extensions {
		user := e.Scope == codexsetup.ScopeUser
		feat := harness.Feature{Name: e.Name, Status: harness.StatusTranslate}
		switch e.Kind {
		case codexsetup.ExtensionSkill:
			feat.Area, feat.Calls = harness.AreaSkills, loads[e.Name]
			switch {
			case inAgentsSkills(e.Path):
				feat.Status, feat.How = harness.StatusNative, "read from .agents/skills as is"
			case user:
				feat.How, feat.Migrated = "copy to ~/.config/opencode/skills; OpenCode reads nothing from the Codex home", true
			default:
				feat.How = "copy to the project's .agents/skills, which OpenCode reads"
			}
		case codexsetup.ExtensionPrompt:
			feat.Area = harness.AreaCommands
			feat.How = "copy to commands and run it as /" + e.Name + "; $1 to $9 and $ARGUMENTS work the same, named placeholders such as $FILE do not"
			feat.Migrated = user
		case codexsetup.ExtensionAgent:
			feat.Area = harness.AreaSubagents
			feat.How = "an agent in agents with mode: subagent and the same instructions; its model and reasoning settings give way to the local model"
			feat.Migrated = user
			if !user {
				feat.How = "an agent in the project's .opencode/agents with mode: subagent and the same instructions"
			}
		default:
			continue
		}
		if feat.Migrated && claudeHasExtension(claude, e.Kind, e.Name) {
			feat.Migrated = false
			feat.How = "your Claude Code setup has one of the same name, which lo0 carries over instead; rename one to keep both"
		}
		out = append(out, feat)
	}
	return out
}

// inAgentsSkills reports whether a SKILL.md is in a .agents/skills folder,
// the user's or a project's, which OpenCode searches too.
func inAgentsSkills(path string) bool {
	skills := filepath.Dir(filepath.Dir(path))
	return filepath.Base(skills) == "skills" && filepath.Base(filepath.Dir(skills)) == ".agents"
}

// claudeHasExtension reports whether the Claude Code setup has a user
// command, subagent or plugin skill that lo0 writes to the same file as the
// Codex prompt, agent or skill of the same name.
func claudeHasExtension(inv setup.Inventory, k codexsetup.ExtensionKind, name string) bool {
	kind, scope := setup.ExtensionCommand, setup.ScopeUser
	switch k {
	case codexsetup.ExtensionPrompt:
	case codexsetup.ExtensionAgent:
		kind = setup.ExtensionAgent
	case codexsetup.ExtensionSkill:
		kind, scope = setup.ExtensionSkill, setup.ScopePlugin
	}
	return slices.ContainsFunc(inv.ExtensionsOf(kind), func(e setup.Extension) bool { return e.Scope == scope && e.Name == name })
}

// codexRules maps rule files. OpenCode's shell permissions match commands
// by pattern, with the last matching rule winning, where Codex applies the
// most restrictive.
func codexRules(inv codexsetup.Inventory, r release) []harness.Feature {
	out := make([]harness.Feature, 0, len(inv.Rules))
	for _, f := range inv.Rules {
		out = append(out, harness.Feature{
			Area: harness.AreaPermissions, Name: f.Path, Status: harness.StatusTranslate,
			How: `rewrite each prefix_rule as a "` + r.permissionKey() + `" ` + r.shellPermission() + ` rule such as "git push *": forbidden becomes deny and prompt ask, and the last matching rule wins`,
		})
	}
	return out
}

func codexPlugins(inv codexsetup.Inventory) []harness.Feature {
	var out []harness.Feature
	for _, p := range inv.Plugins {
		if p.Enabled {
			out = append(out, harness.Feature{
				Area: harness.AreaPlugins, Name: p.Name, Status: harness.StatusLost,
				How: "OpenCode does not load Codex plugins; add its skills and MCP servers one by one",
			})
		}
	}
	return out
}
