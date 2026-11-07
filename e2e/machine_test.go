package e2e_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// runTimeout bounds one lo0 command. A scan of the fixture takes well under
// a second; anything near this is a hang.
const runTimeout = 2 * time.Minute

// machine is one user's computer as lo0 sees it: a home directory copied
// from testdata/home, an empty working directory, a PATH with only the
// system's own tools, the fake GPU's tool and the fake harnesses installed
// on it, and the services lo0 can reach.
type machine struct {
	t      *testing.T
	root   string
	home   string
	cwd    string
	bin    string
	proxy  *recordingProxy
	ollama *fakeOllama
	// online lets lo0 make the lookups --offline would skip. They still
	// reach only the recording proxy.
	online bool
}

func newMachine(t *testing.T) *machine {
	t.Helper()
	root := t.TempDir()
	m := &machine{
		t:     t,
		root:  root,
		home:  filepath.Join(root, "home"),
		cwd:   filepath.Join(root, "work"),
		bin:   filepath.Join(root, "bin"),
		proxy: newRecordingProxy(t),
	}
	if err := os.CopyFS(m.home, os.DirFS(filepath.Join("testdata", "home"))); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{m.cwd, m.bin, filepath.Join(root, "tmp")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	m.install(gpuTool())
	return m
}

// install puts fake harnesses, or the fake GPU's tool, on lo0's PATH.
func (m *machine) install(harnesses ...string) {
	m.t.Helper()
	src, err := os.ReadFile(selfBinary)
	if err != nil {
		m.t.Fatal(err)
	}
	for _, h := range harnesses {
		if err := os.WriteFile(filepath.Join(m.bin, h+exe()), src, 0o755); err != nil {
			m.t.Fatal(err)
		}
	}
}

// startOllama runs a fake Ollama with models already pulled and points
// OLLAMA_HOST at it.
func (m *machine) startOllama(models ...string) *fakeOllama {
	m.ollama = newFakeOllama(m.t, models...)
	return m.ollama
}

// environ is lo0's whole environment. Nothing is inherited that could make
// a run depend on the machine running the tests, apart from what the
// operating system itself needs.
func (m *machine) environ() []string {
	ollama := "127.0.0.1:9" // the discard port: nothing answers there
	if m.ollama != nil {
		ollama = m.ollama.host()
	}
	env := []string{
		"OLLAMA_HOST=" + ollama,
		"HTTP_PROXY=" + m.proxy.url(),
		"HTTPS_PROXY=" + m.proxy.url(),
	}
	if !m.online {
		env = append(env, "LO0_OFFLINE=1")
	}
	tmp := filepath.Join(m.root, "tmp")
	if runtime.GOOS != "windows" {
		return append(env,
			"HOME="+m.home,
			"TMPDIR="+tmp,
			"PATH="+m.bin+string(os.PathListSeparator)+"/usr/bin:/bin:/usr/sbin:/sbin",
		)
	}
	sys := os.Getenv("SystemRoot")
	env = append(env,
		"USERPROFILE="+m.home,
		"HOME="+m.home,
		"LOCALAPPDATA="+filepath.Join(m.home, "AppData", "Local"),
		"APPDATA="+filepath.Join(m.home, "AppData", "Roaming"),
		"TEMP="+tmp,
		"TMP="+tmp,
		"PATH="+strings.Join([]string{
			m.bin,
			filepath.Join(sys, "System32"),
			sys,
			filepath.Join(sys, "System32", "Wbem"),
			filepath.Join(sys, "System32", "WindowsPowerShell", "v1.0"),
		}, string(os.PathListSeparator)),
	)
	for _, k := range []string{"SystemRoot", "windir", "ComSpec", "PATHEXT", "OS", "PROCESSOR_ARCHITECTURE", "NUMBER_OF_PROCESSORS"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// result is what one lo0 command did.
type result struct {
	args   []string
	stdout string
	stderr string
	code   int
}

// run runs lo0 to completion with no input.
func (m *machine) run(args ...string) result {
	m.t.Helper()
	ctx, cancel := context.WithTimeout(m.t.Context(), runTimeout)
	defer cancel()
	cmd := m.command(ctx, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	r := result{args: args, stdout: stdout.String(), stderr: stderr.String(), code: cmd.ProcessState.ExitCode()}
	if ctx.Err() != nil {
		m.t.Fatalf("lo0 %s did not finish within %s\nstderr:\n%s", strings.Join(args, " "), runTimeout, r.stderr)
	}
	if _, exited := errors.AsType[*exec.ExitError](err); err != nil && !exited {
		m.t.Fatalf("starting lo0: %v", err)
	}
	return r
}

func (m *machine) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, lo0Binary, args...)
	cmd.Dir = m.cwd
	cmd.Env = m.environ()
	return cmd
}

// ok fails the test unless the command exited 0.
func (r result) ok(t *testing.T) result {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("lo0 %s exited %d\nstdout:\n%s\nstderr:\n%s", strings.Join(r.args, " "), r.code, r.stdout, r.stderr)
	}
	return r
}

// output is everything the command printed.
func (r result) output() string { return r.stdout + r.stderr }

// tree is a directory's contents: each path, slash-separated, maps to the
// SHA-256 of a file's contents or to "dir".
type tree map[string]string

func snapshot(t *testing.T, root string) tree {
	t.Helper()
	out := tree{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			out[rel] = "dir"
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		out[rel] = hex.EncodeToString(h.Sum(nil))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// changes lists every path added, removed or changed between two trees,
// leaving out anything under the ignored prefixes.
func changes(before, after tree, ignore ...string) []string {
	skip := func(p string) bool {
		return slices.ContainsFunc(ignore, func(prefix string) bool { return p == prefix || strings.HasPrefix(p, prefix+"/") })
	}
	var out []string
	for p, sum := range after {
		switch was, ok := before[p]; {
		case skip(p):
		case !ok:
			out = append(out, "added "+p)
		case was != sum:
			out = append(out, "changed "+p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok && !skip(p) {
			out = append(out, "removed "+p)
		}
	}
	slices.Sort(out)
	return out
}

// stateDir is where lo0 keeps its journal, relative to home, as
// wiring.StateDir places it on this platform.
func stateDir() string {
	switch runtime.GOOS {
	case "darwin":
		return "Library/Application Support/lo0"
	case "windows":
		return "AppData/Local/lo0"
	default:
		return ".local/state/lo0"
	}
}

// files is t without its directories.
func files(t tree) tree {
	out := tree{}
	for p, sum := range t {
		if sum != "dir" {
			out[p] = sum
		}
	}
	return out
}
