package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Selection is which benchmarks to run and how — the one config shape
// every entry point that can produce it converges on: CLI flags, the TUI
// setup screen, and (formerly) the plain text prompt.
type Selection struct {
	CPU, Mem, Disk, Net, GPU bool
	Duration                 time.Duration
	DiskPath, NetTarget      string
	JSON                     bool
}

const (
	rowCPU = iota
	rowMem
	rowDisk
	rowNet
	rowGPU
	rowDuration
	rowDiskPath
	rowNetTarget
	rowJSON
	rowStart
	rowCount
)

// setupState is the interactive "what should we run" screen shown before
// the live progress view, for a bare terminal invocation with nothing
// specified on the command line.
type setupState struct {
	cpu, mem, disk, net, gpu, jsonOut bool
	duration, diskPath, netTarget     textinput.Model
	cursor                            int
	editing                           bool
}

func newSetupState(defaults Selection) *setupState {
	s := &setupState{
		cpu: defaults.CPU, mem: defaults.Mem, disk: defaults.Disk, net: defaults.Net, gpu: defaults.GPU,
		jsonOut: defaults.JSON,
	}

	// bubbles' default PlaceholderStyle hardcodes a fixed ANSI gray
	// ("240"), same problem as dimStyle above — nearly invisible on a
	// light-background terminal. Faint(true) instead dims relative to
	// the terminal's own default foreground.
	placeholderStyle := lipgloss.NewStyle().Faint(true)

	s.duration = textinput.New()
	s.duration.Prompt = ""
	s.duration.Placeholder = fmt.Sprintf("%g", defaults.Duration.Seconds())
	s.duration.PlaceholderStyle = placeholderStyle
	s.duration.CharLimit = 16
	s.duration.Width = 10

	s.diskPath = textinput.New()
	s.diskPath.Prompt = ""
	s.diskPath.Placeholder = "OS temp dir"
	s.diskPath.PlaceholderStyle = placeholderStyle
	s.diskPath.SetValue(defaults.DiskPath)
	s.diskPath.CharLimit = 200
	s.diskPath.Width = 40

	s.netTarget = textinput.New()
	s.netTarget.Prompt = ""
	s.netTarget.Placeholder = "loopback self-test"
	s.netTarget.PlaceholderStyle = placeholderStyle
	s.netTarget.SetValue(defaults.NetTarget)
	s.netTarget.CharLimit = 100
	s.netTarget.Width = 30

	return s
}

// Update handles one message. The returned bool reports whether the user
// activated Start.
func (s *setupState) Update(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s.updateFocused(msg), false
	}

	if s.editing {
		switch key.String() {
		case "enter":
			s.editing = false
			s.blurAll()
			return nil, false
		case "esc", "up", "down":
			s.editing = false
			s.blurAll()
			if key.String() == "up" {
				s.cursor = prevRow(s.cursor)
			} else if key.String() == "down" {
				s.cursor = nextRow(s.cursor)
			}
			return nil, false
		}
		return s.updateFocused(msg), false
	}

	switch key.String() {
	case "up", "k":
		s.cursor = prevRow(s.cursor)
	case "down", "j":
		s.cursor = nextRow(s.cursor)
	case " ":
		s.toggleCursor()
	case "enter":
		switch s.cursor {
		case rowDuration, rowDiskPath, rowNetTarget:
			s.editing = true
			s.focusCursor()
		case rowStart:
			return nil, true
		default:
			s.toggleCursor()
		}
	}
	return nil, false
}

func (s *setupState) updateFocused(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch s.cursor {
	case rowDuration:
		s.duration, cmd = s.duration.Update(msg)
	case rowDiskPath:
		s.diskPath, cmd = s.diskPath.Update(msg)
	case rowNetTarget:
		s.netTarget, cmd = s.netTarget.Update(msg)
	}
	return cmd
}

func (s *setupState) toggleCursor() {
	switch s.cursor {
	case rowCPU:
		s.cpu = !s.cpu
	case rowMem:
		s.mem = !s.mem
	case rowDisk:
		s.disk = !s.disk
	case rowNet:
		s.net = !s.net
	case rowGPU:
		s.gpu = !s.gpu
	case rowJSON:
		s.jsonOut = !s.jsonOut
	}
}

func (s *setupState) focusCursor() {
	switch s.cursor {
	case rowDuration:
		s.duration.Focus()
	case rowDiskPath:
		s.diskPath.Focus()
	case rowNetTarget:
		s.netTarget.Focus()
	}
}

func (s *setupState) blurAll() {
	s.duration.Blur()
	s.diskPath.Blur()
	s.netTarget.Blur()
}

func prevRow(r int) int {
	if r == 0 {
		return rowCount - 1
	}
	return r - 1
}

func nextRow(r int) int {
	if r == rowCount-1 {
		return 0
	}
	return r + 1
}

