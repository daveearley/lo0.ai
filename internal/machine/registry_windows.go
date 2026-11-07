package machine

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// displayClass is the registry key Windows files display adapters under,
// one numbered subkey each.
const displayClass = `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`

// RegistryAdapters reads the display adapters from the registry. Subkeys
// that are not adapters, such as Properties, or that cannot be read are
// skipped.
func RegistryAdapters(ctx context.Context) ([]WindowsAdapter, error) {
	class, err := registry.OpenKey(registry.LOCAL_MACHINE, displayClass, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, fmt.Errorf("opening the display adapters' registry key: %w", err)
	}
	defer class.Close()
	names, err := class.ReadSubKeyNames(-1)
	if err != nil {
		return nil, fmt.Errorf("listing display adapters: %w", err)
	}
	var adapters []WindowsAdapter
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return adapters, err
		}
		if !strings.HasPrefix(name, "0") {
			continue
		}
		k, err := registry.OpenKey(class, name, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		desc, _, err := k.GetStringValue("DriverDesc")
		if err == nil {
			// Adapters without the value, such as virtual ones, have no
			// memory of their own to report.
			size, _, _ := k.GetIntegerValue("HardwareInformation.qwMemorySize")
			adapters = append(adapters, WindowsAdapter{DriverDesc: desc, MemorySize: size})
		}
		_ = k.Close()
	}
	return adapters, nil
}
