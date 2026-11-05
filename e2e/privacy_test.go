package e2e_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// secretMarker starts every string in testdata/home that lo0 must never
// print: prompts, replies, reasoning, command arguments, file contents,
// MCP env and header values, settings env values and CLAUDE.md text.
const secretMarker = "SECRET_"

// assessments are the commands that only read.
var assessments = [][]string{
	{"scan"},
	{"scan", "--json"},
	{"match"},
	{"match", "--json"},
	{"migrate", "--dry-run", "--runtime", "ollama", "--model", "qwen3.5-2b"},
	{"migrate", "--dry-run", "--json", "--to", "opencode", "--runtime", "ollama", "--model", "qwen3.5-2b"},
	{"migrate", "--dry-run", "--json", "--to", "claude-code", "--runtime", "ollama", "--model", "qwen3.5-2b"},
	{"try", "--dry-run", "--harness", "opencode", "--runtime", "ollama", "--model", "qwen3.5-2b"},
	{"try", "--json", "--harness", "claude-code", "--runtime", "ollama", "--model", "qwen3.5-2b"},
	{"migrate", "--undo"},
}

// TestNoContentLeaves runs every assessment and fails if any of them prints
// something lo0 promises to reduce to counts and names.
func TestNoContentLeaves(t *testing.T) {
	for _, args := range assessments {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			m := newMachine(t)
			m.startOllama()
			m.install("claude", "opencode")
			r := m.run(args...).ok(t)
			noSecrets(t, r.output())
		})
	}
}

// TestRedact checks --redact removes the home directory and every project
// path from text and JSON alike.
func TestRedact(t *testing.T) {
	for _, args := range [][]string{{"scan"}, {"scan", "--json"}, {"match"}, {"match", "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			m := newMachine(t)
			m.startOllama()
			plain := m.run(args...).ok(t).stdout
			if !strings.Contains(plain, "/home/u/code/app") {
				t.Fatalf("without --redact the output does not name the project path the test looks for:\n%s", plain)
			}
			r := m.run(append(args, "--redact")...).ok(t)
			for _, private := range append(spellings(m.home), "/home/u/code/app", "/r/app") {
				if strings.Contains(r.output(), private) {
					t.Errorf("--redact output still contains %q:\n%s", private, r.output())
				}
			}
		})
	}
}

// spellings are the ways a path can appear in output: as the OS writes it,
// with forward slashes, and escaped in JSON.
func spellings(path string) []string {
	quoted, _ := json.Marshal(path)
	out := []string{path, filepath.ToSlash(path), strings.Trim(string(quoted), `"`)}
	return slices.Compact(slices.Sorted(slices.Values(out)))
}

func noSecrets(t *testing.T, s string) {
	t.Helper()
	for line := range strings.Lines(s) {
		if strings.Contains(line, secretMarker) {
			t.Errorf("output contains private content: %s", strings.TrimSpace(line))
		}
	}
}

// TestAssessmentOnlyReads runs every assessment against one machine and
// checks that afterwards nothing in the home or working directory has
// changed, Ollama holds what it held, and no request left the machine.
func TestAssessmentOnlyReads(t *testing.T) {
	m := newMachine(t)
	o := m.startOllama("qwen3:4b")
	m.install("claude", "opencode")
	home, cwd := snapshot(t, m.home), snapshot(t, m.cwd)

	for _, args := range assessments {
		m.run(args...).ok(t)
	}

	if diff := changes(home, snapshot(t, m.home)); len(diff) > 0 {
		t.Errorf("assessments changed the home directory:\n%s", strings.Join(diff, "\n"))
	}
	if diff := changes(cwd, snapshot(t, m.cwd)); len(diff) > 0 {
		t.Errorf("assessments changed the working directory:\n%s", strings.Join(diff, "\n"))
	}
	if got := o.changed(); len(got) > 0 {
		t.Errorf("assessments changed Ollama: %v", got)
	}
	if got := m.proxy.seen(); len(got) > 0 {
		t.Errorf("with LO0_OFFLINE set, lo0 still tried to reach %v", got)
	}
}

// TestOnlineLookups lets lo0 make the lookups that only add information
// and checks they go only to the sources the README names, and that a
// failed lookup does not fail the command.
func TestOnlineLookups(t *testing.T) {
	m := newMachine(t)
	m.online = true
	m.install("claude")
	for _, args := range [][]string{{"scan", "--json"}, {"match"}} {
		r := m.run(args...).ok(t)
		noSecrets(t, r.output())
	}
	allowed := []string{"registry.npmjs.org:443"}
	for _, host := range m.proxy.seen() {
		if !slices.Contains(allowed, host) {
			t.Errorf("lo0 tried to reach %s, which is not one of %v", host, allowed)
		}
	}
	if len(m.proxy.seen()) == 0 {
		t.Errorf("online, lo0 made no lookups; it should ask the npm registry for the newest Claude Code")
	}

	m.proxy.reset()
	m.run("scan", "--json", "--offline").ok(t)
	if got := m.proxy.seen(); len(got) > 0 {
		t.Errorf("with --offline, lo0 still tried to reach %v", got)
	}
}
