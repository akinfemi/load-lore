// Package design defines the Loadlore design file: its Go types (the source
// of truth for the schema), parsing, defaults and validation.
package design

// Design is one experiment: the system, how it is loaded, what goes wrong and
// what the learner expects to happen.
type Design struct {
	Version     int                 `yaml:"version" json:"version" jsonschema:"required,enum=1" jsonschema_description:"Schema version. Always 1 for now."`
	Name        string              `yaml:"name" json:"name" jsonschema:"required,pattern=^[a-z0-9]+(-[a-z0-9]+)*$,maxLength=63" jsonschema_description:"Short kebab-case name used in run ids and reports."`
	Services    map[string]*Service `yaml:"services" json:"services" jsonschema:"required,minProperties=1" jsonschema_description:"Map of service name to service. Names are lowercase letters, digits and '-'."`
	Workload    *Workload           `yaml:"workload" json:"workload" jsonschema:"required"`
	Scenario    []*Fault            `yaml:"scenario,omitempty" json:"scenario,omitempty" jsonschema_description:"Timed fault injections."`
	Expect      []*Expectation      `yaml:"expect,omitempty" json:"expect,omitempty" jsonschema_description:"The learner's predictions, scored after the run."`
	Assumptions []*Assumption       `yaml:"assumptions,omitempty" json:"assumptions,omitempty" jsonschema_description:"Every value the user did not state, with its basis. Printed in every report."`
	Modes       *Modes              `yaml:"modes,omitempty" json:"modes,omitempty" jsonschema_description:"Per-mode overrides."`
}

// Service is either an application service (Image set) or a built-in
// datastore or infrastructure component (Kind set).
type Service struct {
	// Common.
	Image     string     `yaml:"image,omitempty" json:"image,omitempty" jsonschema_description:"Container image for an application service: loadlore/synthetic or your own. Mutually exclusive with kind."`
	Kind      string     `yaml:"kind,omitempty" json:"kind,omitempty" jsonschema:"enum=postgres,enum=postgres-replica,enum=redis,enum=queue,enum=pgbouncer,enum=lb" jsonschema_description:"Built-in component. Mutually exclusive with image. queue is a job queue on Redis Streams: application services publish jobs to it, and workers consume them."`
	Replicas  *int       `yaml:"replicas,omitempty" json:"replicas,omitempty" jsonschema:"minimum=1,default=1" jsonschema_description:"Number of replicas. A postgres-replica can run several, each streaming from its primary, and callers spread their reads across them. Other datastores, pgbouncer and lb run exactly one."`
	Resources *Resources `yaml:"resources,omitempty" json:"resources,omitempty" jsonschema_description:"Default 1 CPU and 512m; lb defaults to 0.5 CPU and 256m."`

	// Application services.
	Role        string          `yaml:"role,omitempty" json:"role,omitempty" jsonschema:"enum=server,enum=worker,default=server" jsonschema_description:"server handles HTTP requests. worker takes jobs from a queue (consume) and runs its behavior for each one."`
	Behavior    *Behavior       `yaml:"behavior,omitempty" json:"behavior,omitempty" jsonschema_description:"What the synthetic service does per request. Only for loadlore/synthetic."`
	Pools       map[string]Pool `yaml:"pools,omitempty" json:"pools,omitempty" jsonschema_description:"Connection pool per call target: a size (connections per replica, default 10), or {size, min}."`
	Concurrency *Concurrency    `yaml:"concurrency,omitempty" json:"concurrency,omitempty"`
	Consume     *Consume        `yaml:"consume,omitempty" json:"consume,omitempty" jsonschema_description:"role: worker only: the queue it takes jobs from, and how many jobs each replica runs at once."`
	Autoscale   *Autoscale      `yaml:"autoscale,omitempty" json:"autoscale,omitempty" jsonschema_description:"Add and remove replicas during the run from a metric, as a Kubernetes HorizontalPodAutoscaler or a cloud autoscaling group does. replicas is how many it starts with. Needs an lb in front."`

	// Datastores and infrastructure.
	Version string                 `yaml:"version,omitempty" json:"version,omitempty" jsonschema_description:"Major version. postgres default \"17\"; redis and queue default \"8\"."`
	Config  map[string]ConfigValue `yaml:"config,omitempty" json:"config,omitempty" jsonschema_description:"Native config keys: postgresql.conf for postgres, redis.conf for redis and queue, pgbouncer.ini for pgbouncer."`
	Primary string                 `yaml:"primary,omitempty" json:"primary,omitempty" jsonschema_description:"postgres-replica: the postgres service it streams from. The replica starts from a copy of the primary's data and serves reads only."`
	Dataset *Dataset               `yaml:"dataset,omitempty" json:"dataset,omitempty" jsonschema_description:"Rows seeded before the run. postgres only."`
	Disk    *Disk                  `yaml:"disk,omitempty" json:"disk,omitempty" jsonschema_description:"Model the data disk like a cloud volume: a wait per read and an IOPS cap. postgres only; without it the data sits on the Docker host's disk, which on a laptop is mostly served from memory."`

	// pgbouncer and lb.
	Target string `yaml:"target,omitempty" json:"target,omitempty" jsonschema_description:"pgbouncer: the postgres service it pools connections to. lb: the application service whose replicas it spreads traffic across."`

	// pgbouncer.
	PoolMode        string `yaml:"pool_mode,omitempty" json:"pool_mode,omitempty" jsonschema:"enum=session,enum=transaction,enum=statement,default=transaction"`
	DefaultPoolSize *int   `yaml:"default_pool_size,omitempty" json:"default_pool_size,omitempty" jsonschema:"minimum=1,default=20" jsonschema_description:"Server connections per database and user pair."`

	// lb.
	Algorithm   string       `yaml:"algorithm,omitempty" json:"algorithm,omitempty" jsonschema:"enum=round-robin,enum=least-conn,enum=random,default=round-robin" jsonschema_description:"How the lb picks a healthy replica for each request."`
	HealthCheck *HealthCheck `yaml:"health_check,omitempty" json:"health_check,omitempty" jsonschema_description:"Active health checks the lb runs against each replica."`
}

