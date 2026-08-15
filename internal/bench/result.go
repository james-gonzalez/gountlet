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
