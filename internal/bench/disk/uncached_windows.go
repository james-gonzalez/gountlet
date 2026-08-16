//go:build windows

package disk

import (
	"os"
	"syscall"
)

// fileFlagNoBuffering is Win32's FILE_FLAG_NO_BUFFERING. Not exported by
// the syscall package, but a stable, documented constant.
const fileFlagNoBuffering = 0x20000000

// openUncached opens path via CreateFile with FILE_FLAG_NO_BUFFERING,
// bypassing the OS cache so reads reflect real device I/O. Requires
// sector-aligned buffers/offsets/lengths, which is why callers use
// alignedBuffer. Falls back to a normal buffered open on failure.
func openUncached(path string) (*os.File, bool) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err == nil {
		h, err := syscall.CreateFile(
			pathPtr,
			syscall.GENERIC_READ,
			syscall.FILE_SHARE_READ,
			nil,
			syscall.OPEN_EXISTING,
			syscall.FILE_ATTRIBUTE_NORMAL|fileFlagNoBuffering,
			0,
		)
		if err == nil {
			return os.NewFile(uintptr(h), path), true
		}
	}
	f, ferr := os.Open(path)
	if ferr != nil {
		return nil, false
	}
	return f, false
}
