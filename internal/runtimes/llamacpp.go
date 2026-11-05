package runtimes

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"regexp"

	"lo0.ai/lo0/internal/system"
)

// LlamaCppCacheDirs returns the directories llama.cpp keeps downloaded
// models in. Since build b8498 (March 2026) -hf downloads go to the Hugging
// Face hub cache; older builds used a llama.cpp cache directory, which is
// still read for models downloaded before. LLAMA_CACHE overrides both.
func LlamaCppCacheDirs(goos, home string, getenv func(string) string) []string {
	if dir := getenv("LLAMA_CACHE"); dir != "" {
		return []string{dir}
	}
	hub := cmp.Or(getenv("HF_HUB_CACHE"), getenv("HUGGINGFACE_HUB_CACHE"))
	if hub == "" {
		hub = filepath.Join(cmp.Or(getenv("HF_HOME"), filepath.Join(cmp.Or(getenv("XDG_CACHE_HOME"), filepath.Join(home, ".cache")), "huggingface")), "hub")
	}
	var legacy string
	switch goos {
	case "darwin":
		legacy = filepath.Join(home, "Library", "Caches", "llama.cpp")
	case "windows":
		legacy = filepath.Join(cmp.Or(getenv("LOCALAPPDATA"), filepath.Join(home, "AppData", "Local")), "llama.cpp")
	default:
		legacy = filepath.Join(cmp.Or(getenv("XDG_CACHE_HOME"), filepath.Join(home, ".cache")), "llama.cpp")
	}
	return []string{hub, legacy}
}

// LlamaCppDetector finds llama.cpp binaries and the models in its cache.
type LlamaCppDetector struct {
	Runner system.Runner
	// Caches are the directories llama.cpp downloads models into.
	Caches []fs.FS
}

var llamaCppVersion = regexp.MustCompile(`version: (\d+)`)

// Detect implements the runtime detector contract.
func (d LlamaCppDetector) Detect(ctx context.Context) (Runtime, error) {
	rt := Runtime{ID: LlamaCpp, Name: "llama.cpp"}
	for _, bin := range []string{"llama-server", "llama-cli"} {
		if _, err := d.Runner.LookPath(bin); err != nil {
			continue
		}
		rt.Installed = true
		if out, err := d.Runner.Run(ctx, bin, "--version"); err == nil {
			if m := llamaCppVersion.FindSubmatch(out); m != nil {
				rt.Version = "b" + string(m[1])
			}
		}
		break
	}
	errs := make([]error, 0, len(d.Caches))
	for _, cache := range d.Caches {
		models, err := ScanGGUF(cache)
		rt.Models = append(rt.Models, models...)
		errs = append(errs, err)
	}
	return rt, errors.Join(errs...)
}
