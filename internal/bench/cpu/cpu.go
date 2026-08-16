// Package cpu benchmarks CPU throughput with two workloads — SHA-256
// hashing (integer/bitwise) and dense matrix multiplication (floating
// point) — each at single-core, all-core, and a handful of intermediate
// thread counts in between.
package cpu

import (
	"crypto/sha256"
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/james-gonzalez/gountlet/internal/bench"
	"github.com/james-gonzalez/gountlet/internal/sysinfo"
)

// scalingSampleDuration is how long each intermediate thread-count sample
// runs for when building the scaling curve. Deliberately much shorter than
// the caller's duration — the curve only needs to show relative shape, not
// the same statistical rigor as the single-core/all-core endpoints either
// side of it.
const scalingSampleDuration = 500 * time.Millisecond

// hashWorker hashes a fixed-size buffer in a tight loop until stop fires,
// returning the number of hashes completed.
func hashWorker(stop <-chan struct{}) int64 {
	buf := make([]byte, 4096)
	var count int64
	var sum [32]byte
	for {
		select {
		case <-stop:
			return count
		default:
		}
		sum = sha256.Sum256(buf)
		buf[0] = sum[0] // feed output back in so the compiler can't hoist the loop away
		count++
	}
}

// matmulN is the matrix dimension for the floating-point workload: big
// enough that a single multiplication is a meaningful chunk of work, small
// enough that the stop-channel check between multiplications stays fine
// grained relative to scalingSampleDuration.
const matmulN = 128

// flopsPerMatmul is the FLOP count of one matmulN x matmulN multiplication:
// one multiply-add pair per (i,j,k) triple.
const flopsPerMatmul = 2 * matmulN * matmulN * matmulN

// matmulWorker repeatedly multiplies two matmulN x matmulN matrices until
// stop fires, returning the number of multiplications completed.
func matmulWorker(stop <-chan struct{}) int64 {
	a := randMatrix(1)
	b := randMatrix(2)
	c := make([]float64, matmulN*matmulN)
	var count int64
	for {
		select {
		case <-stop:
			return count
		default:
		}
		matmul(a, b, c)
		a[0] = c[0] // feed output back in so the compiler can't hoist the loop away
		count++
	}
}

// randMatrix fills an matmulN x matmulN matrix using a simple xorshift PRNG
// (no crypto/rand needed here).
func randMatrix(seed uint32) []float64 {
	m := make([]float64, matmulN*matmulN)
	state := seed | 1
	for i := range m {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		m[i] = float64(state) / float64(1<<32)
	}
	return m
}

// matmul computes c = a*b for matmulN x matmulN row-major matrices, using
// the ikj loop order for better cache behavior than the naive ijk.
func matmul(a, b, c []float64) {
	clear(c)
	for i := 0; i < matmulN; i++ {
		for k := 0; k < matmulN; k++ {
			aik := a[i*matmulN+k]
			row := b[k*matmulN : k*matmulN+matmulN]
			out := c[i*matmulN : i*matmulN+matmulN]
			for j := range row {
				out[j] += aik * row[j]
			}
		}
	}
}

// runWorkers runs n copies of work in parallel for duration, returning the
// summed count across all of them.
func runWorkers(work func(<-chan struct{}) int64, n int, duration time.Duration) int64 {
	stop := make(chan struct{})
	var total int64
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			atomic.AddInt64(&total, work(stop))
		}()
	}
	time.Sleep(duration)
	close(stop)
	wg.Wait()
	return total
}

// intermediateThreadCounts returns the powers of 2 strictly between 1 and
// max, for sampling a thread-scaling curve without repeating the
// single-core/all-core endpoints the caller measures separately.
func intermediateThreadCounts(upTo int) []int {
	var counts []int
	for n := 2; n < upTo; n *= 2 {
		counts = append(counts, n)
	}
	return counts
}

// addWorkload measures work's throughput at 1 thread and at maxThreads
// threads (each for the full duration), plus an intermediate thread-count
// scaling curve (each sampled for the much shorter scalingSampleDuration),
// adding all of it to res as "single-core-<label>", "multi-core-<label>",
// "<label>-scaling", and "<label>-threads-<n>" metrics.
func addWorkload(res *bench.Result, label, unit string, work func(<-chan struct{}) int64, rate func(count int64, d time.Duration) float64, maxThreads int, duration time.Duration) {
	single := rate(runWorkers(work, 1, duration), duration)
	multi := rate(runWorkers(work, maxThreads, duration), duration)
	scaling := multi / single
	efficiency := scaling / float64(maxThreads) * 100

	res.Add("single-core-"+label, single, unit)
	res.Add("multi-core-"+label, multi, unit)
	res.Add(label+"-scaling", scaling, "x", fmt.Sprintf("%.0f%% of ideal %dx linear scaling", efficiency, maxThreads))

	for _, threads := range intermediateThreadCounts(maxThreads) {
		v := rate(runWorkers(work, threads, scalingSampleDuration), scalingSampleDuration)
		res.Add(fmt.Sprintf("%s-threads-%d", label, threads), v, unit)
	}
}

// Run measures single-core, multi-core, and intermediate thread-count
// throughput for both workloads, for the given duration each (except the
// intermediate scaling-curve points, which use a fixed shorter duration).
func Run(duration time.Duration) bench.Result {
	res := bench.Result{Name: "cpu"}
	n := runtime.NumCPU()

	addWorkload(&res, "hash", "MH/s", hashWorker, func(count int64, d time.Duration) float64 {
		return float64(count) / d.Seconds() / 1e6
	}, n, duration)

	addWorkload(&res, "fp", "GFLOPS", matmulWorker, func(count int64, d time.Duration) float64 {
		return float64(count) * flopsPerMatmul / d.Seconds() / 1e9
	}, n, duration)

	res.Add("cores", float64(n), "count")

	if info := sysinfo.GetCPU(); info.Model != "" {
		res.AddInfo("model", info.Model)
		res.AddInfo("topology", strconv.Itoa(info.PhysicalCores)+" physical / "+strconv.Itoa(info.LogicalCores)+" logical")
	}
	return res
}
