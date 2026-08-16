//go:build !cgo

package gpu

import (
	"fmt"
	"time"

	"github.com/james-gonzalez/gountlet/internal/bench"
)

// Run reports failure: the Vulkan compute benchmark requires cgo.
func Run(_ time.Duration) bench.Result {
	return bench.Fail("gpu", fmt.Errorf("GPU benchmark requires building with cgo enabled (CGO_ENABLED=1) and the Vulkan runtime loader installed"))
}
