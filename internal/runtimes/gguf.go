package runtimes

import (
	"cmp"
	"errors"
	"io/fs"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
)

var (
	shardSuffix  = regexp.MustCompile(`-\d{5}-of-\d{5}$`)
	quantPattern = regexp.MustCompile(`(?i)(?:^|[-_.])(I?Q\d+(?:_[A-Z0-9]+)*|BF16|F16|FP16|F32|FP8)(?:$|[-_.])`)
	paramPattern = regexp.MustCompile(`(?i)(?:^|[-_.])(\d+(?:\.\d+)?)B(?:$|[-_.])`)
)

// GGUFName is what a GGUF file name says about its model.
type GGUFName struct {
	Name         string
	Parameters   string
	Quantization string
}

// ParseGGUFName extracts the model name, parameter count and quantization
// from a GGUF file name such as "Qwen3-Coder-30B-A3B-Instruct-Q4_K_M.gguf".
// Shard suffixes like "-00001-of-00003" are dropped so that every shard of a
// model yields the same name.
func ParseGGUFName(file string) GGUFName {
	base := strings.TrimSuffix(path.Base(file), path.Ext(file))
	base = shardSuffix.ReplaceAllString(base, "")
	n := GGUFName{Name: base}
	if m := quantPattern.FindStringSubmatch(base); m != nil {
		n.Quantization = strings.ToUpper(m[1])
	}
	if m := paramPattern.FindStringSubmatch(base); m != nil {
		n.Parameters = strings.ToUpper(m[1]) + "B"
	}
	return n
}

// repoDir is the directory a file's model is grouped under. In the Hugging
// Face hub cache, models--<org>--<name>/snapshots/<revision>/, every
// revision of a repository counts as one.
func repoDir(p string) string {
	dir := path.Dir(p)
	if parent, _ := path.Split(dir); path.Base(parent) == "snapshots" {
		return path.Dir(path.Clean(parent))
	}
	return dir
}

// ScanGGUF walks fsys for GGUF models. Shards of one model are combined into a
// single entry and vision projector files are skipped. A missing root yields
// no models rather than an error, and a folder or file that cannot be read,
// such as a broken link in the Hugging Face cache, is skipped and reported.
func ScanGGUF(fsys fs.FS) ([]Model, error) {
	byName := map[string]*Model{}
	// The Hugging Face cache can hold the same file in several revisions;
	// only the first revision of each model counts towards its size.
	revision := map[string]string{}
	var errs []error
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			switch {
			case errors.Is(err, fs.ErrNotExist) && p == ".":
				return fs.SkipAll
			case p == ".":
				return err
			}
			errs = append(errs, err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.EqualFold(path.Ext(p), ".gguf") {
			return nil
		}
		if strings.HasPrefix(strings.ToLower(path.Base(p)), "mmproj") {
			return nil
		}
		// The Hugging Face hub cache links each snapshot's files to blobs,
		// so the size is the link target's.
		info, err := fs.Stat(fsys, p)
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		parsed := ParseGGUFName(p)
		key := path.Join(repoDir(p), parsed.Name)
		if first, ok := revision[key]; ok && first != path.Dir(p) {
			return nil
		}
		revision[key] = path.Dir(p)
		m, ok := byName[key]
		if !ok {
			m = &Model{
				Name:         parsed.Name,
				Parameters:   parsed.Parameters,
				Quantization: parsed.Quantization,
				Format:       FormatGGUF,
			}
			byName[key] = m
		}
		m.SizeBytes += uint64(info.Size())
		return nil
	})
	// The same file name can come from two repositories, so the repository
	// breaks ties and the order is the same on every run.
	keys := slices.SortedFunc(maps.Keys(byName), func(a, b string) int {
		return cmp.Or(cmp.Compare(byName[a].Name, byName[b].Name), cmp.Compare(a, b))
	})
	models := make([]Model, 0, len(keys))
	for _, k := range keys {
		models = append(models, *byName[k])
	}
	return models, errors.Join(err, errors.Join(errs...))
}
