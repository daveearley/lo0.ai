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
			name:    "no GPU",
			profile: machine.Profile{Memory: machine.Memory{TotalBytes: 32 << 30}},
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
