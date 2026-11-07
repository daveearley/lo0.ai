package match

import (
	"slices"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/machine"
)

func TestSimulateResolvesNames(t *testing.T) {
	scanned := machine.Profile{
		OS:     machine.OS{GOOS: "linux", Arch: "amd64", Platform: "ubuntu"},
		CPU:    machine.CPU{Model: "AMD Ryzen 9 7950X", PhysicalCores: 16},
		Memory: machine.Memory{TotalBytes: 64 << 30, AvailableBytes: 40 << 30},
		GPUs:   []machine.GPU{{Name: "NVIDIA GeForce RTX 3060", Vendor: machine.VendorNVIDIA, VRAMBytes: 12 << 30}},
		Disk:   machine.Disk{Path: "/", TotalBytes: 1 << 40, FreeBytes: 1 << 39},
	}
	tests := []struct {
		name      string
		query     HardwareQuery
		gpu       string
		count     int
		vram      uint64
		cores     int
		memory    uint64
		unified   bool
		bandwidth float64
	}{
		{"short card name", HardwareQuery{GPU: "RTX 4090"}, "NVIDIA GeForce RTX 4090", 1, 24 << 30, 0, 64 << 30, false, 1008},
		{"number alone skips variants", HardwareQuery{GPU: "4090"}, "NVIDIA GeForce RTX 4090", 1, 24 << 30, 0, 64 << 30, false, 1008},
		{"run together and lower case", HardwareQuery{GPU: "rtx4090d"}, "NVIDIA GeForce RTX 4090 D", 1, 24 << 30, 0, 64 << 30, false, 1008},
		{"full driver name", HardwareQuery{GPU: "NVIDIA GeForce RTX 5070 Ti"}, "NVIDIA GeForce RTX 5070 Ti", 1, 16 << 30, 0, 64 << 30, false, 896},
		{"two cards with more RAM", HardwareQuery{GPU: "RTX 3090", GPUs: 2, RAM: 128}, "NVIDIA GeForce RTX 3090", 2, 24 << 30, 0, 128 << 30, false, 936},
		{"card sold in two sizes", HardwareQuery{GPU: "RTX 4060 Ti", VRAM: 16}, "NVIDIA GeForce RTX 4060 Ti", 1, 16 << 30, 0, 64 << 30, false, 288},
		{"datacentre part", HardwareQuery{GPU: "A100 SXM4 80GB"}, "NVIDIA A100-SXM4-80GB", 1, 80 << 30, 0, 64 << 30, false, 2039},
		{"AMD card", HardwareQuery{GPU: "RX 7900 XTX"}, "AMD Radeon RX 7900 XTX", 1, 24 << 30, 0, 64 << 30, false, 960},
		{"full Mac chip", HardwareQuery{Mac: "M4 Max", Memory: 128}, "Apple M4 Max", 1, 0, 40, 128 << 30, true, 546},
		{"binned Mac chip", HardwareQuery{Mac: "Apple M4 Max (32-core GPU)", Memory: 36}, "Apple M4 Max", 1, 0, 32, 36 << 30, true, 410},
		{"lower case chip", HardwareQuery{Mac: "m5 pro", Memory: 48}, "Apple M5 Pro", 1, 0, 20, 48 << 30, true, 307},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sim, err := Simulate(tt.query)
			if err != nil {
				t.Fatal(err)
			}
			p := sim.Apply(scanned)
			if !p.Simulated {
				t.Error("profile not marked simulated")
			}
			if len(p.GPUs) != tt.count {
				t.Fatalf("%d GPUs, want %d", len(p.GPUs), tt.count)
			}
			g := p.GPUs[0]
			if g.Name != tt.gpu || g.VRAMBytes != tt.vram || g.Cores != tt.cores || g.Unified != tt.unified {
				t.Errorf("GPU = %+v, want %s with %d bytes and %d cores", g, tt.gpu, tt.vram, tt.cores)
			}
			if p.Memory.TotalBytes != tt.memory || p.Memory.Unified != tt.unified || p.Memory.AvailableBytes != 0 {
				t.Errorf("memory = %+v, want %d bytes", p.Memory, tt.memory)
			}
			if p.Disk != scanned.Disk {
				t.Errorf("disk = %+v, want the scanned disk", p.Disk)
			}
			if bw := MemoryBandwidth(p); bw.GBps != tt.bandwidth || bw.Estimated {
				t.Errorf("bandwidth = %+v, want %v GB/s from the table", bw, tt.bandwidth)
			}
		})
	}
}

