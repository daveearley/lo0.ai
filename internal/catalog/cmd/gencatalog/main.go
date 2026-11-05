// Command gencatalog regenerates catalog.json from the curated sources and
// the Hugging Face API. Run it through `go generate ./internal/catalog`.
// Set HF_TOKEN to raise rate limits.
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"lo0.ai/lo0/internal/catalog"
)

func main() {
	sourcesPath := flag.String("sources", "sources.json", "curated source list")
	outPath := flag.String("out", "catalog.json", "catalog to write")
	flag.Parse()
	hf := hub{base: "https://huggingface.co", client: &http.Client{Timeout: 30 * time.Second}, token: os.Getenv("HF_TOKEN")}
	if err := run(context.Background(), hf, *sourcesPath, *outPath, time.Now, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "gencatalog:", err)
		os.Exit(1)
	}
}

// run writes the catalog and logs to log every score it took from Hugging
// Face eval results and every one that disagrees with the sources, so the
// person regenerating can check them against the model cards.
func run(ctx context.Context, hf hub, sourcesPath, outPath string, now func() time.Time, log io.Writer) error {
	data, err := os.ReadFile(sourcesPath)
	if err != nil {
		return err
	}
	sources, err := catalog.ParseSources(data)
	if err != nil {
		return err
	}
	models := make([]catalog.Model, len(sources.Models))
	notes := make([][]string, len(sources.Models))
	errs := make([]error, len(sources.Models))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, src := range sources.Models {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			models[i], notes[i], errs[i] = build(ctx, hf, src, sources.Benchmarks, sources.TierNotes[src.Tier])
		})
	}
	wg.Wait()

	var failed []string
	var kept []catalog.Model
	for i, err := range errs {
		if err != nil {
			failed = append(failed, fmt.Sprintf("  %s: %v", sources.Models[i].Repo, err))
			continue
		}
		kept = append(kept, models[i])
		for _, n := range notes[i] {
			fmt.Fprintf(log, "%s: %s\n", sources.Models[i].Repo, n)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d models failed:\n%s", len(failed), strings.Join(failed, "\n"))
	}
	slices.SortFunc(kept, func(a, b catalog.Model) int { return cmp.Compare(a.ID, b.ID) })

	out, err := json.MarshalIndent(catalog.Catalog{Generated: now().UTC().Format(time.DateOnly), Models: kept}, "", "  ")
	if err != nil {
		return err
	}
	if _, err := catalog.Parse(out); err != nil {
		return fmt.Errorf("generated catalog is invalid: %w", err)
	}
	return os.WriteFile(outPath, append(out, '\n'), 0o644)
}

func build(ctx context.Context, hf hub, src catalog.Source, defs []catalog.BenchmarkDef, tierNote string) (catalog.Model, []string, error) {
	info, err := hf.info(ctx, src.Repo)
	if err != nil {
		return catalog.Model{}, nil, err
	}
	config, err := hf.get(ctx, "/"+src.Repo+"/resolve/main/config.json")
	if err != nil {
		return catalog.Model{}, nil, fmt.Errorf("config.json: %w", err)
	}
	arch, err := catalog.DeriveArchitecture(config)
	if err != nil {
		return catalog.Model{}, nil, err
	}
	benchmarks, err := catalog.ResolveBenchmarks(defs, src.Benchmarks)
	if err != nil {
		return catalog.Model{}, nil, err
	}
	var notes []string
	if src.Card != "" {
		added, conflicts := catalog.FillFromEvals(defs, benchmarks, info.Evals, src.Card)
		benchmarks = append(benchmarks, added...)
		for _, b := range added {
			notes = append(notes, "added "+b.Name+" "+b.Score+" from the card's eval results")
		}
		notes = append(notes, conflicts...)
	}
	src.Benchmarks = benchmarks
	m := catalog.Build(src, info, arch, tierNote)
	if src.GGUF != "" {
		files, err := hf.files(ctx, src.GGUF)
		if err != nil {
			return catalog.Model{}, nil, fmt.Errorf("GGUF repository %s: %w", src.GGUF, err)
		}
		if m.Quants = catalog.Published(m.Quants, files); len(m.Quants) == 0 {
			return catalog.Model{}, nil, fmt.Errorf("GGUF repository %s publishes none of the quantizations lo0 considers", src.GGUF)
		}
	}
	return m, notes, nil
}

// files lists the paths in a repository.
func (h hub) files(ctx context.Context, repo string) ([]string, error) {
	data, err := h.get(ctx, "/api/models/"+repo)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Siblings []struct {
			Name string `json:"rfilename"`
		} `json:"siblings"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("repository files: %w", err)
	}
	out := make([]string, 0, len(resp.Siblings))
	for _, s := range resp.Siblings {
		out = append(out, s.Name)
	}
	return out, nil
}

type hub struct {
	base   string
	client *http.Client
	token  string
}

// infoFields are the fields info asks for. Naming any field limits the
// response to the named ones, and eval results come only when named.
const infoFields = "?expand%5B%5D=createdAt&expand%5B%5D=safetensors&expand%5B%5D=cardData&expand%5B%5D=evalResults"

func (h hub) info(ctx context.Context, repo string) (catalog.RepoInfo, error) {
	data, err := h.get(ctx, "/api/models/"+repo+infoFields)
	if err != nil {
		return catalog.RepoInfo{}, err
	}
	var resp struct {
		CreatedAt   string `json:"createdAt"`
		Safetensors struct {
			Total int64 `json:"total"`
		} `json:"safetensors"`
		CardData struct {
			License string `json:"license"`
		} `json:"cardData"`
		// An eval result the Hub could not parse has an error in place of
		// its data.
		EvalResults []struct {
			Data *struct {
				Dataset struct {
					ID   string `json:"id"`
					Task string `json:"task_id"`
				} `json:"dataset"`
				Value  *float64 `json:"value"`
				Source struct {
					URL string `json:"url"`
				} `json:"source"`
			} `json:"data"`
		} `json:"evalResults"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return catalog.RepoInfo{}, fmt.Errorf("model info: %w", err)
	}
	if resp.Safetensors.Total == 0 {
		return catalog.RepoInfo{}, errors.New("model info has no parameter count")
	}
	released, _, _ := strings.Cut(resp.CreatedAt, "T")
	info := catalog.RepoInfo{Params: resp.Safetensors.Total, License: resp.CardData.License, Released: released}
	for _, e := range resp.EvalResults {
		if e.Data != nil && e.Data.Value != nil {
			info.Evals = append(info.Evals, catalog.EvalResult{Dataset: e.Data.Dataset.ID, Task: e.Data.Dataset.Task, Value: *e.Data.Value, Source: e.Data.Source.URL})
		}
	}
	return info, nil
}

func (h hub) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.base+path, nil)
	if err != nil {
		return nil, err
	}
	if h.token != "" {
		req.Header.Set("Authorization", "Bearer "+h.token)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", path, resp.Status)
	}
	return io.ReadAll(resp.Body)
}
