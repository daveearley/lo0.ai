package web

import (
	"sync"
	"time"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/scan"
)

// snapshot is one finished scan and what lo0 concluded from it.
type snapshot struct {
	assessment app.Assessment
	inventory  app.Inventory
	harnesses  []app.HarnessView
	next       app.Overview
	at         time.Time
}

// stage is one scan stage as the progress list shows it.
type stage struct {
	Stage    scan.Stage
	Status   scan.Status
	Done     int
	Total    int
	Warnings int
	Found    scan.Found
}

// state is the latest scan and the progress of the one running, shared by
// every request.
type state struct {
	mu       sync.Mutex
	scanning bool
	snap     *snapshot
	err      error
	stages   []stage
	// waiting are the event streams to tell about progress. Each channel
	// holds one pending signal; a stream that is behind reads the latest
	// state when it catches up.
	waiting []chan struct{}
}

func newState() *state {
	return &state{stages: freshStages()}
}

func freshStages() []stage {
	out := make([]stage, len(scan.Stages))
	for i, st := range scan.Stages {
		out[i] = stage{Stage: st}
	}
	return out
}

// begin marks a scan as started and reports whether the caller should run
// it: false when one is already running.
func (s *state) begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scanning {
		return false
	}
	s.scanning, s.err, s.stages = true, nil, freshStages()
	s.notify()
	return true
}

// observe records a scan event. It is a scan.Observer.
func (s *state) observe(e scan.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.stages {
		if s.stages[i].Stage == e.Stage {
			s.stages[i] = stage{Stage: e.Stage, Status: e.Status, Done: e.Done, Total: e.Total, Warnings: e.Warnings, Found: e.Found}
		}
	}
	s.notify()
}

func (s *state) finish(snap snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scanning, s.snap = false, &snap
	s.notify()
}

func (s *state) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scanning, s.err = false, err
	s.notify()
}

// view is what a request sees: the scan in progress, the latest result and
// the last error.
type view struct {
	scanning bool
	snap     *snapshot
	err      error
	stages   []stage
}

func (s *state) view() view {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view{scanning: s.scanning, snap: s.snap, err: s.err, stages: append([]stage(nil), s.stages...)}
}

// subscribe returns a channel signalled on every change and a function
// that stops the signals.
func (s *state) subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	s.waiting = append(s.waiting, ch)
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, w := range s.waiting {
			if w == ch {
				s.waiting = append(s.waiting[:i], s.waiting[i+1:]...)
				return
			}
		}
	}
}

// notify signals every subscriber without blocking. The caller holds mu.
func (s *state) notify() {
	for _, ch := range s.waiting {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
