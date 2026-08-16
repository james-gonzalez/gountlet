package report

import (
	"fmt"
	"html"
	"os"
	"strings"
	"time"

	"github.com/james-gonzalez/gountlet/internal/bench"
)

// HTML writes a self-contained HTML report (inline CSS, no JS, no external
// resources) to path: a bar chart per group of same-unit metrics for every
// result, the cpu thread-scaling curves rendered as their own series
// charts, and a plain data table underneath for accessibility/completeness.
func HTML(path string, results []bench.Result) error {
	var b strings.Builder
	b.WriteString(htmlHead)
	fmt.Fprintf(&b, "<h1>gountlet report</h1>\n<p class=\"meta\">%s</p>\n", time.Now().Format("2006-01-02 15:04:05 MST"))

	for _, r := range results {
		writeResultSection(&b, r)
	}

	b.WriteString("<h2>Raw data</h2>\n")
	writeDataTable(&b, results)
	b.WriteString(htmlFoot)
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeResultSection(b *strings.Builder, r bench.Result) {
	fmt.Fprintf(b, "<section>\n<h2>%s</h2>\n", html.EscapeString(r.Name))

	if r.Error != "" {
		fmt.Fprintf(b, "<p class=\"error\">error: %s</p>\n</section>\n", html.EscapeString(r.Error))
		return
	}

	if len(r.Info) > 0 {
		b.WriteString("<dl class=\"info\">\n")
		for _, k := range sortedKeys(r.Info) {
			fmt.Fprintf(b, "<dt>%s</dt><dd>%s</dd>\n", html.EscapeString(k), html.EscapeString(r.Info[k]))
		}
		b.WriteString("</dl>\n")
	}

	if r.Name == "cpu" {
		writeSeriesChart(b, "SHA-256 hashing", bench.CPUThreadSeries(r, "hash"))
		writeSeriesChart(b, "Matrix multiply (floating point)", bench.CPUThreadSeries(r, "fp"))
		claimed := map[string]bool{"cores": true}
		for _, label := range []string{"hash", "fp"} {
			claimed["single-core-"+label] = true
			claimed["multi-core-"+label] = true
			claimed[label+"-scaling"] = true
		}
		for _, m := range r.Metrics {
			if strings.HasPrefix(m.Name, "hash-threads-") || strings.HasPrefix(m.Name, "fp-threads-") {
				claimed[m.Name] = true
			}
		}
		writeGroupedBars(b, r, claimed)
	} else {
		writeGroupedBars(b, r, nil)
	}

	b.WriteString("</section>\n")
}

// writeSeriesChart renders a cpu thread-scaling series as its own
// labeled bar chart.
func writeSeriesChart(b *strings.Builder, title string, samples []bench.ThreadSample) {
	if len(samples) == 0 {
		return
	}
	items := make([]barItem, len(samples))
	for i, s := range samples {
		plural := "s"
		if s.Threads == 1 {
			plural = ""
		}
		items[i] = barItem{Label: fmt.Sprintf("%d thread%s", s.Threads, plural), Value: s.Value, Unit: s.Unit}
	}
	fmt.Fprintf(b, "<h3>%s</h3>\n", html.EscapeString(title))
	writeBars(b, items)
}

// writeGroupedBars groups r's metrics by unit (skipping any name already
// claimed) and renders one bar chart per group.
func writeGroupedBars(b *strings.Builder, r bench.Result, claimed map[string]bool) {
	order := []string{}
	groups := map[string][]barItem{}
	for _, m := range r.Metrics {
		if claimed[m.Name] {
			continue
		}
		if _, ok := groups[m.Unit]; !ok {
			order = append(order, m.Unit)
		}
		groups[m.Unit] = append(groups[m.Unit], barItem{Label: m.Name, Value: m.Value, Unit: m.Unit, Context: m.Context})
	}
	for _, unit := range order {
		writeBars(b, groups[unit])
	}
}

// scaleGroupUnit applies report.humanize's unit scaling (MB/s -> GB/s,
// IOPS -> K IOPS, ...) to a whole group of bars at once, using the group's
// max value to pick a single target unit — so every bar in the group
// stays on the same scale, rather than each bar rounding independently
// and ending up with mismatched units.
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

type barItem struct {
	Label   string
	Value   float64
	Unit    string
	Context string
}

// writeBars renders a horizontal bar per item, all scaled to the same max
// (the largest value among them) so relative magnitude reads correctly.
// Bars use a single hue (magnitude comparison, not series identity), so no
// legend is needed — see dataviz skill: sequential/one-hue is the color
// job for "compare magnitude" charts.
func writeBars(b *strings.Builder, items []barItem) {
	if len(items) == 0 {
		return
	}
	items = scaleGroupUnit(items)

	maxVal := 0.0
	for _, it := range items {
		if it.Value > maxVal {
			maxVal = it.Value
		}
	}

	b.WriteString("<div class=\"chart\">\n")
	for _, it := range items {
		pct := 0.0
		if maxVal > 0 {
			pct = it.Value / maxVal * 100
		}
		title := ""
		if it.Context != "" {
			title = " title=\"" + html.EscapeString(it.Context) + "\""
		}
		fmt.Fprintf(b, "<div class=\"bar-row\"%s>\n", title)
		fmt.Fprintf(b, "  <span class=\"bar-label\">%s</span>\n", html.EscapeString(it.Label))
		fmt.Fprintf(b, "  <span class=\"bar-track\"><span class=\"bar-fill\" style=\"width:%.2f%%\"></span></span>\n", pct)
		fmt.Fprintf(b, "  <span class=\"bar-value\">%s %s</span>\n", formatValue(it.Value), html.EscapeString(it.Unit))
		b.WriteString("</div>\n")
	}
	b.WriteString("</div>\n")
}

func writeDataTable(b *strings.Builder, results []bench.Result) {
	b.WriteString("<table>\n<thead><tr><th>benchmark</th><th>metric</th><th>value</th><th>unit</th><th>context</th></tr></thead>\n<tbody>\n")
	for _, r := range results {
		if r.Error != "" {
			fmt.Fprintf(b, "<tr><td>%s</td><td colspan=\"4\" class=\"error\">error: %s</td></tr>\n", html.EscapeString(r.Name), html.EscapeString(r.Error))
			continue
		}
		for _, m := range r.Metrics {
			fmt.Fprintf(b, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n",
				html.EscapeString(r.Name), html.EscapeString(m.Name), formatValue(m.Value), html.EscapeString(m.Unit), html.EscapeString(m.Context))
		}
	}
	b.WriteString("</tbody>\n</table>\n")
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

const htmlHead = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>gountlet report</title>
<style>
:root {
  color-scheme: light;
  --surface:     #fcfcfb;
  --page:        #f9f9f7;
  --ink:         #0b0b0b;
  --ink-2:       #52514e;
  --ink-muted:   #898781;
  --gridline:    #e1e0d9;
  --accent:      #1f7a89;
  --accent-track:#e1e0d9;
  --danger:      #d03b3b;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    color-scheme: dark;
    --surface:     #1a1a19;
    --page:        #0d0d0d;
    --ink:         #ffffff;
    --ink-2:       #c3c2b7;
    --ink-muted:   #898781;
    --gridline:    #2c2c2a;
    --accent:      #35b0c4;
    --accent-track:#2c2c2a;
    --danger:      #e66767;
  }
}
:root[data-theme="dark"] {
  color-scheme: dark;
  --surface:     #1a1a19;
  --page:        #0d0d0d;
  --ink:         #ffffff;
  --ink-2:       #c3c2b7;
  --ink-muted:   #898781;
  --gridline:    #2c2c2a;
  --accent:      #35b0c4;
  --accent-track:#2c2c2a;
  --danger:      #e66767;
}
* { box-sizing: border-box; }
body {
  margin: 0;
  padding: 32px 24px 64px;
  background: var(--page);
  color: var(--ink);
  font: 15px/1.5 system-ui, -apple-system, "Segoe UI", sans-serif;
}
h1 { font-size: 22px; margin: 0 0 4px; }
h2 { font-size: 17px; margin: 0 0 12px; color: var(--accent); }
h3 { font-size: 13px; margin: 20px 0 8px; color: var(--ink-2); font-weight: 600; }
.meta { color: var(--ink-muted); margin: 0 0 28px; font-size: 13px; }
section {
  max-width: 720px;
  margin: 0 0 28px;
  padding: 20px 24px;
  background: var(--surface);
  border: 1px solid var(--gridline);
  border-radius: 8px;
}
.info { display: grid; grid-template-columns: max-content 1fr; gap: 2px 12px; margin: 0 0 16px; font-size: 13px; }
.info dt { color: var(--ink-muted); }
.info dd { margin: 0; color: var(--ink-2); }
.chart { margin: 0 0 8px; }
.bar-row { display: grid; grid-template-columns: 130px 1fr 110px; align-items: center; gap: 10px; padding: 3px 0; }
.bar-label { font-size: 13px; color: var(--ink-2); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.bar-track { height: 16px; background: var(--accent-track); border-radius: 4px; overflow: hidden; }
.bar-fill { display: block; height: 100%; background: var(--accent); border-radius: 4px; }
.bar-value { font-size: 13px; color: var(--ink-2); text-align: right; font-variant-numeric: tabular-nums; }
.error { color: var(--danger); }
table { max-width: 720px; width: 100%; border-collapse: collapse; font-size: 13px; }
th, td { text-align: left; padding: 6px 10px; border-bottom: 1px solid var(--gridline); font-variant-numeric: tabular-nums; }
th { color: var(--ink-muted); font-weight: 600; }
td:first-child, th:first-child { font-variant-numeric: normal; }
</style>
</head>
<body>
`

const htmlFoot = `</body>
</html>
`
