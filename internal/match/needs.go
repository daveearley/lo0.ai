package match

import (
	"fmt"
	"slices"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/usage"
)

const (
	// longChain is the tool calls per request above which the 90th
	// percentile request counts as a long agent loop.
	longChain = 20
	// thinkingHeavy is the share of turns at high effort or above from which
	// a missing thinking mode is worth a mention.
	thinkingHeavy = 0.5
)

// needsFor compares a candidate with what the history shows the user relies
// on: images, extended thinking and long chains of tool calls.
func needsFor(c Candidate, p usage.Profile) Insights {
	var in Insights
	if n := p.Images.Sessions; n > 0 {
		if c.Model.Vision {
			in.Strengths = append(in.Strengths, fmt.Sprintf("Reads images, which %s of your sessions used", format.Int(n)))
		} else {
			in.Concerns = append(in.Concerns, fmt.Sprintf("Cannot read images: %s of your sessions had screenshots or pasted images", format.Int(n)))
		}
	}
	if share := p.Reasoning.HighEffortShare; share >= thinkingHeavy {
		switch thinks, known := thinking(c); {
		case known && thinks:
			in.Strengths = append(in.Strengths, fmt.Sprintf("Has a thinking mode for the %s of your turns you ran at high effort", format.Percent(share)))
		case known:
			in.Concerns = append(in.Concerns, fmt.Sprintf("No thinking mode, and %s of your turns asked Claude to think hard", format.Percent(share)))
		}
	}
	if chain := p.Requests.ToolCalls.P90; chain >= longChain && c.Model.Tier != catalog.TierA {
		in.Concerns = append(in.Concerns, fmt.Sprintf("1 in 10 of your requests chains %s or more tool calls; tier %s models are weaker on loops that long",
			format.Int(chain), c.Model.Tier))
	}
	return in
}

// thinking reports whether the installed build has a thinking mode, and
// whether that is known at all: only runtimes that list capabilities say.
func thinking(c Candidate) (thinks, known bool) {
	if c.Installed == nil || len(c.Installed.Capabilities) == 0 {
		return false, false
	}
	return slices.Contains(c.Installed.Capabilities, runtimes.CapabilityThinking), true
}
