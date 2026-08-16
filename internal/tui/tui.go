// Package tui drives gountlet's terminal UI (via bubbletea): an optional
// setup screen for picking what to run, a live progress view while
// benchmarks run, and a bar-chart summary once they're all done.
package tui

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"

	"github.com/james-gonzalez/gountlet/internal/bench"
)

// ErrCancelled is returned by Run/RunFull when the user quit (ctrl+c)
// before the benchmarks finished, as opposed to a real bubbletea failure
// (e.g. an incompatible terminal) — callers can tell the two apart with
// errors.Is to decide whether a fallback makes sense.
var ErrCancelled = errors.New("cancelled")

// NamedBench pairs a benchmark's display name with the function that runs
// it — callers (cmd/gountlet) build these from the selected flags so this
// package never needs to import the individual bench/* packages. Fn is
// handed a bench.ProgressFunc so it can report which sub-test it's
// currently on; the TUI uses that to show live per-phase status.
type NamedBench struct {
	Name string
	Fn   func(bench.ProgressFunc) bench.Result
}

type phase int

const (
	phaseSetup phase = iota
	phaseRunning
)

type status int

const (
	statusPending status = iota
	statusRunning
	statusDone
)

// subtest tracks one named phase within a running benchmark (e.g.
// "sequential-write" within "disk"), as reported live via bench.ProgressFunc.
// value/unit are only meaningful once status is statusDone.
type subtest struct {
	name   string
	status status
	value  float64
	unit   string
}

type entry struct {
	name   string
	fn     func(bench.ProgressFunc) bench.Result
	status status
	result bench.Result
	start  time.Time
	subs   []subtest
	subIdx map[string]int
}

type tickMsg time.Time

type benchDoneMsg struct {
	index  int
	result bench.Result
}

// subProgressMsg reports a sub-test starting or finishing within the
// benchmark at index, sent from that benchmark's goroutine via
// model.program.Send as it runs — not returned as a tea.Cmd result, since
// a single Cmd can only deliver one message once it's done.
type subProgressMsg struct {
	index int
	name  string
	done  bool
	value float64
	unit  string
}

type model struct {
	phase phase

	setup           *setupState
	build           func(Selection) []NamedBench
	defaultDuration time.Duration

	entries    []*entry
	spinnerIdx int
	done       bool
	cancelled  bool

	program       *tea.Program
	width, height int
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m *model) Init() tea.Cmd {
	if m.phase == phaseSetup {
		return nil
	}
	return m.startRunning()
}

// startRunning kicks off the first entry (or, if there's nothing to run,
// jumps straight to an empty results view) plus the spinner ticker.
func (m *model) startRunning() tea.Cmd {
	if len(m.entries) == 0 {
		m.done = true
		return nil
	}
	return tea.Batch(m.runEntry(0), tick())
}

func (m *model) runEntry(i int) tea.Cmd {
	m.entries[i].status = statusRunning
	m.entries[i].start = time.Now()
	fn := m.entries[i].fn
	program := m.program
	return func() tea.Msg {
		progress := func(name string, done bool, value float64, unit string) {
			program.Send(subProgressMsg{index: i, name: name, done: done, value: value, unit: unit})
		}
		return benchDoneMsg{index: i, result: fn(progress)}
	}
}

func tick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = wsm.Width, wsm.Height
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
		m.cancelled = true
		return m, tea.Quit
	}

	if m.phase == phaseSetup {
		return m.updateSetup(msg)
	}
	return m.updateRunning(msg)
}

func (m *model) updateSetup(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd, start := m.setup.Update(msg)
	if !start {
		return m, cmd
	}

	sel := m.setup.toSelection(m.defaultDuration)
	benches := m.build(sel)
	m.entries = make([]*entry, len(benches))
	for i, nb := range benches {
		m.entries[i] = &entry{name: nb.Name, fn: nb.Fn}
	}
	m.phase = phaseRunning
	cmd = m.startRunning()
	return m, cmd
}

func (m *model) updateRunning(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "enter":
			if m.done {
				return m, tea.Quit
			}
		}
	case tickMsg:
		if m.done {
			return m, nil
		}
		m.spinnerIdx++
		return m, tick()
	case benchDoneMsg:
		m.entries[msg.index].status = statusDone
		m.entries[msg.index].result = msg.result
		next := msg.index + 1
		if next < len(m.entries) {
			cmd := m.runEntry(next)
			return m, cmd
		}
		m.done = true
		return m, nil
	case subProgressMsg:
		e := m.entries[msg.index]
		if e.subIdx == nil {
			e.subIdx = make(map[string]int)
		}
		if idx, ok := e.subIdx[msg.name]; ok {
			if msg.done {
				e.subs[idx].status = statusDone
				e.subs[idx].value = msg.value
				e.subs[idx].unit = msg.unit
			}
		} else {
			st := subtest{name: msg.name, status: statusRunning}
			if msg.done {
				st.status, st.value, st.unit = statusDone, msg.value, msg.unit
			}
			e.subIdx[msg.name] = len(e.subs)
			e.subs = append(e.subs, st)
		}
		return m, nil
	}
	return m, nil
}

