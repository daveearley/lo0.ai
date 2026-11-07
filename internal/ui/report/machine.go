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
	mm := m.ModelMemory()
	d.row("Memory", d.join(total, modelMemory(d, mm)))

	if len(m.GPUs) == 0 {
		d.row("GPU", d.t.Warn.Render("none found, so models run on the CPU"))
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
		if !g.CanHoldModels() && mm.Source == machine.MemoryCPU {
			parts = append(parts, d.t.Warn.Render("too small for models"))
		}
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
	case machine.MemoryCPU:
		return d.t.Warn.Render(format.Bytes(mm.Bytes) + " usable by models on the CPU")
	default:
		return d.t.Warn.Render("too little for a model")
	}
}

// BudgetPhrase states the memory models can use and, without a GPU that
// can hold one, that they run on the CPU.
func BudgetPhrase(mm machine.ModelMemory) string {
	if mm.Source == machine.MemoryCPU {
		return format.Bytes(mm.Bytes) + " for models on the CPU"
	}
	return format.Bytes(mm.Bytes) + " for models"
}

// MachineChips are the short facts a front end shows about the machine
// beside every screen: the chip models run on and the memory they can use.
// Simulated hardware and a machine without a GPU say so first.
func MachineChips(p machine.Profile, mm machine.ModelMemory) []string {
	if p.Simulated {
		real := p
		real.Simulated = false
		return append([]string{"simulated"}, MachineChips(real, mm)...)
	}
	chip := p.CPU.Model
	if len(p.GPUs) > 0 {
		chip = p.GPUs[0].Name
	}
	if mm.Source == machine.MemoryCPU {
		return []string{"no GPU", cmp.Or(p.CPU.Model, "CPU"), BudgetPhrase(mm)}
	}
	var out []string
	if chip != "" {
		out = append(out, chip)
	}
	if mm.Bytes > 0 {
		out = append(out, BudgetPhrase(mm))
	}
	return out
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
