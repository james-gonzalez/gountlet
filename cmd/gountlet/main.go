// Command gountlet runs cross-platform CPU, memory, disk, network, and GPU
// performance benchmarks.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/james-gonzalez/gountlet/internal/bench"
	"github.com/james-gonzalez/gountlet/internal/bench/cpu"
	"github.com/james-gonzalez/gountlet/internal/bench/disk"
	"github.com/james-gonzalez/gountlet/internal/bench/gpu"
	"github.com/james-gonzalez/gountlet/internal/bench/memory"
	"github.com/james-gonzalez/gountlet/internal/bench/network"
	"github.com/james-gonzalez/gountlet/internal/report"
	"github.com/james-gonzalez/gountlet/internal/tui"
)

// stressDuration is how long each timed benchmark runs under -stress.
const stressDuration = 5 * time.Minute

// explicitlySet reports whether the named flag was passed on the command
// line, as opposed to left at its default.
func explicitlySet(name string) bool {
	set := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

func main() {
	var (
		all        = flag.Bool("all", false, "run every benchmark")
		runCPU     = flag.Bool("cpu", false, "run CPU benchmark")
		runMemory  = flag.Bool("mem", false, "run memory benchmark")
		runDisk    = flag.Bool("disk", false, "run disk benchmark")
		runNetwork = flag.Bool("net", false, "run network benchmark")
		runGPU     = flag.Bool("gpu", false, "run GPU benchmark")
		asJSON     = flag.Bool("json", false, "output JSON instead of a table")
		duration   = flag.Duration("duration", 3*time.Second, "how long to run each timed benchmark")
		stress     = flag.Bool("stress", false, "stress test: run each timed benchmark for a long duration ("+stressDuration.String()+") instead of the default; overridden by an explicit -duration")
		diskPath   = flag.String("disk-path", "", "directory for the disk benchmark's temp file (default: OS temp dir)")
		netTarget  = flag.String("net-target", "", "gountlet net-server address to test against (default: local loopback)")
		serveAddr  = flag.String("net-serve", "", "run only a network benchmark server on this address (e.g. :9494) and block")
		tuiMode    = flag.Bool("tui", false, "show a live terminal dashboard while benchmarks run, with a bar-chart summary at the end")
		htmlPath   = flag.String("html", "", "also write an HTML report (with charts) to this path")
	)
	flag.Parse()

	if *stress && !explicitlySet("duration") {
		*duration = stressDuration
	}

	if *serveAddr != "" {
		serveNetwork(*serveAddr)
		return
	}

	sel := tui.Selection{
		CPU: *runCPU, Mem: *runMemory, Disk: *runDisk, Net: *runNetwork, GPU: *runGPU,
		Duration: *duration, DiskPath: *diskPath, NetTarget: *netTarget, JSON: *asJSON,
	}
	anySelected := *all || sel.CPU || sel.Mem || sel.Disk || sel.Net || sel.GPU

	if !anySelected && isTerminal(os.Stdin) {
		// Bare invocation in a terminal: the full graphical experience,
		// setup screen through results, all in one continuous TUI.
		runGraphical(sel, *htmlPath)
		return
	}

	sel = resolveSelection(sel, *all)
	results, err := runSelected(selectBenchmarks(sel), *tuiMode)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	writeOutputs(results, *htmlPath, sel.JSON, *tuiMode)
}

func serveNetwork(addr string) {
	fmt.Printf("gountlet network server listening on %s\n", addr)
	if err := network.Serve(addr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// resolveSelection fills in which benchmarks to run when none were named
// on the command line: either -all, or the "run everything" default for a
// piped/scripted invocation (the bare-terminal case is handled separately,
// before this is called, by the graphical setup screen instead).
func resolveSelection(sel tui.Selection, all bool) tui.Selection {
	anySelected := all || sel.CPU || sel.Mem || sel.Disk || sel.Net || sel.GPU
	if !anySelected || all {
		sel.CPU, sel.Mem, sel.Disk, sel.Net, sel.GPU = true, true, true, true, true
	}
	return sel
}

// selectBenchmarks builds the run list for sel's selected benchmarks.
func selectBenchmarks(sel tui.Selection) []tui.NamedBench {
	var selected []tui.NamedBench
	if sel.CPU {
		selected = append(selected, tui.NamedBench{Name: "cpu", Fn: func(p bench.ProgressFunc) bench.Result { return cpu.Run(sel.Duration, p) }})
	}
	if sel.Mem {
		selected = append(selected, tui.NamedBench{Name: "memory", Fn: func(p bench.ProgressFunc) bench.Result { return memory.Run(sel.Duration, p) }})
	}
	if sel.Disk {
		selected = append(selected, tui.NamedBench{Name: "disk", Fn: func(p bench.ProgressFunc) bench.Result { return disk.Run(sel.DiskPath, sel.Duration, p) }})
	}
	if sel.Net {
		selected = append(selected, tui.NamedBench{Name: "network", Fn: func(p bench.ProgressFunc) bench.Result { return network.Run(sel.NetTarget, sel.Duration, p) }})
	}
	if sel.GPU {
		selected = append(selected, tui.NamedBench{Name: "gpu", Fn: func(p bench.ProgressFunc) bench.Result { return gpu.Run(sel.Duration, p) }})
	}
	return selected
}

// runGraphical drives the full setup->progress->results TUI for a bare
// terminal invocation. If bubbletea itself can't run in this terminal, it
// falls back to the plain text prompt rather than hard-failing; a
// cancellation (ctrl+c in either) exits quietly.
func runGraphical(sel tui.Selection, htmlPath string) {
	defaults := sel
	defaults.CPU, defaults.Mem, defaults.Disk, defaults.Net, defaults.GPU = true, true, true, true, true

	finalSel, results, err := tui.RunFull(defaults, selectBenchmarks)
	if err == nil {
		writeOutputs(results, htmlPath, finalSel.JSON, true)
		return
	}
	if errors.Is(err, tui.ErrCancelled) {
		return
	}

	fmt.Fprintln(os.Stderr, "tui unavailable, falling back to plain prompt:", err)
	sel = promptInteractive(sel)
	results, err = runSelected(selectBenchmarks(sel), false)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	writeOutputs(results, htmlPath, sel.JSON, false)
}

// runSelected runs the selected benchmarks either through the live TUI
// dashboard or, plainly, one at a time with a "running X..." line each.
func runSelected(selected []tui.NamedBench, tuiMode bool) ([]bench.Result, error) {
	if tuiMode {
		return tui.Run(selected)
	}
	results := make([]bench.Result, 0, len(selected))
	for _, nb := range selected {
		fmt.Fprintf(os.Stderr, "running %s benchmark...\n", nb.Name)
		results = append(results, nb.Fn(nil))
	}
	return results, nil
}

// writeOutputs emits the HTML report (if requested) and then the
// table/JSON results — skipping the plain table when the TUI already
// displayed a results view.
func writeOutputs(results []bench.Result, htmlPath string, asJSON, tuiMode bool) {
	if htmlPath != "" {
		if err := report.HTML(htmlPath, results); err != nil {
			fmt.Fprintln(os.Stderr, "error writing html report:", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "wrote html report to %s\n", htmlPath)
	}

	if asJSON {
		if err := report.JSON(os.Stdout, results); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
	if !tuiMode {
		report.Table(os.Stdout, results)
	}
}