// Consume is a worker's queue and how many jobs it runs at once.
type Consume struct {
	From        string `yaml:"from" json:"from" jsonschema:"required" jsonschema_description:"The kind: queue service it takes jobs from."`
	Concurrency *int   `yaml:"concurrency,omitempty" json:"concurrency,omitempty" jsonschema:"minimum=1,default=5" jsonschema_description:"Jobs each replica runs at once, as Sidekiq's concurrency or a Celery worker's: 5 is Sidekiq's default."`
	// Retries, when set, puts a failed job back on the queue after a
	// growing wait, up to Max times, then sets it aside as dead. Without
	// it a failed job is dropped.
	Retries *JobRetries `yaml:"retries,omitempty" json:"retries,omitempty" jsonschema_description:"Retry failed jobs: each failure waits base_ms, then twice as long each time, give or take a quarter, before the job goes back on the queue; after max retries it is set aside as dead. Default: none, a failed job is dropped."`
}

// JobRetries is how a worker retries a failed job, as Sidekiq does: from
// a retry set, after a growing wait, then into a dead set.
type JobRetries struct {
	Max    int  `yaml:"max" json:"max" jsonschema:"required,minimum=1" jsonschema_description:"Retries after the first attempt; then the job is dead. Sidekiq's default is 25."`
	BaseMs *int `yaml:"base_ms,omitempty" json:"base_ms,omitempty" jsonschema:"minimum=100,default=15000" jsonschema_description:"The wait before the first retry, doubling for each one after it: 15000 is close to Sidekiq's first waits (about 16 s, then 31 s and 96 s)."`
}

// Disk models a postgres's data disk. Profile names a disk from
// DiskProfiles (laptop, gp3, io2), whose waits, IOPS and throughput come
// from published specs; IOPS may be provisioned within the profile's
// range. Without a profile, the older raw form: every read that misses
// memory waits ReadMs, and reads and writes are each capped at IOPS a
// second. Without a disk block, the data sits on the Docker host's disk;
// on a laptop, a read that misses the container's memory comes back from
// the host's own, in about a tenth of a millisecond and with no cap.
type Disk struct {
	Profile *string `yaml:"profile,omitempty" json:"profile,omitempty" jsonschema:"enum=laptop,enum=gp3,enum=io2" jsonschema_description:"A named disk with numbers from published specs: laptop (the Docker host's own), gp3 (1 ms per read and write, 3,000 IOPS and 125 MiB/s baseline) or io2 (provisioned IOPS, no added wait). Every report prints the profile and its source."`
	ReadMs  *int    `yaml:"read_ms,omitempty" json:"read_ms,omitempty" jsonschema:"minimum=0,maximum=100,default=1" jsonschema_description:"Without a profile only: milliseconds every read from the disk waits, in whole milliseconds."`
	IOPS    *int    `yaml:"iops,omitempty" json:"iops,omitempty" jsonschema:"minimum=100" jsonschema_description:"Reads a second the disk serves, and writes a second, each. With gp3, provisioned IOPS from 3000 (the default) to 80000; with io2, required, 100 to 256000; not with laptop. Without a profile, at least 100, default 3000. Scaled with --scale."`
}

