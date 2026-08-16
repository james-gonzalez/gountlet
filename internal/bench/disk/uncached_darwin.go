//go:build darwin

package disk

import (
	"os"
	"syscall"
)

// openUncached opens path normally, then sets F_NOCACHE on the resulting
// descriptor so the OS bypasses its cache for it, unlike Linux/Windows this
// doesn't require aligned buffers/offsets. Falls back to a normal cached
// read if the fcntl call fails.
func openUncached(path string) (*os.File, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), uintptr(syscall.F_NOCACHE), 1)
	return f, errno == 0
}

// openUncachedWrite is openUncached's write-side counterpart: bypasses the
// page cache so writes (and their real completion time) reflect actual
// device I/O rather than a write-back cache absorbing them instantly.
func openUncachedWrite(path string) (*os.File, bool) {
	f, err := os.OpenFile(path, os.O_WRONLY, 0o600)
	if err != nil {
		return nil, false
	}
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), uintptr(syscall.F_NOCACHE), 1)
	return f, errno == 0
}
