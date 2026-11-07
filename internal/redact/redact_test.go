package redact_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/redact"
)

func TestPaths(t *testing.T) {
	home := filepath.FromSlash("/Users/ada")
	app := filepath.FromSlash("/Users/ada/code/acme")
	web := filepath.FromSlash("/Users/ada/code/acme-web")
	elsewhere := filepath.FromSlash("/srv/client")
	projects := []string{app, web, elsewhere, app, ""}

	tests := []struct {
		name, in, want string
	}{
		{"absolute", "repo " + app + " done", "repo project 1 done"},
		{"prefix of another project", web + " and " + app, "project 2 and project 1"},
		{"tilde form", filepath.FromSlash("~/code/acme-web") + " 61 sessions", "project 2 61 sessions"},
		{"outside home", elsewhere, "project 3"},
		{"file inside a project", filepath.Join(app, "CLAUDE.md"), filepath.Join("project 1", "CLAUDE.md")},
		{"other home paths", filepath.Join(home, ".claude", "settings.json"), filepath.Join("~", ".claude", "settings.json")},
		{"Claude Code's folder for a project", "opening -Users-ada-code-acme/s2.jsonl", "opening project 1/s2.jsonl"},
		{"Claude Code's folder for another project", "-Users-ada-code-SECRET-other/s.jsonl", "another project/s.jsonl"},
		{"Claude Code's folder for a subfolder", "-Users-ada-code-acme-SECRET-billing/s.jsonl", "project 1/s.jsonl"},
		{"a longer name is another path", filepath.FromSlash("/Users/ada/code/acme2/x"), filepath.FromSlash("~/code/acme2/x")},
		{"Claude Code's folder for home", "-Users-ada/s.jsonl", "~/s.jsonl"},
		{"nothing to hide", "Bash 35,349", "Bash 35,349"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redact.Paths(tt.in, home, projects); got != tt.want {
				t.Errorf("Paths(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPathsInJSON(t *testing.T) {
	home := `C:\Users\ada`
	project := `C:\Users\ada\code\acme`
	data, err := json.Marshal(map[string]string{"repo": project, "config": home + `\.claude`})
	if err != nil {
		t.Fatal(err)
	}
	got := redact.Paths(string(data), home, []string{project})
	if strings.Contains(got, "ada") {
		t.Errorf("Paths left the user's name in %s", got)
	}
}

func TestPathsInEveryWindowsSpelling(t *testing.T) {
	home := `C:\Users\ada`
	project := `C:\Users\ada\code\acme`
	for _, in := range []string{
		`C:\Users\ada\code\acme`,
		`C:/Users/ada/code/acme`,
		`c:\Users\ada\code\acme`,
		`c:/Users/ada/code/acme`,
		`"C:\\Users\\ada\\code\\acme"`,
		`"c:\\Users\\ada\\code\\acme"`,
	} {
		t.Run(in, func(t *testing.T) {
			if got := redact.Paths(in, home, []string{project}); strings.Contains(got, "ada") || !strings.Contains(got, "project 1") {
				t.Errorf("Paths(%q) = %q", in, got)
			}
		})
	}
	for _, in := range []string{`C:/Users/ada/.claude`, `c:\Users\ada\.claude`} {
		if got := redact.Paths(in, home, []string{project}); strings.Contains(got, "ada") {
			t.Errorf("Paths(%q) = %q, want the home directory replaced", in, got)
		}
	}
}

// A session started in the root directory must not make "/" a project, or
// every slash in the output would become "project 1".
func TestPathsNeverHidesARoot(t *testing.T) {
	for _, root := range []string{"/", `C:\`, "C:/"} {
		in := `{"schema":"lo0.scan/v1","dir":"/Users/ada/x"}`
		got := redact.Paths(in, "/Users/ada", []string{root, "/Users/ada/code/acme"})
		if want := `{"schema":"lo0.scan/v1","dir":"~/x"}`; got != want {
			t.Errorf("with %q as a project, Paths = %s, want %s", root, got, want)
		}
	}
}