// HealthCheck is an lb's active check of each replica: an HTTP GET every
// interval. A replica is taken out after fall failures in a row and put
// back after rise successes in a row.
type HealthCheck struct {
	Path       string `yaml:"path,omitempty" json:"path,omitempty" jsonschema:"pattern=^/,default=/healthz" jsonschema_description:"HTTP path checked on each replica."`
	IntervalMs *int   `yaml:"interval_ms,omitempty" json:"interval_ms,omitempty" jsonschema:"minimum=100,default=2000" jsonschema_description:"Time between checks of one replica."`
	TimeoutMs  *int   `yaml:"timeout_ms,omitempty" json:"timeout_ms,omitempty" jsonschema:"minimum=1,default=1000" jsonschema_description:"A check slower than this fails."`
	Fall       *int   `yaml:"fall,omitempty" json:"fall,omitempty" jsonschema:"minimum=1,default=3" jsonschema_description:"Failed checks in a row that take a replica out."`
	Rise       *int   `yaml:"rise,omitempty" json:"rise,omitempty" jsonschema:"minimum=1,default=2" jsonschema_description:"Passed checks in a row that put a replica back."`
}

// Resources are per-replica limits: cgroup limits in local mode.
type Resources struct {
	CPU    *float64 `yaml:"cpu,omitempty" json:"cpu,omitempty" jsonschema:"exclusiveMinimum=0,default=1" jsonschema_description:"CPU cores per replica."`
	Memory *Size    `yaml:"memory,omitempty" json:"memory,omitempty" jsonschema:"default=512m" jsonschema_description:"Memory per replica, such as 512m or 1g."`
	// CPUPeriodMs is the CFS period the CPU limit is enforced over. The
	// quota is cpu × period, so a 0.5-CPU container may run 50 ms in each
	// 100 ms period and then waits for the next one.
	CPUPeriodMs *int `yaml:"cpu_period_ms,omitempty" json:"cpu_period_ms,omitempty" jsonschema:"minimum=1,maximum=1000,default=100" jsonschema_description:"Period in ms over which the CPU limit is enforced. A container that uses its share early in a period stalls until the next one, so short periods behave like a slower CPU and long ones like Kubernetes-style throttling."`
}

// Behavior is what loadlore/synthetic does for each request.
type Behavior struct {
	CPUMs      float64     `yaml:"cpu_ms,omitempty" json:"cpu_ms,omitempty" jsonschema:"minimum=0" jsonschema_description:"Mean CPU time per request in milliseconds."`
	CPUJitter  *float64    `yaml:"cpu_jitter,omitempty" json:"cpu_jitter,omitempty" jsonschema:"minimum=0,maximum=1,default=0.1" jsonschema_description:"Variation of cpu_ms as a fraction of it."`
	AllocKB    int         `yaml:"alloc_kb,omitempty" json:"alloc_kb,omitempty" jsonschema:"minimum=0" jsonschema_description:"Memory allocated per request in KB."`
	Calls      []*Call     `yaml:"calls,omitempty" json:"calls,omitempty" jsonschema_description:"Outbound calls, executed in order for each request."`
	Resilience *Resilience `yaml:"resilience,omitempty" json:"resilience,omitempty"`
}

