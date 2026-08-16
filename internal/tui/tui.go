// Package tui drives gountlet's terminal UI (via bubbletea): an optional
// setup screen for picking what to run, a live progress view while
// benchmarks run, and a bar-chart summary once they're all done.
package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/james-gonzalez/gountlet/internal/bench"
)

// ErrCancelled is returned by Run/RunFull when the user quit (ctrl+c)
// before the benchmarks finished, as opposed to a real bubbletea failure
// (e.g. an incompatible terminal) — callers can tell the two apart with
// errors.Is to decide whether a fallback makes sense.
var ErrCancelled = errors.New("cancelled")

// NamedBench pairs a benchmark's display name with the function that runs
// it — callers (cmd/gountlet) build these from the selected flags so this
// package never needs to import the individual bench/* packages.
type NamedBench struct {
	Name string
	Fn   func() bench.Result
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

type entry struct {
	name   string
	fn     func() bench.Result
	status status
	result bench.Result
	start  time.Time
}

type tickMsg time.Time

type benchDoneMsg struct {
	index  int
	result bench.Result
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
	return func() tea.Msg {
		return benchDoneMsg{index: i, result: fn()}
	}
}

func tick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	}
	return m, nil
}

func (m *model) View() string {
	if m.phase == phaseSetup {
		return m.setup.View()
	}
	if m.done {
		return m.renderResults()
	}
	return m.renderProgress()
}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#2596a8"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	doneMark     = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✓")
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#2596a8"))
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#2596a8")).Bold(true)
)

func (m *model) renderProgress() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("gountlet") + dimStyle.Render(" — running benchmarks (ctrl+c to cancel)"))
	b.WriteString("\n\n")
	for _, e := range m.entries {
		switch e.status {
		case statusDone:
			fmt.Fprintf(&b, "  %s %s\n", doneMark, e.name)
		case statusRunning:
			frame := spinnerStyle.Render(spinnerFrames[m.spinnerIdx%len(spinnerFrames)])
			elapsed := time.Since(e.start).Round(time.Second)
			fmt.Fprintf(&b, "  %s %s %s\n", frame, e.name, dimStyle.Render(elapsed.String()))
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
	p := tea.NewProgram(m, tea.WithAltScreen())
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
