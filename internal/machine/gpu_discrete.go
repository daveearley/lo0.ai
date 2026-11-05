package machine

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"

	"lo0.ai/lo0/internal/system"
)

// NVIDIAProbe reads GPUs from nvidia-smi.
type NVIDIAProbe struct {
	Runner system.Runner
}

// GPUs implements GPUProbe.
func (p NVIDIAProbe) GPUs(ctx context.Context) ([]GPU, error) {
	out, err := p.Runner.Run(ctx, "nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits")
	if err != nil {
		return nil, err
	}
	return ParseNVIDIASMI(out)
}

// ParseNVIDIASMI parses `nvidia-smi --query-gpu=name,memory.total
// --format=csv,noheader,nounits`, which reports memory in MiB.
func ParseNVIDIASMI(data []byte) ([]GPU, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = 2
	var gpus []GPU
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			return gpus, nil
		}
		if err != nil {
			return gpus, fmt.Errorf("parsing nvidia-smi output: %w", err)
		}
		mib, err := strconv.ParseUint(strings.TrimSpace(rec[1]), 10, 64)
		if err != nil {
			return gpus, fmt.Errorf("parsing nvidia-smi memory %q: %w", rec[1], err)
		}
		gpus = append(gpus, GPU{
			Name:      strings.TrimSpace(rec[0]),
			Vendor:    VendorNVIDIA,
			VRAMBytes: mib << 20,
			API:       "CUDA",
		})
	}
}

// AMDProbe reads GPUs from rocm-smi.
type AMDProbe struct {
	Runner system.Runner
}

// GPUs implements GPUProbe.
func (p AMDProbe) GPUs(ctx context.Context) ([]GPU, error) {
	out, err := p.Runner.Run(ctx, "rocm-smi", "--showproductname", "--showmeminfo", "vram", "--json")
	if err != nil {
		return nil, err
	}
	return ParseROCmSMI(out)
}

// ParseROCmSMI parses `rocm-smi --showproductname --showmeminfo vram --json`.
// Field capitalisation differs between ROCm releases, so keys are matched
// case-insensitively.
func ParseROCmSMI(data []byte) ([]GPU, error) {
	var cards map[string]map[string]string
	if err := json.Unmarshal(data, &cards); err != nil {
		return nil, fmt.Errorf("parsing rocm-smi output: %w", err)
	}
	var gpus []GPU
	for _, id := range slices.Sorted(maps.Keys(cards)) {
		if !strings.HasPrefix(id, "card") {
			continue
		}
		fields := make(map[string]string, len(cards[id]))
		for k, v := range cards[id] {
			fields[strings.ToLower(k)] = v
		}
		vram, _ := strconv.ParseUint(fields["vram total memory (b)"], 10, 64)
		gpus = append(gpus, GPU{
			Name:      strings.TrimSpace(firstNonBlank(fields["card series"], fields["card model"], id)),
			Vendor:    VendorAMD,
			VRAMBytes: vram,
			API:       "ROCm",
		})
	}
	return gpus, nil
}

// WindowsGPUProbe reads display adapters from the Windows registry, which,
// unlike WMI's AdapterRAM, reports video memory above 4 GiB correctly.
type WindowsGPUProbe struct {
	Runner system.Runner
}

const windowsAdapterQuery = `Get-ItemProperty -Path 'HKLM:\SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}\0*' -ErrorAction SilentlyContinue | Select-Object DriverDesc,'HardwareInformation.qwMemorySize' | ConvertTo-Json -Compress`

// GPUs implements GPUProbe.
func (p WindowsGPUProbe) GPUs(ctx context.Context) ([]GPU, error) {
	out, err := p.Runner.Run(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", windowsAdapterQuery)
	if err != nil {
		return nil, err
	}
	return ParseWindowsAdapters(out)
}

type windowsAdapter struct {
	DriverDesc string `json:"DriverDesc"`
	MemorySize uint64 `json:"HardwareInformation.qwMemorySize"`
}

// ParseWindowsAdapters parses the JSON that windowsAdapterQuery prints.
// ConvertTo-Json emits a bare object for one adapter and an array for more.
func ParseWindowsAdapters(data []byte) ([]GPU, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil
	}
	var adapters []windowsAdapter
	if data[0] == '{' {
		var one windowsAdapter
		if err := json.Unmarshal(data, &one); err != nil {
			return nil, fmt.Errorf("parsing display adapters: %w", err)
		}
		adapters = append(adapters, one)
	} else if err := json.Unmarshal(data, &adapters); err != nil {
		return nil, fmt.Errorf("parsing display adapters: %w", err)
	}
	var gpus []GPU
	for _, a := range adapters {
		if a.DriverDesc == "" || isVirtualAdapter(a.DriverDesc) {
			continue
		}
		vendor := vendorFromName(a.DriverDesc)
		g := GPU{Name: a.DriverDesc, Vendor: vendor, VRAMBytes: a.MemorySize}
		switch vendor {
		case VendorNVIDIA:
			g.API = "CUDA"
		case VendorAMD, VendorIntel:
			g.API = "Vulkan"
		default:
		}
		gpus = append(gpus, g)
	}
	return gpus, nil
}

func isVirtualAdapter(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "basic display") ||
		strings.Contains(lower, "remote display") ||
		strings.Contains(lower, "virtual")
}

func firstNonBlank(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
