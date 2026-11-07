package harness

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"

	"lo0.ai/lo0/internal/claudecode/setup"
	codexsetup "lo0.ai/lo0/internal/codex/setup"
	"lo0.ai/lo0/internal/usage"
)

// CodexUserServers names the MCP servers the user's own config.toml sets
// up and switches on, which a migration can carry over. Servers the Codex
// app added for itself are left out, since they work only inside it.
func CodexUserServers(inv codexsetup.Inventory) []string {
	var out []string
	for _, s := range inv.MCPServers {
		if s.Scope == codexsetup.ScopeUser && !s.Disabled && !s.App {
			out = append(out, s.Name)
		}
	}
	return out
}

// CodexAppServers reports the switched-on MCP servers the Codex app added
// for itself, with their calls in the Codex history. No target can run them.
func CodexAppServers(inv codexsetup.Inventory, history usage.Profile) []Feature {
	var out []Feature
	for _, s := range inv.MCPServers {
		if !s.App || s.Disabled {
			continue
		}
		calls := 0
		if i := slices.IndexFunc(history.MCPServers, func(t usage.Tally) bool { return t.Name == s.Name }); i >= 0 {
			calls = history.MCPServers[i].Count
		}
		out = append(out, Feature{
			Area: AreaAppTools, Name: s.Name, Calls: calls, Status: StatusLost,
			How: "added to config.toml by the Codex app, and works only inside it",
		})
	}
	return out
}

// CodexServers reads the servers CodexUserServers names from config.toml,
// values included, so they move from one file on this machine to another
// without being shown.
func CodexServers(read func(string) ([]byte, error), inv codexsetup.Inventory) (map[string]codexsetup.MCPConfig, error) {
	names := CodexUserServers(inv)
	if len(names) == 0 {
		return nil, nil
	}
	data, err := read(inv.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", inv.Path, err)
	}
	all, err := codexsetup.MCPConfigs(data)
	if err != nil {
		return nil, fmt.Errorf("reading MCP servers from %s: %w", inv.Path, err)
	}
	out := make(map[string]codexsetup.MCPConfig, len(names))
	for name, c := range all {
		if slices.Contains(names, name) {
			out[name] = c
		}
	}
	return out, nil
}

// CodexUserExtensions lists the user's own extensions of kind k.
func CodexUserExtensions(inv codexsetup.Inventory, k codexsetup.ExtensionKind) []codexsetup.Extension {
	var out []codexsetup.Extension
	for _, e := range inv.ExtensionsOf(k) {
		if e.Scope == codexsetup.ScopeUser {
			out = append(out, e)
		}
	}
	return out
}

// CodexOwnInstructions lists the Codex setup's AGENTS.md files that the
// Claude Code setup does not list too, as it does a project's AGENTS.md.
func CodexOwnInstructions(inv codexsetup.Inventory, claude setup.Inventory) []codexsetup.InstructionFile {
	var out []codexsetup.InstructionFile
	for _, f := range inv.Instructions {
		if !slices.ContainsFunc(claude.Instructions, func(c setup.InstructionFile) bool { return filepath.Clean(c.Path) == filepath.Clean(f.Path) }) {
			out = append(out, f)
		}
	}
	return out
}
