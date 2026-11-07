package machine

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode"

	"lo0.ai/lo0/internal/system"
)

// GPUProbes runs several probes and merges their results. A probe whose tool
// is not installed is skipped silently. When two probes report the same card,
// the earlier probe wins, so list the most precise source first.
type GPUProbes []GPUProbe

var _ GPUProbe = GPUProbes(nil)

// GPUs implements GPUProbe.
func (ps GPUProbes) GPUs(ctx context.Context) ([]GPU, error) {
	var (
		all  []GPU
		errs []error
	)
	for _, p := range ps {
		gpus, err := p.GPUs(ctx)
		if err != nil && !errors.Is(err, system.ErrNotFound) {
			errs = append(errs, err)
		}
		for _, g := range gpus {
			if !slices.ContainsFunc(all, func(seen GPU) bool { return sameCard(seen, g) }) {
				all = append(all, g)
			}
		}
	}
	return all, errors.Join(errs...)
}

func sameCard(a, b GPU) bool {
	return strings.EqualFold(strings.TrimSpace(a.Name), strings.TrimSpace(b.Name))
}

// DefaultGPUProbes returns the probes that make sense on goos, most precise
// first.
func DefaultGPUProbes(goos string, r system.Runner) GPUProbes {
	switch goos {
	case "darwin":
		return GPUProbes{AppleGPUProbe{Runner: r}}
	case "windows":
		return GPUProbes{NVIDIAProbe{Runner: r}, WindowsGPUProbe{Adapters: RegistryAdapters}}
	default:
		return GPUProbes{NVIDIAProbe{Runner: r}, AMDProbe{Runner: r}}
	}
}

func vendorFromName(name string) Vendor {
	words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, w := range words {
		switch w {
		case "nvidia", "geforce", "quadro", "tesla":
			return VendorNVIDIA
		case "amd", "radeon", "ati":
			return VendorAMD
		case "intel":
			return VendorIntel
		case "apple":
			return VendorApple
		}
	}
	return VendorUnknown
}
