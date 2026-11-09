package synthetic

import (
	"time"

	"golang.org/x/sys/unix"
)

// threadCPU returns the CPU time used by the calling OS thread.
func threadCPU() time.Duration {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_THREAD_CPUTIME_ID, &ts); err != nil {
		return time.Duration(time.Now().UnixNano())
	}
	return time.Duration(ts.Nano())
}
