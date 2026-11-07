package machine

import (
	"context"
	"strings"
)

// WindowsAdapter is one display adapter's entry in the Windows registry,
// under the display class key.
type WindowsAdapter struct {
	DriverDesc string `json:"DriverDesc"`
	// MemorySize is HardwareInformation.qwMemorySize, which, unlike WMI's
	// AdapterRAM, reports video memory above 4 GiB correctly.
	MemorySize uint64 `json:"HardwareInformation.qwMemorySize"`
}

// WindowsGPUProbe reads display adapters from the Windows registry.
type WindowsGPUProbe struct {
	// Adapters lists the registry's display adapters. RegistryAdapters is
	// the real one; it reads the registry in process because PowerShell,
	// the usual way to query it, writes caches into the user's profile.
	Adapters func(ctx context.Context) ([]WindowsAdapter, error)
}

// GPUs implements GPUProbe.
func (p WindowsGPUProbe) GPUs(ctx context.Context) ([]GPU, error) {
	adapters, err := p.Adapters(ctx)
	if err != nil {
		return nil, err
	}
	return WindowsGPUs(adapters), nil
}

// WindowsGPUs turns registry adapters into GPUs, leaving out virtual ones
// such as the basic display and remote display adapters.
func WindowsGPUs(adapters []WindowsAdapter) []GPU {
	var gpus []GPU
	for _, a := range adapters {
		if strings.TrimSpace(a.DriverDesc) == "" || isVirtualAdapter(a.DriverDesc) {
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
	return gpus
}
