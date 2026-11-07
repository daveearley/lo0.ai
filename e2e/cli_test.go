package e2e_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestVersion(t *testing.T) {
	r := newMachine(t).run("version").ok(t)
	if !strings.HasPrefix(r.stdout, "lo0 ") || !strings.HasSuffix(r.stdout, "\n") || strings.Count(r.stdout, "\n") != 1 {
		t.Fatalf("lo0 version printed %q, want one line starting with lo0", r.stdout)
	}
	if got := strings.TrimSpace(strings.TrimPrefix(r.stdout, "lo0 ")); wantVersion != "" && got != wantVersion {
		t.Errorf("lo0 version = %q, want %q", got, wantVersion)
	}
	if r.stderr != "" {
		t.Errorf("lo0 version wrote to stderr: %q", r.stderr)
	}
}

func TestCommandLine(t *testing.T) {
	commands := []string{"scan", "match", "try", "migrate", "benchmark", "ui", "web", "version"}
	tests := []struct {
		name   string
		args   []string
		code   int
		stdout []string
		stderr []string
	}{
		{name: "no arguments outside a terminal prints usage", args: nil, code: 2, stdout: commands},
		{name: "help", args: []string{"help"}, code: 0, stdout: commands},
		{name: "--help", args: []string{"--help"}, code: 0, stdout: commands},
		{name: "unknown command", args: []string{"frobnicate"}, code: 2, stderr: []string{`unknown command "frobnicate"`, "Usage"}},
		{name: "command help", args: []string{"match", "-h"}, code: 0, stderr: []string{"lo0 match", "--json", "--redact", "--offline"}},
		{name: "unknown flag", args: []string{"scan", "--frobnicate"}, code: 2, stderr: []string{"frobnicate"}},
		{name: "stray argument", args: []string{"scan", "extra"}, code: 2, stderr: []string{`unexpected argument "extra"`}},
		{name: "port out of range", args: []string{"web", "--port", "70000"}, code: 2, stderr: []string{"--port"}},
		{name: "undo takes no target", args: []string{"migrate", "--undo", "--to", "opencode"}, code: 2, stderr: []string{"--undo"}},
		{name: "unknown model", args: []string{"migrate", "--dry-run", "--model", "no-such-model"}, code: 1, stderr: []string{"no-such-model", "catalogue"}},
		{name: "ui needs a terminal", args: []string{"ui"}, code: 1, stderr: []string{"needs a terminal", "lo0 match"}},
		{name: "try needs a terminal", args: []string{"try"}, code: 1, stderr: []string{"needs a terminal", "--dry-run"}},
		{name: "nothing to undo", args: []string{"migrate", "--undo"}, code: 0, stdout: []string{"nothing to undo"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := newMachine(t).run(tt.args...)
			if r.code != tt.code {
				t.Fatalf("exit code %d, want %d\nstdout:\n%s\nstderr:\n%s", r.code, tt.code, r.stdout, r.stderr)
			}
			for _, want := range tt.stdout {
				if !strings.Contains(r.stdout, want) {
					t.Errorf("stdout lacks %q:\n%s", want, r.stdout)
				}
			}
			for _, want := range tt.stderr {
				if !strings.Contains(r.stderr, want) {
					t.Errorf("stderr lacks %q:\n%s", want, r.stderr)
				}
			}
		})
	}
}

// TestReports checks the UI rules every printed report follows when stdout
// is not a terminal: plain text, 80 columns, the report on stdout alone.
func TestReports(t *testing.T) {
	tests := []struct {
		args []string
		want []string
	}{
		{[]string{"scan"}, []string{"Machine", "Local runtimes", "Claude Code setup", "Session history"}},
		{[]string{"match"}, []string{"Verdict", "Models for this machine", "What your Codex use is worth", "ChatGPT Plus", "Claude Code and Codex together"}},
		{[]string{"migrate", "--dry-run", "--runtime", "ollama", "--model", "qwen3.5-2b"}, []string{"Plan", "Steps", "lo0 migrate --undo"}},
		{[]string{"try", "--dry-run", "--harness", "opencode", "--runtime", "ollama", "--model", "qwen3.5-2b"}, []string{"Plan", "Steps"}},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			t.Parallel()
			m := newMachine(t)
			m.startOllama()
			m.install("claude", "opencode")
			r := m.run(tt.args...).ok(t)
			plainText(t, r.stdout)
			for _, want := range tt.want {
				if !strings.Contains(r.stdout, want) {
					t.Errorf("report lacks %q:\n%s", want, r.stdout)
				}
			}
			if strings.ContainsRune(r.stderr, '\x1b') {
				t.Errorf("stderr has escape codes though it is not a terminal:\n%q", r.stderr)
			}
		})
	}
}

