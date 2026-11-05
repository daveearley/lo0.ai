package claude_test

import (
	"errors"
	"slices"
	"testing"

	"lo0.ai/lo0/internal/harness/claude"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/system"
)

func TestSessionPassesTheModelInTheEnvironment(t *testing.T) {
	req := migrate.Request{GOOS: "darwin", Home: "/Users/u", LookPath: func(string) (string, error) { return "/Users/u/.local/bin/claude", nil }}
	s, steps, err := claude.Harness{}.Session(req, served())
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 0 {
		t.Errorf("steps = %+v, want none when Claude Code is installed", steps)
	}
	if s.Program != "claude" || !slices.Equal(s.Unset, []string{"ANTHROPIC_API_KEY"}) {
		t.Errorf("session = %+v", s)
	}
	env := map[string]string{}
	for _, v := range s.Env {
		env[v.Name] = v.Value
	}
	for name, want := range map[string]string{
		"ANTHROPIC_BASE_URL":             "http://127.0.0.1:11434",
		"ANTHROPIC_MODEL":                "lo0/qwen3-coder-30b-a3b:64k",
		"CLAUDE_CODE_SUBAGENT_MODEL":     "lo0/qwen3-coder-30b-a3b:64k",
		"CLAUDE_CODE_MAX_CONTEXT_TOKENS": "65536",
	} {
		if env[name] != want {
			t.Errorf("%s = %q, want %q", name, env[name], want)
		}
	}
}

func TestSessionAsksForClaudeCodeWhenMissing(t *testing.T) {
	missing := func(name string) (string, error) { return "", errors.Join(system.ErrNotFound, errors.New(name)) }
	_, steps, err := claude.Harness{}.Session(migrate.Request{GOOS: "linux", LookPath: missing}, served())
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0].Kind != migrate.KindManual || steps[0].Title != "Install Claude Code" {
		t.Errorf("steps = %+v", steps)
	}
}
