package backend

import (
	"cmp"
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/runtimes"
)

// LMStudio prepares models in LM Studio. LM Studio has no documented way to
// download a given repository and quantization non-interactively, so the
// download is the user's; lo0 then loads the model with the context window
// set, under a name the harness can rely on.
type LMStudio struct{}

// ID implements migrate.Backend.
func (LMStudio) ID() runtimes.ID { return runtimes.LMStudio }

// Prepare implements migrate.Backend.
func (LMStudio) Prepare(req migrate.Request, _ migrate.Protocol) (migrate.Served, []migrate.Step, error) {
	if req.Model.GGUF == "" {
		return migrate.Served{}, nil, fmt.Errorf("the catalogue has no GGUF download for %s", req.Model.Name)
	}
	rt := req.Runtime
	name := lmStudioName(req.Model, req.Context)
	served := migrate.Served{
		Runtime: runtimes.LMStudio, RuntimeName: "LM Studio", BaseURL: cmp.Or(rt.Endpoint, runtimes.DefaultLMStudioURL),
		Model: name, DisplayName: req.Model.Name, Context: req.Context, Token: "lmstudio",
	}

	var steps []migrate.Step
	switch {
	case !rt.Installed:
		steps = append(steps, manual("Install LM Studio", "Download it from https://lmstudio.ai/download and open it once."))
	case !rt.Running:
		steps = append(steps, manual("Start the LM Studio server", "Run: lms server start --port 1234",
			"Or turn on the server in the app's Developer tab."))
	}

	steps = append(steps, lmStudioModel(req, req.Model, req.Quant, req.Context, name)...)
	switch bg := req.Background; {
	case bg == nil:
	case bg.Model.GGUF == "":
		served.BackgroundNote = noDownload(bg.Model, req.Model)
	default:
		bgName := lmStudioName(bg.Model, bg.Context)
		served.Background = &migrate.ServedModel{Model: bgName, DisplayName: bg.Model.Name, Quant: bg.Quant.Name, Context: bg.Context}
		steps = append(steps, lmStudioModel(req, bg.Model, bg.Quant, bg.Context, bgName)...)
	}
	return served, steps, nil
}

// lmStudioName is the identifier lo0 loads m under. LM Studio does not
// document which characters an identifier may hold, so the name avoids the
// slash and colon Ollama's names use.
func lmStudioName(m catalog.Model, tokens int) string {
	return strings.NewReplacer("/", "-", ":", "-").Replace(LocalName(m, tokens))
}

// lmStudioModel returns the user's download of m at q and the step that
// loads it as name with a window of tokens, which records the load so undo
// unloads it.
func lmStudioModel(req migrate.Request, m catalog.Model, q catalog.Quant, tokens int, name string) []migrate.Step {
	rt := req.Runtime
	key := downloaded(rt.Models, m.GGUF, q.Name)
	download := manual("Download "+m.Name+" at "+q.Name+" in LM Studio",
		"Search for "+m.GGUF+" and download the "+q.Name+" file, "+format.Bytes(uint64(q.Bytes))+".")
	download.Bytes, download.Done = q.Bytes, key != ""

	args := []string{"load", cmp.Or(key, "<the downloaded model>"), "--context-length", strconv.Itoa(tokens), "--identifier", name}
	load := migrate.Step{
		Kind:   migrate.KindRuntime,
		Title:  "Load it as " + name + " with a " + format.Tokens(tokens) + " context window",
		Detail: []string{"lms " + strings.Join(args, " "), "LM Studio unloads models when it quits; run this again after a restart."},
		Done:   loaded(rt.Models, name),
		Run: func(ctx context.Context, fx migrate.Effects, _ func(done, total int64)) error {
			if key == "" {
				return fmt.Errorf("download %s first", m.GGUF)
			}
			if _, err := fx.Runner.Run(ctx, "lms", args...); err != nil {
				return fmt.Errorf("loading %s: %w", key, err)
			}
			return fx.Record(migrate.Change{Runtime: runtimes.LMStudio, RuntimeName: "LM Studio", Model: name, Undo: "Unload " + name + " in LM Studio"})
		},
	}
	if key == "" || !onPath(req, "lms") {
		load.Kind, load.Run = migrate.KindManual, nil
	}
	return []migrate.Step{download, load}
}

// Revert implements migrate.Backend: it unloads a model lo0 loaded. The
// download is the user's, so it stays. LM Studio unloads every model when it
// quits, so an unload that fails almost always means the model is already
// gone, and blocking the rest of the undo on it would help no one.
func (LMStudio) Revert(ctx context.Context, fx migrate.Effects, c migrate.Change) error {
	_, _ = fx.Runner.Run(ctx, "lms", "unload", c.Model)
	return ctx.Err()
}

// downloaded finds the model LM Studio has from repository repo at quant and
// returns the name LM Studio lists it under. LM Studio names downloads after
// the repository without its -GGUF suffix.
func downloaded(models []runtimes.Model, repo, quant string) string {
	base := strings.ToLower(strings.TrimSuffix(path.Base(repo), "-GGUF"))
	for _, m := range models {
		if strings.Contains(strings.ToLower(m.Name), base) && strings.EqualFold(m.Quantization, quant) {
			return m.Name
		}
	}
	return ""
}

func loaded(models []runtimes.Model, name string) bool {
	for _, m := range models {
		if m.Name == name {
			return true
		}
	}
	return false
}
