package migrate_test

import (
	"slices"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/migrate"
)

func TestSessionEnviron(t *testing.T) {
	s := migrate.Session{
		Env:   []migrate.EnvVar{{Name: "BASE_URL", Value: "http://127.0.0.1:11434"}, {Name: "MODEL", Value: "lo0/x"}},
		Unset: []string{"API_KEY"},
	}
	for _, tt := range []struct {
		name       string
		base, want []string
	}{
		{"adds to an empty environment", nil, []string{"BASE_URL=http://127.0.0.1:11434", "MODEL=lo0/x"}},
		{
			"keeps unrelated variables",
			[]string{"HOME=/home/u", "PATH=/usr/bin"},
			[]string{"HOME=/home/u", "PATH=/usr/bin", "BASE_URL=http://127.0.0.1:11434", "MODEL=lo0/x"},
		},
		{
			"replaces rather than duplicates",
			[]string{"MODEL=claude-opus", "HOME=/home/u"},
			[]string{"HOME=/home/u", "BASE_URL=http://127.0.0.1:11434", "MODEL=lo0/x"},
		},
		{
			"removes unset variables",
			[]string{"API_KEY=sk-secret", "API_KEY_FILE=/k"},
			[]string{"API_KEY_FILE=/k", "BASE_URL=http://127.0.0.1:11434", "MODEL=lo0/x"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.Environ(tt.base); !slices.Equal(got, tt.want) {
				t.Errorf("Environ(%q) = %q, want %q", tt.base, got, tt.want)
			}
		})
	}
}

func TestSessionCommand(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string
	}{
		{nil, "claude"},
		{[]string{"--resume", "x"}, "claude --resume x"},
		{[]string{"-p", "Reply with ready"}, "claude -p 'Reply with ready'"},
		{[]string{"-p", "it's"}, `claude -p 'it'\''s'`},
		{[]string{""}, "claude ''"},
	} {
		if got := (migrate.Session{Program: "claude", Args: tt.args}).Command(); got != tt.want {
			t.Errorf("Command() with %q = %q, want %q", tt.args, got, tt.want)
		}
	}
}

type fakeLauncher struct{ fakeHarness }

func (fakeLauncher) Session(_ migrate.Request, s migrate.Served) (migrate.Session, []migrate.Step, error) {
	return migrate.Session{Program: "fake", Env: []migrate.EnvVar{{Name: "MODEL", Value: s.Model}}},
		[]migrate.Step{{Kind: migrate.KindManual, Title: "Install Fake", Done: true}}, nil
}

func TestBuildTryStartsASessionWithoutWrites(t *testing.T) {
	req := migrate.Request{Model: catalog.Model{ID: "qwen3-coder-30b-a3b", Name: "Qwen3-Coder-30B-A3B"}}
	p, err := migrate.BuildTry(req, fakeBackend{}, fakeLauncher{})
	if err != nil {
		t.Fatal(err)
	}
	titles := make([]string, len(p.Steps))
	for i, s := range p.Steps {
		titles[i] = s.Title
		if s.Kind == migrate.KindWrite {
			t.Errorf("a try writes %q", s.Title)
		}
	}
	if want := []string{"pull", "Install Fake", "Check that Ollama answers as lo0/openai"}; !slices.Equal(titles, want) {
		t.Errorf("steps = %q, want %q", titles, want)
	}
	if p.Session == nil || p.Session.Program != "fake" || p.Session.Env[0].Value != "lo0/openai" {
		t.Fatalf("session = %+v", p.Session)
	}
	if len(p.Then) != 1 || !strings.Contains(p.Then[0], "lo0 migrate --to opencode --model qwen3-coder-30b-a3b --runtime ollama") {
		t.Errorf("then = %q, want how to keep the setup", p.Then)
	}
}

func TestPlanChanges(t *testing.T) {
	for _, tt := range []struct {
		name  string
		steps []migrate.Step
		want  bool
	}{
		{"nothing but the check", []migrate.Step{{Kind: migrate.KindCheck}}, false},
		{"a manual step", []migrate.Step{{Kind: migrate.KindManual}, {Kind: migrate.KindCheck}}, false},
		{"a finished download", []migrate.Step{{Kind: migrate.KindDownload, Done: true}}, false},
		{"a download", []migrate.Step{{Kind: migrate.KindDownload}}, true},
		{"a runtime change", []migrate.Step{{Kind: migrate.KindRuntime}}, true},
		{"a write", []migrate.Step{{Kind: migrate.KindWrite}}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := (migrate.Plan{Steps: tt.steps}).Changes(); got != tt.want {
				t.Errorf("Changes() = %v, want %v", got, tt.want)
			}
		})
	}
}
