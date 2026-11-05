// Package web is lo0's browser front end. It serves the assessment as pages
// to a browser on this machine that holds the token lo0 printed, and every
// page only reads: changes are made from the command line or lo0 ui, whose
// commands the pages show.
package web

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"sync"
	"time"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/scan"
)

//go:embed assets templates
var files embed.FS

// Service is the part of the application service the pages read.
type Service interface {
	Assess(ctx context.Context, observe scan.Observer) (app.Assessment, error)
	Inventory(a app.Assessment) (app.Inventory, error)
	Harnesses(ctx context.Context, a app.Assessment) []app.HarnessView
}

// Options configures a Server.
type Options struct {
	Service Service
	// Token is the secret a browser must present, once in the URL and then
	// in a cookie. It must be long and random.
	Token string
	// Home is the user's home directory, shown as ~ in paths.
	Home    string
	Version string
	Now     func() time.Time
}

// Server serves the pages and runs the scans behind them.
type Server struct {
	opts  Options
	pages map[string]*template.Template
	// assets is a hash of the stylesheet and script, added to their
	// addresses so a browser never keeps a copy from an older lo0.
	assets string
	state  *state
	// scans owns the scan goroutines, so Serve can wait for them.
	scans sync.WaitGroup
}

// minToken is the shortest token New accepts, in bytes of text.
const minToken = 32

// New returns a Server. It fails if the service or clock is missing, the
// token is shorter than minToken, or the embedded templates do not parse.
func New(opts Options) (*Server, error) {
	if opts.Service == nil || opts.Now == nil {
		return nil, errors.New("web: a service and a clock are required")
	}
	if len(opts.Token) < minToken {
		return nil, fmt.Errorf("web: the token must be at least %d characters", minToken)
	}
	pages, err := parsePages()
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	for _, name := range []string{"assets/app.css", "assets/app.js"} {
		b, err := files.ReadFile(name)
		if err != nil {
			return nil, err
		}
		h.Write(b)
	}
	return &Server{opts: opts, pages: pages, assets: hex.EncodeToString(h.Sum(nil))[:12], state: newState()}, nil
}

// Handler returns the server's routes for a listener at addr, such as
// "127.0.0.1:7410". Requests naming any other host are refused, so a page
// on another site cannot reach lo0 by pointing its own name at this
// machine. Scans the pages start stop when ctx is cancelled.
func (s *Server) Handler(ctx context.Context, addr string) http.Handler {
	mux := http.NewServeMux()
	assets, _ := fs.Sub(files, "assets")
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", cached(http.FileServerFS(assets))))
	for _, p := range pageRoutes(s) {
		mux.HandleFunc("GET "+p.pattern, s.page(ctx, p))
	}
	mux.HandleFunc("GET /events", s.events)
	mux.HandleFunc("POST /scan", s.rescan(ctx))
	return guard(addr, s.opts.Token, mux)
}

// Serve starts a scan, then serves on ln until ctx is cancelled. It waits
// for open pages and the scan to stop before it returns.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           s.Handler(ctx, ln.Addr().String()),
		ReadHeaderTimeout: 10 * time.Second,
		// Requests share ctx, so open event streams end with it rather
		// than holding up the shutdown.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	s.scan(ctx)
	errs := make(chan error, 1)
	go func() { errs <- srv.Serve(ln) }()

	var err error
	select {
	case err = <-errs:
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		err = srv.Shutdown(shutdown)
		cancel()
		if served := <-errs; err == nil {
			err = served
		}
	}
	s.scans.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// scan starts a scan unless one is running.
func (s *Server) scan(ctx context.Context) {
	if !s.state.begin() {
		return
	}
	s.scans.Go(func() {
		a, err := s.opts.Service.Assess(ctx, s.state.observe)
		if err != nil {
			s.state.fail(err)
			return
		}
		snap := snapshot{assessment: a, at: s.opts.Now()}
		// An inventory lo0 cannot read leaves the Installed page empty
		// rather than failing the whole scan.
		snap.inventory, _ = s.opts.Service.Inventory(a)
		snap.harnesses = s.opts.Service.Harnesses(ctx, a)
		snap.next = app.Next(a, snap.inventory, snap.harnesses)
		s.state.finish(snap)
	})
}

// cached lets the browser keep the embedded assets, which change only with
// the binary.
func cached(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, max-age=3600")
		h.ServeHTTP(w, r)
	})
}
