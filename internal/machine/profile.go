// Package machine describes the hardware lo0 is running on: the operating
// system, CPU, memory, GPUs and free disk space.
package machine

// Profile is a snapshot of the host hardware.
type Profile struct {
	OS     OS     `json:"os"`
	CPU    CPU    `json:"cpu"`
	Memory Memory `json:"memory"`
	GPUs   []GPU  `json:"gpus,omitzero"`
	Disk   Disk   `json:"disk,omitzero"`
	// Simulated is true when the user named the hardware with --gpu or --mac.
	Simulated bool `json:"simulated,omitzero"`
}

// OS identifies the operating system.
type OS struct {
	// GOOS is the Go name of the operating system, such as "darwin".
	GOOS string `json:"goos"`
	// Arch is the Go name of the CPU architecture, such as "arm64".
	Arch string `json:"arch"`
	// Platform is the distribution or product name reported by the OS.
	Platform string `json:"platform,omitzero"`
	// Version is the OS release version.
	Version string `json:"version,omitzero"`
}

// CPU describes the processor.
type CPU struct {
	Model         string `json:"model,omitzero"`
	PhysicalCores int    `json:"physicalCores,omitzero"`
	LogicalCores  int    `json:"logicalCores,omitzero"`
}

// Memory describes system RAM.
type Memory struct {
	TotalBytes     uint64 `json:"totalBytes"`
	AvailableBytes uint64 `json:"availableBytes,omitzero"`
	// Unified is true when the CPU and GPU share one pool of memory, as on
	// Apple Silicon.
	Unified bool `json:"unified,omitzero"`
}

// GPU describes one graphics processor.
type GPU struct {
	Name   string `json:"name"`
	Vendor Vendor `json:"vendor"`
	// VRAMBytes is dedicated video memory. It is zero for unified-memory GPUs,
	// whose budget comes from Memory.
	VRAMBytes uint64 `json:"vramBytes,omitzero"`
	Cores     int    `json:"cores,omitzero"`
	// API is the compute API models run on, such as "Metal 4" or "CUDA".
	API     string `json:"api,omitzero"`
	Unified bool   `json:"unified,omitzero"`
}

// Vendor is a GPU manufacturer.
type Vendor string

// Known GPU vendors.
const (
	VendorApple   Vendor = "apple"
	VendorNVIDIA  Vendor = "nvidia"
	VendorAMD     Vendor = "amd"
	VendorIntel   Vendor = "intel"
	VendorUnknown Vendor = "unknown"
)

// Disk describes the volume that holds model files.
type Disk struct {
	Path       string `json:"path"`
	TotalBytes uint64 `json:"totalBytes"`
	FreeBytes  uint64 `json:"freeBytes"`
}

// MemorySource says where the model memory budget comes from.
type MemorySource string

// Memory budget sources.
const (
	MemoryUnified MemorySource = "unified"
	MemoryVRAM    MemorySource = "vram"
	// MemoryCPU means no GPU can hold a model, so models run on the CPU
	// from system memory.
	MemoryCPU  MemorySource = "cpu"
	MemoryNone MemorySource = "none"
)

// ModelMemory is how much memory a local model can use for its weights and
// KV cache, on the GPU or, without one that can hold a model, on the CPU.
type ModelMemory struct {
	Bytes  uint64       `json:"bytes"`
	Source MemorySource `json:"source"`
}

// unifiedLargeThreshold is the RAM size above which macOS lets the GPU wire
// about three quarters of memory rather than about two thirds. These match
// the recommendedMaxWorkingSetSize Metal reports on Apple Silicon.
const unifiedLargeThreshold = 36 << 30

// MinModelVRAM is the least video memory a card needs to hold any coding
// model. Integrated graphics report a small carve-out of system memory,
// often 128 MiB, which would otherwise read as a GPU with no room at all.
const MinModelVRAM = 2 << 30

// CanHoldModels reports whether models can run on g: it shares memory with
// the CPU, as Apple Silicon does, or has at least MinModelVRAM of its own.
func (g GPU) CanHoldModels() bool {
	return g.Unified || g.VRAMBytes >= MinModelVRAM
}

// ModelMemory returns the memory budget for models. On unified memory that
// is the share macOS lets the GPU wire; on discrete GPUs it is the total
// VRAM across cards that can hold a model. Without such a GPU, models run on
// the CPU from system memory, and take the same share of it macOS gives
// unified memory, leaving the rest for the system and the user's other apps.
func (p Profile) ModelMemory() ModelMemory {
	total := p.Memory.TotalBytes
	if p.Memory.Unified {
		return ModelMemory{Bytes: systemShare(total), Source: MemoryUnified}
	}
	var vram uint64
	for _, g := range p.GPUs {
		if g.CanHoldModels() {
			vram += g.VRAMBytes
		}
	}
	if vram > 0 {
		return ModelMemory{Bytes: vram, Source: MemoryVRAM}
	}
	if total == 0 {
		return ModelMemory{Source: MemoryNone}
	}
	return ModelMemory{Bytes: systemShare(total), Source: MemoryCPU}
}

// ExpertMemory is the system memory MoE experts may take beside a discrete
// GPU: the share a machine without one gives models. Zero elsewhere.
func (p Profile) ExpertMemory() uint64 {
	if p.ModelMemory().Source != MemoryVRAM {
		return 0
	}
	return systemShare(p.Memory.TotalBytes)
}

// systemShare is the part of total system memory models can take: about
// two thirds, or three quarters above unifiedLargeThreshold.
func systemShare(total uint64) uint64 {
	if total > unifiedLargeThreshold {
		return total / 4 * 3
	}
	return total / 3 * 2
}
