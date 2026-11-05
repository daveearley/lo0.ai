package setup_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"lo0.ai/lo0/internal/claudecode"
	"lo0.ai/lo0/internal/claudecode/setup"
)

type missingFS struct{}

func (missingFS) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }

// tree maps absolute directories to their contents.
type tree map[string]fstest.MapFS

func (t tree) open(dir string) fs.FS {
	if fsys, ok := t[dir]; ok {
		return fsys
	}
	return missingFS{}
}

func abs(p string) string { return filepath.FromSlash(p) }

func file(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }

func mustJSON(t *testing.T, v any) *fstest.MapFile {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return &fstest.MapFile{Data: data}
}

func fixtureTree(t *testing.T) (claudecode.Layout, tree) {
	t.Helper()
	layout := claudecode.Layout{
		ConfigDir:       abs("/home/u/.claude"),
		GlobalConfig:    abs("/home/u/.claude.json"),
		ManagedSettings: abs("/etc/claude-code/managed-settings.json"),
	}
	app := abs("/home/u/code/app")
	pluginRoot := abs("/home/u/.claude/plugins/cache/acme/deploy/1.2.0")
	disabledRoot := abs("/home/u/.claude/plugins/cache/acme/lint/0.1.0")

	global := map[string]any{
		"oauthAccount": map[string]any{
			"organizationType": "claude_max", "organizationRateLimitTier": "default_claude_max_20x",
			"hasExtraUsageEnabled": true, "emailAddress": "secret@example.com",
		},
		"claudeMaxTier": "5x",
		"mcpServers": map[string]any{
			"stitch": map[string]any{"type": "http", "url": "https://stitch.example", "headers": map[string]string{"X-Api-Key": "secret-value"}},
		},
		"projects": map[string]any{
			app: map[string]any{"mcpServers": map[string]any{
				"railway": map[string]any{"command": "/usr/local/bin/railway-mcp", "args": []string{"--token", "secret-arg"}, "env": map[string]string{"RAILWAY_TOKEN": "secret-env"}},
			}},
			abs("/home/u/code/app/.claude/worktrees/fix-auth"): map[string]any{},
			abs("/home/u/code/deleted"):                        map[string]any{},
		},
	}
	return layout, tree{
		abs("/home/u"): {".claude.json": mustJSON(t, global)},
		abs("/home/u/.claude"): {
			"CLAUDE.md": file("# Me\nUse tabs.\n"),
			"settings.json": mustJSON(t, map[string]any{
				"model":       "opus",
				"permissions": map[string]any{"allow": []string{"Bash(git status)", "Read"}, "deny": []string{"Bash(rm -rf:*)"}},
				"hooks": map[string]any{
					"PostToolUse": []any{map[string]any{"matcher": "Edit|Write", "hooks": []any{map[string]any{"type": "command", "command": "gofmt -w"}}}},
					"Stop":        []any{map[string]any{"hooks": []any{map[string]any{"type": "command"}, map[string]any{"type": "prompt"}}}},
				},
				"enabledPlugins": map[string]bool{"deploy@acme": true, "lint@acme": false},
			}),
			"commands/review.md":      file("Review the diff."),
			"commands/git/squash.md":  file("Squash."),
			"commands/notes.txt":      file("ignored"),
			"agents/tester.md":        file("---\nname: test-runner\ndescription: runs tests\n---\nRun tests."),
			"agents/plain.md":         file("No frontmatter."),
			"skills/release/SKILL.md": file("---\nname: \"release-notes\"\n---\n"),
			"skills/scratch/notes.md": file("not a skill"),
			"rules/go.md":             file("Prefer small interfaces."),
			"plugins/installed_plugins.json": mustJSON(t, map[string]any{
				"version": 2,
				"plugins": map[string]any{
					"deploy@acme": []any{map[string]any{"installPath": pluginRoot, "version": "1.2.0"}},
					"lint@acme":   []any{map[string]any{"installPath": disabledRoot, "version": "0.1.0"}},
				},
			}),
		},
		pluginRoot: {
			".claude-plugin/plugin.json": mustJSON(t, map[string]any{"name": "deploy", "mcpServers": "./mcp.json"}),
			"mcp.json":                   mustJSON(t, map[string]any{"mcpServers": map[string]any{"deployer": map[string]any{"type": "sse", "url": "https://deploy.example/sse"}}}),
			"hooks/hooks.json":           mustJSON(t, map[string]any{"hooks": map[string]any{"PreToolUse": []any{map[string]any{"matcher": "Bash", "hooks": []any{map[string]any{"type": "command"}}}}}}),
			"commands/ship.md":           file("Ship it."),
			"skills/rollback/SKILL.md":   file("Roll back."),
		},
		disabledRoot: {
			".claude-plugin/plugin.json": mustJSON(t, map[string]any{"name": "lint", "mcpServers": map[string]any{"linter": map[string]any{"command": "npx"}}}),
		},
		abs("/etc/claude-code"): {
			"managed-settings.json": mustJSON(t, map[string]any{"permissions": map[string]any{"deny": []string{"WebFetch"}, "defaultMode": "acceptEdits"}}),
		},
		app: {
			"CLAUDE.md":                   file("# App\n\nRun make test.\n"),
			"AGENTS.md":                   file("Shared rules"),
			"CLAUDE.local.md":             file("my notes\n"),
			".claude/rules/frontend.md":   file("Use React."),
			".claude/settings.json":       mustJSON(t, map[string]any{"permissions": map[string]any{"ask": []string{"Bash(git push:*)"}}}),
			".claude/settings.local.json": mustJSON(t, map[string]any{"env": map[string]string{"DEBUG": "1"}}),
			".mcp.json":                   mustJSON(t, map[string]any{"mcpServers": map[string]any{"postgres": map[string]any{"command": "npx", "args": []string{"pg-mcp"}}}}),
			".claude/agents/reviewer.md":  file("Review."),
		},
	}
}

