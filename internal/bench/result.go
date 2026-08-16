// Package bench defines the common result type shared by every benchmark module.
package bench

import "fmt"

// Metric is one measured number within a benchmark's result.
type Metric struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
	// Context is an optional short note interpreting Value, e.g. "NVMe
	// SSD-class" or "efficiency: 82% of ideal". Empty when there's no
	// well-grounded way to classify the number.
	Context string `json:"context,omitempty"`
}

// Result is the outcome of running one benchmark (cpu, memory, disk, network, gpu).
type Result struct {
	Name    string            `json:"name"`
	Metrics []Metric          `json:"metrics"`
	Info    map[string]string `json:"info,omitempty"`
	Error   string            `json:"error,omitempty"`
}

// Add appends a numeric metric to the result. context is optional: pass one
// string to attach an interpretive note (see Metric.Context).
func (r *Result) Add(name string, value float64, unit string, context ...string) {
	m := Metric{Name: name, Value: value, Unit: unit}
	if len(context) > 0 {
		m.Context = context[0]
	}
	r.Metrics = append(r.Metrics, m)
}

// ProgressFunc reports a benchmark's progress through its named sub-tests
// as they start and finish, so a caller (e.g. the TUI) can show live
// per-phase status — and, once a sub-test finishes, its measured value —
// instead of just "running" for the whole benchmark. value/unit are only
// meaningful when done is true; callers that don't care pass nil, and
// benchmark packages call Emit rather than checking for nil themselves at
// every call site.
type ProgressFunc func(subtest string, done bool, value float64, unit string)

// Emit calls fn(subtest, done, value, unit) if fn is non-nil.
func Emit(fn ProgressFunc, subtest string, done bool, value float64, unit string) {
	if fn != nil {
		fn(subtest, done, value, unit)
	}
}

// AddInfo attaches a non-numeric fact to the result, e.g. a device name.
func (r *Result) AddInfo(name, value string) {
	if r.Info == nil {
		r.Info = map[string]string{}
	}
	r.Info[name] = value
}

// Fail wraps err as a failed result for the named benchmark.
func Fail(name string, err error) Result {
	return Result{Name: name, Error: err.Error()}
}

// ThreadSample is one point in a cpu benchmark's thread-count scaling
// curve: the throughput a workload achieved running on Threads goroutines.
type ThreadSample struct {
	Threads int
	Value   float64
	Unit    string
}

// CPUThreadSeries reconstructs the ordered 1..N thread-count series for a
// cpu workload (label is "hash" or "fp") from a cpu Result's
// single-core-<label> / <label>-threads-N / multi-core-<label> metrics —
// shared by every renderer (tui, HTML report) that wants to chart it,
// since a Result only carries the flat metric list, not the series shape.
func CPUThreadSeries(r Result, label string) []ThreadSample {
	byName := make(map[string]Metric, len(r.Metrics))
	for _, m := range r.Metrics {
		byName[m.Name] = m
	}

	cores := 0
	if m, ok := byName["cores"]; ok {
		cores = int(m.Value)
	}

	var out []ThreadSample
	if m, ok := byName["single-core-"+label]; ok {
		out = append(out, ThreadSample{Threads: 1, Value: m.Value, Unit: m.Unit})
	}
	for n := 2; n < cores; n *= 2 {
		if m, ok := byName[fmt.Sprintf("%s-threads-%d", label, n)]; ok {
			out = append(out, ThreadSample{Threads: n, Value: m.Value, Unit: m.Unit})
		}
	}
	if m, ok := byName["multi-core-"+label]; ok {
		out = append(out, ThreadSample{Threads: cores, Value: m.Value, Unit: m.Unit})
	}
	return out
}

// unitScales lists, for each base unit a metric might report in, the chain
// of SI-prefixed units to step through as the value crosses each 1000
// threshold. Only for display — JSON keeps the original fixed unit so
// results stay consistent to parse across runs.
var unitScales = map[string][]string{
	"MB/s":   {"MB/s", "GB/s", "TB/s"},
	"GB/s":   {"GB/s", "TB/s"},
	"IOPS":   {"IOPS", "K IOPS", "M IOPS"},
	"Mbps":   {"Mbps", "Gbps", "Tbps"},
	"MH/s":   {"MH/s", "GH/s"},
	"GFLOPS": {"GFLOPS", "TFLOPS"},
}

// Humanize steps value up through unitScales[unit] while it's >= 1000 and a
// larger unit is available, e.g. 10863 "MB/s" -> 10.86 "GB/s". Units not in
// the table are returned unchanged. Shared by every display renderer
// (table, HTML report, TUI) so they all scale the same way.
func Humanize(value float64, unit string) (scaled float64, scaledUnit string) {
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

// FormatBytes renders a byte count as a human-readable size, e.g. "16.0 GB".
func FormatBytes(b uint64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := "KMGTPE"
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), units[exp])
}
