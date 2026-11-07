package machine_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/system"
	"lo0.ai/lo0/internal/system/systemtest"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return data
}

func TestParseSystemProfiler(t *testing.T) {
	tests := []struct {
		fixture string
		want    []machine.GPU
	}{
		{
			fixture: "system_profiler_m5.json",
			want: []machine.GPU{
				{Name: "Apple M5 Pro", Vendor: machine.VendorApple, Cores: 20, API: "Metal 4", Unified: true},
			},
		},
		{
			fixture: "system_profiler_intel_amd.json",
			want: []machine.GPU{
				{Name: "Intel UHD Graphics 630", Vendor: machine.VendorIntel},
				{Name: "AMD Radeon Pro 5500M", Vendor: machine.VendorAMD, VRAMBytes: 8 << 30, API: "Metal 3"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			got, err := machine.ParseSystemProfiler(readFixture(t, tt.fixture))
			if err != nil {
				t.Fatalf("ParseSystemProfiler: %v", err)
			}
			assertGPUs(t, got, tt.want)
		})
	}
}

func TestParseNVIDIASMI(t *testing.T) {
	got, err := machine.ParseNVIDIASMI(readFixture(t, "nvidia_smi.csv"))
	if err != nil {
		t.Fatalf("ParseNVIDIASMI: %v", err)
	}
	assertGPUs(t, got, []machine.GPU{
		{Name: "NVIDIA GeForce RTX 4090", Vendor: machine.VendorNVIDIA, VRAMBytes: 24564 << 20, API: "CUDA"},
		{Name: "NVIDIA RTX A6000", Vendor: machine.VendorNVIDIA, VRAMBytes: 49140 << 20, API: "CUDA"},
	})
}

func TestParseNVIDIASMIRejectsMalformedMemory(t *testing.T) {
	if _, err := machine.ParseNVIDIASMI([]byte("RTX 4090, [N/A]\n")); err == nil {
		t.Fatal("ParseNVIDIASMI succeeded on malformed memory")
	}
}

func TestParseROCmSMI(t *testing.T) {
	got, err := machine.ParseROCmSMI(readFixture(t, "rocm_smi.json"))
	if err != nil {
		t.Fatalf("ParseROCmSMI: %v", err)
	}
	assertGPUs(t, got, []machine.GPU{
		{Name: "Navi 31 [Radeon RX 7900 XTX]", Vendor: machine.VendorAMD, VRAMBytes: 25753026560, API: "ROCm"},
	})
}

func TestWindowsGPUs(t *testing.T) {
	var adapters []machine.WindowsAdapter
	if err := json.Unmarshal(readFixture(t, "windows_adapters.json"), &adapters); err != nil {
		t.Fatal(err)
	}
	assertGPUs(t, machine.WindowsGPUs(adapters), []machine.GPU{
		{Name: "NVIDIA GeForce RTX 4080", Vendor: machine.VendorNVIDIA, VRAMBytes: 17171480576, API: "CUDA"},
		{Name: "Intel(R) UHD Graphics 770", Vendor: machine.VendorIntel, VRAMBytes: 134217728, API: "Vulkan"},
	})
	if got := machine.WindowsGPUs(nil); len(got) != 0 {
		t.Errorf("WindowsGPUs(nil) = %+v", got)
	}
}

func TestGPUProbesMergesAndSkipsMissingTools(t *testing.T) {
	runner := &systemtest.Runner{Commands: map[string]systemtest.Result{
		"nvidia-smi --query-gpu=name,memory.total --format=csv,noheader,nounits": {Output: "NVIDIA GeForce RTX 4080, 16376\n"},
	}}
	var adapters []machine.WindowsAdapter
	if err := json.Unmarshal(readFixture(t, "windows_adapters.json"), &adapters); err != nil {
		t.Fatal(err)
	}
	probes := machine.GPUProbes{machine.NVIDIAProbe{Runner: runner}, machine.WindowsGPUProbe{Adapters: func(context.Context) ([]machine.WindowsAdapter, error) { return adapters, nil }}}
	got, err := probes.GPUs(t.Context())
	if err != nil {
		t.Fatalf("GPUs: %v", err)
	}
	assertGPUs(t, got, []machine.GPU{
		{Name: "NVIDIA GeForce RTX 4080", Vendor: machine.VendorNVIDIA, VRAMBytes: 16376 << 20, API: "CUDA"},
		{Name: "Intel(R) UHD Graphics 770", Vendor: machine.VendorIntel, VRAMBytes: 134217728, API: "Vulkan"},
	})
}

func TestGPUProbesReportsToolFailures(t *testing.T) {
	boom := errors.New("driver mismatch")
	runner := &systemtest.Runner{Commands: map[string]systemtest.Result{
		"nvidia-smi --query-gpu=name,memory.total --format=csv,noheader,nounits": {Err: boom},
	}}
	if _, err := machine.DefaultGPUProbes("linux", runner).GPUs(t.Context()); !errors.Is(err, boom) {
		t.Fatalf("GPUs error = %v, want %v", err, boom)
	}
}

func assertGPUs(t *testing.T, got, want []machine.GPU) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d GPUs %+v, want %d %+v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("GPU %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// RegistryAdapters reads the real registry on Windows, where CI runs this
// test natively, and reports the tool as missing everywhere else.
func TestRegistryAdapters(t *testing.T) {
	adapters, err := machine.RegistryAdapters(t.Context())
	if runtime.GOOS != "windows" {
		if !errors.Is(err, system.ErrNotFound) {
			t.Errorf("RegistryAdapters on %s = %v, want ErrNotFound", runtime.GOOS, err)
		}
		return
	}
	if err != nil {
		t.Fatalf("RegistryAdapters: %v", err)
	}
	for _, a := range adapters {
		if a.DriverDesc == "" {
			t.Errorf("adapter without a name: %+v", a)
		}
	}
}
