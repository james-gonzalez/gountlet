package disk

import "unsafe"

// alignedBlockSize is the alignment (and, for O_DIRECT/no-buffering reads,
// implicitly the minimum I/O granularity) most platforms require for
// unbuffered I/O: covers the common 512-byte and 4096-byte logical sector
// sizes seen on real drives.
const alignedBlockSize = 4096

// alignedBuffer returns a size-byte slice whose backing array starts at an
// alignedBlockSize-aligned address, as required for O_DIRECT-style
// unbuffered reads on Linux/Windows. It's harmless to use even when the
// underlying read isn't actually unbuffered (e.g. macOS, or a fallback).
func alignedBuffer(size int) []byte {
	buf := make([]byte, size+alignedBlockSize)
	addr := uintptr(unsafe.Pointer(&buf[0]))
	pad := int((alignedBlockSize - addr%alignedBlockSize) % alignedBlockSize)
	return buf[pad : pad+size]
}
