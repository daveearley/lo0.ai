package e2e_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	// The tests run a binary rather than call these packages, so go test
	// cannot tell that a change to lo0 changes their result. Linking the
	// command-line front end, which reaches every package cmd/lo0 links,
	// ties the cached result to lo0's source.
	_ "lo0.ai/lo0/internal/cli"
)

// builtVersion is the version linked into the binary these tests build, so
// lo0 version can be checked exactly.
const builtVersion = "v0.0.0-e2e"

// Versions the fake harnesses report.
const (
	fakeClaudeVersion   = "2.1.300"
	fakeOpenCodeVersion = "2.0.4"
)

var (
	// lo0Binary is the binary under test.
	lo0Binary string
	// wantVersion is what lo0 version should print after "lo0 ", or empty
	// when the binary came from LO0_E2E_BINARY and its version is not known.
	wantVersion string
	// selfBinary is this test binary, copied onto lo0's PATH as each fake
	// harness.
	selfBinary string
)

func TestMain(m *testing.M) {
	// Copied onto PATH as claude or opencode, this binary plays that
	// harness instead of running the tests.
	switch harness := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe"); harness {
	case "claude", "opencode":
		os.Exit(fakeHarness(harness, os.Args[1:]))
	}
	os.Exit(run(m))
}

func run(m *testing.M) int {
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}
	selfBinary = self

	if bin := os.Getenv("LO0_E2E_BINARY"); bin != "" {
		abs, err := filepath.Abs(bin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "e2e:", err)
			return 1
		}
		lo0Binary, wantVersion = abs, os.Getenv("LO0_E2E_VERSION")
		return m.Run()
	}

	dir, err := os.MkdirTemp("", "lo0-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}
	defer os.RemoveAll(dir)
	lo0Binary = filepath.Join(dir, "lo0"+exe())
	if err := build(lo0Binary); err != nil {
		fmt.Fprintln(os.Stderr, "e2e: building lo0:", err)
		return 1
	}
	wantVersion = builtVersion
	return m.Run()
}

// build compiles cmd/lo0 the way make build does. go test puts its own Go
// first on PATH, so this is the toolchain running the tests.
func build(out string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath",
		"-ldflags", "-s -w -X lo0.ai/lo0/internal/buildinfo.version="+builtVersion,
		"-o", out, "lo0.ai/lo0/cmd/lo0")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w\n%s", err, b)
	}
	return nil
}

// fakeHarness answers the way the real harness does to everything lo0 asks
// of it: its version.
func fakeHarness(name string, args []string) int {
	if len(args) == 1 && args[0] == "--version" {
		switch name {
		case "claude":
			fmt.Println(fakeClaudeVersion + " (Claude Code)")
		default:
			fmt.Println(fakeOpenCodeVersion)
		}
		return 0
	}
	fmt.Fprintf(os.Stderr, "fake %s: unexpected arguments %q\n", name, args)
	return 2
}

func exe() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
