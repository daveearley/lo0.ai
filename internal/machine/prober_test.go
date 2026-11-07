package machine_test

import (
	"context"
	"errors"
	"testing"

	"lo0.ai/lo0/internal/machine"
)

type fakeHost struct {
	os      machine.OS
	cpu     machine.CPU
	mem     machine.Memory
	disk    machine.Disk
	diskErr error
}

func (f fakeHost) OS(context.Context) (machine.OS, error)         { return f.os, nil }
func (f fakeHost) CPU(context.Context) (machine.CPU, error)       { return f.cpu, nil }
func (f fakeHost) Memory(context.Context) (machine.Memory, error) { return f.mem, nil }
func (f fakeHost) Disk(_ context.Context, path string) (machine.Disk, error) {
	d := f.disk
	d.Path = path
	return d, f.diskErr
}

type fakeGPUs []machine.GPU

func (f fakeGPUs) GPUs(context.Context) ([]machine.GPU, error) { return f, nil }

func TestProberMarksUnifiedMemory(t *testing.T) {
	host := fakeHost{
		os:  machine.OS{GOOS: "darwin", Arch: "arm64"},
		mem: machine.Memory{TotalBytes: 64 << 30},
	}
	gpus := fakeGPUs{{Name: "Apple M5 Pro", Vendor: machine.VendorApple, Unified: true}}
	p, err := machine.NewProber(host, gpus, "/Users/test").Probe(t.Context())
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if !p.Memory.Unified {
		t.Error("Memory.Unified = false, want true for an Apple GPU")
	}
	if p.Disk.Path != "/Users/test" {
		t.Errorf("Disk.Path = %q, want /Users/test", p.Disk.Path)
	}
}

func TestProberReturnsPartialProfileOnError(t *testing.T) {
	boom := errors.New("statfs failed")
	host := fakeHost{cpu: machine.CPU{Model: "Ryzen 9"}, diskErr: boom}
	p, err := machine.NewProber(host, fakeGPUs(nil), "/").Probe(t.Context())
	if !errors.Is(err, boom) {
		t.Fatalf("Probe error = %v, want %v", err, boom)
	}
	if p.CPU.Model != "Ryzen 9" {
		t.Errorf("CPU.Model = %q, want the value probed before the failure", p.CPU.Model)
	}
}

func TestModelMemory(t *testing.T) {
	tests := []struct {
		name    string
		profile machine.Profile
		want    machine.ModelMemory
	}{
		{
			name:    "unified 64 GiB wires three quarters",
			profile: machine.Profile{Memory: machine.Memory{TotalBytes: 64 << 30, Unified: true}},
			want:    machine.ModelMemory{Bytes: 48 << 30, Source: machine.MemoryUnified},
		},
		{
			name:    "unified 24 GiB wires two thirds",
			profile: machine.Profile{Memory: machine.Memory{TotalBytes: 24 << 30, Unified: true}},
			want:    machine.ModelMemory{Bytes: 16 << 30, Source: machine.MemoryUnified},
		},
		{
			name: "discrete GPUs sum their VRAM",
			profile: machine.Profile{
				Memory: machine.Memory{TotalBytes: 128 << 30},
				GPUs:   []machine.GPU{{VRAMBytes: 24 << 30}, {VRAMBytes: 24 << 30}},
			},
			want: machine.ModelMemory{Bytes: 48 << 30, Source: machine.MemoryVRAM},
		},
		{
			name:    "no GPU runs on the CPU with three quarters of 64 GiB",
			profile: machine.Profile{Memory: machine.Memory{TotalBytes: 64 << 30}},
			want:    machine.ModelMemory{Bytes: 48 << 30, Source: machine.MemoryCPU},
		},
		{
			name:    "no GPU on an 8 GiB machine takes two thirds",
			profile: machine.Profile{Memory: machine.Memory{TotalBytes: 8 << 30}},
			want:    machine.ModelMemory{Bytes: 8 << 30 / 3 * 2, Source: machine.MemoryCPU},
		},
		{
			name: "integrated graphics with a small carve-out run on the CPU",
			profile: machine.Profile{
				Memory: machine.Memory{TotalBytes: 32 << 30},
				GPUs:   []machine.GPU{{Name: "Intel(R) Iris(R) Xe Graphics", Vendor: machine.VendorIntel, VRAMBytes: 128 << 20}},
			},
			want: machine.ModelMemory{Bytes: 32 << 30 / 3 * 2, Source: machine.MemoryCPU},
		},
		{
			name: "a card too small for models is left out",
			profile: machine.Profile{
				Memory: machine.Memory{TotalBytes: 32 << 30},
				GPUs:   []machine.GPU{{VRAMBytes: 512 << 20}, {VRAMBytes: 12 << 30}},
			},
			want: machine.ModelMemory{Bytes: 12 << 30, Source: machine.MemoryVRAM},
		},
		{
			name:    "memory unknown",
			profile: machine.Profile{},
			want:    machine.ModelMemory{Source: machine.MemoryNone},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.profile.ModelMemory(); got != tt.want {
				t.Errorf("ModelMemory() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSystemHostSmoke(t *testing.T) {
	host := machine.SystemHost{}
	mem, err := host.Memory(t.Context())
	if err != nil {
		t.Fatalf("Memory: %v", err)
	}
	if mem.TotalBytes == 0 {
		t.Error("Memory.TotalBytes = 0")
	}
	c, err := host.CPU(t.Context())
	if err != nil {
		t.Fatalf("CPU: %v", err)
	}
	if c.LogicalCores == 0 {
		t.Error("CPU.LogicalCores = 0")
	}
	if _, err := host.Disk(t.Context(), t.TempDir()); err != nil {
		t.Fatalf("Disk: %v", err)
	}
}

func TestExpertMemory(t *testing.T) {
	gpu := []machine.GPU{{VRAMBytes: 8 << 30}}
	tests := []struct {
		name    string
		profile machine.Profile
		want    uint64
	}{
		{"discrete GPU with 32 GiB of RAM", machine.Profile{Memory: machine.Memory{TotalBytes: 32 << 30}, GPUs: gpu}, 32 << 30 / 3 * 2},
		{"discrete GPU with 64 GiB of RAM", machine.Profile{Memory: machine.Memory{TotalBytes: 64 << 30}, GPUs: gpu}, 48 << 30},
		{"unified memory already holds the whole model", machine.Profile{Memory: machine.Memory{TotalBytes: 64 << 30, Unified: true}}, 0},
		{"no GPU", machine.Profile{Memory: machine.Memory{TotalBytes: 64 << 30}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.profile.ExpertMemory(); got != tt.want {
				t.Errorf("ExpertMemory() = %d, want %d", got, tt.want)
			}
		})
	}
}
