package install_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"lo0.ai/lo0/internal/claudecode"
	"lo0.ai/lo0/internal/claudecode/install"
	"lo0.ai/lo0/internal/system"
	"lo0.ai/lo0/internal/system/systemtest"
	"lo0.ai/lo0/internal/usage"
)

func registry(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/-/package/@anthropic-ai/claude-code/dist-tags" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDetect(t *testing.T) {
	srv := registry(t, http.StatusOK, `{"stable":"2.1.311","latest":"2.1.318","next":"2.1.318"}`)
	d := install.Detector{
		Runner: &systemtest.Runner{
			Paths:    map[string]string{"claude": "/home/u/.local/bin/claude"},
			Commands: map[string]systemtest.Result{"claude --version": {Output: "2.1.221 (Claude Code)\n"}},
		},
		App: fstest.MapFS{
			"2.1.309/claude":            {},
			"2.1.317/claude":            {},
			"2.1.99/claude":             {},
			"auto-update-state.json":    {Data: []byte("{}")},
			"not-a-version/placeholder": {},
		},
		Client:      srv.Client(),
		RegistryURL: srv.URL,
	}
	got, err := d.Detect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := install.Installs{CLI: "2.1.221", App: "2.1.317", Latest: "2.1.318", Stable: "2.1.311"}
	if got != want {
		t.Errorf("Detect = %+v, want %+v", got, want)
	}
}

func TestDetectDegrades(t *testing.T) {
	t.Run("nothing installed and offline", func(t *testing.T) {
		got, err := install.Detector{Runner: &systemtest.Runner{}}.Detect(t.Context())
		if err != nil || got != (install.Installs{}) {
			t.Errorf("Detect = %+v, %v; want nothing and no error", got, err)
		}
	})
	t.Run("registry fails", func(t *testing.T) {
		srv := registry(t, http.StatusServiceUnavailable, "")
		d := install.Detector{Runner: &systemtest.Runner{}, Client: srv.Client(), RegistryURL: srv.URL}
		got, err := d.Detect(t.Context())
		if err == nil || !strings.Contains(err.Error(), "newest Claude Code release") || got.Latest != "" {
			t.Errorf("Detect = %+v, %v; want the lookup failure reported", got, err)
		}
	})
	t.Run("offline", func(t *testing.T) {
		srv := registry(t, http.StatusOK, "{}")
		srv.Close()
		d := install.Detector{Runner: &systemtest.Runner{}, Client: srv.Client(), RegistryURL: srv.URL}
		if got, err := d.Detect(t.Context()); err != nil || got.Latest != "" {
			t.Errorf("Detect = %+v, %v; want no release and no warning when offline", got, err)
		}
	})
	t.Run("claude command fails", func(t *testing.T) {
		d := install.Detector{Runner: &systemtest.Runner{
			Paths:    map[string]string{"claude": "/bin/claude"},
			Commands: map[string]systemtest.Result{"claude --version": {Err: system.ErrNotFound}},
		}}
		if _, err := d.Detect(t.Context()); err == nil {
			t.Error("Detect returned no error for a failing claude command")
		}
	})
}

func TestAppDir(t *testing.T) {
	if got := install.AppDir("darwin", "/Users/u"); got != filepath.Join("/Users/u", "Library", "Application Support", "Claude", "claude-code") {
		t.Errorf("AppDir(darwin) = %q", got)
	}
	if got := install.AppDir("linux", "/home/u"); got != "" {
		t.Errorf("AppDir(linux) = %q, want none", got)
	}
}

func TestCheck(t *testing.T) {
	newer := "9.0.0"
	tests := []struct {
		name    string
		in      install.Installs
		history usage.Format
		want    []string
	}{
		{"verified version", install.Installs{App: claudecode.VerifiedVersion}, usage.Format{Responses: 100}, nil},
		{"newer app", install.Installs{App: newer}, usage.Format{}, []string{"You use Claude Code 9.0.0, newer than"}},
		{"newer history", install.Installs{}, usage.Format{Versions: []usage.Tally{{Name: newer}}}, []string{"You use Claude Code 9.0.0"}},
		{"missing usage", install.Installs{}, usage.Format{Responses: 100, WithoutUsage: 40}, []string{"40 of 100 responses have no token counts"}},
		{"a few missing", install.Installs{}, usage.Format{Responses: 100, WithoutUsage: 5}, nil},
		{"unknown types", install.Installs{}, usage.Format{UnknownTypes: []usage.Tally{{Name: "hologram"}}}, []string{"1 record type lo0 does not know (hologram)"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := install.Check(tt.in, tt.history)
			if len(got) != len(tt.want) {
				t.Fatalf("Check = %q, want %d notes", got, len(tt.want))
			}
			for i, w := range tt.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("note %d = %q, want it to contain %q", i, got[i], w)
				}
			}
		})
	}
}

func TestNewest(t *testing.T) {
	got := install.Newest(install.Installs{CLI: "2.1.221", App: "2.1.317"}, usage.Format{Versions: []usage.Tally{{Name: "2.1.300"}}})
	if got != "2.1.317" {
		t.Errorf("Newest = %q", got)
	}
}
