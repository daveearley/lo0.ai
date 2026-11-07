package web

import (
	"errors"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

const testAddr = "127.0.0.1:7410"

// get requests path from h as a browser holding the token would.
func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+testAddr+path, nil)
	r.AddCookie(&http.Cookie{Name: "lo0-7410", Value: testToken})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// scanned returns a handler for s once its first scan has finished.
func scanned(t *testing.T, s *Server) http.Handler {
	t.Helper()
	h := s.Handler(t.Context(), testAddr)
	get(t, h, "/")
	s.scans.Wait()
	return h
}

func TestPagesGolden(t *testing.T) {
	h := scanned(t, newTestServer(t, fakeService{}))
	for _, tc := range []struct{ name, path string }{
		{"home", "/"},
		{"models", "/models"},
		{"models-all", "/models?show=all"},
		{"model", "/models/Coder-30B-A3B"},
		{"model-too-large", "/models/Giant-480B"},
		{"installed", "/installed"},
		{"claude", "/claude"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := get(t, h, tc.path)
			if w.Code != http.StatusOK {
				t.Fatalf("GET %s = %d: %s", tc.path, w.Code, w.Body)
			}
			golden(t, tc.name+".html", w.Body.String())
		})
	}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	// Fixtures use Unix paths and the renderers join them with the host's
	// separator, so goldens are kept and compared in slash form.
	got = filepath.ToSlash(got)
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/web -update)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs from the golden file; run go test ./internal/web -update and review the diff", name)
	}
}

func TestPageContent(t *testing.T) {
	h := scanned(t, newTestServer(t, fakeService{}))
	for _, tc := range []struct {
		name, path string
		want       []string
	}{
		{"home leads with the share and the next step", "/", []string{"of your sessions can run on this machine today", "Compare it with General-27B", "claude-local"}},
		{"models filter to installed", "/models?show=installed", []string{"Coder-30B-A3B", "aria-current=\"page\">Installed"}},
		{"model keeps the lab's claims apart", "/models/Coder-30B-A3B", []string{"From the model card", "On this machine", "Tool calling in long agent loops"}},
		{"too large says what it needs", "/models/Giant-480B", []string{"Needs "}},
		{"installed shortens home", "/installed", []string{"~/.local/bin/claude-local", "lo0 migrate --undo"}},
		{"claude shows the verdict", "/claude", []string{"readiness", "Peak context, every session"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := get(t, h, tc.path).Body.String()
			for _, want := range tc.want {
				if !strings.Contains(filepath.ToSlash(body), want) {
					t.Errorf("GET %s lacks %q", tc.path, want)
				}
			}
			if strings.Contains(body, testHome) {
				t.Errorf("GET %s shows the home directory in full", tc.path)
			}
		})
	}
}

func TestHomeWithoutHistoryStatesNoShare(t *testing.T) {
	body := get(t, scanned(t, newTestServer(t, fakeService{noHistory: true})), "/").Body.String()
	for _, want := range []string{"Local models for this machine", "found no Claude Code or Codex sessions", "is set up as your local model."} {
		if !strings.Contains(body, want) {
			t.Errorf("home without history lacks %q", want)
		}
	}
	if strings.Contains(body, "class=\"figure\"") || strings.Contains(body, "of your sessions") {
		t.Errorf("home without history states a share:\n%s", body)
	}
}

func TestUnknownModel(t *testing.T) {
	h := scanned(t, newTestServer(t, fakeService{}))
	if w := get(t, h, "/models/nope"); w.Code != http.StatusNotFound {
		t.Errorf("GET /models/nope = %d, want 404", w.Code)
	}
}

func TestScanningPage(t *testing.T) {
	release, started := make(chan struct{}), make(chan struct{})
	s := newTestServer(t, fakeService{release: release, started: started})
	h := s.Handler(t.Context(), testAddr)
	t.Cleanup(func() {
		close(release)
		s.scans.Wait()
	})
	get(t, h, "/")
	<-started
	w := get(t, h, "/models")
	body := w.Body.String()
	for _, want := range []string{"Reading this machine", "data-scanning", `data-state="running"`, "Hardware", "animateMotion"} {
		if !strings.Contains(body, want) {
			t.Errorf("scanning page lacks %q", want)
		}
	}
}

func TestScanFailure(t *testing.T) {
	h := scanned(t, newTestServer(t, fakeService{err: errors.New("disk on fire")}))
	body := get(t, h, "/").Body.String()
	if !strings.Contains(body, "The scan stopped: disk on fire") {
		t.Errorf("page does not say the scan failed:\n%s", body)
	}
}

func TestEventsEndWhenScanIsDone(t *testing.T) {
	h := scanned(t, newTestServer(t, fakeService{}))
	w := get(t, h, "/events")
	body := w.Body.String()
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q", ct)
	}
	if strings.Count(body, "event: stage") != 4 || !strings.HasSuffix(body, "event: done\ndata: {}\n\n") {
		t.Errorf("events = %q, want four stages then done", body)
	}
	if !strings.Contains(body, `"state":"done","note":"Apple M5 Pro · 64 GiB"`) {
		t.Errorf("events lack what the hardware stage found: %q", body)
	}
}

func TestRescan(t *testing.T) {
	s := newTestServer(t, fakeService{})
	h := scanned(t, s)
	for _, tc := range []struct{ back, want string }{
		{"/models", "/models"},
		{"//evil.example", "/"},
		{"https://evil.example", "/"},
		{"", "/"},
	} {
		form := url.Values{"back": {tc.back}}
		r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+testAddr+"/scan", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "http://"+testAddr)
		r.AddCookie(&http.Cookie{Name: "lo0-7410", Value: testToken})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		s.scans.Wait()
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != tc.want {
			t.Errorf("back %q: %d to %q, want 303 to %q", tc.back, w.Code, w.Header().Get("Location"), tc.want)
		}
	}
}

func TestAssetsAreVersioned(t *testing.T) {
	s := newTestServer(t, fakeService{})
	h := scanned(t, s)
	body := get(t, h, "/").Body.String()
	if !strings.Contains(body, "/assets/app.css?v="+s.assets) {
		t.Errorf("stylesheet link lacks the asset version %s", s.assets)
	}
	w := get(t, h, "/assets/app.css")
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Type"), "text/css") {
		t.Errorf("GET /assets/app.css = %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	b, _ := io.ReadAll(w.Body)
	if !strings.Contains(string(b), "@font-face") {
		t.Error("stylesheet is not the embedded one")
	}
}

func TestNewRejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts Options
	}{
		{"no service", Options{Token: testToken, Now: testClock}},
		{"short token", Options{Service: fakeService{}, Token: "short", Now: testClock}},
		{"no clock", Options{Service: fakeService{}, Token: testToken}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.opts); err == nil {
				t.Error("New accepted it")
			}
		})
	}
}
