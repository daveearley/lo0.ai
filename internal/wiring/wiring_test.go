package wiring_test

import (
	"path/filepath"
	"testing"

	"lo0.ai/lo0/internal/wiring"
)

func env(t *testing.T) wiring.Environment {
	t.Helper()
	return wiring.Environment{GOOS: "linux", Home: t.TempDir(), Getenv: func(string) string { return "" }}
}

func TestServiceBuilds(t *testing.T) {
	svc, err := wiring.Service(env(t))
	if err != nil || svc == nil {
		t.Fatalf("Service = %v, %v", svc, err)
	}
	if len(svc.Models()) == 0 {
		t.Error("service has an empty catalog")
	}
}

func TestWithOverride(t *testing.T) {
	base := env(t)
	base.Getenv = func(k string) string { return "base-" + k }
	e := base.WithOverride("CLAUDE_CONFIG_DIR", "/tmp/claude")
	if got := e.Getenv("CLAUDE_CONFIG_DIR"); got != "/tmp/claude" {
		t.Errorf("override = %q", got)
	}
	if got := e.Getenv("HOME"); got != "base-HOME" {
		t.Errorf("other keys = %q, want them passed through", got)
	}
	if got := base.Getenv("CLAUDE_CONFIG_DIR"); got != "base-CLAUDE_CONFIG_DIR" {
		t.Errorf("override leaked into the original environment: %q", got)
	}
}

func TestFromOS(t *testing.T) {
	e, err := wiring.FromOS()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}
	if e.Home == "" || e.GOOS == "" || e.Getenv == nil {
		t.Errorf("FromOS = %+v", e)
	}
}

func TestStateDir(t *testing.T) {
	none := func(string) string { return "" }
	tests := []struct {
		env  wiring.Environment
		want string
	}{
		{wiring.Environment{GOOS: "darwin", Home: "/Users/u", Getenv: none}, filepath.Join("/Users/u", "Library", "Application Support", "lo0")},
		{wiring.Environment{GOOS: "linux", Home: "/home/u", Getenv: none}, filepath.Join("/home/u", ".local", "state", "lo0")},
		{wiring.Environment{GOOS: "linux", Home: "/home/u", Getenv: func(k string) string {
			if k == "XDG_STATE_HOME" {
				return "/state"
			}
			return ""
		}}, filepath.Join("/state", "lo0")},
		{wiring.Environment{GOOS: "windows", Home: `C:\Users\u`, Getenv: func(k string) string {
			if k == "LOCALAPPDATA" {
				return `C:\Users\u\AppData\Local`
			}
			return ""
		}}, filepath.Join(`C:\Users\u\AppData\Local`, "lo0")},
	}
	for _, tt := range tests {
		if got := wiring.StateDir(tt.env); got != tt.want {
			t.Errorf("StateDir(%s) = %q, want %q", tt.env.GOOS, got, tt.want)
		}
	}
	if j := wiring.Migrator(tests[1].env).Effects.Journal.Path; j != filepath.Join(tests[1].want, "migrations.json") {
		t.Errorf("journal = %q", j)
	}
}
