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
			line := fmt.Sprintf("  %-16s %s %s", m.Name+":", formatValue(m.Value), m.Unit)
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
