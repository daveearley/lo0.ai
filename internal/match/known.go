package match

import "slices"

// KnownGPU is a graphics card Simulate accepts, with the sizes it is sold in.
type KnownGPU struct {
	Name string `json:"name"`
	VRAM []int  `json:"vram"`
}

// KnownMac is an Apple chip Simulate accepts, with its known GPU core
// counts, fewest first.
type KnownMac struct {
	Chip  string `json:"chip"`
	Cores []int  `json:"cores,omitzero"`
}

// KnownGPUs lists the cards Simulate accepts, by the names it accepts them
// under, in the order lo0 lists them.
func KnownGPUs() []KnownGPU {
	out := make([]KnownGPU, len(cards))
	for i, c := range cards {
		out[i] = KnownGPU{Name: c.short(), VRAM: slices.Clone(c.vram)}
	}
	return out
}

// KnownMacs lists the Apple chips Simulate accepts, newest first.
func KnownMacs() []KnownMac {
	var out []KnownMac
	for _, s := range appleBandwidth {
		if !slices.ContainsFunc(out, func(m KnownMac) bool { return m.Chip == s.match }) {
			out = append(out, KnownMac{Chip: s.match, Cores: chipCores(s.match)})
		}
	}
	return out
}
