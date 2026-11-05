package backend

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"lo0.ai/lo0/internal/bench"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/runtimes"
)

// benchmarkReply is how many tokens each step asks the model to write.
const benchmarkReply = 300

// tokensPerLine is roughly how many tokens a line of the benchmark prompt
// is for the models lo0 recommends, about 47 for Qwen's tokenizer; the
// runtime reports the exact count, which is what is recorded.
const tokensPerLine = 47

// minRead is the share of the prompt the runtime must read for a
// measurement to count. Ollama drops part of a prompt that overflows its
// window rather than failing, which would record a shorter context.
const minRead = 0.9

// Benchmark returns one step per context size. Each sends a fresh prompt of
// about that many tokens, asks for benchmarkReply tokens and records Ollama's
// own read and write timings. Every step shares one window, the largest
// prompt plus a quarter and 4k, so the model loads once and none overflows.
func (Ollama) Benchmark(rt runtimes.Runtime, model string, contexts []int, into *bench.Result) []migrate.Step {
	base := cmp.Or(rt.Endpoint, runtimes.DefaultOllamaURL)
	window := slices.Max(contexts)*5/4 + 4096
	steps := make([]migrate.Step, 0, len(contexts))
	for _, n := range contexts {
		steps = append(steps, migrate.Step{
			Kind:   migrate.KindMeasure,
			Title:  "Measure " + model + " at " + format.Tokens(n) + " of context",
			Detail: []string{"reads about " + format.Tokens(n) + " fresh tokens, then writes " + strconv.Itoa(benchmarkReply)},
			Run: func(ctx context.Context, fx migrate.Effects, progress func(done, total int64)) error {
				p, err := ollamaMeasure(ctx, fx.Client, base, model, n, window, progress)
				if err != nil {
					return err
				}
				into.Points = append(into.Points, p)
				progress(benchmarkReply, benchmarkReply)
				return nil
			},
		})
	}
	return steps
}

// ollamaMeasure sends one benchmark prompt and reads the timings Ollama
// reports with its reply. The reply is streamed so progress can count the
// tokens as they are written, for a front end that shows the race live;
// the timings recorded are still Ollama's own, from its last line.
func ollamaMeasure(ctx context.Context, client *http.Client, base, model string, tokens, window int, progress func(done, total int64)) (bench.Point, error) {
	body, err := json.Marshal(map[string]any{
		"model": model, "prompt": benchmarkPrompt(tokens), "stream": true,
		"options": map[string]any{"num_predict": benchmarkReply, "num_ctx": window, "temperature": 0},
	})
	if err != nil {
		return bench.Point{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return bench.Point{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return bench.Point{}, fmt.Errorf("asking Ollama to run %s: %w", model, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return bench.Point{}, fmt.Errorf("Ollama answered %s", resp.Status)
	}
	type line struct {
		Response           string `json:"response"`
		Thinking           string `json:"thinking"`
		Error              string `json:"error"`
		PromptEvalCount    int    `json:"prompt_eval_count"`
		PromptEvalDuration int64  `json:"prompt_eval_duration"`
		EvalCount          int    `json:"eval_count"`
		EvalDuration       int64  `json:"eval_duration"`
	}
	var r line
	written := int64(0)
	// Say how long the reply will be before it starts, so a live view can
	// size the race while the prompt is still being read.
	progress(0, benchmarkReply)
	dec := json.NewDecoder(resp.Body)
	for {
		var l line
		err := dec.Decode(&l)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return bench.Point{}, fmt.Errorf("reading Ollama's timings: %w", err)
		}
		if l.Error != "" {
			return bench.Point{}, errors.New(l.Error)
		}
		// A model that thinks by default streams its first tokens as
		// thinking; they are written tokens all the same, and count
		// towards eval_count.
		if l.Response != "" || l.Thinking != "" {
			written++
			progress(min(written, benchmarkReply), benchmarkReply)
		}
		if l.EvalDuration > 0 {
			r = l
		}
	}
	if r.PromptEvalDuration <= 0 || r.EvalDuration <= 0 || r.EvalCount == 0 {
		return bench.Point{}, fmt.Errorf("Ollama reported no timings for %s", model)
	}
	if float64(r.PromptEvalCount) < minRead*float64(tokens) {
		return bench.Point{}, fmt.Errorf("Ollama read %s of the %s-token prompt, so the measurement would be of a shorter context",
			format.Int(r.PromptEvalCount), format.Int(tokens))
	}
	return bench.Point{
		Context:              r.PromptEvalCount,
		ReadTokensPerSecond:  float64(r.PromptEvalCount) / (float64(r.PromptEvalDuration) / 1e9),
		WriteTokensPerSecond: float64(r.EvalCount) / (float64(r.EvalDuration) / 1e9),
	}, nil
}

// benchmarkPrompt is about tokens of ordinary Go handlers, the kind of text
// an agent reads, with a random first line so no runtime has it cached.
func benchmarkPrompt(tokens int) string {
	nonce := make([]byte, 8)
	_, _ = rand.Read(nonce)
	var b strings.Builder
	b.WriteString("// " + hex.EncodeToString(nonce) + "\n")
	for i := range max(tokens/tokensPerLine, 1) {
		fmt.Fprintf(&b, "func handler%d(w http.ResponseWriter, r *http.Request) { if r.Method != \"GET\" { w.WriteHeader(%d); return }; fmt.Fprintf(w, \"ok %%d\", %d) }\n", i, 400+i%50, i)
	}
	b.WriteString("\nSummarise what these handlers do in about 300 words.")
	return b.String()
}
