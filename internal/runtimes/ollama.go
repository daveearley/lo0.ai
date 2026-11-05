package runtimes

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"lo0.ai/lo0/internal/system"
)

// DefaultOllamaURL is where Ollama listens unless OLLAMA_HOST says otherwise.
const DefaultOllamaURL = "http://127.0.0.1:11434"

// OllamaBaseURL turns an OLLAMA_HOST value into an API base URL. It accepts
// the same shapes Ollama does: empty, "host", "host:port", "0.0.0.0:port" and
// full URLs. Like Ollama, it drops any user and password in the value, so the
// URL is safe to show in reports and errors.
func OllamaBaseURL(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return DefaultOllamaURL
	}
	if !strings.Contains(host, "://") {
		host = "http://" + host
	}
	u, err := url.Parse(host)
	if err != nil || u.Hostname() == "" {
		return DefaultOllamaURL
	}
	hostname := u.Hostname()
	if hostname == "0.0.0.0" || hostname == "::" {
		hostname = "127.0.0.1"
	}
	u.Host = net.JoinHostPort(hostname, cmp.Or(u.Port(), "11434"))
	u.User = nil
	return strings.TrimSuffix(u.String(), "/")
}

// OllamaDetector finds Ollama and lists its models through the HTTP API.
type OllamaDetector struct {
	Runner  system.Runner
	Client  *http.Client
	BaseURL string
}

var ollamaVersion = regexp.MustCompile(`version is (\S+)`)

// Detect reports whether Ollama is installed and running, and which models
// it has pulled. A stopped server is not an error.
func (d OllamaDetector) Detect(ctx context.Context) (Runtime, error) {
	rt := Runtime{ID: Ollama, Name: "Ollama", Service: true}
	_, lookErr := d.Runner.LookPath("ollama")
	rt.Installed = lookErr == nil

	var v struct {
		Version string `json:"version"`
	}
	if err := requestJSON(ctx, d.Client, http.MethodGet, d.BaseURL+"/api/version", nil, &v); err == nil {
		rt.Installed, rt.Running = true, true
		rt.Version, rt.Endpoint = v.Version, d.BaseURL
	}
	if !rt.Installed {
		return rt, nil
	}
	if !rt.Running {
		if out, err := d.Runner.Run(ctx, "ollama", "--version"); err == nil {
			if m := ollamaVersion.FindSubmatch(out); m != nil {
				rt.Version = string(m[1])
			}
		}
		return rt, nil
	}
	models, err := d.models(ctx)
	rt.Models = models
	return rt, err
}

type ollamaTags struct {
	Models []struct {
		Name    string   `json:"name"`
		Size    uint64   `json:"size"`
		Caps    []string `json:"capabilities"`
		Details struct {
			Format        string `json:"format"`
			Family        string `json:"family"`
			ParameterSize string `json:"parameter_size"`
			Quantization  string `json:"quantization_level"`
			ContextLength int    `json:"context_length"`
			ParentModel   string `json:"parent_model"`
		} `json:"details"`
	} `json:"models"`
}

type ollamaShow struct {
	Capabilities []string       `json:"capabilities"`
	ModelInfo    map[string]any `json:"model_info"`
}

func (d OllamaDetector) models(ctx context.Context) ([]Model, error) {
	var tags ollamaTags
	if err := requestJSON(ctx, d.Client, http.MethodGet, d.BaseURL+"/api/tags", nil, &tags); err != nil {
		return nil, fmt.Errorf("listing Ollama models: %w", err)
	}
	models := make([]Model, 0, len(tags.Models))
	var errs []error
	for _, t := range tags.Models {
		m := Model{
			Name:          t.Name,
			Family:        t.Details.Family,
			Parameters:    t.Details.ParameterSize,
			Quantization:  t.Details.Quantization,
			Format:        Format(t.Details.Format),
			SizeBytes:     t.Size,
			Parent:        t.Details.ParentModel,
			ContextLength: t.Details.ContextLength,
			Capabilities:  ollamaCapabilities(t.Caps),
		}
		// A model /api/show cannot describe is still installed; it is
		// listed without the details and the failure becomes a warning.
		if t.Caps == nil || m.ContextLength == 0 {
			if err := d.enrich(ctx, &m); err != nil {
				errs = append(errs, err)
			}
		}
		models = append(models, m)
	}
	return models, errors.Join(errs...)
}

// enrich fills in capabilities and context length from /api/show, for Ollama
// releases whose /api/tags does not include them.
func (d OllamaDetector) enrich(ctx context.Context, m *Model) error {
	var show ollamaShow
	body, err := json.Marshal(map[string]string{"model": m.Name})
	if err != nil {
		return err
	}
	if err := requestJSON(ctx, d.Client, http.MethodPost, d.BaseURL+"/api/show", body, &show); err != nil {
		return fmt.Errorf("reading Ollama model %s: %w", m.Name, err)
	}
	if m.Capabilities == nil {
		m.Capabilities = ollamaCapabilities(show.Capabilities)
	}
	if m.ContextLength == 0 {
		for k, v := range show.ModelInfo {
			if n, ok := v.(float64); ok && strings.HasSuffix(k, ".context_length") {
				m.ContextLength = int(n)
			}
		}
	}
	return nil
}

func ollamaCapabilities(raw []string) []Capability {
	var caps []Capability
	for _, c := range raw {
		switch c {
		case "tools":
			caps = append(caps, CapabilityTools)
		case "thinking":
			caps = append(caps, CapabilityThinking)
		case "vision":
			caps = append(caps, CapabilityVision)
		}
	}
	return caps
}
