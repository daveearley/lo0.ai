package app

import (
	"bytes"
	"cmp"
	"io/fs"
	"maps"
	"slices"
	"time"

	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/runtimes"
)

// Inventory is what is installed on this machine for running models
// locally, and what lo0 changed.
type Inventory struct {
	Runtimes []InstalledRuntime `json:"runtimes"`
	// DiskBytes is the space the models take, counting weights that several
	// models share once.
	DiskBytes uint64 `json:"diskBytes"`
	// FreeBytes is the free space on the volume models are stored on.
	FreeBytes uint64 `json:"freeBytes,omitzero"`
	// Changes are the changes lo0 made and can undo, oldest first.
	Changes []migrate.Change `json:"changes,omitzero"`
}

// InstalledRuntime is one local runtime and the models it has.
type InstalledRuntime struct {
	ID        runtimes.ID `json:"id"`
	Name      string      `json:"name"`
	Installed bool        `json:"installed"`
	Running   bool        `json:"running"`
	Version   string      `json:"version,omitzero"`
	// Models are the runtime's models, each followed by those made from it.
	Models []InstalledModel `json:"models,omitzero"`
	// Removable is true when lo0 can remove the runtime's models itself.
	Removable bool `json:"removable"`
}

// InstalledModel is one model a runtime has.
type InstalledModel struct {
	Name string `json:"name"`
	// Bytes is the space the model's weights take. A model made from
	// another shares its weights and takes none of its own.
	Bytes uint64 `json:"bytes"`
	// Parent names the model this one was made from, when the runtime has
	// it.
	Parent string `json:"parent,omitzero"`
	// CatalogID and CatalogName identify the model in lo0's catalogue, when
	// it is there, and Quant is its quantization.
	CatalogID   string `json:"catalogId,omitzero"`
	CatalogName string `json:"catalogName,omitzero"`
	Quant       string `json:"quant,omitzero"`
	// MadeByLo0 is set to when lo0 created or downloaded the model.
	MadeByLo0 time.Time `json:"madeByLo0,omitzero"`
	// UsedBy are files lo0 wrote that point at the model, such as a
	// harness's configuration.
	UsedBy []string `json:"usedBy,omitzero"`
}

// Inventory lists the runtimes the scan found with their models, what lo0
// changed and which of its files use each model. It only reads.
func (s *Service) Inventory(a Assessment) (Inventory, error) {
	inv := Inventory{FreeBytes: a.Scan.Machine.Disk.FreeBytes}
	changes, err := s.migrator.Effects.Journal.Load(s.files())
	if err != nil {
		return inv, err
	}
	inv.Changes = changes

	// The files lo0 wrote that are still there, to see which models they
	// name.
	written := map[string][]byte{}
	for _, c := range changes {
		if c.Path == "" {
			continue
		}
		if data, err := s.files().ReadFile(c.Path); err == nil {
			written[c.Path] = data
		}
	}

	for _, rt := range a.Scan.Runtimes {
		ir := InstalledRuntime{
			ID: rt.ID, Name: rt.Name, Installed: rt.Installed, Running: rt.Running, Version: rt.Version,
			Removable: s.remover(rt.ID) != nil,
		}
		names := map[string]bool{}
		for _, m := range rt.Models {
			names[m.Name] = true
		}
		for _, m := range rt.Models {
			im := InstalledModel{Name: m.Name, Bytes: m.SizeBytes, Quant: m.Quantization}
			if m.Parent != "" && names[m.Parent] {
				im.Parent, im.Bytes = m.Parent, 0
			}
			if cm, ok := s.catalog.ByOllamaTag(cmp.Or(im.Parent, m.Name)); ok && rt.ID == runtimes.Ollama {
				im.CatalogID, im.CatalogName = cm.ID, cm.Name
			}
			for _, c := range changes {
				if c.Runtime == rt.ID && c.Model == m.Name && im.MadeByLo0.IsZero() {
					im.MadeByLo0 = c.At
				}
			}
			for _, path := range slices.Sorted(maps.Keys(written)) {
				if bytes.Contains(written[path], []byte(m.Name)) {
					im.UsedBy = append(im.UsedBy, path)
				}
			}
			inv.DiskBytes += im.Bytes
			ir.Models = append(ir.Models, im)
		}
		ir.Models = grouped(ir.Models)
		inv.Runtimes = append(inv.Runtimes, ir)
	}
	return inv, nil
}

// grouped orders models by name with each model's variants straight after
// it.
func grouped(models []InstalledModel) []InstalledModel {
	slices.SortFunc(models, func(a, b InstalledModel) int { return cmp.Compare(a.Name, b.Name) })
	out := make([]InstalledModel, 0, len(models))
	for _, m := range models {
		if m.Parent != "" {
			continue
		}
		out = append(out, m)
		for _, v := range models {
			if v.Parent == m.Name {
				out = append(out, v)
			}
		}
	}
	return out
}

func (s *Service) files() migrate.Files {
	if s.migrator.Effects.Files == nil {
		return noFiles{}
	}
	return s.migrator.Effects.Files
}

// noFiles is a service without a migrator's files: there is nothing to
// read.
type noFiles struct{}

func (noFiles) ReadFile(string) ([]byte, error)             { return nil, fs.ErrNotExist }
func (noFiles) WriteFile(string, []byte, fs.FileMode) error { return fs.ErrPermission }
func (noFiles) Remove(string) error                         { return fs.ErrPermission }

// ModelRemover is a backend that can remove a model from its runtime.
type ModelRemover interface {
	// Remove returns the step that removes model from rt and forgets any
	// record lo0 kept of making it.
	Remove(rt runtimes.Runtime, model string) migrate.Step
}

// remover is the backend that can remove models from runtime id, if any.
func (s *Service) remover(id runtimes.ID) ModelRemover {
	for _, b := range s.migrator.Backends {
		if r, ok := b.(ModelRemover); ok && b.ID() == id {
			return r
		}
	}
	return nil
}