// plainText fails unless s is uncoloured text that fits in 80 columns.
func plainText(t *testing.T, s string) {
	t.Helper()
	if s == "" || !strings.HasSuffix(s, "\n") {
		t.Fatalf("report is empty or does not end in a newline:\n%q", s)
	}
	if strings.ContainsRune(s, '\x1b') {
		t.Errorf("report has escape codes though stdout is not a terminal")
	}
	for line := range strings.Lines(s) {
		if w := ansi.StringWidth(strings.TrimRight(line, "\r\n")); w > 80 {
			t.Errorf("line is %d columns wide, more than 80:\n%s", w, line)
		}
	}
}

func TestJSON(t *testing.T) {
	tests := []struct {
		args   []string
		schema string
	}{
		{[]string{"scan", "--json"}, "lo0.scan/v1"},
		{[]string{"match", "--json"}, "lo0.match/v1"},
		{[]string{"migrate", "--dry-run", "--json", "--runtime", "ollama", "--model", "qwen3.5-2b"}, "lo0.migrate/v1"},
		{[]string{"migrate", "--json", "--to", "opencode", "--runtime", "ollama", "--model", "qwen3.5-2b"}, "lo0.migrate/v1"},
		{[]string{"try", "--json", "--harness", "claude-code", "--runtime", "ollama", "--model", "qwen3.5-2b"}, "lo0.try/v1"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			t.Parallel()
			m := newMachine(t)
			o := m.startOllama()
			m.install("claude", "opencode")
			r := m.run(tt.args...).ok(t)
			env := decodeOne(t, r.stdout)
			if env.Schema != tt.schema {
				t.Errorf("schema = %q, want %q", env.Schema, tt.schema)
			}
			if wantVersion != "" && env.Version != wantVersion {
				t.Errorf("lo0Version = %q, want %q", env.Version, wantVersion)
			}
			if r.stderr != "" {
				t.Errorf("--json printed to stderr as well:\n%s", r.stderr)
			}
			if got := o.changed(); len(got) > 0 {
				t.Errorf("a plan changed Ollama: %v", got)
			}
		})
	}
}

// envelope is the part of every --json document that names it.
type envelope struct {
	Schema  string          `json:"schema"`
	Version string          `json:"lo0Version"`
	Result  json.RawMessage `json:"result"`
}

// decodeOne decodes s as exactly one JSON document and nothing else.
func decodeOne(t *testing.T, s string) envelope {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	var env envelope
	if err := dec.Decode(&env); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, s)
	}
	if dec.More() {
		t.Fatalf("stdout holds more than one JSON document:\n%s", s)
	}
	if len(env.Result) == 0 {
		t.Fatalf("JSON has no result:\n%s", s)
	}
	return env
}

// scannedGPU is the part of a GPU in the scan's JSON the tests check.
type scannedGPU struct {
	Name      string `json:"name"`
	VRAMBytes uint64 `json:"vramBytes"`
}

// TestScanFindsEverything checks that the scan reads every source the
// README lists from where a real installation keeps it.
func TestScanFindsEverything(t *testing.T) {
	m := newMachine(t)
	m.startOllama("qwen3:4b")
	m.install("claude")
	var scan struct {
		Runtimes []struct {
			ID      string `json:"id"`
			Running bool   `json:"running"`
			Version string `json:"version"`
			Models  []struct {
				Name string `json:"name"`
			} `json:"models"`
		} `json:"runtimes"`
		Machine struct {
			GPUs []scannedGPU `json:"gpus"`
		} `json:"machine"`
		Claude json.RawMessage `json:"claude"`
		Setup  struct {
			MCPServers []struct {
				Name string `json:"name"`
			} `json:"mcpServers"`
		} `json:"setup"`
		Usage struct {
			Sessions  int `json:"sessions"`
			Harnesses []struct {
				Harness  string `json:"harness"`
				Sessions int    `json:"sessions"`
			} `json:"harnesses"`
		} `json:"usage"`
	}
	env := decodeOne(t, m.run("scan", "--json").ok(t).stdout)
	if err := json.Unmarshal(env.Result, &scan); err != nil {
		t.Fatal(err)
	}

	if scan.Usage.Sessions != 2 {
		t.Errorf("found %d sessions, want 2: one Claude Code and one Codex", scan.Usage.Sessions)
	}
	var ollama bool
	for _, rt := range scan.Runtimes {
		if rt.ID == "ollama" {
			ollama = rt.Running && rt.Version == "0.21.0" && len(rt.Models) == 1 && rt.Models[0].Name == "qwen3:4b"
		}
	}
	if !ollama {
		t.Errorf("scan did not find the running Ollama with its one model: %+v", scan.Runtimes)
	}
	if !strings.Contains(string(scan.Claude), fakeClaudeVersion) {
		t.Errorf("scan did not find Claude Code %s on PATH: %s", fakeClaudeVersion, scan.Claude)
	}
	// The fake card, not the test machine's own, decides what fits.
	want := fakeGPUName
	if runtime.GOOS == "darwin" {
		want = fakeMacGPUName
	}
	if !slices.Contains(scan.Machine.GPUs, scannedGPU{Name: want, VRAMBytes: fakeGPUMiB << 20}) {
		t.Errorf("scan did not find the fake %s with %d MiB: %+v", want, fakeGPUMiB, scan.Machine.GPUs)
	}
}