func TestSimulateRefuses(t *testing.T) {
	tests := []struct {
		name  string
		query HardwareQuery
		want  []string
	}{
		{"unknown card names the closest", HardwareQuery{GPU: "RTX 4091"}, []string{`"RTX 4091"`, "RTX 4090"}},
		{"unknown chip names the closest", HardwareQuery{Mac: "M7 Max", Memory: 64}, []string{`"M7 Max"`, "M"}},
		{"ambiguous card lists the choices", HardwareQuery{GPU: "H100"}, []string{"H100 SXM", "H100 PCIe", "H100 NVL", "name one"}},
		{"card sold in several sizes", HardwareQuery{GPU: "RTX 5060 Ti"}, []string{"8 or 16 GB", "--vram"}},
		{"Mac without memory", HardwareQuery{Mac: "M4 Pro"}, []string{"--memory"}},
		{"Mac core count lo0 has no figures for", HardwareQuery{Mac: "M4 Max 36-core", Memory: 64}, []string{"32 or 40-core"}},
		{"both kinds", HardwareQuery{GPU: "RTX 4090", Mac: "M4 Max", Memory: 64}, []string{"not both"}},
		{"card sizes on a Mac", HardwareQuery{Mac: "M4 Max", Memory: 64, VRAM: 24}, []string{"--memory"}},
		{"Mac memory on a card", HardwareQuery{GPU: "RTX 4090", Memory: 64}, []string{"--ram"}},
		{"sizes without hardware", HardwareQuery{RAM: 64}, []string{"--gpu or --mac"}},
		{"negative", HardwareQuery{GPU: "RTX 4090", VRAM: -1}, []string{"negative"}},
		{"too many cards", HardwareQuery{GPU: "RTX 4090", GPUs: 9}, []string{"at most 8"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Simulate(tt.query)
			if err == nil {
				t.Fatal("Simulate succeeded")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q lacks %q", err, w)
				}
			}
		})
	}
}

func TestSimulatedCardOnAMacBecomesAPC(t *testing.T) {
	mac := machine.Profile{
		OS:     machine.OS{GOOS: "darwin", Arch: "arm64"},
		CPU:    machine.CPU{Model: "Apple M2"},
		Memory: machine.Memory{TotalBytes: 16 << 30, Unified: true},
		GPUs:   []machine.GPU{{Name: "Apple M2", Vendor: machine.VendorApple, Unified: true, Cores: 10}},
	}
	sim, err := Simulate(HardwareQuery{GPU: "RTX 5090", RAM: 96})
	if err != nil {
		t.Fatal(err)
	}
	p := sim.Apply(mac)
	if p.OS.GOOS != "linux" || p.CPU.Model != "" || p.Memory.Unified {
		t.Errorf("profile = %+v, want a Linux PC without the Mac's CPU or unified memory", p)
	}
	if mm := p.ModelMemory(); mm.Source != machine.MemoryVRAM || mm.Bytes != 32<<30 {
		t.Errorf("model memory = %+v, want the card's 32 GiB", mm)
	}
	if got := p.ExpertMemory(); got != 72<<30 {
		t.Errorf("expert memory = %d, want three quarters of 96 GiB", got)
	}
}

func TestEveryCardHasFigures(t *testing.T) {
	for _, c := range cards {
		q := HardwareQuery{GPU: c.short(), VRAM: c.vram[0]}
		sim, err := Simulate(q)
		if err != nil {
			t.Errorf("%s: %v", c.short(), err)
			continue
		}
		p := sim.Apply(machine.Profile{Memory: machine.Memory{TotalBytes: 64 << 30}})
		_, _, published := publishedFor(p)
		if bw := MemoryBandwidth(p); bw.Estimated && !published {
			t.Errorf("%s has no bandwidth figure or published result", c.name)
		}
	}
	for _, s := range appleBandwidth {
		sim, err := Simulate(HardwareQuery{Mac: s.match, Memory: 64})
		if err != nil {
			t.Errorf("%s: %v", s.match, err)
			continue
		}
		p := sim.Apply(machine.Profile{})
		if bw := MemoryBandwidth(p); bw.Estimated {
			t.Errorf("%s has no bandwidth figure", s.match)
		}
		if _, _, ok := publishedFor(p); !ok {
			t.Errorf("%s has no published result", s.match)
		}
	}
	for _, r := range cudaPublished {
		if !slices.ContainsFunc(cards, func(c card) bool { return isDevice(c.name, r.match) }) {
			t.Errorf("published result %q has no card to simulate", r.match)
		}
	}
}
