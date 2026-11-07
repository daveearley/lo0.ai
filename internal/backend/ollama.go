// Package backend prepares a catalogued model in a local runtime for a
// harness to use: one type per runtime, each implementing migrate.Backend.
// Ollama is driven through its API end to end; runtimes without an API for
// downloads get exact instructions for the steps the user takes.
package backend

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/runtimes"
)

// Ollama prepares models in Ollama: it pulls the build lo0 recommends and
// creates a model with the context window set. Ollama's default depends on
// the GPU's memory, 4k below 24 GiB up to 256k at 48 GiB, so it is often too
// small for an agent and never matches the window lo0 planned.
type Ollama struct{}

// ID implements migrate.Backend.
func (Ollama) ID() runtimes.ID { return runtimes.Ollama }

// minOllamaAnthropic is the first Ollama release that serves the Anthropic
// Messages API.
const minOllamaAnthropic = "0.14.0"

// minOllamaResponses is the first Ollama release Codex accepts for the
// Responses API (min_responses_version in codex-rs/ollama/src/lib.rs).
const minOllamaResponses = "0.13.4"

// Prepare implements migrate.Backend.
func (Ollama) Prepare(req migrate.Request, protocol migrate.Protocol) (migrate.Served, []migrate.Step, error) {
	rt := req.Runtime
	base := cmp.Or(rt.Endpoint, runtimes.OllamaBaseURL(getenv(req, "OLLAMA_HOST")))
	source, err := ollamaSource(req)
	if err != nil {
		return migrate.Served{}, nil, err
	}
	name := LocalName(req.Model, req.Context)
	served := migrate.Served{
		Runtime: runtimes.Ollama, RuntimeName: "Ollama", BaseURL: base,
		Model: name, DisplayName: req.Model.Name, Context: req.Context, Token: "ollama",
	}

	var steps []migrate.Step
	switch {
	case !rt.Installed:
		steps = append(steps, manual("Install Ollama", "Download it from https://ollama.com/download and start it."))
	case !rt.Running:
		steps = append(steps, manual("Start Ollama", "Open the Ollama app, or run: ollama serve"))
	case protocol == migrate.Anthropic && rt.Version != "" && olderVersion(rt.Version, minOllamaAnthropic):
		steps = append(steps, manual("Update Ollama",
			"Ollama "+rt.Version+" does not serve the Anthropic API that Claude Code uses; "+minOllamaAnthropic+" and later do.",
			"Download the current release from https://ollama.com/download."))
	case protocol == migrate.Responses && rt.Version != "" && olderVersion(rt.Version, minOllamaResponses):
		steps = append(steps, manual("Update Ollama",
			"Ollama "+rt.Version+" does not serve the Responses API that Codex uses; "+minOllamaResponses+" and later do.",
			"Download the current release from https://ollama.com/download."))
	}

	steps = append(steps, ollamaModel(rt, base, req.Model, req.Quant, req.Context, source, req.Installed != "")...)
	bg := req.Background
	if bg == nil {
		return served, steps, nil
	}
	// A background model is an extra: without a download the main model
	// does its work, as it would with no background model at all.
	bgSource, err := ollamaSource(migrate.Request{Model: bg.Model, Quant: bg.Quant})
	if errors.Is(err, errNoDownload) {
		served.BackgroundNote = noDownload(bg.Model, req.Model)
		return served, steps, nil
	}
	if err != nil {
		return migrate.Served{}, nil, err
	}
	served.Background = &migrate.ServedModel{
		Model: LocalName(bg.Model, bg.Context), DisplayName: bg.Model.Name, Quant: bg.Quant.Name, Context: bg.Context,
	}
	return served, append(steps, ollamaModel(rt, base, bg.Model, bg.Quant, bg.Context, bgSource, false)...), nil
}

