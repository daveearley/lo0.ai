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
	fakeCodexVersion    = "0.167.0"
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
	// Copied onto PATH as claude, opencode or codex, this binary plays that
	// harness instead of running the tests; as nvidia-smi or
	// system_profiler, it reports the fake GPU.
	switch name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe"); name {
	case "claude", "opencode", "codex":
		os.Exit(fakeHarness(name, os.Args[1:]))
	case "nvidia-smi", "system_profiler":
		os.Exit(fakeGPUTool(name, os.Args[1:]))
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
		case "codex":
			fmt.Println("codex-cli " + fakeCodexVersion)
		default:
			fmt.Println(fakeOpenCodeVersion)
		}
		return 0
	}
	fmt.Fprintf(os.Stderr, "fake %s: unexpected arguments %q\n", name, args)
	return 2
}

// fakeGPU is the graphics card every test machine has, so a model fits
// whatever hardware runs the tests: CI runners have no GPU, and a real one
// would make the results depend on the machine. It is discrete, so the
// memory for models is its own and not a share of the runner's RAM.
const (
	fakeGPUName = "NVIDIA GeForce RTX 4090"
	fakeGPUMiB  = 24 << 10
	// fakeMacGPUName is the card on macOS, where lo0 asks system_profiler.
	fakeMacGPUName = "AMD Radeon PRO W6800X"
)

// gpuTool is the tool lo0 asks about GPUs on this platform, which the
// machine fakes. Windows also asks the registry through PowerShell, whose
// adapters, without video memory of their own, add nothing to the budget.
func gpuTool() string {
	if runtime.GOOS == "darwin" {
		return "system_profiler"
	}
	return "nvidia-smi"
}

// fakeGPUTool answers lo0's GPU query with the fake card.
func fakeGPUTool(name string, args []string) int {
	switch {
	case name == "nvidia-smi" && len(args) == 2 && args[0] == "--query-gpu=name,memory.total":
		fmt.Printf("%s, %d\n", fakeGPUName, fakeGPUMiB)
	case name == "system_profiler" && len(args) == 2 && args[0] == "SPDisplaysDataType":
		fmt.Printf(`{"SPDisplaysDataType":[{"sppci_model":%q,"spdisplays_vendor":"sppci_vendor_amd","spdisplays_vram":"%d GB"}]}`+"\n", fakeMacGPUName, fakeGPUMiB>>10)
	default:
		fmt.Fprintf(os.Stderr, "fake %s: unexpected arguments %q\n", name, args)
		return 2
	}
	return 0
}

func exe() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
