package tui

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/ui/draw"
	"lo0.ai/lo0/internal/ui/report"
)

// Actions plans and applies every change the interface can make: tries,
// keeping a setup, removing and downloading models, and undoing what lo0
// changed.
type Actions interface {
	Inventory(a app.Assessment) (app.Inventory, error)
	Harnesses(ctx context.Context, a app.Assessment) []app.HarnessView
	PlanInstall(a app.Assessment, id harness.ID) (migrate.Plan, error)
	PlanTry(a app.Assessment, choice app.MigrationChoice) (migrate.Plan, error)
	PlanMigration(a app.Assessment, choice app.MigrationChoice) (migrate.Plan, error)
	PlanRemove(a app.Assessment, rt runtimes.ID, model string) (migrate.Plan, error)
	PlanPull(a app.Assessment, choice app.MigrationChoice) (migrate.Plan, error)
	PlanBenchmark(a app.Assessment, model string) (migrate.Plan, error)
	PlanUndo() (migrate.Plan, error)
	Migrate(ctx context.Context, p migrate.Plan, observe migrate.Observer) (migrate.Outcome, error)
	SessionReport(ctx context.Context, a app.Assessment, p migrate.Plan, started, ended time.Time, exit int) (app.SessionReport, error)
}

// kind is what a flow does.
type kind int

const (
	kindTry kind = iota
	// kindKeep makes a tried setup permanent.
	kindKeep
	kindRemove
	kindPull
	kindUndo
	kindInstall
	kindBenchmark
)

// stage is where a try is.
type stage int

const (
	// stagePick asks which harness to try the model in.
	stagePick stage = iota
	// stagePlan shows the plan, from lo0 try's own renderer.
	stagePlan
	stageRunning
	// stageOutcome shows a plan that failed, is waiting on the user, or
	// was a migration and is done.
	stageOutcome
	// stageReading covers the session, then waits for it to be read back.
	stageReading
	stageReport
)

// flow is a try in progress, drawn over the active screen. It is a value
// like the screens: its slices are replaced, never written in place.
type flow struct {
	active bool
	// run identifies this flow, so messages from one that was closed are
	// ignored.
	run     int
	stage   stage
	model   string
	name    string
	targets []match.TargetFit
	// cursor is the picked harness while picking and the chosen action on
	// the report; harness is the harness picked.
	cursor  int
	harness harness.ID
	// preset is true when the harness was chosen before the flow opened,
	// so there is no picker to go back to.
	preset bool
	kind   kind
	plan   migrate.Plan
	// changed is true once a plan that changes the machine has run, so the
	// machine is scanned again when the flow closes.
	changed bool
	err     error
	states  []migrate.State
	// step is the running step and done and total its progress.
	step        int
	done, total int64
	outcome     migrate.Outcome
	started     time.Time
	// ended is true once the harness has given the terminal back.
	ended  bool
	report app.SessionReport
	scroll int
	cancel context.CancelFunc
	// stopping is true while lo0 asks whether to stop a running plan, so
	// one stray esc never throws away a long download.
	stopping bool
	// stopped is true once the user stopped the plan.
	stopped bool
	// stepStarted is when the running step began and now the time of the
	// latest race tick, for the live race drawn while a model is measured.
	stepStarted, now time.Time
	// firstToken is when the measured model began writing, which is when
	// the hosted models in the race start too.
	firstToken time.Time
	ticking    bool
}

// raceTickMsg redraws the live race while a measurement runs.
type raceTickMsg struct{ run int }

// raceFrame is how often the live race is redrawn.
const raceFrame = 100 * time.Millisecond

func raceTick(run int) tea.Cmd {
	return tea.Tick(raceFrame, func(time.Time) tea.Msg { return raceTickMsg{run: run} })
}

// measuring reports whether the running step measures a model's speed.
func (f flow) measuring() bool {
	return f.stage == stageRunning && f.step < len(f.plan.Steps) && f.plan.Steps[f.step].Kind == migrate.KindMeasure &&
		f.step < len(f.states) && f.states[f.step] == migrate.StateRunning
}

// pullMsg asks the root to plan downloading a catalogue model, and
// benchmarkMsg measuring a downloaded one.
type (
	pullMsg      struct{ model string }
	benchmarkMsg struct{ model string }
)

// tryMsg asks the root to try a catalogue model.
type tryMsg struct{ model string }

func try(model string) tea.Cmd {
	return func() tea.Msg { return tryMsg{model: model} }
}

