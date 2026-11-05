package runtimes_test

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"lo0.ai/lo0/internal/runtimes"
)

func TestParseGGUFName(t *testing.T) {
	tests := []struct {
		file string
		want runtimes.GGUFName
	}{
		{
			file: "Qwen3-Coder-30B-A3B-Instruct-Q4_K_M.gguf",
			want: runtimes.GGUFName{Name: "Qwen3-Coder-30B-A3B-Instruct-Q4_K_M", Parameters: "30B", Quantization: "Q4_K_M"},
		},
		{
			file: "models/gpt-oss-120b-F16-00002-of-00003.gguf",
			want: runtimes.GGUFName{Name: "gpt-oss-120b-F16", Parameters: "120B", Quantization: "F16"},
		},
		{
			file: "Devstral-Small-2.5-24B.IQ4_XS.gguf",
			want: runtimes.GGUFName{Name: "Devstral-Small-2.5-24B.IQ4_XS", Parameters: "24B", Quantization: "IQ4_XS"},
		},
		{
			file: "phi-4-mini-q8_0.gguf",
			want: runtimes.GGUFName{Name: "phi-4-mini-q8_0", Quantization: "Q8_0"},
		},
		{
			file: "llama-3.2-1.5B-bf16.gguf",
			want: runtimes.GGUFName{Name: "llama-3.2-1.5B-bf16", Parameters: "1.5B", Quantization: "BF16"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			if got := runtimes.ParseGGUFName(tt.file); got != tt.want {
				t.Errorf("ParseGGUFName(%q) = %+v, want %+v", tt.file, got, tt.want)
			}
		})
	}
}

func TestScanGGUF(t *testing.T) {
	fsys := fstest.MapFS{
		"unsloth/Qwen3-Coder/Qwen3-Coder-30B-A3B-Instruct-Q4_K_M.gguf": {Data: make([]byte, 30)},
		"openai/gpt-oss/gpt-oss-120b-F16-00001-of-00002.gguf":          {Data: make([]byte, 50)},
		"openai/gpt-oss/gpt-oss-120b-F16-00002-of-00002.gguf":          {Data: make([]byte, 20)},
		"google/gemma/mmproj-gemma-4-F16.gguf":                         {Data: make([]byte, 9)},
		"google/gemma/README.md":                                       {Data: []byte("docs")},
	}
	got, err := runtimes.ScanGGUF(fsys)
	if err != nil {
		t.Fatalf("ScanGGUF: %v", err)
	}
	want := []runtimes.Model{
		{Name: "Qwen3-Coder-30B-A3B-Instruct-Q4_K_M", Parameters: "30B", Quantization: "Q4_K_M", Format: runtimes.FormatGGUF, SizeBytes: 30},
		{Name: "gpt-oss-120b-F16", Parameters: "120B", Quantization: "F16", Format: runtimes.FormatGGUF, SizeBytes: 70},
	}
	if len(got) != len(want) {
		t.Fatalf("ScanGGUF = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].Name != want[i].Name || got[i].SizeBytes != want[i].SizeBytes ||
			got[i].Parameters != want[i].Parameters || got[i].Quantization != want[i].Quantization {
			t.Errorf("model %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestScanGGUFMissingRoot(t *testing.T) {
	got, err := runtimes.ScanGGUF(os.DirFS(filepath.Join(t.TempDir(), "missing")))
	if err != nil || len(got) != 0 {
		t.Fatalf("ScanGGUF on a missing root = %v, %v; want no models and no error", got, err)
	}
}
