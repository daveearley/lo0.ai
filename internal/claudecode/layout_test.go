package claudecode_test

import (
	"path/filepath"
	"testing"

	"lo0.ai/lo0/internal/claudecode"
)

func TestResolveLayout(t *testing.T) {
	none := func(string) string { return "" }
	l := claudecode.ResolveLayout("darwin", "/Users/u", none)
	if l.ConfigDir != filepath.Join("/Users/u", ".claude") || l.GlobalConfig != filepath.Join("/Users/u", ".claude.json") {
		t.Errorf("default layout = %+v", l)
	}
	if l.ManagedSettings != "/Library/Application Support/ClaudeCode/managed-settings.json" {
		t.Errorf("ManagedSettings = %q", l.ManagedSettings)
	}
	if got, want := l.ProjectsDir(), filepath.Join("/Users/u", ".claude", "projects"); got != want {
		t.Errorf("ProjectsDir = %q, want %q", got, want)
	}

	custom := claudecode.ResolveLayout("linux", "/home/u", func(k string) string {
		if k == "CLAUDE_CONFIG_DIR" {
			return "/srv/claude"
		}
		return ""
	})
	if custom.ConfigDir != "/srv/claude" || custom.GlobalConfig != filepath.Join("/srv/claude", ".claude.json") {
		t.Errorf("CLAUDE_CONFIG_DIR layout = %+v", custom)
	}
	if custom.ManagedSettings != "/etc/claude-code/managed-settings.json" {
		t.Errorf("linux ManagedSettings = %q", custom.ManagedSettings)
	}
	if w := claudecode.ResolveLayout("windows", `C:\Users\u`, none); w.ManagedSettings != `C:\Program Files\ClaudeCode\managed-settings.json` {
		t.Errorf("windows ManagedSettings = %q", w.ManagedSettings)
	}
}

func TestIsWorktree(t *testing.T) {
	tests := map[string]bool{
		"/Users/u/Code/app":                            false,
		"/Users/u/Code/app/.claude/worktrees/fix-auth": true,
		`C:\Code\app\.claude\worktrees\fix-auth`:       filepath.Separator == '\\',
		"/Users/u/Code/claude/worktrees":               false,
	}
	for dir, want := range tests {
		if got := claudecode.IsWorktree(dir); got != want {
			t.Errorf("IsWorktree(%q) = %v, want %v", dir, got, want)
		}
	}
}