type stepMsg struct {
	run   int
	event migrate.Event
	next  tea.Cmd
}

type appliedMsg struct {
	run     int
	outcome migrate.Outcome
	err     error
}

type sessionEndedMsg struct {
	run   int
	ended time.Time
	err   error
}

type sessionReportMsg struct {
	run    int
	report app.SessionReport
}

// canTry reports whether the interface can run tries.
func (m Model) canTry() bool {
	return m.opts.Actions != nil && m.opts.Session != nil && m.opts.Now != nil
}

// startTry opens the flow for a model, asking for the harness when there
// is more than one, with lo0's recommendation first.
func (m Model) startTry(id string) (tea.Model, tea.Cmd) {
	if !m.canTry() || m.try.active {
		return m, nil
	}
	name := id
	for _, c := range m.assessment.Match.Candidates {
		if c.Model.ID == id {
			name = c.Model.Name
		}
	}
	targets := m.assessment.Match.Targets
	cursor := 0
	for i, t := range targets {
		if t.ID == m.assessment.Match.TargetID {
			cursor = i
		}
	}
	m.try = flow{active: true, run: m.try.run + 1, stage: stagePick, model: id, name: name, targets: targets, cursor: cursor}
	if len(targets) == 1 {
		return m.planTry()
	}
	return m, nil
}

// startTryIn tries the best model in a harness the user has already
// chosen, so there is no harness to pick.
func (m Model) startTryIn(h harness.ID) (tea.Model, tea.Cmd) {
	best, ok := m.assessment.Match.Best()
	if !m.canTry() || m.try.active || !ok {
		return m, nil
	}
	m.try = flow{
		active: true, run: m.try.run + 1, stage: stagePlan, model: best.Model.ID, name: best.Model.Name,
		targets: m.assessment.Match.Targets, harness: h, preset: true,
	}
	return m.planTry()
}

func (m Model) choice() app.MigrationChoice {
	return app.MigrationChoice{Harness: m.try.harness, Model: m.try.model}
}

// harnessName names the harness picked.
func (f flow) harnessName() string {
	for _, t := range f.targets {
		if t.ID == f.harness {
			return t.Name
		}
	}
	return ""
}

// planTry plans the try. Planning only reads a few small files, so it runs
// at once.
func (m Model) planTry() (tea.Model, tea.Cmd) {
	if m.try.stage == stagePick && m.try.cursor < len(m.try.targets) {
		m.try.harness = m.try.targets[m.try.cursor].ID
	}
	plan, err := m.opts.Actions.PlanTry(m.assessment, m.choice())
	return m.planned(kindTry, plan, err)
}

// planned shows a plan of kind k.
func (m Model) planned(k kind, plan migrate.Plan, err error) (tea.Model, tea.Cmd) {
	m.try.active = true
	m.try.stage, m.try.kind, m.try.plan, m.try.err, m.try.scroll = stagePlan, k, plan, err, 0
	return m, nil
}

// startPlan opens the flow straight on a plan, for changes that need no
// harness: removing or downloading a model, or undoing.
func (m Model) startPlan(k kind, name string, plan migrate.Plan, err error) (tea.Model, tea.Cmd) {
	if m.opts.Actions == nil || m.try.active {
		return m, nil
	}
	m.try = flow{run: m.try.run + 1, name: name}
	return m.planned(k, plan, err)
}

// planKeep plans the migration that makes the tried setup permanent.
func (m Model) planKeep() (tea.Model, tea.Cmd) {
	plan, err := m.opts.Actions.PlanMigration(m.assessment, m.choice())
	return m.planned(kindKeep, plan, err)
}

// apply runs the plan's steps in the background, reporting each step as
// it changes.
func (m Model) apply() (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.try.cancel = cancel
	m.try.stage = stageRunning
	m.try.states = make([]migrate.State, len(m.try.plan.Steps))
	// A benchmark changes nothing on the machine but every estimate after
	// it, so the scan runs again too.
	m.try.changed = m.try.changed || m.try.plan.Changes() || m.try.plan.Undo || m.try.kind == kindBenchmark
	events := make(chan migrate.Event, 64)
	done := make(chan appliedMsg, 1)
	trier, plan, run := m.opts.Actions, m.try.plan, m.try.run
	go func() {
		out, err := trier.Migrate(ctx, plan, func(e migrate.Event) {
			// Progress updates are dropped when the interface is behind:
			// the next one supersedes them. State changes always get
			// through.
			if e.State == migrate.StateProgress {
				select {
				case events <- e:
				default:
				}
				return
			}
			select {
			case events <- e:
			case <-ctx.Done():
			}
		})
		close(events)
		done <- appliedMsg{run: run, outcome: out, err: err}
	}()
	return m, listenSteps(run, events, done)
}

