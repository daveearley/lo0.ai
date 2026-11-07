package web

import (
	"net/http"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/match"
)

func TestSimulatedHardwareIsStatedOnEveryPage(t *testing.T) {
	sim, err := match.Simulate(match.HardwareQuery{GPU: "RTX 5090", RAM: 96})
	if err != nil {
		t.Fatal(err)
	}
	h := scanned(t, newTestServer(t, fakeService{hardware: &sim}))
	for _, path := range []string{"/", "/models", "/installed", "/claude"} {
		w := get(t, h, path)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", path, w.Code, w.Body)
		}
		body := w.Body.String()
		if !strings.Contains(body, `<p class="simulated">Simulated machine: NVIDIA GeForce RTX 5090 with 32 GiB of VRAM`) || !strings.Contains(body, "<li>simulated</li>") {
			t.Errorf("%s does not say the machine is simulated", path)
		}
		if path == "/" {
			golden(t, "home-simulated.html", body)
		}
	}
}

func TestRealMachineHasNoSimulatedNote(t *testing.T) {
	w := get(t, scanned(t, newTestServer(t, fakeService{})), "/")
	if strings.Contains(w.Body.String(), `class="simulated"`) {
		t.Error("the real machine's page says it is simulated")
	}
}
