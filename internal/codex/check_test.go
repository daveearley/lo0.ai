package codex_test

import (
	"strings"
	"testing"

	"lo0.ai/lo0/internal/codex"
	"lo0.ai/lo0/internal/usage"
)

func TestCheck(t *testing.T) {
	if notes := codex.Check(usage.Format{Versions: []usage.Tally{{Name: codex.VerifiedVersion}}}); len(notes) != 0 {
		t.Errorf("Check(verified) = %q, want nothing", notes)
	}
	notes := codex.Check(usage.Format{Versions: []usage.Tally{{Name: "9.0.0"}}, UnknownTypes: []usage.Tally{{Name: "hologram"}}})
	if len(notes) != 2 || !strings.Contains(notes[0], "Codex 9.0.0, newer than") || !strings.Contains(notes[1], "(hologram)") {
		t.Errorf("Check = %q", notes)
	}
}