// ollamaModel returns the steps that pull m at q from source, unless Ollama
// already has it, and create lo0's model from it with a window of tokens.
// installed means source is a build the user already had. Both steps
// record what they add, so undo removes them.
func ollamaModel(rt runtimes.Runtime, base string, m catalog.Model, q catalog.Quant, tokens int, source string, installed bool) []migrate.Step {
	has := func(n string) bool {
		for _, have := range rt.Models {
			if strings.EqualFold(strings.TrimSuffix(have.Name, ":latest"), strings.TrimSuffix(n, ":latest")) {
				return true
			}
		}
		return false
	}
	name := LocalName(m, tokens)
	pull := migrate.Step{
		Kind:   migrate.KindDownload,
		Title:  "Download " + m.Name + " at " + q.Name,
		Detail: []string{"ollama pull " + source, format.Bytes(uint64(q.Bytes)) + " from Hugging Face, fetched by Ollama"},
		Bytes:  q.Bytes,
		Done:   has(source),
		Run: func(ctx context.Context, fx migrate.Effects, progress func(done, total int64)) error {
			// The scan can miss a model, or the user can pull it after the
			// plan was made. Recording a pull of a model the user already had
			// would let undo delete it, so the pull is lo0's only when Ollama
			// said plainly that it did not have the model.
			switch ollamaHolds(ctx, fx.Client, base, source) {
			case holdsYes:
				progress(q.Bytes, q.Bytes)
				return nil
			case holdsNo:
				// Recorded before the pull, so undo also clears what an
				// interrupted or failed pull left. Revert takes a missing
				// model as already removed.
				if err := fx.Record(migrate.Change{Runtime: runtimes.Ollama, RuntimeName: "Ollama", BaseURL: base, Model: source, Bytes: q.Bytes}); err != nil {
					return err
				}
			case holdsUnknown:
			}
			// Ollama reports each layer on its own and names the later ones
			// only when it reaches them, so the planned size is the floor of
			// the total: the bar never shows the first layer as the whole.
			atLeast := func(done, total int64) { progress(done, max(total, q.Bytes)) }
			return ollamaStream(ctx, fx.Client, base+"/api/pull", map[string]any{"model": source}, atLeast)
		},
	}
	if installed {
		pull.Title = m.Name + " at " + q.Name + " is already in Ollama"
		pull.Detail = []string{source}
		pull.Bytes, pull.Done = 0, true
	}
	create := migrate.Step{
		Kind:   migrate.KindRuntime,
		Title:  "Create " + name + " with a " + format.Tokens(tokens) + " context window",
		Detail: []string{"based on " + source, "num_ctx " + format.Int(tokens)},
		Done:   has(name),
		Run: func(ctx context.Context, fx migrate.Effects, progress func(done, total int64)) error {
			body := map[string]any{"model": name, "from": source, "parameters": map[string]any{"num_ctx": tokens}}
			if err := ollamaStream(ctx, fx.Client, base+"/api/create", body, progress); err != nil {
				return err
			}
			return fx.Record(migrate.Change{Runtime: runtimes.Ollama, RuntimeName: "Ollama", BaseURL: base, Model: name})
		},
	}
	return []migrate.Step{pull, create}
}