func listenSteps(run int, events <-chan migrate.Event, done <-chan appliedMsg) tea.Cmd {
	return func() tea.Msg {
		if e, ok := <-events; ok {
			return stepMsg{run: run, event: e, next: listenSteps(run, events, done)}
		}
		return <-done
	}
}

// launch hands the terminal to the harness. Bubble Tea restores the
// interface when it exits.
func (m Model) launch() (tea.Model, tea.Cmd) {
	s := *m.try.plan.Session
	m.try.started = m.opts.Now()
	m.try.stage = stageReading
	run, now := m.try.run, m.opts.Now
	cmd := announced{
		ExecCommand: m.opts.Session(s),
		text:        "Starting " + s.Program + " with " + m.try.plan.Model + " through " + m.try.plan.RuntimeName + ". Quit it to return to lo0.\n\n",
	}
	return m, tea.Exec(&cmd, func(err error) tea.Msg { return sessionEndedMsg{run: run, ended: now(), err: err} })
}

// announced prints a line to the terminal before the harness takes it, so
// the user knows what is starting and how to come back.
type announced struct {
	tea.ExecCommand
	text string
	out  io.Writer
}

func (a *announced) SetStdout(w io.Writer) {
	a.out = w
	a.ExecCommand.SetStdout(w)
}

func (a *announced) Run() error {
	if a.out != nil {
		_, _ = io.WriteString(a.out, a.text)
	}
	return a.ExecCommand.Run()
}

// exitCoder is the error a program that exited unsuccessfully returns.
type exitCoder interface {
	error
	ExitCode() int
}

// readBack reads the session back from the harness's logs.
func (m Model) readBack(ended time.Time, exit int) tea.Cmd {
	trier, a, plan, started, run := m.opts.Actions, m.assessment, m.try.plan, m.try.started, m.try.run
	return func() tea.Msg {
		r, _ := trier.SessionReport(context.Background(), a, plan, started, ended, exit)
		return sessionReportMsg{run: run, report: r}
	}
}