// TestFreshMachine runs the quick start on a machine with nothing on it:
// no history, no harness, no runtime. Every assessment must still work.
func TestFreshMachine(t *testing.T) {
	m := newMachine(t)
	if err := os.RemoveAll(m.home); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(m.home, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"scan"}, {"match"}, {"migrate", "--dry-run"}, {"try", "--dry-run"}} {
		r := m.run(args...).ok(t)
		plainText(t, r.stdout)
	}
	for _, args := range [][]string{{"scan", "--json"}, {"match", "--json"}} {
		decodeOne(t, m.run(args...).ok(t).stdout)
	}
	if diff := changes(tree{}, snapshot(t, m.home)); len(diff) > 0 {
		t.Errorf("assessing a fresh machine wrote to its home directory:\n%s", strings.Join(diff, "\n"))
	}
}

// TestClaudeDir checks --claude-dir reads Claude Code's files from where
// the flag says.
func TestClaudeDir(t *testing.T) {
	m := newMachine(t)
	moved := filepath.Join(m.root, "elsewhere")
	if err := os.Rename(filepath.Join(m.home, ".claude"), moved); err != nil {
		t.Fatal(err)
	}
	sessions := func(args ...string) int {
		var res struct {
			Usage struct {
				Sessions int `json:"sessions"`
			} `json:"usage"`
		}
		env := decodeOne(t, m.run(append([]string{"scan", "--json"}, args...)...).ok(t).stdout)
		if err := json.Unmarshal(env.Result, &res); err != nil {
			t.Fatal(err)
		}
		return res.Usage.Sessions
	}
	if got := sessions(); got != 1 {
		t.Errorf("without ~/.claude, found %d sessions, want the 1 from Codex", got)
	}
	if got := sessions("--claude-dir", moved); got != 2 {
		t.Errorf("with --claude-dir, found %d sessions, want 2", got)
	}
}

// TestCodexDir checks --codex-dir reads Codex's session logs from where the
// flag says, archived ones included.
func TestCodexDir(t *testing.T) {
	m := newMachine(t)
	moved := filepath.Join(m.root, "codex-elsewhere")
	if err := os.Rename(filepath.Join(m.home, ".codex"), moved); err != nil {
		t.Fatal(err)
	}
	harnesses := func(args ...string) map[string]int {
		var res struct {
			Usage struct {
				Harnesses []struct {
					Name    string `json:"name"`
					Profile struct {
						Sessions int `json:"sessions"`
					} `json:"profile"`
				} `json:"harnesses"`
			} `json:"usage"`
		}
		env := decodeOne(t, m.run(append([]string{"scan", "--json"}, args...)...).ok(t).stdout)
		if err := json.Unmarshal(env.Result, &res); err != nil {
			t.Fatal(err)
		}
		out := map[string]int{}
		for _, h := range res.Usage.Harnesses {
			out[h.Name] = h.Profile.Sessions
		}
		return out
	}
	if got := harnesses()["Codex"]; got != 0 {
		t.Errorf("without ~/.codex, found %d Codex sessions, want none", got)
	}
	if got := harnesses("--codex-dir", moved)["Codex"]; got != 1 {
		t.Errorf("with --codex-dir, found %d Codex sessions, want 1", got)
	}
	archived := filepath.Join(moved, "archived_sessions")
	if err := os.MkdirAll(archived, 0o755); err != nil {
		t.Fatal(err)
	}
	logs, err := filepath.Glob(filepath.Join(moved, "sessions", "*", "*", "*", "rollout-*.jsonl"))
	if err != nil || len(logs) != 1 {
		t.Fatalf("Codex fixture logs = %v, %v", logs, err)
	}
	if err := os.Rename(logs[0], filepath.Join(archived, filepath.Base(logs[0]))); err != nil {
		t.Fatal(err)
	}
	if got := harnesses("--codex-dir", moved)["Codex"]; got != 1 {
		t.Errorf("with the session archived, found %d Codex sessions, want 1", got)
	}
}