// Revert implements migrate.Backend: it deletes a model lo0 pulled or
// created.
func (Ollama) Revert(ctx context.Context, fx migrate.Effects, c migrate.Change) error {
	body, err := json.Marshal(map[string]string{"model": c.Model})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.BaseURL+"/api/delete", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := fx.Client.Do(req)
	if err != nil {
		return fmt.Errorf("asking Ollama to delete %s: %w", c.Model, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("Ollama answered %s when deleting %s", resp.Status, c.Model)
	}
	return nil
}

// Remove returns the step that deletes model from Ollama. Ollama keeps
// weights another model still uses, so removing a model frees only what
// nothing else shares.
func (o Ollama) Remove(rt runtimes.Runtime, model string) migrate.Step {
	base := cmp.Or(rt.Endpoint, runtimes.DefaultOllamaURL)
	return migrate.Step{
		Kind:   migrate.KindRuntime,
		Title:  "Remove " + model + " from Ollama",
		Detail: []string{"ollama rm " + model},
		Run: func(ctx context.Context, fx migrate.Effects, progress func(done, total int64)) error {
			if err := o.Revert(ctx, fx, migrate.Change{BaseURL: base, Model: model}); err != nil {
				return err
			}
			progress(1, 1)
			return fx.Forget(runtimes.Ollama, model)
		},
	}
}

// errNoDownload means the catalogue names nothing Ollama can pull for a
// model.
var errNoDownload = errors.New("the catalogue has no download")

// ollamaSource is the name Ollama pulls the recommended build under. Models
// published in their own format, such as gpt-oss in MXFP4, come from
// Ollama's library; everything else comes from the catalogue's GGUF
// repository at the recommended quantization.
func ollamaSource(req migrate.Request) (string, error) {
	m := req.Model
	switch {
	case req.Installed != "":
		return req.Installed, nil
	case len(m.Quants) == 1 && len(m.Ollama) > 0:
		return m.Ollama[0], nil
	case m.GGUF != "":
		return "hf.co/" + m.GGUF + ":" + req.Quant.Name, nil
	default:
		return "", fmt.Errorf("%w for %s", errNoDownload, m.Name)
	}
}

// LocalName is the name lo0 gives the model it creates in a runtime for m
// with a context window of tokens, such as lo0/qwen3-coder-30b-a3b:64k.
func LocalName(m catalog.Model, tokens int) string {
	return catalog.LocalPrefix + m.ID + ":" + strings.ToLower(format.Tokens(tokens))
}

// holds is what Ollama said when asked whether it has a model.
type holds int

const (
	// holdsUnknown means the check failed: a timeout, an error or an
	// answer other than yes or no.
	holdsUnknown holds = iota
	holdsYes
	holdsNo
)

// ollamaHolds asks Ollama whether it already has model. Only a 404 is a
// no: anything else that is not a yes is unknown, and a pull after an
// unknown answer is never recorded, so undo cannot delete the user's own
// model.
func ollamaHolds(ctx context.Context, client *http.Client, base, model string) holds {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	data, err := json.Marshal(map[string]string{"model": model})
	if err != nil {
		return holdsUnknown
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/show", bytes.NewReader(data))
	if err != nil {
		return holdsUnknown
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return holdsUnknown
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	switch resp.StatusCode {
	case http.StatusOK:
		return holdsYes
	case http.StatusNotFound:
		return holdsNo
	default:
		return holdsUnknown
	}
}

// ollamaStream posts body to one of Ollama's streaming endpoints and reports
// progress until it says success. Ollama sends errors as a line with an
// "error" field, after a 200 status. A pull reports each layer by digest, so
// progress is the sum over every layer seen so far.
func ollamaStream(ctx context.Context, client *http.Client, url string, body any, progress func(done, total int64)) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Ollama answered %s", resp.Status)
	}
	type layer struct{ done, total int64 }
	layers := map[string]layer{}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		var line struct {
			Status    string `json:"status"`
			Error     string `json:"error"`
			Digest    string `json:"digest"`
			Total     int64  `json:"total"`
			Completed int64  `json:"completed"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		switch {
		case line.Error != "":
			return errors.New(line.Error)
		case line.Status == "success":
			return nil
		case line.Total > 0:
			layers[line.Digest] = layer{line.Completed, line.Total}
			var done, total int64
			for _, l := range layers {
				done += l.done
				total += l.total
			}
			progress(done, total)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return errors.New("Ollama stopped before it finished")
}

// noDownload is the note for a background model the catalogue has no
// download for in a runtime.
func noDownload(bg, main catalog.Model) string {
	return "The catalogue has no download of " + bg.Name + " for this runtime, so " + main.Name + " does background work too."
}

func manual(title string, detail ...string) migrate.Step {
	return migrate.Step{Kind: migrate.KindManual, Title: title, Detail: detail}
}

func onPath(req migrate.Request, program string) bool {
	if req.LookPath == nil {
		return false
	}
	_, err := req.LookPath(program)
	return err == nil
}

func getenv(req migrate.Request, key string) string {
	if req.Getenv == nil {
		return ""
	}
	return req.Getenv(key)
}

// olderVersion reports whether dotted version v is older than min. Parts that
// are not numbers compare as zero, so a pre-release counts as its release.
func olderVersion(v, min string) bool {
	a, b := strings.Split(strings.TrimPrefix(v, "v"), "."), strings.Split(min, ".")
	for i := range max(len(a), len(b)) {
		x, y := versionPart(a, i), versionPart(b, i)
		if x != y {
			return x < y
		}
	}
	return false
}

func versionPart(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n := 0
	for _, r := range parts[i] {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}
