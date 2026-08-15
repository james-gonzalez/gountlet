// Package bench defines the common result type shared by every benchmark module.
package bench

// Metric is one measured number within a benchmark's result.
type Metric struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

// Result is the outcome of running one benchmark (cpu, memory, disk, network, gpu).
type Result struct {
	Name    string            `json:"name"`
	Metrics []Metric          `json:"metrics"`
	Info    map[string]string `json:"info,omitempty"`
	Error   string            `json:"error,omitempty"`
}

// Add appends a numeric metric to the result.
func (r *Result) Add(name string, value float64, unit string) {
	r.Metrics = append(r.Metrics, Metric{Name: name, Value: value, Unit: unit})
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
