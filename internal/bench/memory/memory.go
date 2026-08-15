// Package memory benchmarks sequential read/write bandwidth and random
// access latency against a large in-process buffer.
package memory

import (
	"time"

	"github.com/james-gonzalez/gountlet/internal/bench"
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

	res.Add("sequential-write", writeGBs, "GB/s")
	res.Add("sequential-read", readGBs, "GB/s")
	res.Add("random-access", randomNsPerOp, "ns/op")
	return res
}

// sink keeps the compiler from proving the read/random-access loops above
// have no observable effect and eliding them.
//
//nolint:unused // intentionally write-only
var sink uint64
