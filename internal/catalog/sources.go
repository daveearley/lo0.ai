package catalog

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
)

// Sources is the hand-curated list the catalog is generated from. Only the
// judgement calls live here: which models matter, their tier and their
// Ollama tags. Everything measurable comes from Hugging Face.
type Sources struct {
	TierNotes map[Tier]string `json:"tierNotes"`
	// Benchmarks are the benchmarks compared across models.
	Benchmarks []BenchmarkDef `json:"benchmarks"`
	Models     []Source       `json:"models"`
}

// Source is one curated model.
type Source struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Family string   `json:"family"`
	Repo   string   `json:"repo"`
	Ollama []string `json:"ollama,omitzero"`
	// GGUF is the Hugging Face repository lo0 downloads llama.cpp builds
	// from. The generator keeps only the quantizations it publishes.
	GGUF string `json:"gguf,omitzero"`
	Tier Tier   `json:"tier"`
	// Quants replaces the GGUF ladder for models that ship in their own
	// format, such as gpt-oss in MXFP4.
	Quants []Quant `json:"quants,omitzero"`
	// ActiveParams overrides the derived count when the model card states
	// it and the config cannot show it, for example because large lookup
	// tables are not read in full per token.
	ActiveParams int64 `json:"activeParams,omitzero"`
	// StatedActiveParams is the active count the model card states, shown
	// to users. The derived ActiveParams still drives speed estimates: it
	// also counts embedding tables, so it errs towards slower.
	StatedActiveParams int64 `json:"statedActiveParams,omitzero"`
	// ContextLength replaces the config's window when the card states a
	// different native one.
	ContextLength int `json:"contextLength,omitzero"`
	// Released replaces the repository's creation date when the lab
	// announced the model on a different day.
	Released   string      `json:"released,omitzero"`
	Summary    string      `json:"summary,omitzero"`
	GoodAt     []string    `json:"goodAt,omitzero"`
	WatchOut   []string    `json:"watchOut,omitzero"`
	Benchmarks []Benchmark `json:"benchmarks,omitzero"`
	Card       string      `json:"card,omitzero"`
}

// ParseSources decodes the curated source list.
func ParseSources(data []byte) (Sources, error) {
	var s Sources
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("parsing sources: %w", err)
	}
	return s, nil
}

// RepoInfo is what the Hugging Face API reports about a repository.
type RepoInfo struct {
	Params   int64
	License  string
	Released string
	Evals    []EvalResult
}

// ggufLadder lists the common llama.cpp quantizations with their average
// bits per weight across a whole model, including the tensors kept at higher
// precision.
var ggufLadder = []Quant{
	{Name: "Q8_0", BitsPerWeight: 8.5},
	{Name: "Q6_K", BitsPerWeight: 6.56},
	{Name: "Q5_K_M", BitsPerWeight: 5.69},
	{Name: "Q4_K_M", BitsPerWeight: 4.85},
	{Name: "Q3_K_M", BitsPerWeight: 3.91},
	{Name: "Q2_K", BitsPerWeight: 3.0},
}

// Build turns a curated source, its repository info and its config into a
// catalog entry.
func Build(src Source, info RepoInfo, arch Architecture, tierNote string) Model {
	m := Model{
		ID:            src.ID,
		Name:          src.Name,
		Family:        src.Family,
		Repo:          src.Repo,
		License:       info.License,
		Released:      info.Released,
		Ollama:        src.Ollama,
		GGUF:          src.GGUF,
		Params:        info.Params,
		ActiveParams:  arch.ActiveParams(info.Params),
		ContextLength: arch.ContextLength,
		Attention:     arch.Attention,
		Experts:       arch.expertsOf(),
		Vision:        arch.Vision,
		Tier:          src.Tier,
		TierNote:      tierNote,
		Profile: Profile{
			Summary:    src.Summary,
			GoodAt:     src.GoodAt,
			WatchOut:   src.WatchOut,
			Benchmarks: src.Benchmarks,
			Card:       src.Card,
		},
	}
	if src.ActiveParams > 0 {
		m.ActiveParams = src.ActiveParams
	}
	m.StatedActiveParams = src.StatedActiveParams
	if src.ContextLength > 0 {
		m.ContextLength = src.ContextLength
	}
	if src.Released != "" {
		m.Released = src.Released
	}
	quants := src.Quants
	if len(quants) == 0 {
		quants = ggufLadder
	}
	for _, q := range quants {
		switch {
		case q.Bytes == 0:
			q.Bytes = int64(math.Round(float64(info.Params) * q.BitsPerWeight / 8))
		case q.BitsPerWeight == 0:
			q.BitsPerWeight = math.Round(float64(q.Bytes)*8/float64(info.Params)*100) / 100
		}
		m.Quants = append(m.Quants, q)
	}
	slices.SortFunc(m.Quants, func(a, b Quant) int { return cmp.Compare(b.BitsPerWeight, a.BitsPerWeight) })
	return m
}

// Published keeps the quantizations that have a GGUF file among files, the
// paths in a Hugging Face repository. A quantization matches when its name
// appears in a path as a whole token, so Q4_K_M does not match UD-Q4_K_XL.
// Vision projector files are ignored.
func Published(quants []Quant, files []string) []Quant {
	var out []Quant
	for _, q := range quants {
		token := regexp.MustCompile(`(?i)(^|[-_./])` + regexp.QuoteMeta(q.Name) + `([-_./]|$)`)
		if slices.ContainsFunc(files, func(f string) bool {
			lower := strings.ToLower(f)
			return strings.HasSuffix(lower, ".gguf") && !strings.Contains(lower, "mmproj") && token.MatchString(f)
		}) {
			out = append(out, q)
		}
	}
	return out
}
