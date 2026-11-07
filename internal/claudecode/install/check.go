package install

import (
	"fmt"
	"strings"

	"lo0.ai/lo0/internal/claudecode"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/usage"
	"lo0.ai/lo0/internal/version"
)

// missingUsageShare is the share of responses without token counts above
// which the history's numbers can no longer be trusted.
const missingUsageShare = 0.1

// Newest is the newest Claude Code version found anywhere: installed or in
// the history.
func Newest(in Installs, history usage.Format) string {
	newest := ""
	for _, v := range []string{in.CLI, in.App, history.Newest()} {
		if version.Compare(v, newest) > 0 {
			newest = v
		}
	}
	return newest
}

// Check says what could make lo0's reading of Claude Code incomplete: a
// version newer than the one lo0 was checked against, signs that the
// transcript format changed.
func Check(in Installs, history usage.Format) []string {
	var notes []string
	if newest := Newest(in, history); version.Compare(newest, claudecode.VerifiedVersion) > 0 {
		notes = append(notes, fmt.Sprintf("You use Claude Code %s, newer than the %s lo0 was checked against. If it moved or renamed its files, some of your setup or history may be missing here.",
			newest, claudecode.VerifiedVersion))
	}
	if r := history.Responses; r > 0 && float64(history.WithoutUsage)/float64(r) > missingUsageShare {
		notes = append(notes, fmt.Sprintf("%s of %s responses have no token counts, so costs and context sizes here are too low. Claude Code may have changed how it records them.",
			format.Int(history.WithoutUsage), format.Int(r)))
	}
	if len(history.UnknownTypes) > 0 {
		names := make([]string, 0, len(history.UnknownTypes))
		for _, t := range history.UnknownTypes[:min(len(history.UnknownTypes), 3)] {
			names = append(names, t.Name)
		}
		notes = append(notes, fmt.Sprintf("Your transcripts have %s lo0 does not know (%s), so newer Claude Code features may not be counted.",
			format.Plural(len(history.UnknownTypes), "record type", "record types"), strings.Join(names, ", ")))
	}
	return notes
}
