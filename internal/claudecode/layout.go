// Package claudecode knows where Claude Code keeps its configuration and
// history on each platform.
package claudecode

import (
	"path/filepath"
	"strings"
)

// Harness is how the report names Claude Code.
const Harness = "Claude Code"

// Layout holds the absolute paths of Claude Code's files.
type Layout struct {
	// ConfigDir is the user configuration directory, normally ~/.claude.
	ConfigDir string `json:"configDir"`
	// GlobalConfig is the global state file that holds user and per-project
	// MCP servers, normally ~/.claude.json.
	GlobalConfig string `json:"globalConfig"`
	// ManagedSettings is the administrator-managed settings file.
	ManagedSettings string `json:"managedSettings"`
}

// ResolveLayout returns the layout for goos. CLAUDE_CONFIG_DIR moves both the
// configuration directory and the global state file, as it does in Claude
// Code.
func ResolveLayout(goos, home string, getenv func(string) string) Layout {
	l := Layout{
		ConfigDir:    filepath.Join(home, ".claude"),
		GlobalConfig: filepath.Join(home, ".claude.json"),
	}
	if dir := getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		l.ConfigDir = dir
		l.GlobalConfig = filepath.Join(dir, ".claude.json")
	}
	switch goos {
	case "darwin":
		l.ManagedSettings = "/Library/Application Support/ClaudeCode/managed-settings.json"
	case "windows":
		l.ManagedSettings = `C:\Program Files\ClaudeCode\managed-settings.json`
	default:
		l.ManagedSettings = "/etc/claude-code/managed-settings.json"
	}
	return l
}

// ProjectsDir is where Claude Code stores session transcripts.
func (l Layout) ProjectsDir() string {
	return filepath.Join(l.ConfigDir, "projects")
}

// WorktreesDir is where, inside a project, Claude Code creates worktrees.
const WorktreesDir = ".claude/worktrees"

// IsWorktree reports whether dir is a worktree that Claude Code created
// inside another project. Its history belongs to the parent repository.
func IsWorktree(dir string) bool {
	return strings.Contains(filepath.ToSlash(dir), "/"+WorktreesDir+"/")
}
