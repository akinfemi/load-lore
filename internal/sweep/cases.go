package sweep

// Case is one small design that stresses one operation on one component,
// the target, whose CPU the sweep varies. Everything else is sized so the
// target gives out first; a run whose break names another component is
// marked so in the summary rather than counted.
type Case struct {
	Name   string
	What   string
	Target string
	// Also are other components a run may break at and still measure the
	// operation: the fan-out's second shard, loaded the same as the first.
	Also []string
	// PerCPU is an estimate of the requests a second one CPU of the target
	// serves at its break, at Calls calls a request; the ramp climbs to
	// twice that times the CPU, and doubles when a run holds. They are the
	// medians of the 18-19 Nov full sweep, except writes, which held at
	// every size it tried (12,000 a second at 1 CPU, 94% busy).
	PerCPU float64
	// Calls is how many calls to the target each request makes at least.
	// A point whose ramp would need the load generator past MaxRPS makes
	// more calls a request instead, unless Fixed: one request is one call
	// by nature (a hop through the load balancer).
	Calls int
	Fixed bool
	// MaxCPU caps the sizes worth running: Redis runs commands on one
	// thread. 0 for none.
	MaxCPU float64
	// Design is a text/template over Params.
	Design string
}

// Params fill a case's design.
type Params struct {
	Name      string
	CPU       float64
	Calls     int // calls to the target a request
	Low, Peak int // requests a second at the bottom and top of the ramp
}

// The callers in front of the target: one api replica with 2 CPUs and no
// work of its own, so its calls are all it spends CPU on, and a Postgres
// on the Docker host's own disk (laptop), so a CPU sweep isn't a disk one.
const (
	apiHead = `version: 1
name: {{.Name}}

services:
  api:
    image: loadlore/synthetic
    resources: { cpu: 2, memory: 512m }
    behavior:
      cpu_ms: 0
      calls:
`
	ramp = `
workload:
  entry: api
  mix: { read: 1, write: 0 }
  keys: { distribution: uniform, count: 100000 }
  profile:
    - ramp: { from: {{.Low}}, to: {{.Peak}}, over: 3m }
    - hold: { rps: {{.Peak}}, for: 30s }
`
	pgTarget = `
  db:
    kind: postgres
    version: "17"
    resources: { cpu: {{.CPU}}, memory: 1g }
    config: { max_connections: 100, shared_buffers: 256MB }
    dataset: { rows: 100000, row_bytes: 256 }
    disk: { profile: laptop }
`
	dbPool = `    pools: { db: { size: 64, min: 64 } }
`
)