// toSelection reads the form into a Selection, falling back to running
// everything if the user unchecked every benchmark (rather than silently
// doing nothing), and to defaultDuration if the duration field doesn't
// parse as a positive number of seconds.
func (s *setupState) toSelection(defaultDuration time.Duration) Selection {
	dur := defaultDuration
	if v, err := strconv.ParseFloat(strings.TrimSpace(s.duration.Value()), 64); err == nil && v > 0 {
		dur = time.Duration(v * float64(time.Second))
	}

	sel := Selection{
		CPU: s.cpu, Mem: s.mem, Disk: s.disk, Net: s.net, GPU: s.gpu,
		Duration:  dur,
		DiskPath:  strings.TrimSpace(s.diskPath.Value()),
		NetTarget: strings.TrimSpace(s.netTarget.Value()),
		JSON:      s.jsonOut,
	}
	if !sel.CPU && !sel.Mem && !sel.Disk && !sel.Net && !sel.GPU {
		sel.CPU, sel.Mem, sel.Disk, sel.Net, sel.GPU = true, true, true, true, true
	}
	return sel
}

// lines renders the setup screen as one string per row (title/subtitle,
// checkboxes, fields, Start), plus the index of the row under the cursor —
// so ViewWindowed can keep the cursor in view on a terminal too short to
// show every row at once, rather than the terminal silently clipping
// whatever falls past its bottom edge (bubbletea's alt screen doesn't
// scroll on its own).
func (s *setupState) lines() (out []string, cursorLine int) {
	add := func(line string) { out = append(out, line) }
	mark := func(row int) {
		if s.cursor == row {
			cursorLine = len(out)
		}
	}

	add(titleStyle.Render("gountlet") + dimStyle.Render(" — choose what to run"))
	add(dimStyle.Render("space toggles · enter edits/starts · ctrl+c quits"))
	add("")

	checkboxes := []struct {
		row     int
		label   string
		checked bool
	}{
		{rowCPU, "cpu", s.cpu},
		{rowMem, "memory", s.mem},
		{rowDisk, "disk", s.disk},
		{rowNet, "network", s.net},
		{rowGPU, "gpu", s.gpu},
	}
	for _, c := range checkboxes {
		mark(c.row)
		add(s.checkbox(c.row, c.label, c.checked))
	}
	add("")

	fields := []struct {
		row   int
		label string
		input *textinput.Model
	}{
		{rowDuration, "duration, seconds", &s.duration},
		{rowDiskPath, "disk temp dir     ", &s.diskPath},
		{rowNetTarget, "network target    ", &s.netTarget},
	}
	for _, f := range fields {
		mark(f.row)
		add(s.field(f.row, f.label, f.input))
	}
	add("")

	mark(rowJSON)
	add(s.checkbox(rowJSON, "JSON output", s.jsonOut))
	add("")

	mark(rowStart)
	if s.cursor == rowStart {
		add(cursorStyle.Render("> Start"))
	} else {
		add(dimStyle.Render("  Start"))
	}
	return out, cursorLine
}

func (s *setupState) View() string {
	lines, _ := s.lines()
	return strings.Join(lines, "\n")
}

// ViewWindowed is View, but clipped to height rows (keeping the cursor's
// row visible) when the full screen doesn't fit — height <= 0 (size not
// known yet) renders unclipped. When clipped, the last row is replaced
// with a "N more above/below" hint rather than content, so it's obvious
// there's more to scroll to instead of the screen just looking short one
// option (which is exactly what a silently-clipped terminal looks like).
func (s *setupState) ViewWindowed(height int) string {
	lines, cursorLine := s.lines()
	if height <= 0 || len(lines) <= height {
		return strings.Join(lines, "\n")
	}
	if height < 2 {
		height = 2
	}
	contentHeight := height - 1
	start := cursorLine - contentHeight/2
	if start+contentHeight > len(lines) {
		start = len(lines) - contentHeight
	}
	if start < 0 {
		start = 0
	}
	end := start + contentHeight
	if end > len(lines) {
		end = len(lines)
	}

	var indicator string
	switch {
	case start > 0 && end < len(lines):
		indicator = fmt.Sprintf("↑ %d more above · ↓ %d more below", start, len(lines)-end)
	case start > 0:
		indicator = fmt.Sprintf("↑ %d more above", start)
	case end < len(lines):
		indicator = fmt.Sprintf("↓ %d more below", len(lines)-end)
	}

	out := strings.Join(lines[start:end], "\n")
	if indicator != "" {
		out += "\n" + dimStyle.Render(indicator)
	}
	return out
}

func (s *setupState) checkbox(row int, label string, checked bool) string {
	mark := "[ ]"
	if checked {
		mark = "[x]"
	}
	line := mark + " " + label
	if s.cursor == row && !s.editing {
		return cursorStyle.Render("> " + line)
	}
	return "  " + line
}

func (s *setupState) field(row int, label string, input *textinput.Model) string {
	prefix := "  "
	if s.cursor == row {
		prefix = cursorStyle.Render("> ")
	}
	return fmt.Sprintf("%s%s %s", prefix, dimStyle.Render(label+":"), input.View())
}
