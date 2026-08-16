// Package report formats bench.Result slices as either a table or JSON.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"

	"github.com/james-gonzalez/gountlet/internal/bench"
)

// JSON writes results to w as indented JSON.
func JSON(w io.Writer, results []bench.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

// Table writes results to w as a human-readable table.
func Table(w io.Writer, results []bench.Result) {
	for _, r := range results {
		fmt.Fprintf(w, "== %s ==\n", r.Name)
		if r.Error != "" {
			fmt.Fprintf(w, "  error: %s\n", r.Error)
			continue
		}
		keys := make([]string, 0, len(r.Info))
		for k := range r.Info {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(w, "  %-16s %s\n", k+":", r.Info[k])
		}

		for _, m := range r.Metrics {
			value, unit := humanize(m.Value, m.Unit)
			line := fmt.Sprintf("  %-16s %s %s", m.Name+":", formatValue(value), unit)
			if m.Context != "" {
				line += "  (" + m.Context + ")"
			}
			fmt.Fprintln(w, line)
		}
	}
}

func formatValue(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

// unitScales lists, for each base unit a metric might report in, the
// chain of SI-prefixed units to step through as the value crosses each
// 1000 threshold. Only for table display — JSON keeps the original fixed
// unit so results stay consistent to parse across runs.
var unitScales = map[string][]string{
	"MB/s":   {"MB/s", "GB/s", "TB/s"},
	"GB/s":   {"GB/s", "TB/s"},
	"IOPS":   {"IOPS", "K IOPS", "M IOPS"},
	"Mbps":   {"Mbps", "Gbps", "Tbps"},
	"MH/s":   {"MH/s", "GH/s"},
	"GFLOPS": {"GFLOPS", "TFLOPS"},
}

// humanize steps value up through unitScales[unit] while it's >= 1000 and
// a larger unit is available, e.g. 10863 "MB/s" -> 10.86 "GB/s". Units not
// in the table are returned unchanged.
func humanize(value float64, unit string) (scaled float64, scaledUnit string) {
	chain, ok := unitScales[unit]
	if !ok {
		return value, unit
	}
	i := 0
	for value >= 1000 && i < len(chain)-1 {
		value /= 1000
		i++
	}
	return value, chain[i]
}
