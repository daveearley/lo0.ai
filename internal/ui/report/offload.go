package report

import (
	"strconv"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/match"
)

// OffloadLines describe the best option's experts in system memory, or are
// nil when it keeps none.
func OffloadLines(c match.Candidate, m match.Result) []string {
	off := c.Best.Offload
	if off.Layers == 0 {
		return nil
	}
	return []string{
		format.Bytes(uint64(off.Bytes)) + " of " + format.Bytes(m.ExpertMemory) + " · the experts of " +
			OffloadShare(c.Best, c) + " layers",
		"llama.cpp keeps them there: --n-cpu-moe " + strconv.Itoa(off.Leading),
		"speeds assume " + strconv.Itoa(match.HostGBps) + " GB/s memory and a " +
			strconv.Itoa(match.TransferGBps) + " GB/s link to the GPU",
	}
}

// OffloadShare is how many expert layers o keeps in system memory, such as
// "37 of 40".
func OffloadShare(o match.Option, c match.Candidate) string {
	if !o.Offloads() {
		return ""
	}
	return strconv.Itoa(o.Offload.Layers) + " of " + strconv.Itoa(c.Model.Experts.LayerCount())
}
