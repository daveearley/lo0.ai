package machine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"lo0.ai/lo0/internal/system"
)

// ErrNoSwapCount means lo0 cannot read how much this system has swapped.
var ErrNoSwapCount = errors.New("lo0 cannot read swap activity on this system")

// linuxPageSize is the page size /proc/vmstat counts in on every Linux
// architecture lo0 supports.
const linuxPageSize = 4096

// SwapMeter reads how much the system has moved to swap since it started,
// so a measurement can tell whether it ran under memory pressure.
type SwapMeter struct {
	GOOS   string
	Runner system.Runner
	// Root is the file system root, for /proc on Linux.
	Root fs.FS
}

// SwappedOut is the bytes moved to swap since the system started.
func (s SwapMeter) SwappedOut(ctx context.Context) (uint64, error) {
	switch s.GOOS {
	case "darwin":
		out, err := s.Runner.Run(ctx, "vm_stat")
		if err != nil {
			return 0, fmt.Errorf("reading vm_stat: %w", err)
		}
		return darwinSwapouts(string(out))
	case "linux":
		data, err := fs.ReadFile(s.Root, "proc/vmstat")
		if err != nil {
			return 0, fmt.Errorf("reading /proc/vmstat: %w", err)
		}
		pages, ok := field(string(data), "pswpout ")
		if !ok {
			return 0, ErrNoSwapCount
		}
		return pages * linuxPageSize, nil
	default:
		return 0, ErrNoSwapCount
	}
}

// darwinSwapouts reads vm_stat's swapouts, counted in pages of the size
// its first line gives.
func darwinSwapouts(out string) (uint64, error) {
	page := uint64(4096)
	if _, rest, ok := strings.Cut(out, "page size of "); ok {
		if n, err := strconv.ParseUint(strings.Fields(rest)[0], 10, 64); err == nil {
			page = n
		}
	}
	pages, ok := field(out, "Swapouts:")
	if !ok {
		return 0, ErrNoSwapCount
	}
	return pages * page, nil
}

// field finds the number after name at the start of a line.
func field(text, name string) (uint64, bool) {
	for line := range strings.Lines(text) {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), name)
		if !ok {
			continue
		}
		n, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(rest), "."), 10, 64)
		return n, err == nil
	}
	return 0, false
}
