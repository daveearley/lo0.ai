package machine

import (
	"context"
	"errors"
	"fmt"
)

// HostSource reports the parts of the profile that the operating system
// exposes directly.
type HostSource interface {
	OS(ctx context.Context) (OS, error)
	CPU(ctx context.Context) (CPU, error)
	Memory(ctx context.Context) (Memory, error)
	Disk(ctx context.Context, path string) (Disk, error)
}

// GPUProbe discovers GPUs.
type GPUProbe interface {
	GPUs(ctx context.Context) ([]GPU, error)
}

// Prober builds a Profile from a HostSource and a GPUProbe.
type Prober struct {
	host     HostSource
	gpus     GPUProbe
	diskPath string
}

// NewProber returns a Prober that measures free space on the volume holding
// diskPath.
func NewProber(host HostSource, gpus GPUProbe, diskPath string) *Prober {
	return &Prober{host: host, gpus: gpus, diskPath: diskPath}
}

// Probe returns everything it could detect. A non-nil error describes the
// parts it could not; the returned Profile is still usable.
func (p *Prober) Probe(ctx context.Context) (Profile, error) {
	var (
		profile Profile
		errs    []error
		err     error
	)
	if profile.OS, err = p.host.OS(ctx); err != nil {
		errs = append(errs, fmt.Errorf("operating system: %w", err))
	}
	if profile.CPU, err = p.host.CPU(ctx); err != nil {
		errs = append(errs, fmt.Errorf("cpu: %w", err))
	}
	if profile.Memory, err = p.host.Memory(ctx); err != nil {
		errs = append(errs, fmt.Errorf("memory: %w", err))
	}
	if profile.Disk, err = p.host.Disk(ctx, p.diskPath); err != nil {
		errs = append(errs, fmt.Errorf("disk: %w", err))
	}
	if profile.GPUs, err = p.gpus.GPUs(ctx); err != nil {
		errs = append(errs, fmt.Errorf("gpu: %w", err))
	}
	for _, g := range profile.GPUs {
		if g.Unified {
			profile.Memory.Unified = true
		}
	}
	return profile, errors.Join(errs...)
}
