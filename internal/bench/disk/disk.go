// Package disk benchmarks sequential read/write throughput and 4K random
// IOPS against a temp file. Writes go through a normal buffered
// write+fsync, which forces real device I/O regardless of caching. Reads
// use OS-specific cache bypass (openUncached) so they measure real device
// I/O too, rather than a page-cache hit; where that isn't available for
// the filesystem in play (e.g. tmpfs), it falls back to a normal cached
// read and says so in the result.
package disk

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/james-gonzalez/gountlet/internal/bench"
	"github.com/james-gonzalez/gountlet/internal/sysinfo"
)

const (
	fileSize  = 1 << 30 // 1 GiB cap on this benchmark's on-disk footprint, regardless of duration
	blockSize = 4 << 20 // 4 MiB sequential I/O block
	ioSize    = 4 << 10 // 4 KiB random I/O block
	minIOOps  = 20000   // floor on random I/O operations regardless of duration
)

// Run measures sequential write/read MB/s and 4K random read/write IOPS
// using a temp file under dir (os.TempDir() if dir is empty). Every phase
// runs at least once across the full fileSize, then repeats (wrapping
// around within the same fixed-size file, never growing it) until
// duration elapses, so the on-disk footprint stays bounded even under a
// long -stress run.
func Run(dir string, duration time.Duration) bench.Result {
	if dir == "" {
		dir = os.TempDir()
	}
	path := filepath.Join(dir, fmt.Sprintf("gountlet-diskbench-%d.tmp", time.Now().UnixNano()))
	defer os.Remove(path)

	block := make([]byte, blockSize)
	rand.New(rand.NewSource(1)).Read(block)

	writeMBs, err := sequentialWrite(path, block, duration)
	if err != nil {
		return bench.Fail("disk", err)
	}
	readMBs, seqUncached, err := sequentialRead(path, duration)
	if err != nil {
		return bench.Fail("disk", err)
	}

	rng := rand.New(rand.NewSource(2))
	numBlocks := int64(fileSize / ioSize)
	ioBlock := make([]byte, ioSize)
	rng.Read(ioBlock)

	writeIOPS, err := randomWriteIOPS(path, rng, numBlocks, ioBlock, duration)
	if err != nil {
		return bench.Fail("disk", err)
	}
	readIOPS, randUncached, err := randomReadIOPS(path, rng, numBlocks, duration)
	if err != nil {
		return bench.Fail("disk", err)
	}

	res := bench.Result{Name: "disk"}
	res.Add("sequential-write", writeMBs, "MB/s", throughputClass(writeMBs))
	res.Add("sequential-read", readMBs, "MB/s", throughputClass(readMBs)+readCaveat(seqUncached))
	res.Add("random-write", writeIOPS, "IOPS", iopsClass(writeIOPS))
	res.Add("random-read", readIOPS, "IOPS", iopsClass(readIOPS)+readCaveat(randUncached))

	if info := sysinfo.GetDisk(dir); info.Device != "" {
		res.AddInfo("device", info.Device)
		if info.Model != "" {
			res.AddInfo("model", info.Model)
		}
		if info.Filesystem != "" {
			res.AddInfo("filesystem", info.Filesystem)
		}
		if info.TotalBytes > 0 {
			res.AddInfo("capacity", bench.FormatBytes(info.FreeBytes)+" free of "+bench.FormatBytes(info.TotalBytes))
		}
	}
	return res
}

// sequentialWrite fills path with repeated block writes (creating it fresh,
// capped at fileSize, wrapping back to the start) until duration elapses,
// then fsyncs and returns the resulting throughput.
func sequentialWrite(path string, block []byte, duration time.Duration) (mbs float64, err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	start := time.Now()
	var written, off int64
	for {
		n, werr := f.WriteAt(block, off)
		if werr != nil {
			return 0, werr
		}
		written += int64(n)
		off += int64(n)
		if off >= fileSize {
			off = 0
		}
		if written >= fileSize && time.Since(start) >= duration {
			break
		}
	}
	if err := f.Sync(); err != nil {
		return 0, err
	}
	elapsed := time.Since(start)
	return float64(written) / elapsed.Seconds() / 1e6, nil
}

