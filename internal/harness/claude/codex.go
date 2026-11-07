package claude

import (
	"path/filepath"
	"slices"
	"strings"

	"lo0.ai/lo0/internal/claudecode/setup"
	codexhome "lo0.ai/lo0/internal/codex"
	codexsetup "lo0.ai/lo0/internal/codex/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/usage"
	"lo0.ai/lo0/internal/version"
)

// agentsVersion is the first Claude Code release that reads a project's
// AGENTS.md, and then only where the project has no CLAUDE.md
// (code.claude.com/docs/en/memory).
const agentsVersion = "2.1.277"

// readsAgents reports whether the Claude Code that runs reads AGENTS.md.
// One that is not installed is installed at its newest release.
func readsAgents(installed harness.Installation) bool {
	if !installed.Installed {
		return true
	}
	return installed.Version != "" && version.Compare(installed.Version, agentsVersion) >= 0
}

// agentsFile maps a project's AGENTS.md.
func agentsFile(claude setup.Inventory, project string, installed harness.Installation) (harness.Status, string) {
	switch {
	case !readsAgents(installed):
		return harness.StatusTranslate, "add @AGENTS.md to a CLAUDE.md beside it; Claude Code reads AGENTS.md itself from " + agentsVersion
	case hasClaudeMD(claude, project):
		return harness.StatusTranslate, "add @AGENTS.md to the project's CLAUDE.md; Claude Code reads AGENTS.md only where there is no CLAUDE.md"
	}
	return harness.StatusNative, "read as is, since the project has no CLAUDE.md"
}

func hasClaudeMD(inv setup.Inventory, project string) bool {
	return slices.ContainsFunc(inv.Instructions, func(f setup.InstructionFile) bool {
		return (f.Kind == setup.InstructionClaudeMD || f.Kind == setup.InstructionLocalMD) && f.Project != "" &&
			filepath.Clean(f.Project) == filepath.Clean(project)
	})
}

// CodexFeatures implements match.CodexReader: what happens to a Codex
// setup in Claude Code. Claude Code reads nothing from the Codex home or
// .agents, so the user's MCP servers are loaded by claude-local and their
// skills, prompts and agents are copied beside their Claude Code ones.
func (Harness) CodexFeatures(inv codexsetup.Inventory, claude setup.Inventory, history usage.Profile, installed harness.Installation) []harness.Feature {
	out := slices.Concat(
		codexInstructions(inv, claude, installed),
		codexServerFeatures(inv, claude),
		harness.CodexAppServers(inv, history),
		codexHooks(inv, history),
		codexExtensions(inv, claude, history),
		codexRules(inv),
		codexPlugins(inv),
	)
	if inv.Model != "" || inv.Provider != "" {
		out = append(out, harness.Feature{Area: harness.AreaSettings, Name: "model", Status: harness.StatusTranslate, How: "the claude-local command sets the local model", Migrated: true})
	}
	for i := range out {
		out[i].From = codexhome.Harness
	}
	return out
}

func codexInstructions(inv codexsetup.Inventory, claude setup.Inventory, installed harness.Installation) []harness.Feature {
	own := harness.CodexOwnInstructions(inv, claude)
	out := make([]harness.Feature, 0, len(own))
	for _, f := range own {
		feat := harness.Feature{Area: harness.AreaInstructions, Name: f.Path, Status: harness.StatusTranslate}
		switch {
		case f.Scope == codexsetup.ScopeUser:
			feat.How = "import it into ~/.claude/CLAUDE.md with an @ line and its path; Claude Code reads nothing from the Codex home"
		case f.Override:
			feat.How = "add @AGENTS.override.md to the project's CLAUDE.md; Claude Code does not read AGENTS.override.md"
		default:
			feat.Status, feat.How = agentsFile(claude, f.Project, installed)
		}
		out = append(out, feat)
	}
	return out
}

// codexServerFeatures maps MCP servers. Claude Code starts stdio servers
// with its own environment, so the variables Codex passes on with env_vars
// reach them without being listed, but it has no working folder setting.
func codexServerFeatures(inv codexsetup.Inventory, claude setup.Inventory) []harness.Feature {
	var out []harness.Feature
	for _, s := range inv.MCPServers {
		if s.Disabled || s.App {
			continue
		}
		feat := harness.Feature{Area: harness.AreaMCP, Name: s.Name, Status: harness.StatusTranslate}
		switch {
		case s.Scope != codexsetup.ScopeUser:
			feat.How = "add to the project's .mcp.json with the same command or url; headers read from the environment become ${VAR}"
		case claudeHasServer(claude, s.Name):
			feat.How = "your Claude Code setup has a server of the same name, which Claude Code uses; rename one to keep both"
		default:
			feat.How, feat.Migrated = "loaded by claude-local with --mcp-config; headers read from the environment become ${VAR}", true
		}
		if s.Cwd {
			feat.Status = harness.StatusPartial
			feat.How += "; Claude Code has no cwd setting, so the server starts in the folder Claude Code runs in"
		}
		out = append(out, feat)
	}
	return out
}

