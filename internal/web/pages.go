package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"

	"lo0.ai/lo0/internal/format"
)

// errNotFound is returned by a page builder when what the address names
// does not exist.
var errNotFound = errors.New("not found")

// route is one page: where it lives, its template, the rail link it sits
// under and how its content is built from a scan.
type route struct {
	pattern  string
	template string
	nav      string
	title    string
	build    func(r *http.Request, snap *snapshot) (any, error)
}

func pageRoutes(s *Server) []route {
	return []route{
		{"/{$}", "home", "/", "Home", s.home},
		{"/models", "models", "/models", "Models", s.models},
		{"/models/{id}", "model", "/models", "Models", s.model},
		{"/installed", "installed", "/installed", "Installed", s.installed},
		{"/claude", "claude", "/claude", "Your Claude", s.claude},
	}
}

// rail is the rail's links, in order, with the one for current marked.
func rail(current string) []navItem {
	items := []navItem{{Href: "/", Label: "Home"}, {Href: "/models", Label: "Models"}, {Href: "/installed", Label: "Installed"}, {Href: "/claude", Label: "Your Claude"}}
	for i := range items {
		items[i].Current = items[i].Href == current
	}
	return items
}

// navItem is one link in the rail.
type navItem struct {
	Href, Label string
	Current     bool
}

// shell is what every page shares: the rail, the scan state and the
// page's own content.
type shell struct {
	Title    string
	Nav      []navItem
	Machine  []string
	Scanned  string
	Scanning bool
	Path     string
	Version  string
	Assets   string
	Error    string
	Stages   []progressRow
	Content  any
}

func parsePages() (map[string]*template.Template, error) {
	names, err := fs.Glob(files, "templates/*.html")
	if err != nil {
		return nil, err
	}
	pages := map[string]*template.Template{}
	for _, n := range names {
		name := strings.TrimSuffix(strings.TrimPrefix(n, "templates/"), ".html")
		if name == "layout" || name == "parts" {
			continue
		}
		t, err := template.New("layout").Funcs(funcs).ParseFS(files, "templates/layout.html", "templates/parts.html", n)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", n, err)
		}
		pages[name] = t
	}
	return pages, nil
}

var funcs = template.FuncMap{
	// share sets --w, the filled part of a bar, from a share between 0
	// and 1.
	"share": func(f float64) template.CSS { return cssPercent("--w", f) },
	// top places a label at a height from the top of a chart, from 0 to 1.
	"top": func(f float64) template.CSS { return cssPercent("top", f) },
	// height sets --h, a column's height, from 0 to 1.
	"height": func(f float64) template.CSS { return cssPercent("--h", f) },
}

// cssPercent is a CSS declaration of a share as a percentage. The value is
// a number lo0 computed, never text from outside, so it is safe as CSS.
func cssPercent(property string, f float64) template.CSS {
	return template.CSS(fmt.Sprintf("%s:%.2f%%", property, min(max(f, 0), 1)*100))
}

// page serves one route. Before the first scan finishes it shows the scan's
// progress instead, and it starts that scan if nothing has.
func (s *Server) page(ctx context.Context, rt route) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v := s.state.view()
		if v.snap == nil && !v.scanning && v.err == nil {
			s.scan(ctx)
			v = s.state.view()
		}
		sh := shell{
			Title: rt.title, Nav: rail(rt.nav), Scanning: v.scanning, Path: r.URL.Path,
			Version: s.opts.Version, Assets: s.assets, Stages: progressRows(v.stages),
		}
		if v.err != nil {
			sh.Error = "The scan stopped: " + v.err.Error()
		}
		name := rt.template
		if v.snap == nil {
			name, sh.Title = "scanning", "Reading this machine"
		} else {
			sh.Machine = s.machine(v.snap)
			sh.Scanned = "Scanned " + v.snap.at.Format("15:04")
			content, err := rt.build(r, v.snap)
			if errors.Is(err, errNotFound) {
				http.NotFound(w, r)
				return
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			sh.Content = content
		}
		s.render(w, name, sh)
	}
}

func (s *Server) render(w http.ResponseWriter, name string, sh shell) {
	t, ok := s.pages[name]
	if !ok {
		http.Error(w, "no template "+name, http.StatusInternalServerError)
		return
	}
	var b bytes.Buffer
	if err := t.ExecuteTemplate(&b, "layout", sh); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = b.WriteTo(w)
}

// rescan starts a new scan and returns to the page the form was on.
func (s *Server) rescan(ctx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.scan(ctx)
		back := r.FormValue("back")
		if !strings.HasPrefix(back, "/") || strings.HasPrefix(back, "//") || strings.ContainsAny(back, "\\\r\n") {
			back = "/"
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
	}
}

// machine is the rail's summary of this machine: the chip, the memory
// models can use and the runtime that is running.
func (s *Server) machine(snap *snapshot) []string {
	a := snap.assessment
	var out []string
	if cpu := a.Scan.Machine.CPU.Model; cpu != "" {
		out = append(out, cpu)
	}
	if b := a.Match.Budget.Bytes; b > 0 {
		out = append(out, format.Bytes(b)+" for models")
	}
	running := "no runtime running"
	for _, rt := range a.Scan.Runtimes {
		if rt.Running {
			running = strings.TrimSpace(rt.Name + " " + rt.Version)
			break
		}
	}
	return append(out, running)
}
