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
	Kind      string     `yaml:"kind,omitempty" json:"kind,omitempty" jsonschema:"enum=postgres,enum=redis,enum=pgbouncer,enum=lb" jsonschema_description:"Built-in component. Mutually exclusive with image."`
	Replicas  *int       `yaml:"replicas,omitempty" json:"replicas,omitempty" jsonschema:"minimum=1,default=1" jsonschema_description:"Number of replicas. Datastores, pgbouncer and lb run exactly one."`
	Resources *Resources `yaml:"resources,omitempty" json:"resources,omitempty" jsonschema_description:"Default 1 CPU and 512m; lb defaults to 0.5 CPU and 256m."`

	// Application services.
	Role        string          `yaml:"role,omitempty" json:"role,omitempty" jsonschema:"enum=server,default=server" jsonschema_description:"server handles HTTP requests."`
	Behavior    *Behavior       `yaml:"behavior,omitempty" json:"behavior,omitempty" jsonschema_description:"What the synthetic service does per request. Only for loadlore/synthetic."`
	Pools       map[string]Pool `yaml:"pools,omitempty" json:"pools,omitempty" jsonschema_description:"Connection pool per call target: a size (connections per replica, default 10), or {size, min}."`
	Concurrency *Concurrency    `yaml:"concurrency,omitempty" json:"concurrency,omitempty"`

	// Datastores and infrastructure.
	Version string                 `yaml:"version,omitempty" json:"version,omitempty" jsonschema_description:"Major version. postgres default \"17\"; redis default \"8\"."`
	Config  map[string]ConfigValue `yaml:"config,omitempty" json:"config,omitempty" jsonschema_description:"Native config keys: postgresql.conf for postgres, redis.conf for redis, pgbouncer.ini for pgbouncer."`
	Dataset *Dataset               `yaml:"dataset,omitempty" json:"dataset,omitempty" jsonschema_description:"Rows seeded before the run. postgres only."`

	// pgbouncer and lb.
	Target string `yaml:"target,omitempty" json:"target,omitempty" jsonschema_description:"pgbouncer: the postgres service it pools connections to. lb: the application service whose replicas it spreads traffic across."`

	// pgbouncer.
	PoolMode        string `yaml:"pool_mode,omitempty" json:"pool_mode,omitempty" jsonschema:"enum=session,enum=transaction,enum=statement,default=transaction"`
	DefaultPoolSize *int   `yaml:"default_pool_size,omitempty" json:"default_pool_size,omitempty" jsonschema:"minimum=1,default=20" jsonschema_description:"Server connections per database and user pair."`

	// lb.
	Algorithm   string       `yaml:"algorithm,omitempty" json:"algorithm,omitempty" jsonschema:"enum=round-robin,enum=least-conn,enum=random,default=round-robin" jsonschema_description:"How the lb picks a healthy replica for each request."`
	HealthCheck *HealthCheck `yaml:"health_check,omitempty" json:"health_check,omitempty" jsonschema_description:"Active health checks the lb runs against each replica."`
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
	Target string  `yaml:"target" json:"target" jsonschema:"required" jsonschema_description:"A service name."`
	Op     string  `yaml:"op" json:"op" jsonschema:"required,enum=read,enum=write,enum=get,enum=set,enum=del,enum=http" jsonschema_description:"read/write for postgres and pgbouncer; get/set/del for redis; http for application services."`
	Count  *int    `yaml:"count,omitempty" json:"count,omitempty" jsonschema:"minimum=1,default=1" jsonschema_description:"Times per request."`
	When   string  `yaml:"when,omitempty" json:"when,omitempty" jsonschema:"enum=always,enum=read,enum=write,default=always" jsonschema_description:"Run only for read or write requests from the workload mix."`
	Mode   string  `yaml:"mode,omitempty" json:"mode,omitempty" jsonschema:"enum=sync,enum=async,default=sync" jsonschema_description:"sync waits for the result; async fires and continues."`
	OnMiss []*Call `yaml:"on_miss,omitempty" json:"on_miss,omitempty" jsonschema_description:"Calls run on a cache miss. get only."`
	TTLS   *int    `yaml:"ttl_s,omitempty" json:"ttl_s,omitempty" jsonschema:"minimum=0" jsonschema_description:"Key TTL in seconds. set only."`
	Query  string  `yaml:"query,omitempty" json:"query,omitempty" jsonschema:"enum=point,enum=range,enum=batch,enum=scan,default=point" jsonschema_description:"Shape of a read: point (one row by key), range (rows in key order), batch (rows by key in one statement), scan (full table)."`
	Rows   *int    `yaml:"rows,omitempty" json:"rows,omitempty" jsonschema:"minimum=1,default=10" jsonschema_description:"Rows returned by a range read, or keys fetched by a batch read."`
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
	Between  []string   `yaml:"between,omitempty" json:"between,omitempty" jsonschema:"minItems=2,maxItems=2" jsonschema_description:"partition only: the two services to cut apart."`
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
	Size *int `yaml:"size,omitempty" json:"size,omitempty" jsonschema:"minimum=1,default=10" jsonschema_description:"Connections per replica."`
	Min  *int `yaml:"min,omitempty" json:"min,omitempty" jsonschema:"minimum=0,default=0" jsonschema_description:"Connections each replica opens before the load starts and keeps open: a warm pool. postgres and pgbouncer targets only. Default 0: connections open only when callers need them, as pgxpool does by default."`
}
