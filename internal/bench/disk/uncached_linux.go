//go:build linux

package disk

import (
	"os"
	"syscall"
)

// openUncached opens path for reading with O_DIRECT, bypassing the page
// cache so reads reflect real device I/O instead of memory speed. Falls
// back to a normal buffered open if the filesystem doesn't support
// O_DIRECT (tmpfs and some network/overlay filesystems return EINVAL).
func openUncached(path string) (*os.File, bool) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECT, 0)
	if err != nil {
		f, ferr := os.Open(path)
		if ferr != nil {
			return nil, false
		}
		return f, false
	}
	return os.NewFile(uintptr(fd), path), true
}