// Call is one outbound call made while handling a request.
type Call struct {
	Target string `yaml:"target,omitempty" json:"target,omitempty" jsonschema_description:"A service name. A call to sharded data names shards instead."`
	// Shards are the postgres services a sharded call goes to, by key
	// (ShardBy key: one shard per call) or to every one (ShardBy all).
	Shards  []string `yaml:"shards,omitempty" json:"shards,omitempty" jsonschema:"minItems=2" jsonschema_description:"Instead of target, for data split across postgres services by key: the shards, in order. With shard_by key a call goes to the shard its key hashes to (key modulo the shard count), and that shard's row key/count; with shard_by all it goes to every shard at once and waits for all of them (a fan-out read)."`
	ShardBy string   `yaml:"shard_by,omitempty" json:"shard_by,omitempty" jsonschema:"enum=key,enum=all,default=key" jsonschema_description:"With shards: key sends each call to the one shard that holds its key; all sends it to every shard at once (reads only), done when the slowest answers."`
	Op      string   `yaml:"op" json:"op" jsonschema:"required,enum=read,enum=write,enum=get,enum=set,enum=del,enum=publish,enum=http" jsonschema_description:"read/write for postgres and pgbouncer; get/set/del for redis; publish for a queue (adds one job); http for application services."`
	Count   *int     `yaml:"count,omitempty" json:"count,omitempty" jsonschema:"minimum=1,default=1" jsonschema_description:"Times per request."`
	When    string   `yaml:"when,omitempty" json:"when,omitempty" jsonschema:"enum=always,enum=read,enum=write,default=always" jsonschema_description:"Run only for read or write requests from the workload mix."`
	Mode    string   `yaml:"mode,omitempty" json:"mode,omitempty" jsonschema:"enum=sync,enum=async,default=sync" jsonschema_description:"sync waits for the result; async fires and continues."`
	OnMiss  []*Call  `yaml:"on_miss,omitempty" json:"on_miss,omitempty" jsonschema_description:"Calls run on a cache miss. get only."`
	// Coalesce: concurrent misses for the same key in one replica wait for
	// a single run of on_miss (single-flight), instead of each running it.
	Coalesce bool   `yaml:"coalesce,omitempty" json:"coalesce,omitempty" jsonschema:"default=false" jsonschema_description:"Concurrent misses for the same key in one replica wait for one run of on_miss (request coalescing, single-flight) instead of each running it. get with on_miss only."`
	TTLS     *int   `yaml:"ttl_s,omitempty" json:"ttl_s,omitempty" jsonschema:"minimum=0" jsonschema_description:"Key TTL in seconds. set only."`
	Query    string `yaml:"query,omitempty" json:"query,omitempty" jsonschema:"enum=point,enum=range,enum=batch,enum=scan,default=point" jsonschema_description:"Shape of a read: point (one row by key), range (rows in key order), batch (rows by key in one statement), scan (full table)."`
	Rows     *int   `yaml:"rows,omitempty" json:"rows,omitempty" jsonschema:"minimum=1,default=10" jsonschema_description:"Rows returned by a range read, or keys fetched by a batch read."`
	AfterMs  *int   `yaml:"after_ms,omitempty" json:"after_ms,omitempty" jsonschema:"minimum=0,maximum=60000" jsonschema_description:"mode async only: milliseconds the call waits before it runs, while the request goes on without it. A follow-up the client makes later, such as the page that loads after a form post; a read of a key the request wrote is checked for the write."`
}

// Resilience settings apply to every outbound call of the service.
type Resilience struct {
	TimeoutMs      *int            `yaml:"timeout_ms,omitempty" json:"timeout_ms,omitempty" jsonschema:"minimum=0,default=5000" jsonschema_description:"Per outbound call. 0 means no timeout."`
	Retries        *Retries        `yaml:"retries,omitempty" json:"retries,omitempty" jsonschema_description:"Default: no retries."`
	CircuitBreaker *CircuitBreaker `yaml:"circuit_breaker,omitempty" json:"circuit_breaker,omitempty" jsonschema_description:"Default: none."`
	Bulkhead       *Bulkhead       `yaml:"bulkhead,omitempty" json:"bulkhead,omitempty" jsonschema_description:"Default: none."`
}

