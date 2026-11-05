package catalog

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

//go:generate go run ./cmd/gencatalog -sources sources.json -out catalog.json

//go:embed catalog.json
var embedded []byte

// Catalog is a validated set of models.
type Catalog struct {
	// Generated is when the data was last regenerated, as an RFC 3339 date.
	Generated string  `json:"generated"`
	Models    []Model `json:"models"`
}

// Parse decodes and validates catalog JSON.
func Parse(data []byte) (*Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing catalog: %w", err)
	}
	var errs []error
	seen := map[string]bool{}
	for _, m := range c.Models {
		if err := m.validate(); err != nil {
			errs = append(errs, fmt.Errorf("model %q: %w", m.ID, err))
		}
		if seen[m.ID] {
			errs = append(errs, fmt.Errorf("model %q appears twice", m.ID))
		}
		seen[m.ID] = true
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return &c, nil
}

func (m Model) validate() error {
	var errs []error
	if m.ID == "" || m.Name == "" {
		errs = append(errs, errors.New("missing id or name"))
	}
	if m.Params <= 0 || m.ActiveParams <= 0 || m.ActiveParams > m.Params {
		errs = append(errs, fmt.Errorf("implausible parameters %d active of %d", m.ActiveParams, m.Params))
	}
	if m.ContextLength <= 0 {
		errs = append(errs, errors.New("no context length"))
	}
	if m.Attention.FullLayers+m.Attention.SlidingLayers == 0 || m.Attention.FullKV <= 0 {
		errs = append(errs, errors.New("no attention layers"))
	}
	if len(m.Quants) == 0 {
		errs = append(errs, errors.New("no quantizations"))
	}
	if m.Tier.Rank() > TierD.Rank() {
		errs = append(errs, fmt.Errorf("unknown tier %q", m.Tier))
	}
	return errors.Join(errs...)
}

// Default returns the catalog embedded in the binary.
func Default() (*Catalog, error) {
	return Parse(embedded)
}

// LocalPrefix namespaces the models lo0 creates in a runtime, such as an
// Ollama model set to a longer context window: lo0/<model id>:<tag>.
const LocalPrefix = "lo0/"

// ByOllamaTag finds the model an Ollama tag serves. Tags are compared without
// the implicit ":latest" suffix. Besides the library tags, it recognises
// builds pulled from the model's GGUF repository (hf.co/<repo>:<quant>) and
// models lo0 created (lo0/<id>:<tag>).
func (c *Catalog) ByOllamaTag(tag string) (Model, bool) {
	tag = strings.TrimSuffix(strings.ToLower(tag), ":latest")
	name, _, _ := strings.Cut(tag, ":")
	for _, m := range c.Models {
		for _, t := range m.Ollama {
			if strings.EqualFold(t, tag) {
				return m, true
			}
		}
		if m.GGUF != "" && strings.EqualFold(name, "hf.co/"+m.GGUF) || strings.EqualFold(name, LocalPrefix+m.ID) {
			return m, true
		}
	}
	return Model{}, false
}
