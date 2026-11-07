package report

import (
	"cmp"
	"maps"
	"path/filepath"
	"slices"

	"lo0.ai/lo0/internal/claudecode/install"
	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/usage"
)

const maxServersShown = 10

func setupSection(d *doc, inv setup.Inventory, claude install.Installs, history usage.Format, short func(string) string) {
	d.heading("Claude Code setup")
	claudeRow(d, claude, history)
	d.row("Projects", format.Plural(len(inv.Projects), "project", "projects"))
	instructions(d, inv.Instructions)
	mcpServers(d, inv.MCPServers, short)
	hooks(d, inv.Hooks)

	d.row("Extensions", d.join(
		format.Plural(len(inv.ExtensionsOf(setup.ExtensionCommand)), "command", "commands"),
		format.Plural(len(inv.ExtensionsOf(setup.ExtensionAgent)), "subagent", "subagents"),
		format.Plural(len(inv.ExtensionsOf(setup.ExtensionSkill)), "skill", "skills"),
	))

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

	var allow, deny, ask int
	for _, p := range inv.Permissions {
		allow, deny, ask = allow+p.Allow, deny+p.Deny, ask+p.Ask
	}
	d.row("Permissions", d.join(format.Int(allow)+" allow", format.Int(deny)+" deny", format.Int(ask)+" ask"))
}

func instructions(d *doc, files []setup.InstructionFile) {
	if len(files) == 0 {
		d.row("Instructions", d.t.Muted.Render("none"))
		return
	}
	lines := 0
	kinds := map[setup.InstructionKind]int{}
	for _, f := range files {
		lines += f.Lines
		kinds[f.Kind]++
	}
	d.row("Instructions", d.join(format.Plural(len(files), "file", "files"), format.Plural(lines, "line", "lines")))
	var parts []string
	for _, k := range []setup.InstructionKind{setup.InstructionClaudeMD, setup.InstructionLocalMD, setup.InstructionAgentsMD, setup.InstructionRule} {
		n := kinds[k]
		if n == 0 {
			continue
		}
		label := format.Int(n) + " " + string(k)
		if k == setup.InstructionRule {
			label = format.Plural(n, "rule", "rules")
		}
		parts = append(parts, d.t.Muted.Render(label))
	}
	d.more(d.join(parts...))
}

func mcpServers(d *doc, servers []setup.MCPServer, short func(string) string) {
	if len(servers) == 0 {
		d.row("MCP servers", d.t.Muted.Render("none"))
		return
	}
	byTransport := map[setup.Transport]int{}
	for _, s := range servers {
		byTransport[s.Transport]++
	}
	var split []string
	for _, t := range slices.Sorted(maps.Keys(byTransport)) {
		split = append(split, format.Int(byTransport[t])+" "+string(t))
	}
	d.row("MCP servers", d.join(format.Int(len(servers)), d.t.Muted.Render(d.join(split...))))

	sorted := slices.Clone(servers)
	slices.SortStableFunc(sorted, func(a, b setup.MCPServer) int {
		return cmp.Or(cmp.Compare(scopeRank(a.Scope), scopeRank(b.Scope)), cmp.Compare(a.Name, b.Name))
	})
	shown := sorted[:min(len(sorted), maxServersShown)]
	rows := make([][]string, 0, len(shown))
	for _, s := range shown {
		rows = append(rows, []string{s.Name, string(s.Transport), d.t.Muted.Render(where(s.Origin, short))})
	}
	d.table("", []align{left, left, left}, rows)
	if extra := len(sorted) - len(shown); extra > 0 {
		d.more(d.t.Muted.Render("and " + format.Int(extra) + " more"))
	}
}

func scopeRank(s setup.Scope) int {
	return slices.Index([]setup.Scope{setup.ScopeManaged, setup.ScopeUser, setup.ScopePlugin, setup.ScopeProject, setup.ScopeLocal}, s)
}

func where(o setup.Origin, short func(string) string) string {
	switch o.Scope {
	case setup.ScopePlugin:
		return "plugin " + o.Plugin
	case setup.ScopeProject, setup.ScopeLocal:
		return string(o.Scope) + " " + filepath.Base(short(o.Project))
	default:
		return string(o.Scope)
	}
}

func hooks(d *doc, hs []setup.Hook) {
	if len(hs) == 0 {
		d.row("Hooks", d.t.Muted.Render("none"))
		return
	}
	events := map[string]int{}
	for _, h := range hs {
		events[h.Event]++
	}
	names := slices.Sorted(maps.Keys(events))
	slices.SortStableFunc(names, func(a, b string) int { return cmp.Compare(events[b], events[a]) })
	d.row("Hooks", format.Plural(len(hs), "handler", "handlers")+" on "+format.Plural(len(names), "event", "events"))
	items := make([]string, 0, len(names))
	for _, n := range names {
		items = append(items, d.t.Muted.Render(n))
	}
	d.more(d.fit(items))
}
