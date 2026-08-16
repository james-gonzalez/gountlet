package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
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

// promptInteractive walks the user through picking which benchmarks to run
// and their options, using defaults from an already-parsed selection.
func promptInteractive(defaults selection) selection {
	r := bufio.NewReader(os.Stdin)

	fmt.Print(banner)
	fmt.Println()
	fmt.Println("gountlet interactive setup — press Enter to accept the default in [brackets]")
	fmt.Println()

	choice := prompt(r, "Benchmarks to run: [a]ll, or comma list of cpu,mem,disk,net,gpu", "all")
	sel := parseBenchChoice(choice)

	durStr := prompt(r, "Duration for timed benchmarks, in seconds", strconv.FormatFloat(defaults.duration.Seconds(), 'f', -1, 64))
	if secs, err := strconv.ParseFloat(strings.TrimSpace(durStr), 64); err == nil && secs > 0 {
		sel.duration = time.Duration(secs * float64(time.Second))
	} else {
		sel.duration = defaults.duration
	}

	if sel.disk {
		const def = "OS temp dir"
		path := prompt(r, "Disk benchmark temp dir", def)
		if path != def {
			sel.diskPath = path
		}
	}

	if sel.net {
		const def = "loopback self-test"
		target := prompt(r, "Network target host:port", def)
		if target != def {
			sel.netTarget = target
		}
	}

	jsonAns := prompt(r, "Output as JSON instead of a table? (y/N)", "N")
	sel.json = strings.EqualFold(strings.TrimSpace(jsonAns), "y")

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
func parseBenchChoice(s string) selection {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "a" || s == "all" {
		return selection{cpu: true, mem: true, disk: true, net: true, gpu: true}
	}

	var sel selection
	for _, tok := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' }) {
		switch tok {
		case "cpu", "1":
			sel.cpu = true
		case "mem", "memory", "2":
			sel.mem = true
		case "disk", "3":
			sel.disk = true
		case "net", "network", "4":
			sel.net = true
		case "gpu", "5":
			sel.gpu = true
		}
	}

	if !sel.cpu && !sel.mem && !sel.disk && !sel.net && !sel.gpu {
		// Nothing recognized: fall back to running everything rather than
		// silently doing nothing.
		return selection{cpu: true, mem: true, disk: true, net: true, gpu: true}
	}
	return sel
}
