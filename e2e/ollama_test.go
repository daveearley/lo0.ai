package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

// fakeOllama answers every Ollama API call lo0 makes, keeps the models it
// has as state, and records each call that changes them. Pulls finish at
// once; nothing is downloaded.
type fakeOllama struct {
	t   *testing.T
	srv *httptest.Server

	mu      sync.Mutex
	models  []string
	changes []string
}

func newFakeOllama(t *testing.T, models ...string) *fakeOllama {
	o := &fakeOllama{t: t, models: slices.Clone(models)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]string{"version": "0.21.0"})
	})
	mux.HandleFunc("GET /api/tags", o.tags)
	mux.HandleFunc("POST /api/show", o.show)
	mux.HandleFunc("POST /api/pull", o.pull)
	mux.HandleFunc("POST /api/create", o.create)
	mux.HandleFunc("DELETE /api/delete", o.delete)
	mux.HandleFunc("POST /v1/messages", o.answer(map[string]any{
		"id": "msg_e2e", "type": "message", "role": "assistant",
		"content": []map[string]string{{"type": "text", "text": "ready"}},
	}))
	mux.HandleFunc("POST /v1/chat/completions", o.answer(map[string]any{
		"id": "chatcmpl-e2e", "object": "chat.completion",
		"choices": []map[string]any{{"index": 0, "message": map[string]string{"role": "assistant", "content": "ready"}}},
	}))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("fake Ollama: lo0 called %s %s, which the fake does not know; teach it", r.Method, r.URL.Path)
		http.NotFound(w, r)
	})
	o.srv = httptest.NewServer(mux)
	t.Cleanup(o.srv.Close)
	return o
}

// host is the value for OLLAMA_HOST.
func (o *fakeOllama) host() string { return strings.TrimPrefix(o.srv.URL, "http://") }

// has reports the models Ollama holds now.
func (o *fakeOllama) has() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Sorted(slices.Values(o.models))
}

// changed lists every call that added or removed a model, in order.
func (o *fakeOllama) changed() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.changes)
}

func (o *fakeOllama) tags(w http.ResponseWriter, _ *http.Request) {
	type details struct {
		Format        string `json:"format"`
		Family        string `json:"family"`
		ParameterSize string `json:"parameter_size"`
		Quantization  string `json:"quantization_level"`
		ContextLength int    `json:"context_length"`
	}
	type model struct {
		Name         string   `json:"name"`
		Model        string   `json:"model"`
		Size         int64    `json:"size"`
		Details      details  `json:"details"`
		Capabilities []string `json:"capabilities"`
	}
	var list []model
	for _, name := range o.has() {
		list = append(list, model{
			Name: name, Model: name, Size: 1_300_000_000,
			Details:      details{Format: "gguf", Family: "qwen3", ParameterSize: "2B", Quantization: "Q4_K_M", ContextLength: 262144},
			Capabilities: []string{"completion", "tools"},
		})
	}
	writeJSON(w, map[string]any{"models": list})
}

func (o *fakeOllama) show(w http.ResponseWriter, r *http.Request) {
	name, ok := o.named(w, r)
	if !ok {
		return
	}
	if !slices.Contains(o.has(), name) {
		http.Error(w, `{"error":"model not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{
		"capabilities": []string{"completion", "tools"},
		"model_info":   map[string]any{"general.architecture": "qwen3", "qwen3.context_length": 262144},
	})
}

func (o *fakeOllama) pull(w http.ResponseWriter, r *http.Request) {
	name, ok := o.named(w, r)
	if !ok {
		return
	}
	o.add("pull", name)
	stream(w,
		map[string]any{"status": "pulling manifest"},
		map[string]any{"status": "pulling", "digest": "sha256:e2e", "total": 1000, "completed": 500},
		map[string]any{"status": "pulling", "digest": "sha256:e2e", "total": 1000, "completed": 1000},
		map[string]any{"status": "success"},
	)
}

func (o *fakeOllama) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
		From  string `json:"from"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Model == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !slices.Contains(o.has(), req.From) {
		stream(w, map[string]any{"error": fmt.Sprintf("model %q not found, try pulling it first", req.From)})
		return
	}
	o.add("create", req.Model)
	stream(w, map[string]any{"status": "creating new layer"}, map[string]any{"status": "success"})
}

func (o *fakeOllama) delete(w http.ResponseWriter, r *http.Request) {
	name, ok := o.named(w, r)
	if !ok {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	i := slices.Index(o.models, name)
	if i < 0 {
		http.Error(w, `{"error":"model not found"}`, http.StatusNotFound)
		return
	}
	o.models = slices.Delete(o.models, i, i+1)
	o.changes = append(o.changes, "delete "+name)
}

// answer replies to a chat request for a model Ollama has.
func (o *fakeOllama) answer(reply map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, ok := o.named(w, r)
		if !ok {
			return
		}
		if !slices.Contains(o.has(), name) {
			http.Error(w, fmt.Sprintf(`{"error":"model %q not found"}`, name), http.StatusNotFound)
			return
		}
		writeJSON(w, reply)
	}
}

func (o *fakeOllama) add(verb, name string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !slices.Contains(o.models, name) {
		o.models = append(o.models, name)
	}
	o.changes = append(o.changes, verb+" "+name)
}

// named reads the model a request is about.
func (o *fakeOllama) named(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Model == "" {
		o.t.Errorf("fake Ollama: %s %s without a model", r.Method, r.URL.Path)
		http.Error(w, "bad request", http.StatusBadRequest)
		return "", false
	}
	return req.Model, true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// stream writes one JSON object per line, as Ollama's streaming endpoints
// do.
func stream(w http.ResponseWriter, lines ...map[string]any) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	enc := json.NewEncoder(w)
	for _, l := range lines {
		_ = enc.Encode(l)
	}
}
