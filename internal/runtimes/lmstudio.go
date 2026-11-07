package runtimes

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"lo0.ai/lo0/internal/system"
)

// DefaultLMStudioURL is where the LM Studio server listens by default.
const DefaultLMStudioURL = "http://127.0.0.1:1234"

// LMStudioModelsDir returns LM Studio's default model directory.
func LMStudioModelsDir(home string) string {
	return filepath.Join(home, ".lmstudio", "models")
}

// LMStudioDetector finds LM Studio. It prefers the running server's model
// list and falls back to reading the model directory.
type LMStudioDetector struct {
	Runner  system.Runner
	Client  *http.Client
	BaseURL string
	// Models is the LM Studio model directory.
	Models fs.FS
}

type lmStudioModels struct {
	Data []struct {
		ID           string   `json:"id"`
		Type         string   `json:"type"`
		Arch         string   `json:"arch"`
		Format       string   `json:"compatibility_type"`
		Quantization string   `json:"quantization"`
		MaxContext   int      `json:"max_context_length"`
		Capabilities []string `json:"capabilities"`
	} `json:"data"`
}

// Detect implements the runtime detector contract.
func (d LMStudioDetector) Detect(ctx context.Context) (Runtime, error) {
	rt := Runtime{ID: LMStudio, Name: "LM Studio", Service: true}
	if _, err := d.Runner.LookPath("lms"); err == nil {
		rt.Installed = true
	}
	if _, err := fs.Stat(d.Models, "."); err == nil {
		rt.Installed = true
	}

	var listed lmStudioModels
	if err := requestJSON(ctx, d.Client, http.MethodGet, d.BaseURL+"/api/v0/models", nil, &listed); err == nil {
		rt.Installed, rt.Running, rt.Endpoint = true, true, d.BaseURL
		for _, m := range listed.Data {
			if m.Type == "embeddings" {
				continue
			}
			rt.Models = append(rt.Models, Model{
				Name:          m.ID,
				Family:        m.Arch,
				Parameters:    ParseGGUFName(m.ID).Parameters,
				Quantization:  m.Quantization,
				Format:        Format(m.Format),
				ContextLength: m.MaxContext,
				Capabilities:  lmStudioCapabilities(m.Type, m.Capabilities),
			})
		}
		return rt, nil
	}
	if !rt.Installed {
		return rt, nil
	}
	gguf, ggufErr := ScanGGUF(d.Models)
	mlx, mlxErr := ScanMLX(d.Models)
	rt.Models = slices.Concat(gguf, mlx)
	return rt, errors.Join(ggufErr, mlxErr)
}

func lmStudioCapabilities(kind string, raw []string) []Capability {
	var caps []Capability
	if slices.Contains(raw, "tool_use") {
		caps = append(caps, CapabilityTools)
	}
	if kind == "vlm" {
		caps = append(caps, CapabilityVision)
	}
	return caps
}

var mlxBits = regexp.MustCompile(`(?i)(?:^|[-_.])(\d+)-?bit(?:$|[-_.])`)

// ScanMLX walks fsys for MLX models: directories holding a config.json and at
// least one .safetensors file. A missing root yields no models.
func ScanMLX(fsys fs.FS) ([]Model, error) {
	sizes := map[string]uint64{}
	configs := map[string]bool{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && p == "." {
				return fs.SkipAll
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		dir := path.Dir(p)
		switch {
		case path.Base(p) == "config.json":
			configs[dir] = true
		case strings.EqualFold(path.Ext(p), ".safetensors"):
			info, err := d.Info()
			if err != nil {
				return err
			}
			sizes[dir] += uint64(info.Size())
		}
		return nil
	})
	models := make([]Model, 0, len(sizes))
	for _, dir := range slices.Sorted(func(yield func(string) bool) {
		for dir := range sizes {
			if configs[dir] && !yield(dir) {
				return
			}
		}
	}) {
		m := Model{
			Name:       dir,
			Parameters: ParseGGUFName(path.Base(dir)).Parameters,
			Format:     FormatMLX,
			SizeBytes:  sizes[dir],
		}
		if bits := mlxBits.FindStringSubmatch(path.Base(dir)); bits != nil {
			m.Quantization = bits[1] + "-bit"
		}
		models = append(models, m)
	}
	return models, err
}
