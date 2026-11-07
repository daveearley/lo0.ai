// Package bench holds speeds measured on this machine: what lo0's benchmark
// records for a model, and how it is stored between runs. Measurements
// replace estimates for the model, build and runtime they were taken with.
package bench

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"lo0.ai/lo0/internal/runtimes"
)

// Point is one measurement at one context size, taken with nothing cached.
type Point struct {
	// Context is the tokens the runtime read before writing.
	Context int `json:"context"`
	// ReadTokensPerSecond is the average over reading the whole context,
	// and WriteTokensPerSecond the speed writing after it.
	ReadTokensPerSecond  float64 `json:"readTokensPerSecond"`
	WriteTokensPerSecond float64 `json:"writeTokensPerSecond"`
}

// Result is one model measured on this machine.
type Result struct {
	// Model is the catalogue ID, Quant its quantization and KV the cache
	// precision it ran with.
	Model          string      `json:"model"`
	Quant          string      `json:"quant"`
	KV             string      `json:"kv"`
	Runtime        runtimes.ID `json:"runtime"`
	RuntimeVersion string      `json:"runtimeVersion,omitzero"`
	// Device is the chip or card it ran on, so results from another
	// machine are never taken as this one's.
	Device string    `json:"device"`
	At     time.Time `json:"at"`
	Points []Point   `json:"points"`
}

// Same reports whether r and o measure the same model, build and runtime
// on the same device.
func (r Result) Same(o Result) bool {
	return r.Model == o.Model && r.Quant == o.Quant && r.KV == o.KV && r.Runtime == o.Runtime && r.Device == o.Device
}

type file struct {
	Results []Result `json:"results"`
}

// Decode reads stored results. Empty data has none.
func Decode(data []byte) ([]Result, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("reading benchmarks: %w", err)
	}
	return f.Results, nil
}

// Encode writes results for storing, newest last.
func Encode(results []Result) ([]byte, error) {
	sorted := slices.Clone(results)
	slices.SortStableFunc(sorted, func(a, b Result) int { return a.At.Compare(b.At) })
	data, err := json.MarshalIndent(file{Results: sorted}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Merge adds r to results, replacing an older measurement of the same
// model, build and runtime on the same device.
func Merge(results []Result, r Result) []Result {
	out := slices.DeleteFunc(slices.Clone(results), r.Same)
	return append(out, r)
}

// For returns the newest result for a model on a device, if any.
func For(results []Result, device, model, quant, kv string, rt runtimes.ID) (Result, bool) {
	var found Result
	ok := false
	for _, r := range results {
		if r.Device == device && r.Model == model && r.Quant == quant && r.KV == kv && r.Runtime == rt {
			if !ok || r.At.After(found.At) {
				found, ok = r, true
			}
		}
	}
	return found, ok
}

// OnDevice returns every result taken on device, newest first.
func OnDevice(results []Result, device string) []Result {
	var out []Result
	for _, r := range results {
		if r.Device == device {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(a, b Result) int { return cmp.Compare(b.At.Unix(), a.At.Unix()) })
	return out
}
