package codex

import (
	"fmt"
	"strings"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/usage"
	"lo0.ai/lo0/internal/version"
)

// Check says what could make lo0's reading of Codex history incomplete: a
// version newer than the one lo0 was checked against, or record types it
// does not know.
func Check(history usage.Format) []string {
	var notes []string
	if newest := history.Newest(); version.Compare(newest, VerifiedVersion) > 0 {
		notes = append(notes, fmt.Sprintf("You use Codex %s, newer than the %s lo0 was checked against. If its session logs changed, some of your Codex history may be missing here.",
			newest, VerifiedVersion))
	}
	if len(history.UnknownTypes) > 0 {
		names := make([]string, 0, 3)
		for _, t := range history.UnknownTypes[:min(len(history.UnknownTypes), 3)] {
			names = append(names, t.Name)
		}
		notes = append(notes, fmt.Sprintf("Your Codex logs have %s lo0 does not know (%s), so newer Codex features may not be counted.",
			format.Plural(len(history.UnknownTypes), "record type", "record types"), strings.Join(names, ", ")))
	}
	return notes
}
