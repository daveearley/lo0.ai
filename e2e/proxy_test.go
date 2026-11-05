package e2e_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
)

// recordingProxy is the HTTP and HTTPS proxy every lo0 run is given. It
// refuses every request and remembers which host each was for, so a test
// can say exactly which hosts lo0 tried to reach. Go never proxies
// loopback, so the fake Ollama and lo0 web are reached directly.
type recordingProxy struct {
	srv *httptest.Server

	mu    sync.Mutex
	hosts []string
}

func newRecordingProxy(t *testing.T) *recordingProxy {
	p := &recordingProxy{}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CONNECT names host:port; a plain proxied request names a URL.
		host := r.Host
		if r.Method != http.MethodConnect && r.URL.Host != "" {
			host = r.URL.Host
		}
		p.mu.Lock()
		p.hosts = append(p.hosts, host)
		p.mu.Unlock()
		http.Error(w, "the e2e proxy refuses every request", http.StatusForbidden)
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *recordingProxy) url() string { return p.srv.URL }

// seen lists each host lo0 tried to reach, once.
func (p *recordingProxy) seen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Compact(slices.Sorted(slices.Values(p.hosts)))
}

// reset forgets every request seen so far.
func (p *recordingProxy) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.hosts = nil
}
