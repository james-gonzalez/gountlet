// Package memory benchmarks sequential read/write bandwidth and random
// access latency against a large in-process buffer.
package memory

import (
	"fmt"
	"time"

	"github.com/james-gonzalez/gountlet/internal/bench"
	"github.com/james-gonzalez/gountlet/internal/sysinfo"
)

const bufSize = 512 << 20 // 512 MiB, large enough to blow past L2/L3 cache

// Run measures sequential write/read bandwidth and random-access latency.
func Run() bench.Result {
	res := bench.Result{Name: "memory"}

	src := make([]byte, bufSize)
	dst := make([]byte, bufSize)
	for i := range src {
		src[i] = byte(i)
	}

	// Sequential write (copy into dst).
	start := time.Now()
	const writePasses = 4
	for i := 0; i < writePasses; i++ {
		copy(dst, src)
	}
	writeElapsed := time.Since(start)
	writeGBs := float64(bufSize*writePasses) / writeElapsed.Seconds() / 1e9

	// Sequential read (sum every byte so the compiler can't skip the loop).
	start = time.Now()
	const readPasses = 4
	var sum uint64
	for i := 0; i < readPasses; i++ {
		for _, b := range dst {
			sum += uint64(b)
		}
	}
	readElapsed := time.Since(start)
	readGBs := float64(bufSize*readPasses) / readElapsed.Seconds() / 1e9

	// Random access: pointer-chase a permutation over 64-byte-strided
	// indices so each step is a fresh cache line.
	const stride = 64
	n := bufSize / stride
	idx := make([]uint32, n)
	for i := range idx {
		idx[i] = uint32(i)
	}
	// Fisher-Yates shuffle using a simple xorshift PRNG (no crypto/rand needed here).
	var rngState uint32 = 0x9e3779b9
	nextRand := func() uint32 {
		rngState ^= rngState << 13
		rngState ^= rngState >> 17
		rngState ^= rngState << 5
		return rngState
	}
	for i := n - 1; i > 0; i-- {
		j := int(nextRand()) % (i + 1)
		if j < 0 {
			j += i + 1
		}
		idx[i], idx[j] = idx[j], idx[i]
	}

	const randomOps = 4_000_000
	pos := uint32(0)
	var acc byte
	start = time.Now()
	for i := 0; i < randomOps; i++ {
		pos = idx[pos]
		acc += dst[int(pos)*stride]
	}
	randomElapsed := time.Since(start)
	randomNsPerOp := float64(randomElapsed.Nanoseconds()) / float64(randomOps)
	sink += sum + uint64(acc)

	res.Add("sequential-write", writeGBs, "GB/s", bandwidthClass(writeGBs))
	res.Add("sequential-read", readGBs, "GB/s", bandwidthClass(readGBs))
	res.Add("random-access", randomNsPerOp, "ns/op", latencyClass(randomNsPerOp))

	if info := sysinfo.GetMemory(); info.TotalBytes > 0 {
		res.AddInfo("installed", bench.FormatBytes(info.TotalBytes))
		if t := memoryType(info); t != "" {
			res.AddInfo("type", t)
		}
	}
	return res
}

// bandwidthClass buckets a single-thread copy/read bandwidth measurement.
// It's a rough magnitude classification, not a hardware-generation claim —
// a single Go goroutine won't saturate a multi-channel memory controller
// the way a STREAM-style benchmark would.
func bandwidthClass(gbs float64) string {
	switch {
	case gbs < 3:
		return "low single-thread bandwidth"
	case gbs < 10:
		return "moderate single-thread bandwidth"
	case gbs < 20:
		return "high single-thread bandwidth"
	default:
		return "very high single-thread bandwidth"
	}
}

// latencyClass buckets random-access latency against DRAM row-hit
// (~50-70ns) vs row-miss (~90-120ns) latency, which is roughly consistent
// across DDR generations.
func latencyClass(ns float64) string {
	switch {
	case ns < 70:
		return "fast, near DRAM row-hit latency"
	case ns <= 120:
		return "typical DRAM random-access range"
	default:
		return "slower than typical DRAM — possible NUMA or swap effects"
	}
}

func memoryType(info sysinfo.Memory) string {
	switch {
	case info.Type != "" && info.SpeedMHz > 0:
		return fmt.Sprintf("%s-%d", info.Type, info.SpeedMHz)
	case info.Type != "":
		return info.Type
	case info.SpeedMHz > 0:
		return fmt.Sprintf("%d MHz", info.SpeedMHz)
	default:
		return ""
	}
}

// sink keeps the compiler from proving the read/random-access loops above
// have no observable effect and eliding them.
//
//nolint:unused // intentionally write-only
var sink uint64
