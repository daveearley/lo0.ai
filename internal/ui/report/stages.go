package report

import (
	"strings"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/scan"
)

// StageLabel names a scan stage in progress views.
func StageLabel(s scan.Stage) string {
	switch s {
	case scan.StageMachine:
		return "Hardware"
	case scan.StageRuntimes:
		return "Local runtimes"
	case scan.StageSetup:
		return "Claude Code setup"
	case scan.StageHistory:
		return "Session history"
	default:
		return string(s)
	}
}

// StageFound says in a few words what a finished stage found, such as
// "Apple M5 Pro · 64 GiB", so every progress view reads the same.
func StageFound(stage scan.Stage, f scan.Found) string {
	var parts []string
	switch stage {
	case scan.StageMachine:
		parts = append(parts, f.Machine.CPU.Model)
		if b := f.Machine.Memory.TotalBytes; b > 0 {
			parts = append(parts, format.Bytes(b))
		}
	case scan.StageRuntimes:
		for _, rt := range f.Runtimes {
			if !rt.Installed {
				continue
			}
			p := rt.Name
			if len(rt.Models) > 0 {
				p += " " + format.Plural(len(rt.Models), "model", "models")
			}
			parts = append(parts, p)
		}
		if len(parts) == 0 {
			parts = append(parts, "none installed")
		}
	case scan.StageSetup:
		if n := len(f.Setup.Instructions); n > 0 {
			parts = append(parts, format.Plural(n, "instruction file", "instruction files"))
		}
		if n := len(f.Setup.MCPServers); n > 0 {
			parts = append(parts, format.Plural(n, "MCP server", "MCP servers"))
		}
	case scan.StageHistory:
		if f.Usage.Sessions > 0 {
			parts = append(parts, format.Plural(f.Usage.Sessions, "session", "sessions"), format.Plural(f.Usage.Turns, "turn", "turns"))
		}
	}
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " · ")
}
