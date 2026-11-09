//go:build !linux

package synthetic

import "time"

var processStart = time.Now()

// threadCPU falls back to wall time where a per-thread CPU clock isn't
// available. The service only runs on Linux in containers; this keeps unit
// tests working on other systems.
func threadCPU() time.Duration { return time.Since(processStart) }