func TestScanInventory(t *testing.T) {
	layout, files := fixtureTree(t)
	inv, err := setup.NewScanner(layout, files.open).Scan(t.Context())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	app := abs("/home/u/code/app")

	t.Run("account plan without personal details", func(t *testing.T) {
		if inv.Account != (setup.Account{Plan: setup.PlanMax, Tier: "20x", ExtraUsage: true}) {
			t.Errorf("Account = %+v", inv.Account)
		}
		data, _ := json.Marshal(inv)
		if strings.Contains(string(data), "secret@example.com") {
			t.Error("inventory leaks the account email")
		}
	})

	t.Run("projects skip worktrees and missing directories", func(t *testing.T) {
		if !slices.Equal(inv.Projects, []string{app}) {
			t.Errorf("Projects = %v, want [%s]", inv.Projects, app)
		}
	})

	t.Run("instructions", func(t *testing.T) {
		got := map[string]setup.InstructionFile{}
		for _, f := range inv.Instructions {
			got[f.Path] = f
		}
		if len(got) != 6 {
			t.Fatalf("got %d instruction files: %+v", len(got), inv.Instructions)
		}
		user := got[abs("/home/u/.claude/CLAUDE.md")]
		if user.Scope != setup.ScopeUser || user.Lines != 2 || user.Bytes != 15 {
			t.Errorf("user CLAUDE.md = %+v", user)
		}
		if f := got[filepath.Join(app, "CLAUDE.local.md")]; f.Scope != setup.ScopeLocal || f.Kind != setup.InstructionLocalMD {
			t.Errorf("CLAUDE.local.md = %+v", f)
		}
		if f := got[filepath.Join(app, "AGENTS.md")]; f.Kind != setup.InstructionAgentsMD || f.Project != app {
			t.Errorf("AGENTS.md = %+v", f)
		}
		if f := got[filepath.Join(app, ".claude", "rules", "frontend.md")]; f.Kind != setup.InstructionRule {
			t.Errorf("rule = %+v", f)
		}
	})

	t.Run("mcp servers keep shape and drop secrets", func(t *testing.T) {
		byName := map[string]setup.MCPServer{}
		for _, s := range inv.MCPServers {
			byName[s.Name] = s
		}
		want := map[string]struct {
			scope     setup.Scope
			transport setup.Transport
			command   string
		}{
			"stitch":   {setup.ScopeUser, setup.TransportHTTP, ""},
			"railway":  {setup.ScopeLocal, setup.TransportStdio, "railway-mcp"},
			"postgres": {setup.ScopeProject, setup.TransportStdio, "npx"},
			"deployer": {setup.ScopePlugin, setup.TransportSSE, ""},
		}
		if len(byName) != len(want) {
			t.Fatalf("MCPServers = %+v, want %d servers (disabled plugins excluded)", inv.MCPServers, len(want))
		}
		for name, w := range want {
			s := byName[name]
			if s.Scope != w.scope || s.Transport != w.transport || s.Command != w.command {
				t.Errorf("%s = %+v, want %+v", name, s, w)
			}
		}
		if !slices.Equal(byName["railway"].EnvKeys, []string{"RAILWAY_TOKEN"}) || !slices.Equal(byName["stitch"].HeaderKeys, []string{"X-Api-Key"}) {
			t.Errorf("secret key names not kept: %+v %+v", byName["railway"], byName["stitch"])
		}
		data, err := json.Marshal(inv)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"secret-value", "secret-arg", "secret-env"} {
			if strings.Contains(string(data), secret) {
				t.Errorf("inventory JSON leaks %q", secret)
			}
		}
	})

	t.Run("hooks from settings and enabled plugins", func(t *testing.T) {
		var events []string
		for _, h := range inv.Hooks {
			events = append(events, string(h.Scope)+":"+h.Event+":"+h.Type)
		}
		want := []string{"user:PostToolUse:command", "user:Stop:command", "user:Stop:prompt", "plugin:PreToolUse:command"}
		if !slices.Equal(events, want) {
			t.Errorf("hooks = %v, want %v", events, want)
		}
	})

	t.Run("extensions", func(t *testing.T) {
		names := func(k setup.ExtensionKind) []string {
			var out []string
			for _, e := range inv.ExtensionsOf(k) {
				out = append(out, e.Name)
			}
			return out
		}
		if got, want := names(setup.ExtensionCommand), []string{"git:squash", "review", "ship"}; !slices.Equal(got, want) {
			t.Errorf("commands = %v, want %v", got, want)
		}
		if got, want := names(setup.ExtensionAgent), []string{"plain", "test-runner", "reviewer"}; !slices.Equal(got, want) {
			t.Errorf("agents = %v, want %v", got, want)
		}
		if got, want := names(setup.ExtensionSkill), []string{"release-notes", "rollback"}; !slices.Equal(got, want) {
			t.Errorf("skills = %v, want %v", got, want)
		}
	})

	t.Run("plugins", func(t *testing.T) {
		want := []setup.Plugin{
			{Name: "deploy", Marketplace: "acme", Version: "1.2.0", Enabled: true},
			{Name: "lint", Marketplace: "acme", Version: "0.1.0"},
		}
		if !slices.Equal(inv.Plugins, want) {
			t.Errorf("Plugins = %+v, want %+v", inv.Plugins, want)
		}
	})

	t.Run("permissions", func(t *testing.T) {
		got := map[setup.Scope]setup.Permissions{}
		for _, p := range inv.Permissions {
			got[p.Scope] = p
		}
		if p := got[setup.ScopeUser]; p.Allow != 2 || p.Deny != 1 {
			t.Errorf("user permissions = %+v", p)
		}
		if p := got[setup.ScopeManaged]; p.Deny != 1 || p.DefaultMode != "acceptEdits" {
			t.Errorf("managed permissions = %+v", p)
		}
		if p := got[setup.ScopeProject]; p.Ask != 1 {
			t.Errorf("project permissions = %+v", p)
		}
	})

	t.Run("settings keys", func(t *testing.T) {
		for _, s := range inv.Settings {
			if s.Scope == setup.ScopeLocal && !slices.Equal(s.Keys, []string{"env"}) {
				t.Errorf("local settings keys = %v", s.Keys)
			}
		}
	})
}

