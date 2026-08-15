// Package cpu benchmarks single-core and multi-core CPU throughput using a
// SHA-256 hashing workload.
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

// Run measures single-core and multi-core SHA-256 throughput for the given
// duration each.
func Run(duration time.Duration) bench.Result {
	res := bench.Result{Name: "cpu"}

	// Single-core.
	stop := make(chan struct{})
	done := make(chan int64, 1)
	go func() { done <- hashWorker(stop) }()
	time.Sleep(duration)
	close(stop)
	singleHashes := <-done
	singleRate := float64(singleHashes) / duration.Seconds() / 1e6

	// Multi-core: one worker per logical CPU.
	n := runtime.NumCPU()
	stop = make(chan struct{})
	var total int64
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			atomic.AddInt64(&total, hashWorker(stop))
		}()
	}
	time.Sleep(duration)
	close(stop)
	wg.Wait()
	multiRate := float64(total) / duration.Seconds() / 1e6

	scaling := multiRate / singleRate
	efficiency := scaling / float64(n) * 100

	res.Add("cores", float64(n), "count")
	res.Add("single-core", singleRate, "MH/s")
	res.Add("multi-core", multiRate, "MH/s")
	res.Add("scaling", scaling, "x", fmt.Sprintf("%.0f%% of ideal %dx linear scaling", efficiency, n))

	if info := sysinfo.GetCPU(); info.Model != "" {
		res.AddInfo("model", info.Model)
		res.AddInfo("topology", strconv.Itoa(info.PhysicalCores)+" physical / "+strconv.Itoa(info.LogicalCores)+" logical")
	}
	return res
}
