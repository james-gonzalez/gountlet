package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/james-gonzalez/gountlet/internal/bench"
)

const (
	barWidth  = 28
	labelWide = 18
)

var (
	barFilled = lipgloss.NewStyle().Foreground(lipgloss.Color("#2596a8"))
	// barTrack/barLabel/barValue avoid fixed absolute grays (e.g. "240",
	// "252") in favor of the terminal's own default foreground, plain or
	// faint — an absolute gray tuned for a dark terminal can be nearly
	// invisible on a light one, since it isn't relative to the background.
	barTrack    = lipgloss.NewStyle().Faint(true)
	barLabel    = lipgloss.NewStyle()
	barValue    = lipgloss.NewStyle().Faint(true)
	sectionName = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#2596a8"))
)

// barItem is one row in a bar chart: a label, a value, and the unit that
// value's already expressed in.
type barItem struct {
	Label string
	Value float64
	Unit  string
}

// renderBars draws a horizontal bar per item, all scaled to the same max
// (the largest value among them) so relative magnitude reads correctly.
// Every item in items must already share the same unit (see groupedBars);
// that shared unit is scaled once for the whole group via bench.Humanize
// so bars stay comparable instead of each rounding independently.
func renderBars(items []barItem) string {
	if len(items) == 0 {
		return ""
	}
	items = scaleGroupUnit(items)

	maxVal := 0.0
	for _, it := range items {
		if it.Value > maxVal {
			maxVal = it.Value
		}
	}

	var b strings.Builder
	for _, it := range items {
		filled := 0
		if maxVal > 0 {
			filled = int(it.Value / maxVal * barWidth)
		}
		filled = clamp(filled, 0, barWidth)

		track := barFilled.Render(strings.Repeat("█", filled)) +
			barTrack.Render(strings.Repeat("░", barWidth-filled))
		valueText := barValue.Render(fmt.Sprintf("%s %s", formatValue(it.Value), it.Unit))
		fmt.Fprintf(&b, "  %s %s %s\n", barLabel.Render(fitLabel(it.Label, labelWide)), track, valueText)
	}
	return b.String()
}

// fitLabel pads s to width, or truncates with an ellipsis if it's longer —
// lipgloss's own Width() wraps instead of truncating, which breaks a
// fixed-column layout like this one.
func fitLabel(s string, width int) string {
	if len(s) > width {
		if width > 1 {
			return s[:width-1] + "…"
		}
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

// scaleGroupUnit applies bench.Humanize's unit scaling (MB/s -> GB/s, IOPS
// -> K IOPS, ...) to a whole group of bars at once, using the group's max
// value to pick a single target unit — so every bar in the group stays on
// the same scale, rather than each one rounding independently and ending
// up with mismatched units.
func scaleGroupUnit(items []barItem) []barItem {
	maxVal := items[0].Value
	for _, it := range items[1:] {
		if it.Value > maxVal {
			maxVal = it.Value
		}
	}
	scaledMax, scaledUnit := bench.Humanize(maxVal, items[0].Unit)
	divisor := 1.0
	if maxVal > 0 {
		divisor = maxVal / scaledMax
	}

	out := make([]barItem, len(items))
	for i, it := range items {
		out[i] = it
		out[i].Value = it.Value / divisor
		out[i].Unit = scaledUnit
	}
	return out
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func formatValue(v float64) string {
	return fmt.Sprintf("%.2f", v)
}

// cpuThreadSeries adapts bench.CPUThreadSeries's samples into barItems with
// human-readable "N threads" labels.
func cpuThreadSeries(r bench.Result, label string) []barItem {
	samples := bench.CPUThreadSeries(r, label)
	items := make([]barItem, len(samples))
	for i, s := range samples {
		plural := "s"
		if s.Threads == 1 {
			plural = ""
		}
		items[i] = barItem{Label: fmt.Sprintf("%d thread%s", s.Threads, plural), Value: s.Value, Unit: s.Unit}
	}
	return items
}

// groupedBars groups r's metrics by unit (skipping any names already
// claimed by claimed) and renders one bar chart per group — a generic
// fallback for benchmarks without a specialized series view.
func groupedBars(r bench.Result, claimed map[string]bool) string {
	order := []string{}
	groups := map[string][]barItem{}
	for _, m := range r.Metrics {
		if claimed[m.Name] {
			continue
		}
		if _, ok := groups[m.Unit]; !ok {
			order = append(order, m.Unit)
		}
		groups[m.Unit] = append(groups[m.Unit], barItem{Label: m.Name, Value: m.Value, Unit: m.Unit})
	}

	var b strings.Builder
	for _, unit := range order {
		b.WriteString(renderBars(groups[unit]))
	}
	return b.String()
}
