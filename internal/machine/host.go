package machine

import (
	"context"
	"fmt"
	"runtime"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

// SystemHost is a HostSource backed by gopsutil, which supports macOS, Linux
// and Windows.
type SystemHost struct{}

var _ HostSource = SystemHost{}

// OS implements HostSource.
func (SystemHost) OS(ctx context.Context) (OS, error) {
	info := OS{GOOS: runtime.GOOS, Arch: runtime.GOARCH}
	platform, _, version, err := host.PlatformInformationWithContext(ctx)
	if err != nil {
		return info, fmt.Errorf("reading platform: %w", err)
	}
	info.Platform, info.Version = platform, version
	return info, nil
}

// CPU implements HostSource.
func (SystemHost) CPU(ctx context.Context) (CPU, error) {
	var c CPU
	infos, err := cpu.InfoWithContext(ctx)
	if err != nil {
		return c, fmt.Errorf("reading cpu info: %w", err)
	}
	if len(infos) > 0 {
		c.Model = infos[0].ModelName
	}
	if c.PhysicalCores, err = cpu.CountsWithContext(ctx, false); err != nil {
		return c, fmt.Errorf("counting physical cores: %w", err)
	}
	if c.LogicalCores, err = cpu.CountsWithContext(ctx, true); err != nil {
		return c, fmt.Errorf("counting logical cores: %w", err)
	}
	return c, nil
}

// Memory implements HostSource.
func (SystemHost) Memory(ctx context.Context) (Memory, error) {
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return Memory{}, fmt.Errorf("reading memory: %w", err)
	}
	return Memory{TotalBytes: vm.Total, AvailableBytes: vm.Available}, nil
}

// Disk implements HostSource.
func (SystemHost) Disk(ctx context.Context, path string) (Disk, error) {
	u, err := disk.UsageWithContext(ctx, path)
	if err != nil {
		return Disk{Path: path}, fmt.Errorf("reading disk usage of %s: %w", path, err)
	}
	return Disk{Path: path, TotalBytes: u.Total, FreeBytes: u.Free}, nil
}