// Retries on a failed outbound call.
type Retries struct {
	Max     int    `yaml:"max" json:"max" jsonschema:"required,minimum=0" jsonschema_description:"Retries after the first attempt."`
	Backoff string `yaml:"backoff,omitempty" json:"backoff,omitempty" jsonschema:"enum=none,enum=constant,enum=exponential,default=none" jsonschema_description:"none retries immediately."`
	BaseMs  *int   `yaml:"base_ms,omitempty" json:"base_ms,omitempty" jsonschema:"minimum=1,default=100" jsonschema_description:"First backoff delay for constant and exponential."`
	Jitter  bool   `yaml:"jitter,omitempty" json:"jitter,omitempty" jsonschema:"default=false" jsonschema_description:"Randomize each delay (full jitter)."`
}

// CircuitBreaker opens when the error rate over a window passes a threshold.
type CircuitBreaker struct {
	ErrorRate float64 `yaml:"error_rate" json:"error_rate" jsonschema:"required,exclusiveMinimum=0,maximum=1" jsonschema_description:"Error fraction that opens the breaker."`
	WindowS   int     `yaml:"window_s" json:"window_s" jsonschema:"required,minimum=1"`
	OpenS     int     `yaml:"open_s" json:"open_s" jsonschema:"required,minimum=1"`
}

// Bulkhead caps concurrent outbound calls.
type Bulkhead struct {
	MaxConcurrent int `yaml:"max_concurrent" json:"max_concurrent" jsonschema:"required,minimum=1"`
}

// Autoscale adds and removes an application service's replicas during the
// run. Every interval it reads the metric over the running replicas and
// wants ceil(running x value / target) replicas, within min and max, as a
// Kubernetes HorizontalPodAutoscaler does, ignoring changes within 10%.
// By default, as there, it scales up as soon as it wants more and waits
// 300 s after its last change before scaling down; a cloud autoscaling
// group's cooldown also delays scale-ups (scale_up_cooldown_s). New
// replicas start for real and take traffic once healthy; leaving replicas
// are drained first.
type Autoscale struct {
	Min                *int     `yaml:"min,omitempty" json:"min,omitempty" jsonschema:"minimum=1" jsonschema_description:"Fewest replicas. Default: replicas."`
	Max                *int     `yaml:"max" json:"max" jsonschema:"required,minimum=1" jsonschema_description:"Most replicas. Loadlore budgets CPU and memory for all of them."`
	Metric             string   `yaml:"metric,omitempty" json:"metric,omitempty" jsonschema:"enum=cpu,enum=p99,enum=inflight,default=cpu" jsonschema_description:"cpu: mean CPU of the running replicas against their limit. p99: the service's own p99 in ms. inflight: mean requests in flight per replica."`
	Target             *float64 `yaml:"target,omitempty" json:"target,omitempty" jsonschema_description:"The metric's value to hold: a fraction for cpu (default 0.7), milliseconds for p99, requests for inflight. Required for p99 and inflight."`
	IntervalS          *int     `yaml:"interval_s,omitempty" json:"interval_s,omitempty" jsonschema:"minimum=5,default=15" jsonschema_description:"Seconds between decisions; the metric is averaged over this window. Kubernetes' default is 15."`
	ScaleUpCooldownS   *int     `yaml:"scale_up_cooldown_s,omitempty" json:"scale_up_cooldown_s,omitempty" jsonschema:"minimum=0,default=0" jsonschema_description:"Seconds after a change before it scales up again. Kubernetes' default is 0; a cloud autoscaling group's cooldown, often 300, applies here too."`
	StartupS           *int     `yaml:"startup_s,omitempty" json:"startup_s,omitempty" jsonschema:"minimum=0,maximum=600,default=30" jsonschema_description:"Seconds a new replica waits before its container starts, for what a laptop has already done: finding a node or a VM, pulling the image. The app's own start, its pools and health checks then run for real. Kubernetes on a node with the image is a few seconds; a new node or VM is one to several minutes."`
	ScaleDownCooldownS *int     `yaml:"scale_down_cooldown_s,omitempty" json:"scale_down_cooldown_s,omitempty" jsonschema:"minimum=0,default=300" jsonschema_description:"Seconds after a change before it scales down. Kubernetes' scale-down stabilization window is 300."`
}

// Concurrency approximates the real service runtime.
type Concurrency struct {
	Model string `yaml:"model,omitempty" json:"model,omitempty" jsonschema:"enum=threads,enum=event-loop,enum=workers,default=threads"`
	Max   *int   `yaml:"max,omitempty" json:"max,omitempty" jsonschema:"minimum=0" jsonschema_description:"threads: in-flight requests per replica (default 200). event-loop: in-flight requests per replica (default 0, unlimited). workers: worker count (default 4)."`
}

