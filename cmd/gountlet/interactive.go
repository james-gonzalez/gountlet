package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/james-gonzalez/gountlet/internal/tui"
)

// isTerminal reports whether f is attached to an interactive terminal
// rather than a pipe, redirect, or file.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// promptInteractive is the plain-text fallback for when the graphical TUI
// setup screen (tui.RunFull) can't run in this terminal — walks the user
// through the same choices one line at a time.
func promptInteractive(defaults tui.Selection) tui.Selection {
	r := bufio.NewReader(os.Stdin)

	fmt.Print(banner)
	fmt.Println()
	fmt.Println("gountlet interactive setup — press Enter to accept the default in [brackets]")
	fmt.Println()

	choice := prompt(r, "Benchmarks to run: [a]ll, or comma list of cpu,mem,disk,net,gpu", "all")
	sel := parseBenchChoice(choice)

	durStr := prompt(r, "Duration for timed benchmarks, in seconds", strconv.FormatFloat(defaults.Duration.Seconds(), 'f', -1, 64))
	if secs, err := strconv.ParseFloat(strings.TrimSpace(durStr), 64); err == nil && secs > 0 {
		sel.Duration = time.Duration(secs * float64(time.Second))
	} else {
		sel.Duration = defaults.Duration
	}

	if sel.Disk {
		const def = "OS temp dir"
		path := prompt(r, "Disk benchmark temp dir", def)
		if path != def {
			sel.DiskPath = path
		}
	}

	if sel.Net {
		const def = "loopback self-test"
		target := prompt(r, "Network target host:port", def)
		if target != def {
			sel.NetTarget = target
		}
	}

	jsonAns := prompt(r, "Output as JSON instead of a table? (y/N)", "N")
	sel.JSON = strings.EqualFold(strings.TrimSpace(jsonAns), "y")

	fmt.Println()
	return sel
}

func prompt(r *bufio.Reader, label, def string) string {
	fmt.Printf("%s [%s]: ", label, def)
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

// parseBenchChoice turns a comma/space-separated list of benchmark
// names/numbers (or "all"/"a"/empty) into a selection with just the
// cpu/mem/disk/net/gpu booleans set.
func parseBenchChoice(s string) tui.Selection {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "a" || s == "all" {
		return tui.Selection{CPU: true, Mem: true, Disk: true, Net: true, GPU: true}
	}

	var sel tui.Selection
	for _, tok := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' }) {
		switch tok {
		case "cpu", "1":
			sel.CPU = true
		case "mem", "memory", "2":
			sel.Mem = true
		case "disk", "3":
			sel.Disk = true
		case "net", "network", "4":
			sel.Net = true
		case "gpu", "5":
			sel.GPU = true
		}
	}

	if !sel.CPU && !sel.Mem && !sel.Disk && !sel.Net && !sel.GPU {
		// Nothing recognized: fall back to running everything rather than
		// silently doing nothing.
		return tui.Selection{CPU: true, Mem: true, Disk: true, Net: true, GPU: true}
	}
	return sel
}
