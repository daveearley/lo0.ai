package report

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/migrate"
)

// wideInventory is the width from which "used by" gets a column of its own
// rather than a line under its model.
const wideInventory = 100

// InventoryRows lists the models of inv in the order RenderInventory draws
// them, so a front end can map a selection to its model.
func InventoryRows(inv app.Inventory) []InventoryRow {
	var rows []InventoryRow
	for _, rt := range inv.Runtimes {
		for _, m := range rt.Models {
			rows = append(rows, InventoryRow{Runtime: rt, Model: m})
		}
	}
	return rows
}

// InventoryRow is one model in the inventory with its runtime.
type InventoryRow struct {
	Runtime app.InstalledRuntime
	Model   app.InstalledModel
}

// RenderInventory draws what is installed for running models locally: each
// runtime with its models, the space they take counting shared weights
// once, what each model is and which of lo0's files use it, then what lo0
// changed. selected marks one row of InventoryRows, or none when negative.
func RenderInventory(inv app.Inventory, selected int, opts Options) string {
	t := opts.Theme
	width := opts.Width
	if width <= 0 {
		width = DefaultWidth
	}
	wide := width >= wideInventory
	// Fits "Qwen3-Coder-30B-A3B · Q4_K_M"; longer identities are cut with "…".
	const isW = 30
	nameW := min(max(width-2-2-9-2-isW-2, 20), 44)
	if wide {
		nameW = min(nameW, 40)
	}
	var b strings.Builder
	summary := format.Bytes(inv.DiskBytes) + " of models"
	if inv.FreeBytes > 0 {
		summary += " · " + format.Bytes(inv.FreeBytes) + " free"
	}
	b.WriteString(t.Muted.Render(summary) + "\n")

	row := 0
	for _, rt := range inv.Runtimes {
		b.WriteString("\n" + t.Label.Render(strings.ToUpper(rt.Name)) + "  " + runtimeState(rt, opts) + "\n")
		if len(rt.Models) == 0 {
			continue
		}
		header := "  " + pad("MODEL", nameW, false) + "  " + pad("ON DISK", 9, true) + "  " + pad("IS", isW, false)
		if wide {
			header += "  USED BY"
		}
		b.WriteString(t.Label.Render(header) + "\n")
		for _, m := range rt.Models {
			marker := "  "
			nameStyle := t.Value
			if row == selected {
				marker, nameStyle = t.Accent.Render("▌ "), t.Accent.Bold(true)
			}
			row++
			name := m.Name
			size := format.Bytes(m.Bytes)
			if m.Parent != "" {
				name, size = "  "+name, "shares it"
			}
			line := marker + nameStyle.Render(pad(ansi.Truncate(name, nameW, "…"), nameW, false)) + "  " +
				sizeStyle(opts, m).Render(pad(size, 9, true)) + "  " + pad(ansi.Truncate(identity(m, opts), isW, "…"), isW, false)
			used := usedBy(m, opts)
			if wide && used != "" {
				line += "  " + t.Value.Render(used)
			}
			b.WriteString(line + "\n")
			if !wide && used != "" {
				b.WriteString("  " + strings.Repeat(" ", nameW+2) + t.Muted.Render("used by "+used) + "\n")
			}
		}
	}

	if changes := describeChanges(inv.Changes, opts); len(changes) > 0 {
		b.WriteString("\n" + t.Label.Render("CHANGES LO0 MADE") + "\n")
		for _, c := range changes {
			b.WriteString("  " + t.Value.Render(c) + "\n")
		}
	}
	return b.String()
}

func runtimeState(rt app.InstalledRuntime, opts Options) string {
	t := opts.Theme
	switch {
	case !rt.Installed:
		return t.Muted.Render("not installed")
	case rt.Running:
		return t.Muted.Render(joinPlain(rt.Version, "running"))
	default:
		return t.Muted.Render(joinPlain(rt.Version, "installed, not running"))
	}
}

func joinPlain(parts ...string) string {
	kept := parts[:0:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, separator)
}

func sizeStyle(opts Options, m app.InstalledModel) lipgloss.Style {
	if m.Parent != "" {
		return opts.Theme.Muted
	}
	return opts.Theme.Value
}

// identity says what a model is: the catalogued model it is, and whether
// lo0 made it.
func identity(m app.InstalledModel, opts Options) string {
	t := opts.Theme
	switch {
	case !m.MadeByLo0.IsZero():
		return t.Value.Render("made by lo0") + t.Muted.Render(", "+m.MadeByLo0.Format("2 Jan"))
	case m.CatalogName != "":
		return t.Value.Render(m.CatalogName) + t.Muted.Render(joinPrefixed(m.Quant))
	default:
		return t.Muted.Render("not in the catalogue")
	}
}

func joinPrefixed(s string) string {
	if s == "" {
		return ""
	}
	return separator + s
}

func usedBy(m app.InstalledModel, opts Options) string {
	paths := make([]string, 0, len(m.UsedBy))
	for _, p := range m.UsedBy {
		paths = append(paths, opts.paths()(p))
	}
	return strings.Join(paths, ", ")
}

// describeChanges says what each change in the journal did, once per thing
// changed, oldest first.
func describeChanges(changes []migrate.Change, opts Options) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range changes {
		var what string
		switch {
		case c.Path != "" && c.Backup != "":
			what = "Changed " + opts.paths()(c.Path)
		case c.Path != "":
			what = "Wrote " + opts.paths()(c.Path)
		case c.Model != "":
			what = "Added " + c.Model + " to " + c.RuntimeName
		default:
			continue
		}
		if seen[what] {
			continue
		}
		seen[what] = true
		out = append(out, what+opts.Theme.Muted.Render(separator+c.At.Format("2 Jan")))
	}
	return out
}
