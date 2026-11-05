package codex_test

import (
	"path/filepath"
	"testing"

	"lo0.ai/lo0/internal/codex"
)

func TestHome(t *testing.T) {
	none := func(string) string { return "" }
	if got := codex.Home("/home/u", none); got != filepath.Join("/home/u", ".codex") {
		t.Errorf("Home = %q", got)
	}
	moved := func(k string) string {
		if k == codex.HomeVar {
			return "/srv/codex"
		}
		return ""
	}
	if got := codex.Home("/home/u", moved); got != "/srv/codex" {
		t.Errorf("Home with %s = %q", codex.HomeVar, got)
	}
	if got := codex.SessionsDir("/srv/codex"); got != filepath.Join("/srv/codex", "sessions") {
		t.Errorf("SessionsDir = %q", got)
	}
}
