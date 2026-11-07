//go:build !windows

package machine

import (
	"context"

	"lo0.ai/lo0/internal/system"
)

// RegistryAdapters reads the display adapters from the Windows registry,
// which other systems do not have, so it reports the tool as not found and
// GPUProbes skips it.
func RegistryAdapters(context.Context) ([]WindowsAdapter, error) {
	return nil, system.ErrNotFound
}