func TestScanReportsMalformedFilesAndContinues(t *testing.T) {
	layout, files := fixtureTree(t)
	files[abs("/home/u/code/app")][".mcp.json"] = file("{not json")
	inv, err := setup.NewScanner(layout, files.open).Scan(t.Context())
	if err == nil || !strings.Contains(err.Error(), ".mcp.json") {
		t.Fatalf("Scan error = %v, want one naming .mcp.json", err)
	}
	if len(inv.Instructions) == 0 || len(inv.Projects) != 1 {
		t.Errorf("inventory not returned alongside the error: %+v", inv)
	}
}

func TestMaxTierFallsBackToTheCachedField(t *testing.T) {
	layout, files := fixtureTree(t)
	files[abs("/home/u")][".claude.json"] = mustJSON(t, map[string]any{
		"oauthAccount":  map[string]any{"organizationType": "claude_max"},
		"claudeMaxTier": "5x",
	})
	inv, err := setup.NewScanner(layout, files.open).Scan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if inv.Account.Tier != "5x" {
		t.Errorf("Tier = %q, want the cached claudeMaxTier when the profile has no rate limit tier", inv.Account.Tier)
	}
}

func TestScanEmptyInstallation(t *testing.T) {
	layout := claudecode.ResolveLayout("linux", abs("/nobody"), func(string) string { return "" })
	inv, err := setup.NewScanner(layout, tree{}.open).Scan(t.Context())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(inv.Instructions)+len(inv.MCPServers)+len(inv.Hooks)+len(inv.Extensions) != 0 || inv.Account.Plan != setup.PlanUnknown {
		t.Errorf("empty installation produced %+v", inv)
	}
}

func TestScanHonoursCancellation(t *testing.T) {
	layout, files := fixtureTree(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := setup.NewScanner(layout, files.open).Scan(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Scan error = %v, want context.Canceled", err)
	}
}

func TestHomeAsAProjectCountsItsClaudeMDOnce(t *testing.T) {
	layout := claudecode.Layout{ConfigDir: abs("/home/u/.claude"), GlobalConfig: abs("/home/u/.claude.json")}
	global := map[string]any{"projects": map[string]any{abs("/home/u"): map[string]any{}}}
	inv, err := setup.NewScanner(layout, tree{
		abs("/home/u"):         {".claude.json": mustJSON(t, global), ".claude/CLAUDE.md": file("Be brief.\n")},
		abs("/home/u/.claude"): {"CLAUDE.md": file("Be brief.\n")},
	}.open).Scan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Instructions) != 1 || inv.Instructions[0].Scope != setup.ScopeUser {
		t.Errorf("instructions = %+v, want the one file at user scope", inv.Instructions)
	}
}
