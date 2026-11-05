package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"lo0.ai/lo0/internal/catalog"
)

func TestRunBuildsCatalog(t *testing.T) {
	config, err := os.ReadFile(filepath.Join("..", "..", "testdata", "configs", "openai_gpt-oss-20b.json"))
	if err != nil {
		t.Fatal(err)
	}
	var auth string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/models/openai/gpt-oss-20b", func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"createdAt":"2025-08-04T00:00:00.000Z","safetensors":{"total":20914757184},"cardData":{"license":"apache-2.0"},"evalResults":[
			{"data":{"dataset":{"id":"harborframework/terminal-bench-2.0","task_id":"terminalbench_2"},"value":21.5,"source":{"url":"https://huggingface.co/openai/gpt-oss-20b"}}},
			{"data":{"dataset":{"id":"SWE-bench/SWE-bench_Verified","task_id":"swe_bench_%_resolved"},"value":37.4,"source":{"url":"https://huggingface.co/openai/gpt-oss-20b"}}},
			{"data":{"dataset":{"id":"SWE-bench/SWE-bench_Verified","task_id":"swe_bench_%_resolved"},"value":60.7,"source":{"url":"https://huggingface.co/openai/gpt-oss-20b"}}},
			{"data":{"dataset":{"id":"ScaleAI/SWE-bench_Pro","task_id":"SWE_Bench_Pro"},"value":9.9,"source":{"url":"https://example.com/leaderboard"}}},
			{"filename":".eval_results/broken.yaml","error":"Invalid input"}]}`))
	})
	mux.HandleFunc("GET /openai/gpt-oss-20b/resolve/main/config.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(config)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	sources := filepath.Join(dir, "sources.json")
	out := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(sources, []byte(`{"tierNotes":{"B":"general"},
		"benchmarks":[
			{"key":"swe-bench-verified","name":"SWE-bench Verified","dataset":"SWE-bench/SWE-bench_Verified","task":"swe_bench_%_resolved"},
			{"key":"swe-bench-pro","name":"SWE-bench Pro","dataset":"ScaleAI/SWE-bench_Pro","task":"SWE_Bench_Pro"},
			{"key":"terminal-bench-2.0","name":"Terminal-Bench 2.0","dataset":"harborframework/terminal-bench-2.0","task":"terminalbench_2"},
			{"key":"aider-polyglot","name":"Aider Polyglot"}],
		"models":[{"id":"gpt-oss-20b","name":"gpt-oss-20b","repo":"openai/gpt-oss-20b","ollama":["gpt-oss:20b"],"tier":"B",
			"benchmarks":[{"key":"aider-polyglot","setting":"high","score":"34.2%"}],
			"card":"https://huggingface.co/openai/gpt-oss-20b"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.Date(2026, 11, 4, 23, 0, 0, 0, time.UTC) }
	var log strings.Builder
	if err := run(t.Context(), hub{base: srv.URL, client: srv.Client(), token: "hf_test"}, sources, out, now, &log); err != nil {
		t.Fatalf("run: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	c, err := catalog.Parse(data)
	if err != nil {
		t.Fatalf("generated catalog is invalid: %v", err)
	}
	m := c.Models[0]
	if c.Generated != "2026-11-04" || m.Released != "2025-08-04" || m.License != "apache-2.0" || m.TierNote != "general" || m.Attention.Window != 128 {
		t.Errorf("catalog = %+v", c)
	}
	want := []catalog.Benchmark{
		{Key: "aider-polyglot", Name: "Aider Polyglot", Setting: "high", Score: "34.2"},
		{Key: "terminal-bench-2.0", Name: "Terminal-Bench 2.0", Score: "21.5"},
	}
	if !slices.Equal(m.Profile.Benchmarks, want) {
		t.Errorf("benchmarks = %+v, want the curated score named and the card's one unambiguous eval result added", m.Profile.Benchmarks)
	}
	if got := log.String(); got != "openai/gpt-oss-20b: added Terminal-Bench 2.0 21.5 from the card's eval results\n" {
		t.Errorf("log = %q", got)
	}
	if auth != "Bearer hf_test" {
		t.Errorf("Authorization = %q, want the HF token", auth)
	}
}

func TestRunReportsEveryFailure(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	sources := filepath.Join(dir, "sources.json")
	if err := os.WriteFile(sources, []byte(`{"models":[{"id":"a","repo":"x/a"},{"id":"b","repo":"x/b"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	err := run(t.Context(), hub{base: srv.URL, client: srv.Client()}, sources, filepath.Join(dir, "out.json"), time.Now, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "2 models failed") || !strings.Contains(err.Error(), "x/a") || !strings.Contains(err.Error(), "x/b") {
		t.Fatalf("run error = %v, want both failures listed", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "out.json")); statErr == nil {
		t.Error("a partial catalog was written")
	}
}