// codexEvents are the Codex hook events Claude Code has no event for.
var codexEvents = []string{"PreModelRequest", "PostModelRequest", "Interrupt"}

// codexHooks maps hook handlers. Codex reads Claude Code's hook format and
// event names, and reports its shell tool as Bash and MCP tools as
// mcp__server__tool, but calls file edits apply_patch as well as Edit and
// Write (learn.chatgpt.com/docs/hooks).
func codexHooks(inv codexsetup.Inventory, history usage.Profile) []harness.Feature {
	runs := harness.HookRuns(history)
	out := make([]harness.Feature, 0, len(inv.Hooks))
	for _, h := range inv.Hooks {
		feat := harness.Feature{
			Area: harness.AreaHooks, Name: h.Event, Calls: runs[h.Event], Status: harness.StatusTranslate,
			How: `copy to "hooks" in ~/.claude/settings.json, which takes the same shape`,
		}
		if strings.Contains(h.Matcher, "apply_patch") {
			feat.How += "; Claude Code calls file edits Edit and Write, not apply_patch"
		}
		switch {
		case slices.Contains(codexEvents, h.Event):
			feat.Status, feat.How = harness.StatusLost, "Claude Code has no "+h.Event+" event"
		case h.Type == "prompt" || h.Type == "agent":
			feat.Status, feat.How = harness.StatusPartial, "Codex skips "+h.Type+" hooks; Claude Code runs them, with the local model judging"
		}
		out = append(out, feat)
	}
	return out
}

func codexExtensions(inv codexsetup.Inventory, claude setup.Inventory, history usage.Profile) []harness.Feature {
	loads := harness.SkillLoads(history)
	out := make([]harness.Feature, 0, len(inv.Extensions))
	for _, e := range inv.Extensions {
		user := e.Scope == codexsetup.ScopeUser
		feat := harness.Feature{Name: e.Name, Status: harness.StatusTranslate, Migrated: user}
		switch e.Kind {
		case codexsetup.ExtensionSkill:
			feat.Area, feat.Calls = harness.AreaSkills, loads[e.Name]
			feat.How = "copy to ~/.claude/skills; Claude Code reads the same SKILL.md format but not .agents/skills"
			if !user {
				feat.How = "copy to the project's .claude/skills; Claude Code reads the same SKILL.md format but not .agents/skills"
			}
		case codexsetup.ExtensionPrompt:
			feat.Area = harness.AreaCommands
			feat.How = "copy to ~/.claude/commands and run it as /" + e.Name + "; $1 to $9 become $0 to $8, and named placeholders such as $FILE need an arguments list"
		case codexsetup.ExtensionAgent:
			feat.Area = harness.AreaSubagents
			feat.How = "a subagent in ~/.claude/agents with the same instructions; its model and reasoning settings give way to the local model"
			if !user {
				feat.How = "a subagent in the project's .claude/agents with the same instructions"
			}
		default:
			continue
		}
		if user && !safeName(e.Name) {
			feat.Migrated = false
		}
		if user && claudeHasExtension(claude, e.Kind, e.Name) {
			feat.Status, feat.Migrated = harness.StatusNative, false
			feat.How = "your Claude Code setup has one of the same name, which Claude Code uses"
		}
		out = append(out, feat)
	}
	return out
}

// codexRules maps rule files. Claude Code's Bash permission rules match
// command prefixes, and like Codex it applies the most restrictive rule
// that matches.
func codexRules(inv codexsetup.Inventory) []harness.Feature {
	out := make([]harness.Feature, 0, len(inv.Rules))
	for _, f := range inv.Rules {
		out = append(out, harness.Feature{
			Area: harness.AreaPermissions, Name: f.Path, Status: harness.StatusTranslate,
			How: `rewrite each prefix_rule as a permission rule such as "Bash(git push *)": allow stays allow, prompt becomes ask and forbidden deny`,
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
				How: "Claude Code does not load Codex plugins; add its skills and MCP servers one by one",
			})
		}
	}
	return out
}
