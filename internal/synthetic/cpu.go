package synthetic

import (
	"runtime"
	"sync/atomic"
	"time"
)

// sink keeps the burn loop from being optimized away.
var sink atomic.Uint64

// burnCPU spins until the calling thread has used d of CPU time. Measuring
// thread CPU time rather than wall time means a throttled container takes
// longer in wall time to finish the same work, as a real handler would.
func burnCPU(d time.Duration) {
	if d <= 0 {
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	start := threadCPU()
	x := uint64(start)
	for threadCPU()-start < d {
		for i := range 2000 {
			x = x*6364136223846793005 + uint64(i) + 1442695040888963407
		}
	}
	sink.Store(x)
}