// updateTry handles the flow's messages.
func (m Model) updateTry(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tryMsg:
		next, cmd := m.startTry(msg.model)
		return next, cmd, true
	case removeMsg:
		if m.opts.Actions == nil {
			return m, nil, true
		}
		plan, err := m.opts.Actions.PlanRemove(m.assessment, msg.runtime, msg.model)
		next, cmd := m.startPlan(kindRemove, msg.model, plan, err)
		return next, cmd, true
	case pullMsg:
		if m.opts.Actions == nil {
			return m, nil, true
		}
		plan, err := m.opts.Actions.PlanPull(m.assessment, app.MigrationChoice{Model: msg.model})
		next, cmd := m.startPlan(kindPull, plan.Model, plan, err)
		return next, cmd, true
	case tryInMsg:
		next, cmd := m.startTryIn(msg.harness)
		return next, cmd, true
	case keepInMsg:
		if m.opts.Actions == nil {
			return m, nil, true
		}
		best, _ := m.assessment.Match.Best()
		plan, err := m.opts.Actions.PlanMigration(m.assessment, app.MigrationChoice{Harness: msg.harness})
		next, cmd := m.startPlan(kindKeep, best.Model.Name, plan, err)
		if n, ok := next.(Model); ok {
			n.try.harness, n.try.targets = msg.harness, m.assessment.Match.Targets
			next = n
		}
		return next, cmd, true
	case installMsg:
		if m.opts.Actions == nil {
			return m, nil, true
		}
		plan, err := m.opts.Actions.PlanInstall(m.assessment, msg.harness)
		next, cmd := m.startPlan(kindInstall, plan.HarnessName, plan, err)
		return next, cmd, true
	case benchmarkMsg:
		if m.opts.Actions == nil {
			return m, nil, true
		}
		plan, err := m.opts.Actions.PlanBenchmark(m.assessment, msg.model)
		next, cmd := m.startPlan(kindBenchmark, plan.Model, plan, err)
		return next, cmd, true
	case undoMsg:
		if m.opts.Actions == nil {
			return m, nil, true
		}
		plan, err := m.opts.Actions.PlanUndo()
		next, cmd := m.startPlan(kindUndo, "", plan, err)
		return next, cmd, true
	case stepMsg:
		if msg.run != m.try.run {
			return m, msg.next, true
		}
		e := msg.event
		if e.State == migrate.StateProgress {
			m.try.step, m.try.done, m.try.total = e.Step, e.Done, e.Total
			if e.Done > 0 && m.try.firstToken.IsZero() {
				m.try.firstToken = m.opts.Now()
			}
		} else if e.Step < len(m.try.states) {
			states := append([]migrate.State(nil), m.try.states...)
			states[e.Step] = e.State
			m.try.states, m.try.step, m.try.done, m.try.total = states, e.Step, 0, 0
			if e.State == migrate.StateRunning {
				m.try.stepStarted = m.opts.Now()
				m.try.now, m.try.firstToken = m.try.stepStarted, time.Time{}
			}
			if m.try.measuring() && !m.try.ticking {
				m.try.ticking = true
				return m, tea.Batch(msg.next, raceTick(m.try.run)), true
			}
		}
		return m, msg.next, true
	case raceTickMsg:
		if msg.run != m.try.run || !m.try.measuring() {
			m.try.ticking = false
			return m, nil, true
		}
		m.try.now = m.opts.Now()
		return m, raceTick(msg.run), true
	case appliedMsg:
		if msg.run != m.try.run {
			return m, nil, true
		}
		m.try.outcome, m.try.err = msg.outcome, msg.err
		if m.quitting {
			return m, tea.Quit, true
		}
		// A plan the user stopped can still finish its last step; it must
		// not then start the harness.
		if msg.err == nil && !m.try.stopped && msg.outcome.Complete() && m.try.kind == kindTry && m.try.plan.Session != nil {
			next, cmd := m.launch()
			return next, cmd, true
		}
		m.try.stage, m.try.scroll = stageOutcome, 0
		return m, nil, true
	case sessionEndedMsg:
		if msg.run != m.try.run {
			return m, nil, true
		}
		exit := 0
		if e, ok := errors.AsType[exitCoder](msg.err); ok {
			exit = e.ExitCode()
		} else if msg.err != nil {
			m.try.stage, m.try.err = stageOutcome, msg.err
			return m, nil, true
		}
		m.try.ended = true
		return m, m.readBack(msg.ended, exit), true
	case sessionReportMsg:
		if msg.run != m.try.run {
			return m, nil, true
		}
		m.try.stage, m.try.report, m.try.cursor, m.try.scroll = stageReport, msg.report, 0, 0
		return m, nil, true
	}
	return m, nil, false
}

// closeTry closes the flow. The scan that every screen draws from is out
// of date once a plan has changed the machine, so it runs again.
func (m Model) closeTry() (Model, tea.Cmd) {
	if m.try.cancel != nil {
		m.try.cancel()
	}
	changed := m.try.changed
	m.try = flow{run: m.try.run}
	if !changed {
		return m, nil
	}
	m.cancel()
	m.reset()
	return m, tea.Batch(m.progress.Init(), m.start())
}

func (m Model) handleTryKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	f := m.try
	switch f.stage {
	case stagePick:
		switch {
		case key.Matches(msg, k.Back):
			return m.closeTry()
		case key.Matches(msg, k.Up):
			m.try.cursor = max(f.cursor-1, 0)
		case key.Matches(msg, k.Down):
			m.try.cursor = min(f.cursor+1, len(f.targets)-1)
		case key.Matches(msg, k.Open):
			return m.planTry()
		}
	case stagePlan, stageOutcome:
		switch {
		case key.Matches(msg, k.Back):
			if f.stage == stagePlan && f.kind == kindTry && !f.preset && len(f.targets) > 1 {
				m.try.stage = stagePick
				return m, nil
			}
			return m.closeTry()
		case key.Matches(msg, k.Up):
			m.try.scroll = max(f.scroll-1, 0)
		case key.Matches(msg, k.Down):
			cw, ch := m.contentSize()
			m.try.scroll = min(f.scroll+1, maxScroll(m.tryLines(cw), ch))
		case key.Matches(msg, k.Open) && f.stage == stagePlan && f.err == nil:
			return m.apply()
		}
	case stageRunning:
		switch {
		case !key.Matches(msg, k.Back):
			m.try.stopping = false
		case !f.stopping:
			m.try.stopping = true
		case f.cancel != nil:
			m.try.stopping, m.try.stopped = false, true
			f.cancel()
		}
	case stageReading:
	case stageReport:
		switch {
		case key.Matches(msg, k.Back):
			return m.closeTry()
		case key.Matches(msg, k.Up):
			m.try.cursor = max(f.cursor-1, 0)
		case key.Matches(msg, k.Down):
			m.try.cursor = min(f.cursor+1, len(reportActions)-1)
		case key.Matches(msg, k.Open):
			switch reportActions[f.cursor] {
			case actionKeep:
				return m.planKeep()
			case actionAgain:
				return m.planTry()
			default:
				return m.closeTry()
			}
		}
	}
	return m, nil
}

