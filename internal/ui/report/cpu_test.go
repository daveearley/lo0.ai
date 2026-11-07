package report_test

import (
	"slices"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/ui/report"
)

// A machine without a GPU for models says so where the machine is shown,
// so nobody reads its memory as a GPU's.
func TestMachineWithoutAGPU(t *testing.T) {
	tests := map[string]struct {
		gpus []machine.GPU
		want []string
	}{
		"no GPU": {nil, []string{"none found, so models run on the CPU", "48 GiB usable by models on the CPU"}},
		"integrated graphics": {
			[]machine.GPU{{Name: "Intel Iris Xe Graphics", Vendor: machine.VendorIntel, VRAMBytes: 128 << 20}},
			[]string{"Intel Iris Xe Graphics", "too small for models", "48 GiB usable by models on the CPU"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			p := machine.Profile{CPU: machine.CPU{Model: "Intel Core Ultra 7 155H"}, Memory: machine.Memory{TotalBytes: 64 << 30}, GPUs: tt.gpus}
			out := report.Render(scan.Result{Machine: p}, plain(80))
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("report lacks %q:\n%s", want, out)
				}
			}
			chips := report.MachineChips(p, p.ModelMemory())
			if want := []string{"no GPU", "Intel Core Ultra 7 155H", "48 GiB for models on the CPU"}; !slices.Equal(chips, want) {
				t.Errorf("MachineChips = %q, want %q", chips, want)
			}
		})
	}
}
