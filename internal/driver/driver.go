// Package driver defines what a run mode must provide. The local Docker
// driver is the only one today; cloud and simulator drivers implement the
// same interface, so planning, diagnosis and reporting don't change.
package driver

import (
	"context"
	"time"

	"github.com/akinfemi/load-lore/internal/plan"
	"github.com/akinfemi/load-lore/internal/rundir"
)

// Driver runs a plan somewhere.
type Driver interface {
	// Preflight checks the environment can run the plan, before anything
	// is created.
	Preflight(ctx context.Context, p *plan.Plan) (*Preflight, error)
	// Up creates and starts everything except the load: datasets, the
	// stack, monitoring. It returns when the stack is healthy.
	Up(ctx context.Context, p *plan.Plan, dir *rundir.Dir, progress func(string)) error
	// RunLoad starts the load generator, calls started once it runs, and
	// blocks until it finishes or ctx ends.
	RunLoad(ctx context.Context, started func(time.Time)) (*LoadResult, error)
	// Inject applies a fault. undo ends it, for faults that can end.
	Inject(ctx context.Context, f *plan.Fault) (undo func(context.Context) error, err error)
	// Sample reads current numbers for progress output.
	Sample(ctx context.Context, elapsed time.Duration) (*Sample, error)
	// Collect copies metrics, logs and container state into the run
	// folder, so nothing depends on the stack still existing.
	Collect(ctx context.Context, dir *rundir.Dir, from, to time.Time) error
	// Down removes everything the run created. It must be safe to call
	// more than once and after a partial Up.
	Down(ctx context.Context) error
}

// Preflight is what the environment check found.
type Preflight struct {
	Host     Host     `json:"host"`
	Warnings []string `json:"warnings"`
}

// Host describes where the run happens.
type Host struct {
	Driver         string `json:"driver"`
	CPUs           int    `json:"cpus"`
	MemoryBytes    int64  `json:"memory_bytes"`
	OS             string `json:"os"`
	Kernel         string `json:"kernel"`
	Arch           string `json:"arch"`
	EngineVersion  string `json:"engine_version"`
	ComposeVersion string `json:"compose_version"`
	CgroupVersion  string `json:"cgroup_version"`
}

// LoadResult is the load generator's own account of the run.
type LoadResult struct {
	ExitCode          int     `json:"exit_code"`
	Requests          float64 `json:"requests"`
	DroppedIterations float64 `json:"dropped_iterations"`
	FailedShare       float64 `json:"failed_share"`
}

// Sample is a progress snapshot. Nil fields mean no data yet.
type Sample struct {
	Offered float64            `json:"offered"`
	Served  *float64           `json:"served"`
	P99Ms   *float64           `json:"p99_ms"`
	Errors  *float64           `json:"errors"`
	CPU     map[string]float64 `json:"cpu"` // service -> utilization of its limit (0–1)
}
