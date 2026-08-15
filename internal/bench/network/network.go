// Package network benchmarks TCP throughput. It can run a standalone
// server (Serve), or a client that measures upload+download throughput
// against a gountlet server (Run). With no target given, Run spins up a
// loopback server itself so the tool works standalone on one machine.
package network

import (
	"io"
	"net"
	"time"

	"github.com/james-gonzalez/gountlet/internal/bench"
)

const (
	chunkSize = 256 << 10
	magicUp   = 'U'
	magicDown = 'D'
)

// Serve runs a gountlet network-bench server on addr until the process
// exits. Each connection performs one upload or download test based on the
// first byte the client sends, then closes.
func Serve(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	mode := make([]byte, 1)
	if _, err := io.ReadFull(conn, mode); err != nil {
		return
	}
	switch mode[0] {
	case magicUp:
		_, _ = io.Copy(io.Discard, conn)
	case magicDown:
		buf := make([]byte, chunkSize)
		for {
			if _, err := conn.Write(buf); err != nil {
				return
			}
		}
	}
}

// Run measures upload and download throughput for duration each against
// target ("host:port"). If target is empty, a loopback server is started
// automatically so the benchmark is self-contained.
func Run(target string, duration time.Duration) bench.Result {
	res := bench.Result{Name: "network"}

	if target == "" {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return bench.Fail("network", err)
		}
		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				go handleConn(conn)
			}
		}()
		defer ln.Close()
		target = ln.Addr().String()
	}

	upMbps, err := measure(target, magicUp, duration)
	if err != nil {
		return bench.Fail("network", err)
	}
	downMbps, err := measure(target, magicDown, duration)
	if err != nil {
		return bench.Fail("network", err)
	}

	res.Add("upload", upMbps, "Mbps")
	res.Add("download", downMbps, "Mbps")
	return res
}

// measure sends mode to target, then streams data for duration and returns
// throughput in megabits/sec. For magicUp the client writes; for magicDown
// the client reads.
func measure(target string, mode byte, duration time.Duration) (float64, error) {
	conn, err := net.Dial("tcp", target)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	if _, err := conn.Write([]byte{mode}); err != nil {
		return 0, err
	}

	buf := make([]byte, chunkSize)
	var total int64
	deadline := time.Now().Add(duration)
	_ = conn.SetDeadline(deadline.Add(2 * time.Second))

	start := time.Now()
	for time.Now().Before(deadline) {
		var n int
		if mode == magicUp {
			n, err = conn.Write(buf)
		} else {
			n, err = conn.Read(buf)
		}
		if err != nil {
			break
		}
		total += int64(n)
	}
	elapsed := time.Since(start)
	if elapsed <= 0 {
		return 0, nil
	}
	return float64(total*8) / elapsed.Seconds() / 1e6, nil
}