// Dataset is seeded inside the datastore before the run.
type Dataset struct {
	Rows     int64    `yaml:"rows" json:"rows" jsonschema:"required,minimum=1"`
	RowBytes *int     `yaml:"row_bytes,omitempty" json:"row_bytes,omitempty" jsonschema:"minimum=16,default=512"`
	Indexes  []string `yaml:"indexes,omitempty" json:"indexes,omitempty" jsonschema_description:"Indexed columns. id is always the primary key. Columns: id and user_id."`
}

// Workload is how traffic arrives.
type Workload struct {
	Entry     string   `yaml:"entry" json:"entry" jsonschema:"required" jsonschema_description:"Service receiving external traffic: an application service or an lb."`
	Profile   []*Step  `yaml:"profile" json:"profile" jsonschema:"required,minItems=1" jsonschema_description:"Steps run in order."`
	Mix       *Mix     `yaml:"mix,omitempty" json:"mix,omitempty"`
	Keys      *Keys    `yaml:"keys,omitempty" json:"keys,omitempty"`
	PayloadKB *float64 `yaml:"payload_kb,omitempty" json:"payload_kb,omitempty" jsonschema:"minimum=0,default=1"`
	TimeoutMs *int     `yaml:"timeout_ms,omitempty" json:"timeout_ms,omitempty" jsonschema:"minimum=1,default=10000" jsonschema_description:"How long the load generator waits for a response before counting the request as failed."`
	SLO       *SLO     `yaml:"slo,omitempty" json:"slo,omitempty"`
}

// SLO is the latency users accept. The verdict counts a break on latency
// alone only above it.
type SLO struct {
	P99Ms *float64 `yaml:"p99_ms,omitempty" json:"p99_ms,omitempty" jsonschema:"exclusiveMinimum=0,default=500" jsonschema_description:"End-to-end p99 users accept, in ms. A break on p99 alone needs p99 at or above this and at least 10x the low-load p99."`
}

// Step is one stage of the load profile. Exactly one field is set.
type Step struct {
	Ramp  *Ramp  `yaml:"ramp,omitempty" json:"ramp,omitempty" jsonschema_description:"Change the arrival rate linearly."`
	Hold  *Hold  `yaml:"hold,omitempty" json:"hold,omitempty" jsonschema_description:"Keep a constant arrival rate."`
	Spike *Spike `yaml:"spike,omitempty" json:"spike,omitempty" jsonschema_description:"Jump to a rate, then return to the rate before the spike."`
}

// Ramp changes the arrival rate linearly from From to To.
type Ramp struct {
	From float64  `yaml:"from" json:"from" jsonschema:"required,minimum=0"`
	To   float64  `yaml:"to" json:"to" jsonschema:"required,minimum=0"`
	Over Duration `yaml:"over" json:"over" jsonschema:"required"`
}

// Hold keeps a constant arrival rate.
type Hold struct {
	RPS float64  `yaml:"rps" json:"rps" jsonschema:"required,exclusiveMinimum=0"`
	For Duration `yaml:"for" json:"for" jsonschema:"required"`
}

// Spike jumps to a rate for a while, then returns to the previous rate.
type Spike struct {
	To  float64  `yaml:"to" json:"to" jsonschema:"required,exclusiveMinimum=0"`
	For Duration `yaml:"for" json:"for" jsonschema:"required"`
}

// Mix is the read/write split of requests.
type Mix struct {
	Read  *float64 `yaml:"read,omitempty" json:"read,omitempty" jsonschema:"minimum=0,maximum=1,default=0.9"`
	Write *float64 `yaml:"write,omitempty" json:"write,omitempty" jsonschema:"minimum=0,maximum=1,default=0.1"`
}

