package theme_test

import (
	"testing"

	"lo0.ai/lo0/internal/ui/theme"
)

func TestPlainAddsNothing(t *testing.T) {
	if got := theme.Plain().Accent.Render("lo0"); got != "lo0" {
		t.Errorf("Plain accent = %q, want the text alone", got)
	}
}

func TestBackgroundsGetTheirOwnAccent(t *testing.T) {
	dark, light := theme.New(true).Accent.Render("lo0"), theme.New(false).Accent.Render("lo0")
	if dark == light || dark == "lo0" {
		t.Errorf("dark %q and light %q accents should be styled and differ", dark, light)
	}
}
