// Package report formats bench.Result slices as either a table or JSON.
package report

import (
	"encoding/json"
	"fmt"
	"io"
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
		for k, v := range r.Info {
			fmt.Fprintf(w, "  %-16s %s\n", k+":", v)
		}
		for _, m := range r.Metrics {
			fmt.Fprintf(w, "  %-16s %s %s\n", m.Name+":", formatValue(m.Value), m.Unit)
		}
	}
}

func formatValue(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}
