//go:build !linux && !darwin && !windows

package disk

import "os"

// openUncached has no cache-bypass on this platform; reads may still be
// served from the page cache.
func openUncached(path string) (*os.File, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	return f, false
}

// openUncachedWrite has no cache-bypass on this platform; writes may still
// be absorbed by the page cache before fsync.
func openUncachedWrite(path string) (*os.File, bool) {
	f, err := os.OpenFile(path, os.O_WRONLY, 0o600)
	if err != nil {
		return nil, false
	}
	return f, false
}