type reportAction int

const (
	actionKeep reportAction = iota
	actionAgain
	actionBack
)

var reportActions = []reportAction{actionKeep, actionAgain, actionBack}

func (m Model) tryTitle() string {
	f := m.try
	in := " in " + f.harnessName()
	switch {
	case f.stage == stagePick:
		return "Try " + f.name + " in which harness?"
	case f.stage == stagePlan && f.kind == kindKeep, f.stage == stageOutcome && f.kind == kindKeep:
		return "Keep " + f.name + in
	case f.kind == kindRemove:
		return "Remove " + f.name
	case f.kind == kindPull:
		return "Download " + f.name
	case f.kind == kindUndo:
		return "Undo what lo0 changed"
	case f.kind == kindInstall:
		return "Install " + f.name
	case f.kind == kindBenchmark:
		return "Measure " + f.name + " on this machine"
	case f.stage == stageReport:
		return "How the session went"
	default:
		return "Try " + f.name + in
	}
}

func (m Model) tryView(width, height int) string {
	lines := m.tryLines(width)
	return strings.Join(lines[min(m.try.scroll, maxScroll(lines, height)):], "\n")
}

// maxScroll is how far a view of lines can scroll in height rows.
func maxScroll(lines []string, height int) int {
	return max(len(lines)-height, 0)
}

// tryLines is the try's whole view, before scrolling.
func (m Model) tryLines(width int) []string {
	f := m.try
	t := m.theme
	opts := report.Options{Width: width, Theme: t, Home: m.opts.Home, Titled: true}
	var body string
	switch f.stage {
	case stagePick:
		var b strings.Builder
		for i, target := range f.targets {
			marker := "  "
			name := t.Value.Render(padRight(target.Name, 32))
			if i == f.cursor {
				marker = t.Accent.Render("▌ ")
				name = t.Accent.Bold(true).Render(padRight(target.Name, 32))
			}
			note := format.Percent(target.Score) + " of your setup carries over"
			if target.ID == m.assessment.Match.TargetID {
				note += " · recommended"
			}
			b.WriteString(marker + name + t.Muted.Render(note) + "\n")
		}
		b.WriteString("\n" + t.Muted.Render("A try changes the runtime only. The harness gets everything else for one session."))
		body = b.String()
	case stagePlan:
		if f.err != nil {
			body = t.Bad.Render("lo0 cannot plan this: "+f.err.Error()) + "\n"
			break
		}
		body = report.RenderPlan(f.plan, opts)
	case stageRunning:
		var b strings.Builder
		for i, s := range f.plan.Steps {
			state := f.states[i]
			if state == "" {
				b.WriteString("  " + t.Muted.Render("  "+s.Title) + "\n")
				continue
			}
			line := "  " + report.StepMark(state, opts) + " " + s.Title
			if i == f.step && state == migrate.StateRunning {
				if p := report.Progress(s, f.done, f.total); p != "" {
					line += t.Muted.Render("  " + p)
				}
			}
			b.WriteString(line + "\n")
		}
		if f.measuring() {
			b.WriteString("\n" + strings.Join(m.measureRace(width), "\n") + "\n")
		}
		if f.stopping {
			b.WriteString("\n" + t.Warn.Render("Stop now?") + " " + t.Muted.Render(stopNote(f)) + "\n")
		}
		body = b.String()
	case stageOutcome:
		switch {
		case f.stopped:
			body = t.Warn.Render("Stopped.") + " " + t.Muted.Render(stoppedNote(f)) + "\n"
		case f.err != nil:
			body = t.Bad.Render(f.err.Error()) + "\n"
		}
		body += report.RenderOutcome(f.plan, f.outcome, opts)
	case stageReading:
		body = t.Muted.Render("Starting " + f.plan.Session.Program + ".")
		if f.ended {
			body = t.Muted.Render("Reading the session back from its logs.")
		}
	case stageReport:
		var b strings.Builder
		b.WriteString(report.RenderSession(f.report, opts) + "\n")
		labels := map[reportAction]string{
			actionKeep:  "Keep this setup: " + f.name + " in " + f.harnessName(),
			actionAgain: "Try it again",
			actionBack:  "Back",
		}
		for i, a := range reportActions {
			if i == f.cursor {
				b.WriteString(t.Accent.Render("› ") + t.Accent.Bold(true).Render(labels[a]) + "\n")
			} else {
				b.WriteString("  " + t.Value.Render(labels[a]) + "\n")
			}
		}
		body = b.String()
	}
	return strings.Split(strings.TrimRight(body, "\n"), "\n")
}

