package report

import (
	"strconv"
	"strings"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/machine"
)

// SimulatedMachine describes simulated hardware, or is empty for this
// machine.
func SimulatedMachine(p machine.Profile) string {
	if !p.Simulated || len(p.GPUs) == 0 {
		return ""
	}
	g := p.GPUs[0]
	var hw string
	switch {
	case g.Unified:
		hw = g.Name
		if g.Cores > 0 {
			hw += " with a " + strconv.Itoa(g.Cores) + "-core GPU and "
		} else {
			hw += " with "
		}
		hw += format.Bytes(p.Memory.TotalBytes) + " of unified memory"
	case len(p.GPUs) > 1:
		hw = strconv.Itoa(len(p.GPUs)) + " × " + g.Name + " with " + format.Bytes(g.VRAMBytes) + " of VRAM each"
	default:
		hw = g.Name + " with " + format.Bytes(g.VRAMBytes) + " of VRAM"
	}
	if !g.Unified && p.Memory.TotalBytes > 0 {
		hw += ", beside " + format.Bytes(p.Memory.TotalBytes) + " of system memory"
	}
	return "Simulated machine: " + hw + ". Your sessions and setup are real; speeds are estimated, and lo0 sets nothing up for this hardware."
}

func simulatedSection(d *doc, p machine.Profile) {
	s := SimulatedMachine(p)
	if s == "" {
		return
	}
	d.blank()
	for _, line := range strings.Split(Wrap(s, d.width), "\n") {
		d.lines = append(d.lines, d.t.Warn.Render(line))
	}
}
