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

	"lo0.ai/lo0/internal/ui/report"
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
		{"/{$}", "home", "/", report.ScreenHome, s.home},
		{"/claude", "claude", "/claude", report.ScreenSessions, s.claude},
		{"/models", "models", "/models", report.ScreenModels, s.models},
		{"/models/{id}", "model", "/models", report.ScreenModels, s.model},
		{"/installed", "installed", "/installed", report.ScreenMachine, s.installed},
	}
}

// rail is the rail's links, in order, with the one for current marked.
func rail(current string) []navItem {
	items := []navItem{
		{Href: "/", Label: report.ScreenHome},
		{Href: "/claude", Label: report.ScreenSessions},
		{Href: "/models", Label: report.ScreenModels},
		{Href: "/installed", Label: report.ScreenMachine},
	}
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
	// Simulated says the assessment is of hardware the user named.
	Simulated string
	Stages    []progressRow
	Content   any
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
	// Home's labels, worded as every front end words them.
	"replyLabel":  func() string { return report.LabelReply },
	"routesLabel": func() string { return report.LabelRoutes },
	"nextLabel":   func() string { return report.LabelNext },
	"sharePhrase": func() string { return report.SharePhrase },
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
			sh.Simulated = report.SimulatedMachine(v.snap.assessment.Scan.Machine)
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
	out := report.MachineChips(a.Scan.Machine, a.Match.Budget)
	running := "no runtime running"
	for _, rt := range a.Scan.Runtimes {
		if rt.Running {
			running = strings.TrimSpace(rt.Name + " " + rt.Version)
			break
		}
	}
	return append(out, running)
}