// Cases is the sweep's catalogue, in the order it runs.
var Cases = []Case{
	{Name: "pg-point-read", What: "Postgres reads of one row by key", Target: "db", PerCPU: 4300, Calls: 10,
		Design: apiHead + `        - { target: db, op: read, count: {{.Calls}} }
` + dbPool + pgTarget + ramp},
	{Name: "pg-range-read", What: "Postgres reads of 50 rows in key order", Target: "db", PerCPU: 3900, Calls: 5,
		Design: apiHead + `        - { target: db, op: read, query: range, rows: 50, count: {{.Calls}} }
` + dbPool + pgTarget + ramp},
	{Name: "pg-batch-read", What: "Postgres reads of 20 rows by key in one statement", Target: "db", PerCPU: 6000, Calls: 5,
		Design: apiHead + `        - { target: db, op: read, query: batch, rows: 20, count: {{.Calls}} }
` + dbPool + pgTarget + ramp},
	{Name: "pg-write", What: "Postgres single-row updates, each committed and flushed to the WAL", Target: "db", PerCPU: 12000, Calls: 1,
		Design: apiHead + `        - { target: db, op: write, count: {{.Calls}} }
` + dbPool + pgTarget + `
workload:
  entry: api
  mix: { read: 0, write: 1 }
  keys: { distribution: uniform, count: 100000 }
  profile:
    - ramp: { from: {{.Low}}, to: {{.Peak}}, over: 3m }
    - hold: { rps: {{.Peak}}, for: 30s }
`},
	{Name: "redis-get", What: "Redis gets", Target: "cache", PerCPU: 26000, Calls: 10, MaxCPU: 1,
		Design: apiHead + `        - { target: cache, op: get, count: {{.Calls}} }
    pools: { cache: 64 }

  cache:
    kind: redis
    resources: { cpu: {{.CPU}}, memory: 512m }
` + ramp},
	{Name: "redis-set", What: "Redis sets", Target: "cache", PerCPU: 25000, Calls: 10, MaxCPU: 1,
		Design: apiHead + `        - { target: cache, op: set, count: {{.Calls}} }
    pools: { cache: 64 }

  cache:
    kind: redis
    resources: { cpu: {{.CPU}}, memory: 512m }
` + ramp},
	{Name: "app-hop", What: "HTTP calls from one service to another that does no work", Target: "backend", PerCPU: 6600, Calls: 10,
		Design: apiHead + `        - { target: backend, op: http, count: {{.Calls}} }
    pools: { backend: 64 }

  backend:
    image: loadlore/synthetic
    resources: { cpu: {{.CPU}}, memory: 512m }
    behavior:
      cpu_ms: 0
` + ramp},
	{Name: "lb-hop", What: "requests through the load balancer to services that do no work", Target: "lb", PerCPU: 29000, Calls: 1, Fixed: true, MaxCPU: 0.5,
		Design: `version: 1
name: {{.Name}}

services:
  lb:
    kind: lb
    target: api
    resources: { cpu: {{.CPU}}, memory: 256m }

  api:
    image: loadlore/synthetic
    replicas: 2
    resources: { cpu: 1.5, memory: 512m }
    behavior:
      cpu_ms: 0
` + `
workload:
  entry: lb
  mix: { read: 1, write: 0 }
  keys: { distribution: uniform, count: 100000 }
  profile:
    - ramp: { from: {{.Low}}, to: {{.Peak}}, over: 3m }
    - hold: { rps: {{.Peak}}, for: 30s }
`},
	{Name: "pg-replica-read", What: "reads of one row by key from a streaming read replica", Target: "ro", PerCPU: 4300, Calls: 10,
		Design: apiHead + `        - { target: ro, op: read, count: {{.Calls}} }
    pools: { ro: { size: 64, min: 64 } }

  db:
    kind: postgres
    version: "17"
    resources: { cpu: 1, memory: 1g }
    config: { max_connections: 100, shared_buffers: 256MB }
    dataset: { rows: 100000, row_bytes: 256 }
    disk: { profile: laptop }

  ro:
    kind: postgres-replica
    primary: db
    resources: { cpu: {{.CPU}}, memory: 1g }
    config: { max_connections: 100, shared_buffers: 256MB }
    disk: { profile: laptop }
` + ramp},
	{Name: "queue-publish", What: "jobs published to a queue (Redis Streams), taken by a worker that does no work", Target: "jobs", PerCPU: 19000, Calls: 5, MaxCPU: 1,
		Design: apiHead + `        - { target: jobs, op: publish, count: {{.Calls}} }
    pools: { jobs: 64 }

  jobs:
    kind: queue
    resources: { cpu: {{.CPU}}, memory: 512m }

  worker:
    image: loadlore/synthetic
    role: worker
    resources: { cpu: 2, memory: 256m }
    consume: { from: jobs, concurrency: 50 }
    behavior:
      cpu_ms: 0
` + ramp},
	{Name: "pg-shard-fanout", What: "reads of one row from every one of two Postgres shards at once", Target: "db0", Also: []string{"db1"}, PerCPU: 6700, Calls: 5,
		Design: apiHead + `        - { shards: [db0, db1], shard_by: all, op: read, count: {{.Calls}} }
    pools: { db0: { size: 48, min: 48 }, db1: { size: 48, min: 48 } }

  db0:
    kind: postgres
    version: "17"
    resources: { cpu: {{.CPU}}, memory: 1g }
    config: { max_connections: 100, shared_buffers: 256MB }
    dataset: { rows: 50000, row_bytes: 256 }
    disk: { profile: laptop }
  db1:
    kind: postgres
    version: "17"
    resources: { cpu: {{.CPU}}, memory: 1g }
    config: { max_connections: 100, shared_buffers: 256MB }
    dataset: { rows: 50000, row_bytes: 256 }
    disk: { profile: laptop }
` + ramp},
}