func (m *model) View() string {
	if m.phase == phaseSetup {
		return m.setup.ViewWindowed(m.height)
	}
	if m.done {
		return m.renderResults()
	}
	return m.renderProgress()
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#2596a8"))
	// dimStyle uses the terminal's own default foreground, faint rather
	// than an absolute fixed gray — a fixed ANSI color (e.g. "240") reads
	// fine on a dark terminal but can be nearly invisible on a light one,
	// since it's not relative to the background the way Faint is.
	dimStyle     = lipgloss.NewStyle().Faint(true)
	doneMark     = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✓")
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#2596a8"))
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#2596a8")).Bold(true)
)

func (m *model) renderProgress() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("gountlet") + dimStyle.Render(" — running benchmarks (ctrl+c to cancel)"))
	b.WriteString("\n\n")
	spinner := spinnerStyle.Render(spinnerFrames[m.spinnerIdx%len(spinnerFrames)])
	for _, e := range m.entries {
		switch e.status {
		case statusDone:
			fmt.Fprintf(&b, "  %s %s\n", doneMark, e.name)
		case statusRunning:
			elapsed := time.Since(e.start).Round(time.Second)
			fmt.Fprintf(&b, "  %s %s %s\n", spinner, e.name, dimStyle.Render(elapsed.String()))
			for _, s := range e.subs {
				mark := spinner
				line := dimStyle.Render(s.name)
				if s.status == statusDone {
					mark = doneMark
					if s.unit != "" {
						v, u := bench.Humanize(s.value, s.unit)
						line = dimStyle.Render(s.name+"  ") + barValue.Render(formatValue(v)+" "+u)
					}
				}
				fmt.Fprintf(&b, "      %s %s\n", mark, line)
			}
		default:
			fmt.Fprintf(&b, "  %s %s\n", dimStyle.Render("·"), dimStyle.Render(e.name))
		}
	}
	return b.String()
}

func (m *model) renderResults() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("gountlet results") + dimStyle.Render("  (q to quit)"))
	b.WriteString("\n")

	for _, e := range m.entries {
		b.WriteString("\n" + sectionName.Render("== "+e.name+" ==") + "\n")
		if e.result.Error != "" {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("error: "+e.result.Error) + "\n")
			continue
		}

		for _, k := range sortedKeys(e.result.Info) {
			fmt.Fprintf(&b, "  %s %s\n", barLabel.Render(k+":"), barValue.Render(e.result.Info[k]))
		}

		if e.name == "cpu" {
			hash := cpuThreadSeries(e.result, "hash")
			if len(hash) > 0 {
				b.WriteString(dimStyle.Render("  hash (SHA-256)") + "\n")
				b.WriteString(renderBars(hash))
			}
			fp := cpuThreadSeries(e.result, "fp")
			if len(fp) > 0 {
				b.WriteString(dimStyle.Render("  fp (matrix multiply)") + "\n")
				b.WriteString(renderBars(fp))
			}

			// Everything shown above via the two series charts, so the
			// generic fallback only needs to catch anything future
			// versions of the cpu benchmark add that isn't one of them.
			claimed := map[string]bool{"cores": true}
			for _, label := range []string{"hash", "fp"} {
				claimed["single-core-"+label] = true
				claimed["multi-core-"+label] = true
				claimed[label+"-scaling"] = true
			}
			for _, m := range e.result.Metrics {
				if strings.HasPrefix(m.Name, "hash-threads-") || strings.HasPrefix(m.Name, "fp-threads-") {
					claimed[m.Name] = true
				}
			}
			b.WriteString(groupedBars(e.result, claimed))
		} else {
			b.WriteString(groupedBars(e.result, nil))
		}
	}
	return b.String()
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// Run drives the given (already-selected) benches through the live
// dashboard — no setup screen — returning their results in the same order
// once every one has completed.
func Run(benches []NamedBench) ([]bench.Result, error) {
	entries := make([]*entry, len(benches))
	for i, nb := range benches {
		entries[i] = &entry{name: nb.Name, fn: nb.Fn}
	}
	m := &model{phase: phaseRunning, entries: entries}
	return runProgram(m)
}

// RunFull shows a setup screen first — letting the user pick benchmarks
// and options interactively, pre-filled from defaults — then drives them
// through the same live dashboard as Run. build turns the user's final
// choices into the runnable list once they hit Start; it's supplied by
// the caller so this package never needs to import the bench/* packages
// directly.
func RunFull(defaults Selection, build func(Selection) []NamedBench) (Selection, []bench.Result, error) {
	m := &model{
		phase:           phaseSetup,
		setup:           newSetupState(defaults),
		build:           build,
		defaultDuration: defaults.Duration,
	}
	results, err := runProgram(m)
	sel := defaults
	if m.setup != nil {
		sel = m.setup.toSelection(defaults.Duration)
	}
	return sel, results, err
}

func runProgram(m *model) ([]bench.Result, error) {
	// Query the terminal size directly rather than waiting on bubbletea's
	// own initial tea.WindowSizeMsg — that message isn't delivered
	// reliably on every platform/terminal, and without a height the setup
	// screen can't tell it needs to window itself around the cursor,
	// silently clipping rows past the bottom of a short terminal.
	if w, h, err := term.GetSize(os.Stdout.Fd()); err == nil {
		m.width, m.height = w, h
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	m.program = p
	final, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("tui: %w", err)
	}

	fm, ok := final.(*model)
	if !ok {
		return nil, fmt.Errorf("tui: unexpected model type")
	}
	if fm.cancelled || !fm.done {
		return nil, ErrCancelled
	}

	results := make([]bench.Result, len(fm.entries))
	for i, e := range fm.entries {
		results[i] = e.result
	}
	return results, nil
}
