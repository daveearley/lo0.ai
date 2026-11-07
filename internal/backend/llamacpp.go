package backend

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/runtimes"
)

// LlamaCpp prepares models for llama-server. The server downloads the model
// itself on first start and runs in the foreground, so starting it is the
// user's step; lo0 gives the exact command and checks that it is running.
type LlamaCpp struct {
	// BaseURL is where the server is to listen; empty means
	// DefaultLlamaServerURL.
	BaseURL string
}

// ID implements migrate.Backend.
func (LlamaCpp) ID() runtimes.ID { return runtimes.LlamaCpp }

// DefaultLlamaServerURL is where lo0 asks llama-server to listen. The
// command sets the port explicitly rather than relying on the server's
// default.
const DefaultLlamaServerURL = "http://127.0.0.1:8080"

// Prepare implements migrate.Backend.
func (b LlamaCpp) Prepare(req migrate.Request, _ migrate.Protocol) (migrate.Served, []migrate.Step, error) {
	if req.Model.GGUF == "" {
		return migrate.Served{}, nil, fmt.Errorf("the catalogue has no GGUF download for %s", req.Model.Name)
	}
	base := cmp.Or(b.BaseURL, DefaultLlamaServerURL)
	u, err := url.Parse(base)
	if err != nil || u.Port() == "" {
		return migrate.Served{}, nil, fmt.Errorf("llama-server address %q has no port", base)
	}
	name := LocalName(req.Model, req.Context)
	served := migrate.Served{
		Runtime: runtimes.LlamaCpp, RuntimeName: "llama.cpp", BaseURL: base,
		Model: name, DisplayName: req.Model.Name, Context: req.Context, Token: "llamacpp",
	}
	if req.Background != nil {
		// A second server would need its own address, and harnesses reach
		// one; lo0 keeps the setup to the one server the user starts.
		served.BackgroundNote = "llama-server runs one model per process, so " + req.Model.Name + " does background work too."
	}
	var steps []migrate.Step
	if !req.Runtime.Installed {
		steps = append(steps, manual("Install llama.cpp",
			"On macOS and Linux: brew install llama.cpp",
			"Or download a release from https://github.com/ggml-org/llama.cpp/releases."))
	}
	// --jinja applies the model's own chat template, which tool calls need.
	groups := [][]string{
		{"-hf", req.Model.GGUF + ":" + req.Quant.Name},
		{"--jinja", "-c", strconv.Itoa(req.Context), "--port", u.Port()},
		{"--alias", name},
	}
	detail := []string{"It downloads the model on first start. Leave it running while you work."}
	if req.ExpertLayers > 0 {
		// -ngl 99 keeps every layer on the GPU, so only experts move.
		n := strconv.Itoa(req.ExpertLayers)
		groups = append(groups, []string{"-ngl", "99", "--n-cpu-moe", n})
		detail = append(detail, "--n-cpu-moe "+n+" keeps the first "+n+" layers' experts in system memory.")
	}
	start := manual("Start llama-server with "+req.Model.Name, append(command(req.GOOS, "llama-server", groups), detail...)...)
	start.Run = func(ctx context.Context, fx migrate.Effects, _ func(done, total int64)) error {
		return serving(ctx, fx.Client, base, name)
	}
	return served, append(steps, start), nil
}

// command splits a command over lines with the shell's continuation, so it
// fits 80 columns and still pastes.
func command(goos, program string, groups [][]string) []string {
	cont := " \\"
	if goos == "windows" {
		cont = " `"
	}
	lines := make([]string, 0, 1+len(groups))
	lines = append(lines, "Run: "+program+cont)
	for i, g := range groups {
		line := "  " + strings.Join(g, " ")
		if i < len(groups)-1 {
			line += cont
		}
		lines = append(lines, line)
	}
	return lines
}

// OffloadsExperts implements migrate.ExpertOffloader.
func (LlamaCpp) OffloadsExperts() bool { return true }

// Revert implements migrate.Backend. The user starts and stops
// llama-server, so lo0 records no changes to it.
func (LlamaCpp) Revert(context.Context, migrate.Effects, migrate.Change) error {
	return nil
}

// serving checks that an OpenAI-compatible server at base lists model.
func serving(ctx context.Context, client *http.Client, base, model string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/models", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var list struct {
		Data []struct {
			ID      string   `json:"id"`
			Aliases []string `json:"aliases"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return fmt.Errorf("decoding %s/v1/models: %w", base, err)
	}
	for _, m := range list.Data {
		if m.ID == model || slices.Contains(m.Aliases, model) {
			return nil
		}
	}
	return fmt.Errorf("the server at %s does not serve %s", base, model)
}
