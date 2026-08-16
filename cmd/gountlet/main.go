// Command gountlet runs cross-platform CPU, memory, disk, network, and GPU
// performance benchmarks.
package main

import (
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
)

// selection is which benchmarks to run and how, whether it came from flags
// or the interactive prompt.
type selection struct {
	cpu, mem, disk, net, gpu bool
	duration                 time.Duration
	diskPath, netTarget      string
	json                     bool
}

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
	)
	flag.Parse()

	if *stress && !explicitlySet("duration") {
		*duration = stressDuration
	}

	if *serveAddr != "" {
		fmt.Printf("gountlet network server listening on %s\n", *serveAddr)
		if err := network.Serve(*serveAddr); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	sel := selection{
		cpu: *runCPU, mem: *runMemory, disk: *runDisk, net: *runNetwork, gpu: *runGPU,
		duration: *duration, diskPath: *diskPath, netTarget: *netTarget, json: *asJSON,
	}
	anySelected := *all || sel.cpu || sel.mem || sel.disk || sel.net || sel.gpu

	switch {
	case anySelected:
		if *all {
			sel.cpu, sel.mem, sel.disk, sel.net, sel.gpu = true, true, true, true, true
		}
	case isTerminal(os.Stdin):
		// No flags given and we're attached to a terminal: ask instead of
		// silently defaulting to "run everything".
		sel = promptInteractive(sel)
	default:
		// No flags, not a terminal (piped/scripted): keep the old default.
		sel.cpu, sel.mem, sel.disk, sel.net, sel.gpu = true, true, true, true, true
	}

	var results []bench.Result
	if sel.cpu {
		fmt.Fprintln(os.Stderr, "running cpu benchmark...")
		results = append(results, cpu.Run(sel.duration))
	}
	if sel.mem {
		fmt.Fprintln(os.Stderr, "running memory benchmark...")
		results = append(results, memory.Run(sel.duration))
	}
	if sel.disk {
		fmt.Fprintln(os.Stderr, "running disk benchmark...")
		results = append(results, disk.Run(sel.diskPath, sel.duration))
	}
	if sel.net {
		fmt.Fprintln(os.Stderr, "running network benchmark...")
		results = append(results, network.Run(sel.netTarget, sel.duration))
	}
	if sel.gpu {
		fmt.Fprintln(os.Stderr, "running gpu benchmark...")
		results = append(results, gpu.Run(sel.duration))
	}

	if sel.json {
		if err := report.JSON(os.Stdout, results); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
	report.Table(os.Stdout, results)
}
