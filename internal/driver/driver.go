// Package driver defines what a run mode must provide. The local Docker
// driver is the only one today; cloud and simulator drivers implement the
// same interface, so planning, diagnosis and reporting don't change.
package driver

import (
	"context"
	"time"

	"github.com/akinfemi/load-lore/internal/bench"
	"github.com/akinfemi/load-lore/internal/plan"
	"github.com/akinfemi/load-lore/internal/rundir"
)

// Driver runs a plan somewhere.
type Driver interface {
	// Preflight checks the environment can run the plan, before anything
	// is created.
	Preflight(ctx context.Context, p *plan.Plan) (*Preflight, error)
	// Speed runs the host speed benchmark again, as pre-flight did.
	Speed(ctx context.Context, p *plan.Plan) (*bench.Speed, error)
	// Up creates and starts everything except the load: datasets, the
	// stack, monitoring. It returns when the stack is healthy.
	Up(ctx context.Context, p *plan.Plan, dir *rundir.Dir, progress func(string)) error
	// RunLoad starts the load generator, calls started once it runs, and
	// blocks until it finishes or ctx ends.
	RunLoad(ctx context.Context, started func(time.Time)) (*LoadResult, error)
	// StopLoad ends the load before its profile does. The load generator
	// stops as at the end of the profile: it still writes its summary,
	// and RunLoad returns with K6Stopped as the exit code.
	StopLoad(ctx context.Context) error
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

	// HostCPU is what loadl can see of the CPU under Docker's kernel:
	// HostCPUSampled when that kernel is this machine's and loadl samples
	// its /proc/stat, HostCPUInVM when Docker runs in a VM on this machine
	// (Colima, Docker Desktop), whose host load it can't see, and empty
	// when it can't tell.
	HostCPU string `json:"host_cpu,omitempty"`

	// Speed is the host speed benchmark pre-flight ran, against the
	// reference machine; nil when it couldn't run.
	Speed *bench.Speed `json:"speed,omitempty"`

	// Cooled is the wait before the run for the host to get back near an
	// earlier run's speed, in a series such as --repeat; nil when there was
	// none to wait for.
	Cooled *Cooldown `json:"cooled,omitempty"`
}

// Inspection is what loadl doctor learns about Docker without a plan.
// Docker itself was reachable; the errors are the other problems found.
type Inspection struct {
	Host       Host
	DockerHost string // the address loadl reached Docker at; not saved with runs
	ComposeErr error  // Compose missing or too old
	Image      string // the synthetic image checked
	ImageErr   error  // the image isn't available
	SpeedErr   error  // the speed benchmark failed
}

// Cooldown is how long pre-flight waited for the host speed index to get
// back within a share of Target, re-running the benchmark meanwhile.
type Cooldown struct {
	Target    int     `json:"target"`    // the speed index waited for: the series' first run's
	Tolerance float64 `json:"tolerance"` // the share below Target that counts as close enough
	First     int     `json:"first"`     // the index pre-flight measured first
	Waited    float64 `json:"waited_s"`  // seconds waited; 0 when First was already close enough
	Reached   bool    `json:"reached"`   // false when it stopped waiting at the limit
}

// Values of Host.HostCPU.
const (
	HostCPUSampled = "sampled"
	HostCPUInVM    = "vm"
)

// K6Stopped is k6's exit code when a signal ended the test early
// (errext.ExternalAbort).
const K6Stopped = 105

// LoadResult is the load generator's own account of the run.
type LoadResult struct {
	ExitCode          int     `json:"exit_code"`
	Requests          float64 `json:"requests"`
	DroppedIterations float64 `json:"dropped_iterations"`
	FailedShare       float64 `json:"failed_share"`
}

// Sample is a progress snapshot of the last 10 seconds. Nil fields mean
// no data yet.
type Sample struct {
	Offered float64            `json:"offered"`
	Served  *float64           `json:"served"`
	P50Ms   *float64           `json:"p50_ms,omitempty"`
	P95Ms   *float64           `json:"p95_ms,omitempty"`
	P99Ms   *float64           `json:"p99_ms"`
	Errors  *float64           `json:"errors"`
	CPU     map[string]float64 `json:"cpu"`              // service -> utilization of its limit (0–1)
	Memory  map[string]float64 `json:"memory,omitempty"` // service -> worst replica's working set / its limit (0–1)
	// Pools are callers' connection pools, keyed "service>target".
	Pools map[string]PoolSample `json:"pools,omitempty"`
	// Connections are Postgres connections open, and MaxConnections its
	// max_connections, by service.
	Connections    map[string]float64 `json:"connections,omitempty"`
	MaxConnections map[string]float64 `json:"max_connections,omitempty"`
	HitRate        map[string]float64 `json:"hit_rate,omitempty"` // Redis, by service
	// ReplicationLagS is each Postgres read replica's lag, in seconds.
	ReplicationLagS map[string]float64 `json:"replication_lag_s,omitempty"`
}

// PoolSample is one caller's pool to one target, summed over replicas.
type PoolSample struct {
	InUse   float64 `json:"in_use"`
	Max     float64 `json:"max"`
	Pending float64 `json:"pending"` // callers waiting for a connection
}