// sequentialRead reads through path (wrapping back to the start) until
// duration elapses, bypassing the OS cache where possible (openUncached).
func sequentialRead(path string, duration time.Duration) (mbs float64, uncached bool, err error) {
	f, uncached := openUncached(path)
	if f == nil {
		return 0, false, fmt.Errorf("opening %s for read", path)
	}
	defer f.Close()

	buf := alignedBuffer(blockSize)
	start := time.Now()
	var read, off int64
	for {
		n, rerr := f.ReadAt(buf, off)
		if rerr != nil {
			return 0, uncached, rerr
		}
		read += int64(n)
		off += int64(n)
		if off >= fileSize {
			off = 0
		}
		if read >= fileSize && time.Since(start) >= duration {
			break
		}
	}
	elapsed := time.Since(start)
	return float64(read) / elapsed.Seconds() / 1e6, uncached, nil
}

// randomWriteIOPS overwrites random ioSize-aligned blocks within path
// (already sized to fileSize by sequentialWrite) until at least minIOOps
// have run and duration has elapsed, then fsyncs.
func randomWriteIOPS(path string, rng *rand.Rand, numBlocks int64, ioBlock []byte, duration time.Duration) (iops float64, err error) {
	f, err := os.OpenFile(path, os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	start := time.Now()
	var ops int64
	for {
		off := rng.Int63n(numBlocks) * ioSize
		if _, err := f.WriteAt(ioBlock, off); err != nil {
			return 0, err
		}
		ops++
		if ops >= minIOOps && time.Since(start) >= duration {
			break
		}
	}
	if err := f.Sync(); err != nil {
		return 0, err
	}
	elapsed := time.Since(start)
	return float64(ops) / elapsed.Seconds(), nil
}

// randomReadIOPS reads random ioSize-aligned blocks within path until at
// least minIOOps have run and duration has elapsed, bypassing the OS cache
// where possible (openUncached).
func randomReadIOPS(path string, rng *rand.Rand, numBlocks int64, duration time.Duration) (iops float64, uncached bool, err error) {
	f, uncached := openUncached(path)
	if f == nil {
		return 0, false, fmt.Errorf("opening %s for read", path)
	}
	defer f.Close()

	buf := alignedBuffer(ioSize)
	start := time.Now()
	var ops int64
	for {
		off := rng.Int63n(numBlocks) * ioSize
		if _, err := f.ReadAt(buf, off); err != nil {
			return 0, uncached, err
		}
		ops++
		if ops >= minIOOps && time.Since(start) >= duration {
			break
		}
	}
	elapsed := time.Since(start)
	return float64(ops) / elapsed.Seconds(), uncached, nil
}

// throughputClass buckets sequential MB/s against well-established storage
// technology ranges.
func throughputClass(mbs float64) string {
	switch {
	case mbs < 200:
		return "HDD-class throughput"
	case mbs < 600:
		return "SATA SSD-class throughput"
	case mbs < 3500:
		return "NVMe (PCIe Gen3)-class throughput"
	default:
		return "NVMe (PCIe Gen4/5)-class throughput"
	}
}

// readCaveat notes when a read result couldn't bypass the OS page cache, so
// it may be inflated relative to real device throughput.
func readCaveat(uncached bool) string {
	if uncached {
		return ""
	}
	return " — page cache bypass unsupported here (e.g. tmpfs); may be inflated by OS caching"
}

// iopsClass buckets random 4K IOPS against well-established storage
// technology ranges.
func iopsClass(iops float64) string {
	switch {
	case iops < 500:
		return "HDD-class IOPS"
	case iops < 50_000:
		return "SATA SSD-class IOPS"
	case iops < 500_000:
		return "NVMe-class IOPS"
	default:
		return "high-end NVMe-class IOPS"
	}
}
