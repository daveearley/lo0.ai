package report

import (
	"lo0.ai/lo0/internal/claudecode/setup"
	codexsetup "lo0.ai/lo0/internal/codex/setup"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/usage"
)

// codexSection shows the Codex setup in the shape the Claude Code setup is
// shown, so the two read side by side. It is left out when lo0 found no
// Codex configuration and no Codex sessions.
func codexSection(d *doc, inv codexsetup.Inventory, history usage.Format, short func(string) string) {
	if !inv.Found && inv.Items() == 0 && history.Newest() == "" {
		return
	}
	d.heading("Codex setup")
	if v := history.Newest(); v != "" {
		d.row("Codex", v+d.t.Muted.Render(" in your history"))
	}
	if !inv.Found {
		d.row("Config", d.t.Muted.Render("no config.toml"))
	} else {
		d.row("Model", codexModel(d, inv))
	}
	if len(inv.Providers) > 0 {
		local := 0
		for _, p := range inv.Providers {
			if p.Local {
				local++
			}
		}
		d.row("Providers", d.join(format.Plural(len(inv.Providers), "provider", "providers"), d.t.Muted.Render(format.Int(local)+" local")))
	}
	if len(inv.Profiles) > 0 {
		names := make([]string, 0, len(inv.Profiles))
		for _, p := range inv.Profiles {
			names = append(names, d.t.Muted.Render(p.Name))
		}
		d.row("Profiles", format.Plural(len(inv.Profiles), "profile", "profiles"))
		d.more(d.fit(names))
	}
	d.row("Projects", format.Plural(len(inv.Projects), "project", "projects"))
	codexInstructions(d, inv.Instructions)

	servers := make([]setup.MCPServer, 0, len(inv.MCPServers))
	for _, s := range inv.MCPServers {
		servers = append(servers, setup.MCPServer{
			Origin: setup.Origin{Scope: setup.Scope(s.Scope), Project: s.Project, Path: s.Path},
			Name:   s.Name, Transport: setup.Transport(s.Transport),
		})
	}
	mcpServers(d, servers, short)
	handlers := make([]setup.Hook, 0, len(inv.Hooks))
	for _, h := range inv.Hooks {
		handlers = append(handlers, setup.Hook{Event: h.Event, Matcher: h.Matcher, Type: h.Type})
	}
	hooks(d, handlers)

	rules := 0
	for _, r := range inv.Rules {
		rules += r.Rules
	}
	d.row("Extensions", d.join(
		format.Plural(len(inv.ExtensionsOf(codexsetup.ExtensionSkill)), "skill", "skills"),
		format.Plural(len(inv.ExtensionsOf(codexsetup.ExtensionPrompt)), "prompt", "prompts"),
		format.Plural(len(inv.ExtensionsOf(codexsetup.ExtensionAgent)), "agent", "agents"),
	))
	if len(inv.Rules) > 0 {
		d.row("Rules", d.join(format.Plural(len(inv.Rules), "file", "files"), format.Plural(rules, "rule", "rules")))
	}
	enabled := 0
	for _, p := range inv.Plugins {
		if p.Enabled {
			enabled++
		}
	}
	plugins := format.Plural(len(inv.Plugins), "plugin", "plugins")
	if len(inv.Plugins) > 0 {
		plugins += d.t.Muted.Render(", " + format.Int(enabled) + " enabled")
	}
	d.row("Plugins", plugins)
}

// codexModel names the model Codex starts with when no profile is chosen.
func codexModel(d *doc, inv codexsetup.Inventory) string {
	model := inv.Model
	if model == "" {
		model = d.t.Muted.Render("Codex's default")
	}
	if inv.Provider == "" {
		return model
	}
	return d.join(model, d.t.Muted.Render("through "+inv.Provider))
}

func codexInstructions(d *doc, files []codexsetup.InstructionFile) {
	if len(files) == 0 {
		d.row("Instructions", d.t.Muted.Render("none"))
		return
	}
	lines, global := 0, 0
	for _, f := range files {
		lines += f.Lines
		if f.Scope == codexsetup.ScopeUser {
			global++
		}
	}
	d.row("Instructions", d.join(format.Plural(len(files), "file", "files"), format.Plural(lines, "line", "lines")))
	var parts []string
	if global > 0 {
		parts = append(parts, d.t.Muted.Render(format.Int(global)+" global"))
	}
	if n := len(files) - global; n > 0 {
		parts = append(parts, d.t.Muted.Render(format.Plural(n, "project file", "project files")))
	}
	d.more(d.join(parts...))
}

// pooled is one harness's profile when p pools several, and nothing when p
// is a single harness's profile, which may be another harness's.
func pooled(p usage.Profile, harness string) usage.Profile {
	for _, h := range p.Harnesses {
		if h.Name == harness {
			return h.Profile
		}
	}
	return usage.Profile{}
}
