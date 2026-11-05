package web

import (
	"net/http"
	"slices"
	"strings"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/format"
)

// installedView is the Installed page: the runtimes on this machine, the
// models each has and what lo0 changed.
type installedView struct {
	Summary  string
	Runtimes []runtimeView
	Changes  []string
}

// runtimeView is one local runtime and its models.
type runtimeView struct {
	Name, State string
	Running     bool
	Models      []installedRow
}

// installedRow is one model on disk. A model made from another shares its
// weights and is indented under it.
type installedRow struct {
	Name, Known, Quant, Size, Note string
	Child                          bool
}

func (s *Server) installed(_ *http.Request, snap *snapshot) (any, error) {
	inv := snap.inventory
	models := 0
	v := installedView{}
	for _, rt := range inv.Runtimes {
		if !rt.Installed {
			continue
		}
		state := "installed"
		if rt.Running {
			state = "running"
		}
		rv := runtimeView{Name: rt.Name, State: join([]string{rt.Version, state}), Running: rt.Running}
		for _, m := range rt.Models {
			if m.Parent == "" {
				models++
			}
			rv.Models = append(rv.Models, installedRow{
				Name: m.Name, Known: m.CatalogName, Quant: m.Quant, Size: size(m), Note: note(m, s.opts.Home), Child: m.Parent != "",
			})
		}
		v.Runtimes = append(v.Runtimes, rv)
	}
	parts := []string{format.Plural(models, "model", "models")}
	if inv.DiskBytes > 0 {
		parts = append(parts, format.Bytes(inv.DiskBytes)+" on disk")
	}
	if inv.FreeBytes > 0 {
		parts = append(parts, format.Bytes(inv.FreeBytes)+" free")
	}
	v.Summary = join(parts)
	v.Changes = changes(inv, s.opts.Home)
	return v, nil
}

func size(m app.InstalledModel) string {
	if m.Parent != "" {
		return "shared"
	}
	return format.Bytes(m.Bytes)
}

// note says who made a model and what uses it.
func note(m app.InstalledModel, home string) string {
	var parts []string
	if !m.MadeByLo0.IsZero() {
		parts = append(parts, "added by lo0 "+m.MadeByLo0.Format("2 Jan"))
	}
	if len(m.UsedBy) > 0 {
		used := make([]string, 0, len(m.UsedBy))
		for _, p := range m.UsedBy {
			used = append(used, short(p, home))
		}
		parts = append(parts, "used by "+strings.Join(used, ", "))
	}
	return join(parts)
}

// changes says what each change lo0 made did, once per thing changed,
// newest first.
func changes(inv app.Inventory, home string) []string {
	var out []string
	for _, c := range slices.Backward(inv.Changes) {
		var what string
		switch {
		case c.Path != "" && c.Backup != "":
			what = "Changed " + short(c.Path, home)
		case c.Path != "":
			what = "Wrote " + short(c.Path, home)
		case c.Model != "":
			what = "Added " + c.Model + " to " + c.RuntimeName
		default:
			continue
		}
		what += " · " + c.At.Format("2 Jan")
		if !slices.Contains(out, what) {
			out = append(out, what)
		}
	}
	return out
}

// short replaces the home directory at the start of path with ~.
func short(path, home string) string {
	if home != "" && strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}
