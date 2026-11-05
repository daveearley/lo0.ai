package claude

import (
	"context"
	"slices"

	"lo0.ai/lo0/internal/migrate"
)

// Program is the Claude Code command.
const Program = "claude"

// Session implements migrate.Launcher. Claude Code reads its model and
// endpoint from documented environment variables, so a session needs no
// file: the user's settings and the claude command stay as they are.
func (Harness) Session(req migrate.Request, served migrate.Served) (migrate.Session, []migrate.Step, error) {
	var steps []migrate.Step
	if !installed(req) {
		steps = append(steps, install())
	}
	vars := environment(served)
	detail := envDetail(vars)
	for _, name := range cleared {
		detail = append(detail, name+" unset")
	}
	return migrate.Session{
		Program: Program,
		Env:     vars,
		Unset:   slices.Clone(cleared),
		Detail:  append(detail, "Set for this session only; your Claude Code settings are not changed."),
	}, steps, nil
}

func installed(req migrate.Request) bool {
	if req.LookPath == nil {
		return false
	}
	_, err := req.LookPath(Program)
	return err == nil
}

func install() migrate.Step {
	return migrate.Step{
		Kind:   migrate.KindManual,
		Title:  "Install Claude Code",
		Detail: []string{"Run: " + installScript, "Or: npm install -g @anthropic-ai/claude-code"},
		Run: func(_ context.Context, fx migrate.Effects, _ func(done, total int64)) error {
			_, err := fx.Runner.LookPath(Program)
			return err
		},
	}
}