// Keys describes which keys requests touch.
type Keys struct {
	Distribution string   `yaml:"distribution,omitempty" json:"distribution,omitempty" jsonschema:"enum=uniform,enum=zipf,enum=hotspot,default=uniform"`
	Skew         *float64 `yaml:"skew,omitempty" json:"skew,omitempty" jsonschema:"exclusiveMinimum=0,default=1.1" jsonschema_description:"zipf exponent."`
	Share        *float64 `yaml:"share,omitempty" json:"share,omitempty" jsonschema:"exclusiveMinimum=0,maximum=1,default=0.8" jsonschema_description:"hotspot: fraction of requests that hit the hot keys."`
	Fraction     *float64 `yaml:"fraction,omitempty" json:"fraction,omitempty" jsonschema:"exclusiveMinimum=0,maximum=1,default=0.01" jsonschema_description:"hotspot: fraction of keys that are hot."`
	Count        *int64   `yaml:"count,omitempty" json:"count,omitempty" jsonschema:"minimum=1" jsonschema_description:"Distinct keys the workload touches. Defaults to the dataset rows of the datastore the entry path reads; required when that is ambiguous; can't exceed that row count."`
}

// Fault is one timed fault injection.
type Fault struct {
	At       Duration   `yaml:"at" json:"at" jsonschema:"required" jsonschema_description:"When the fault starts, from the start of the workload."`
	Inject   string     `yaml:"inject" json:"inject" jsonschema:"required,enum=latency,enum=loss,enum=bandwidth,enum=timeout,enum=reset,enum=kill,enum=pause,enum=restart,enum=partition,enum=disk_slow"`
	Target   *TargetRef `yaml:"target,omitempty" json:"target,omitempty" jsonschema_description:"A service name, or name[i] for one replica. Not used by partition."`
	For      *Duration  `yaml:"for,omitempty" json:"for,omitempty" jsonschema_description:"How long the fault lasts. Default: to the end of the run. Required for pause."`
	AddMs    *int       `yaml:"add_ms,omitempty" json:"add_ms,omitempty" jsonschema:"minimum=1" jsonschema_description:"latency and disk_slow."`
	JitterMs *int       `yaml:"jitter_ms,omitempty" json:"jitter_ms,omitempty" jsonschema:"minimum=0" jsonschema_description:"latency only."`
	Percent  *float64   `yaml:"percent,omitempty" json:"percent,omitempty" jsonschema:"exclusiveMinimum=0,maximum=100" jsonschema_description:"loss only."`
	Kbps     *int       `yaml:"kbps,omitempty" json:"kbps,omitempty" jsonschema:"minimum=1" jsonschema_description:"bandwidth only."`
	Between  []string   `yaml:"between,omitempty" json:"between,omitempty" jsonschema:"minItems=2,maxItems=2" jsonschema_description:"partition only: the two services to cut apart, each a name or name[i] for one replica."`
}

// Assumption records a value the user did not state, and why it was chosen.
type Assumption struct {
	Field string      `yaml:"field" json:"field" jsonschema:"required" jsonschema_description:"Path of the field, such as services.api.behavior.calls[0].count."`
	Value ConfigValue `yaml:"value,omitempty" json:"value,omitempty"`
	Basis string      `yaml:"basis" json:"basis" jsonschema:"required,enum=user,enum=from-code,enum=measured,enum=guessed"`
	Why   string      `yaml:"why,omitempty" json:"why,omitempty"`
}

// Modes holds per-mode overrides.
type Modes struct {
	Local *LocalMode `yaml:"local,omitempty" json:"local,omitempty"`
}

// LocalMode overrides for local Docker runs.
type LocalMode struct {
	Scale *float64 `yaml:"scale,omitempty" json:"scale,omitempty" jsonschema:"exclusiveMinimum=0,default=1" jsonschema_description:"Shrinks resources and load together. The --scale flag overrides it."`
}

// Pool is a service's connection pool to one target, per replica. A plain
// number is its size.
type Pool struct {
	Size *int   `yaml:"size,omitempty" json:"size,omitempty" jsonschema:"minimum=1,default=10" jsonschema_description:"Connections per replica."`
	Min  *int   `yaml:"min,omitempty" json:"min,omitempty" jsonschema:"minimum=0,default=0" jsonschema_description:"Connections each replica opens before the load starts and keeps open: a warm pool. postgres and pgbouncer targets only. Default 0: connections open only when callers need them, as pgxpool does by default."`
	Hold string `yaml:"hold,omitempty" json:"hold,omitempty" jsonschema:"enum=call,enum=request,default=call" jsonschema_description:"How long a request keeps a connection: for each call (call), or from its first call to the target until it responds, slower calls in between included (request), as Rails and Django check out a connection per request. postgres and pgbouncer targets only."`
}
