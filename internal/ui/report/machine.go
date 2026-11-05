package report

import (
	"cmp"
	"strings"
	"unicode"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/machine"
)

// lowDiskBytes is the free space below which pulling a coding model is likely
// to fail; capable coding models are 15 to 60 GiB.
const lowDiskBytes = 40 << 30

func machineSection(d *doc, m machine.Profile) {
	d.heading("Machine")
	d.row("System", d.join(osName(m.OS), m.OS.Arch))

	cores := ""
	if m.CPU.PhysicalCores > 0 {
		cores = format.Plural(m.CPU.PhysicalCores, "core", "cores")
		if m.CPU.LogicalCores > m.CPU.PhysicalCores {
			cores += ", " + format.Int(m.CPU.LogicalCores) + " threads"
		}
	}
	d.row("CPU", d.join(cmp.Or(m.CPU.Model, "unknown"), cores))

	total := format.Bytes(m.Memory.TotalBytes)
	if m.Memory.Unified {
		total += " unified"
	}
	d.row("Memory", d.join(total, modelMemory(d, m.ModelMemory())))

	if len(m.GPUs) == 0 {
		d.row("GPU", d.t.Muted.Render("none detected"))
	}
	for i, g := range m.GPUs {
		label := ""
		if i == 0 {
			label = "GPU"
		}
		var parts []string
		parts = append(parts, g.Name)
		if g.Cores > 0 {
			parts = append(parts, format.Plural(g.Cores, "core", "cores"))
		}
		if g.VRAMBytes > 0 {
			parts = append(parts, format.Bytes(g.VRAMBytes)+" VRAM")
		}
		parts = append(parts, g.API)
		d.row(label, d.join(parts...))
	}

	if m.Disk.TotalBytes > 0 {
		free := format.Bytes(m.Disk.FreeBytes) + " free"
		if m.Disk.FreeBytes < lowDiskBytes {
			free = d.t.Warn.Render(free)
		}
		d.row("Disk", free+d.t.Muted.Render(" of "+format.Bytes(m.Disk.TotalBytes)))
	}
}

func modelMemory(d *doc, mm machine.ModelMemory) string {
	switch mm.Source {
	case machine.MemoryUnified, machine.MemoryVRAM:
		return d.t.Accent.Render(format.Bytes(mm.Bytes) + " usable by models")
	default:
		return d.t.Warn.Render("no GPU memory, models would run on the CPU")
	}
}

func osName(o machine.OS) string {
	var name string
	switch o.GOOS {
	case "darwin":
		name = "macOS"
	case "windows":
		name = cmp.Or(o.Platform, "Windows")
	default:
		name = cmp.Or(titleCase(o.Platform), "Linux")
	}
	if o.Version == "" || strings.Contains(name, o.Version) {
		return name
	}
	return name + " " + o.Version
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
