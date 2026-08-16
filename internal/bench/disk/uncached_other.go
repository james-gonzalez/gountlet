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
