package migrate

import (
	"context"
	"errors"
	"fmt"
)

// State is a step's progress through Apply.
type State string

// Step states.
const (
	StateRunning State = "running"
	// StateProgress reports work done within a running step.
	StateProgress State = "progress"
	StateDone     State = "done"
	// StateAlready means the step was satisfied before Apply started.
	StateAlready State = "already"
	// StateWaiting means a manual step is still for the user to do.
	StateWaiting State = "waiting"
	// StateSkipped means the step needs a manual step that is not done.
	StateSkipped State = "skipped"
	StateFailed  State = "failed"
)

// Event reports one step's progress.
type Event struct {
	// Step indexes Plan.Steps.
	Step  int
	State State
	// Done and Total measure a running step's work, when it can.
	Done, Total int64
	Err         error
}

// Observer receives events from the goroutine running Apply.
type Observer func(Event)

// Outcome is what Apply did, one state per step.
type Outcome struct {
	States []State `json:"states"`
}

// Waiting lists the indexes of manual steps still to do.
func (o Outcome) Waiting() []int {
	var out []int
	for i, s := range o.States {
		if s == StateWaiting {
			out = append(out, i)
		}
	}
	return out
}

// Complete reports whether every step is done.
func (o Outcome) Complete() bool {
	for _, s := range o.States {
		if s != StateDone && s != StateAlready {
			return false
		}
	}
	return true
}

// Apply runs the plan's steps in order and stops at the first failure. An
// undo is the exception for runtime steps: a runtime that is not running
// must not keep the user's files from being restored, so a failed runtime
// step leaves its change in the journal for the next undo, the rest of the
// plan runs, and Apply reports the failures at the end.
//
// A manual step may have a Run that checks whether the user has done it; it
// is waiting unless that check passes. A waiting step does not stop the run,
// because file writes never depend on the runtime: they still happen, so the
// user's next run of the plan has less to do. Downloads, runtime changes and
// the final check do depend on it, so those are skipped once a manual step
// is waiting.
func Apply(ctx context.Context, p Plan, fx Effects, observe Observer) (out Outcome, err error) {
	if observe == nil {
		observe = func(Event) {}
	}
	out = Outcome{States: make([]State, len(p.Steps))}
	if fx.Lock != nil && fx.Journal.Path != "" {
		unlock, lockErr := fx.Lock(fx.Journal.lockPath())
		if lockErr != nil {
			return out, lockErr
		}
		defer func() { err = errors.Join(err, unlock()) }()
	}
	waiting := false
	var later []error
	for i, s := range p.Steps {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		set := func(st State) {
			out.States[i] = st
			observe(Event{Step: i, State: st})
		}
		switch {
		case s.Done:
			set(StateAlready)
			continue
		case s.Kind == KindManual && s.Run != nil:
			if s.Run(ctx, fx, func(int64, int64) {}) == nil {
				set(StateDone)
				continue
			}
			waiting = true
			set(StateWaiting)
			continue
		case s.Kind == KindManual || s.Run == nil:
			waiting = true
			set(StateWaiting)
			continue
		case waiting && s.Kind != KindWrite:
			set(StateSkipped)
			continue
		}
		set(StateRunning)
		err := s.Run(ctx, fx, func(done, total int64) {
			observe(Event{Step: i, State: StateProgress, Done: done, Total: total})
		})
		if err != nil {
			out.States[i] = StateFailed
			observe(Event{Step: i, State: StateFailed, Err: err})
			err = fmt.Errorf("%s: %w", s.Title, err)
			if p.Undo && s.Kind == KindRuntime && ctx.Err() == nil {
				later = append(later, err)
				continue
			}
			return out, errors.Join(append(later, err)...)
		}
		set(StateDone)
	}
	return out, errors.Join(later...)
}
