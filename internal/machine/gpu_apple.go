package machine

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"lo0.ai/lo0/internal/system"
)

// AppleGPUProbe reads GPUs from macOS system_profiler.
type AppleGPUProbe struct {
	Runner system.Runner
}

// GPUs implements GPUProbe.
func (p AppleGPUProbe) GPUs(ctx context.Context) ([]GPU, error) {
	out, err := p.Runner.Run(ctx, "system_profiler", "SPDisplaysDataType", "-json")
	if err != nil {
		return nil, err
	}
	return ParseSystemProfiler(out)
}

type systemProfilerReport struct {
	Displays []struct {
		Name        string `json:"_name"`
		Model       string `json:"sppci_model"`
		Vendor      string `json:"spdisplays_vendor"`
		Cores       string `json:"sppci_cores"`
		Metal       string `json:"spdisplays_mtlgpufamilysupport"`
		VRAM        string `json:"spdisplays_vram"`
		VRAMDynamic string `json:"spdisplays_vram_dynamic"`
	} `json:"SPDisplaysDataType"`
}

// ParseSystemProfiler parses the output of
// `system_profiler SPDisplaysDataType -json`.
func ParseSystemProfiler(data []byte) ([]GPU, error) {
	var report systemProfilerReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parsing system_profiler output: %w", err)
	}
	gpus := make([]GPU, 0, len(report.Displays))
	for _, d := range report.Displays {
		name := cmp.Or(d.Model, d.Name)
		g := GPU{
			Name:   name,
			Vendor: appleVendor(d.Vendor, name),
			API:    metalAPI(d.Metal),
		}
		g.Cores, _ = strconv.Atoi(d.Cores)
		g.Unified = g.Vendor == VendorApple
		if !g.Unified {
			g.VRAMBytes = parseAppleSize(cmp.Or(d.VRAM, d.VRAMDynamic))
		}
		gpus = append(gpus, g)
	}
	return gpus, nil
}

func appleVendor(raw, name string) Vendor {
	v := strings.ToLower(strings.TrimPrefix(raw, "sppci_vendor_"))
	switch {
	case strings.HasPrefix(v, "apple"):
		return VendorApple
	case strings.HasPrefix(v, "amd"), strings.HasPrefix(v, "ati"):
		return VendorAMD
	case strings.HasPrefix(v, "intel"):
		return VendorIntel
	case strings.HasPrefix(v, "nvidia"):
		return VendorNVIDIA
	default:
		return vendorFromName(name)
	}
}

func metalAPI(raw string) string {
	family, ok := strings.CutPrefix(raw, "spdisplays_metal")
	if !ok || family == "" {
		return ""
	}
	return "Metal " + family
}

// parseAppleSize parses sizes such as "8 GB" or "1536 MB". system_profiler
// reports video memory in binary units despite the decimal labels.
func parseAppleSize(s string) uint64 {
	fields := strings.Fields(s)
	if len(fields) != 2 {
		return 0
	}
	n, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0
	}
	switch strings.ToUpper(fields[1]) {
	case "GB":
		return n << 30
	case "MB":
		return n << 20
	default:
		return 0
	}
}
