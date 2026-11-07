package e2e_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSimulatedHardware checks the README's promise for --gpu and --mac.
func TestSimulatedHardware(t *testing.T) {
	m := newMachine(t)
	o := m.startOllama("qwen3:4b")
	m.install("claude", "opencode")
	home, cwd := snapshot(t, m.home), snapshot(t, m.cwd)

	var got struct {
		Scan struct {
			Machine struct {
				Simulated bool         `json:"simulated"`
				GPUs      []scannedGPU `json:"gpus"`
			} `json:"machine"`
			Usage struct {
				Sessions int `json:"sessions"`
			} `json:"usage"`
		} `json:"scan"`
		Match struct {
			Budget struct {
				Bytes uint64 `json:"bytes"`
			} `json:"budget"`
		} `json:"match"`
	}
	env := decodeOne(t, m.run("match", "--json", "--gpu", "RTX 4090", "--gpus", "2", "--ram", "128").ok(t).stdout)
	if err := json.Unmarshal(env.Result, &got); err != nil {
		t.Fatal(err)
	}
	want := scannedGPU{Name: "NVIDIA GeForce RTX 4090", VRAMBytes: 24 << 30}
	if !got.Scan.Machine.Simulated || len(got.Scan.Machine.GPUs) != 2 || got.Scan.Machine.GPUs[0] != want {
		t.Errorf("machine = %+v, want two simulated RTX 4090s", got.Scan.Machine)
	}
	if got.Match.Budget.Bytes != 48<<30 {
		t.Errorf("budget = %d bytes, want both cards' 48 GiB", got.Match.Budget.Bytes)
	}
	if got.Scan.Usage.Sessions != 2 {
		t.Errorf("found %d sessions, want this machine's 2", got.Scan.Usage.Sessions)
	}

	text := m.run("match", "--mac", "M4 Max", "--memory", "64").ok(t).stdout
	if !strings.Contains(strings.SplitN(text, "Verdict", 2)[0], "Simulated machine: Apple M4 Max") {
		t.Errorf("the report does not say first that the machine is simulated:\n%s", text)
	}

	r := m.run("match", "--gpu", "RTX 4091")
	if r.code != 1 || !strings.Contains(r.stderr, "RTX 4090") || r.stdout != "" {
		t.Errorf("unknown card: exit %d, stdout %q, stderr %q; want 1 and the closest card", r.code, r.stdout, r.stderr)
	}

	for _, args := range [][]string{
		{"try", "--yes", "--gpu", "RTX 4090", "--runtime", "ollama", "--model", "qwen3.5-2b"},
		{"migrate", "--yes", "--mac", "M4 Max", "--memory", "64", "--runtime", "ollama", "--model", "qwen3.5-2b"},
		{"benchmark", "--yes", "--gpu=RTX 4090"},
	} {
		r := m.run(args...)
		if r.code != 2 || !strings.Contains(r.stderr, "works on this machine") {
			t.Errorf("lo0 %s: exit %d, stderr %q; want a refusal", strings.Join(args, " "), r.code, r.stderr)
		}
	}

	if diff := changes(home, snapshot(t, m.home)); len(diff) > 0 {
		t.Errorf("a simulated run changed the home directory:\n%s", strings.Join(diff, "\n"))
	}
	if diff := changes(cwd, snapshot(t, m.cwd)); len(diff) > 0 {
		t.Errorf("a simulated run changed the working directory:\n%s", strings.Join(diff, "\n"))
	}
	if got := o.changed(); len(got) > 0 {
		t.Errorf("a simulated run changed Ollama: %v", got)
	}
	if got := m.proxy.seen(); len(got) > 0 {
		t.Errorf("a simulated run tried to reach %v", got)
	}
}
