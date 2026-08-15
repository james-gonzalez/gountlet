// Package disk benchmarks sequential read/write throughput and 4K random
// IOPS against a temp file. Portable across Windows/macOS/Linux: it uses
// only os/io and fsync, no OS-specific direct-I/O flags, so repeated reads
// may be influenced by the OS page cache.
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
	fileSize  = 1 << 30 // 1 GiB
	blockSize = 4 << 20 // 4 MiB sequential I/O block
	ioSize    = 4 << 10 // 4 KiB random I/O block
	ioOps     = 20000
)

// Run measures sequential write/read MB/s and 4K random read/write IOPS
// using a temp file under dir (os.TempDir() if dir is empty).
func Run(dir string) bench.Result {
	res := bench.Result{Name: "disk"}

	if dir == "" {
		dir = os.TempDir()
	}
	path := filepath.Join(dir, fmt.Sprintf("gountlet-diskbench-%d.tmp", time.Now().UnixNano()))
	defer os.Remove(path)

	block := make([]byte, blockSize)
	rand.New(rand.NewSource(1)).Read(block)

	// Sequential write.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return bench.Fail("disk", err)
	}
	start := time.Now()
	var written int64
	for written < fileSize {
		n, err := f.Write(block)
		if err != nil {
			f.Close()
			return bench.Fail("disk", err)
		}
		written += int64(n)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return bench.Fail("disk", err)
	}
	writeElapsed := time.Since(start)
	f.Close()
	writeMBs := float64(written) / writeElapsed.Seconds() / 1e6

	// Sequential read.
	f, err = os.Open(path)
	if err != nil {
		return bench.Fail("disk", err)
	}
	start = time.Now()
	var read int64
	buf := make([]byte, blockSize)
	for {
		n, err := f.Read(buf)
		read += int64(n)
		if err != nil {
			break
		}
	}
	readElapsed := time.Since(start)
	f.Close()
	readMBs := float64(read) / readElapsed.Seconds() / 1e6

	// 4K random write IOPS (overwrites within the existing file, then fsync).
	f, err = os.OpenFile(path, os.O_WRONLY, 0o600)
	if err != nil {
		return bench.Fail("disk", err)
	}
	rng := rand.New(rand.NewSource(2))
	numBlocks := int64(fileSize / ioSize)
	ioBlock := make([]byte, ioSize)
	rng.Read(ioBlock)
	start = time.Now()
	for i := 0; i < ioOps; i++ {
		off := rng.Int63n(numBlocks) * ioSize
		if _, err := f.WriteAt(ioBlock, off); err != nil {
			f.Close()
			return bench.Fail("disk", err)
		}
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return bench.Fail("disk", err)
	}
	writeIOPSElapsed := time.Since(start)
	f.Close()
	writeIOPS := float64(ioOps) / writeIOPSElapsed.Seconds()

	// 4K random read IOPS.
	f, err = os.Open(path)
	if err != nil {
		return bench.Fail("disk", err)
	}
	readBuf := make([]byte, ioSize)
	start = time.Now()
	for i := 0; i < ioOps; i++ {
		off := rng.Int63n(numBlocks) * ioSize
		if _, err := f.ReadAt(readBuf, off); err != nil {
			f.Close()
			return bench.Fail("disk", err)
		}
	}
	readIOPSElapsed := time.Since(start)
	f.Close()
	readIOPS := float64(ioOps) / readIOPSElapsed.Seconds()

	res.Add("sequential-write", writeMBs, "MB/s", throughputClass(writeMBs))
	res.Add("sequential-read", readMBs, "MB/s", throughputClass(readMBs)+" — reads may be inflated by OS page cache")
	res.Add("random-write", writeIOPS, "IOPS", iopsClass(writeIOPS))
	res.Add("random-read", readIOPS, "IOPS", iopsClass(readIOPS)+" — reads may be inflated by OS page cache")

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