// measureRace draws the measurement as it runs: how long reading the prompt
// takes, then, from the first token, each hosted model the user works with
// writing the same reply at its measured speed. Reading is left out of the
// race because no real session reads that much uncached.
func (m Model) measureRace(width int) []string {
	f := m.try
	t := m.theme
	total := max(f.total, 1)
	writing := 0.0
	if !f.firstToken.IsZero() {
		writing = max(f.now.Sub(f.firstToken).Seconds(), 0)
	}
	type row struct {
		name, note string
		frac       float64
		style      lipgloss.Style
	}
	var rows []row
	for _, r := range m.assessment.Match.References {
		if r.TokensPerSecond <= 0 {
			continue
		}
		frac := min(r.TokensPerSecond*writing/float64(total), 1)
		note := format.Int(int(frac*float64(total))) + " tokens"
		switch {
		case f.firstToken.IsZero():
			note = "waits for the first token"
		case frac >= 1:
			note = "done in " + format.Seconds(float64(total)/r.TokensPerSecond)
		}
		rows = append(rows, row{r.Name + " · " + r.Harness, note, frac, t.Label})
	}
	note := "reading the prompt · " + format.Seconds(max(f.now.Sub(f.stepStarted).Seconds(), 0))
	if f.done > 0 {
		note = format.Int(int(f.done)) + " of " + format.Int(int(total)) + " tokens · " + format.Seconds(writing)
	}
	rows = append(rows, row{f.name, note, float64(f.done) / float64(total), t.Accent})
	nameW := 0
	for _, r := range rows {
		nameW = max(nameW, lipgloss.Width(r.name))
	}
	nameW = min(nameW, 24)
	cells := min(max(width-4-nameW-2-2-28, 8), 40)
	out := make([]string, 0, 1+len(rows))
	out = append(out, "  "+t.Label.Render("THE SAME REPLY, WRITTEN SIDE BY SIDE"))
	for _, r := range rows {
		filled, track := draw.Bar(r.frac, cells)
		out = append(out, "  "+t.Value.Render(padRight(ansi.Truncate(r.name, nameW, "…"), nameW))+"  "+
			r.style.Render(filled)+t.Muted.Render(track)+"  "+t.Muted.Render(r.note))
	}
	return out
}

// downloading reports whether the running step fetches a model.
func (f flow) downloading() bool {
	return f.step < len(f.plan.Steps) && f.plan.Steps[f.step].Bytes > 0
}

// stopNote says what stopping now leaves behind.
func stopNote(f flow) string {
	if f.downloading() {
		return "What has downloaded stays on disk, and downloading again picks up from there."
	}
	return "Steps already done stay done."
}

// stoppedNote says what a stopped plan left behind and how to go on.
func stoppedNote(f flow) string {
	if f.downloading() {
		return "The part already downloaded stays on disk, and downloading again picks up from there."
	}
	return "Steps already done stay done."
}

func (m Model) tryHints() []hint {
	f := m.try
	switch f.stage {
	case stagePick:
		return []hint{{"↑↓", "choose"}, {"enter", "see the plan"}, {"esc", "cancel"}}
	case stagePlan:
		if f.err != nil {
			return []hint{{"esc", "back"}}
		}
		run := "run it"
		if f.kind != kindTry {
			run = "apply it"
		}
		return []hint{{"enter", run}, {"↑↓", "scroll"}, {"esc", "back"}}
	case stageRunning:
		if f.stopping {
			return []hint{{"esc", "stop now"}, {"any other key", "keep going"}}
		}
		return []hint{{"esc", "stop"}}
	case stageReading:
		return nil
	case stageReport:
		return []hint{{"↑↓", "choose"}, {"enter", "go"}, {"esc", "back"}}
	default:
		return []hint{{"↑↓", "scroll"}, {"esc", "close"}}
	}
}
