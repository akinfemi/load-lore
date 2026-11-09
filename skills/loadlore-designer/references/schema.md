# Loadlore design file schema (v1, draft)

`loadl schema --json` is the authority for the installed version. This file explains the fields and their defaults.

## Top level

| Key | Required | Purpose |
| --- | --- | --- |
| `version` | yes | Always `1` for now |
| `name` | yes | Short kebab-case name, used in run ids and reports |
| `services` | yes | Map of service name to service definition |
| `workload` | yes | How traffic arrives |
| `scenario` | no | Timed fault injections |
| `expect` | no | The user's predictions |
| `assumptions` | no, but always written by AI | Every inferred value and its basis |
| `modes` | no | Per-mode overrides |

Service names: lowercase, digits and `-`. Connections between services are inferred from `calls`; there is no separate edges list.

## Services

Every service has either `image` (an application service) or `kind` (a built-in datastore or infrastructure component).

### Common fields

| Field | Default | Notes |
| --- | --- | --- |
| `replicas` | `1` | |
| `resources.cpu` | `1` | CPU cores per replica (cgroup limit locally) |
| `resources.memory` | `512m` | Per replica |

### Application services (`image: loadlore/synthetic`)

See `synthetic-service.md` for how to choose values.

```yaml
api:
  image: loadlore/synthetic        # or the user's own image
  role: server                  # server (default) | worker
  replicas: 2
  resources: { cpu: 1, memory: 512m }
  behavior:
    cpu_ms: 5                   # mean CPU time per request
    cpu_jitter: 0.2             # optional, fraction of cpu_ms, default 0.1
    alloc_kb: 64                # optional, memory allocated per request
    calls:                      # executed in order for each request
      - { target: cache, op: get, on_miss: [ { target: db, op: read }, { target: cache, op: set, ttl_s: 300 } ] }
      - { target: db, op: read, count: 3 }
      - { target: db, op: write, count: 1, when: write }
      - { target: payments, op: http, mode: sync }
      - { target: orders, op: publish, mode: async, when: write }
    resilience:
      timeout_ms: 1000          # per outbound call, default 5000; 0 = no timeout
      retries: { max: 3, backoff: exponential, base_ms: 50, jitter: true }   # default: no retries
      circuit_breaker: { error_rate: 0.5, window_s: 10, open_s: 30 }         # default: none
      bulkhead: { max_concurrent: 50 }                                       # default: none
  pools: { db: 20, cache: 50 }  # connections per replica, per target; default 10
  concurrency: { model: threads, max: 200 }   # threads (default) | event-loop | workers
```

`concurrency` approximates the real runtime: `threads` (Go, Java, .NET; `max` = in-flight requests per replica), `event-loop` (Node.js; CPU work is serialized on one core per replica, I/O is concurrent), `workers` (Python/Ruby process or thread pools such as gunicorn or puma; `max` = worker count, one request per worker at a time).

Call fields:

| Field | Default | Values |
| --- | --- | --- |
| `target` | required | A service name |
| `op` | required | `read`, `write` (datastores) · `get`, `set`, `del` (caches) · `http` (services) · `publish` (queues) |
| `count` | `1` | Times per request |
| `when` | `always` | `always`, `read`, `write` (from the workload mix) |
| `mode` | `sync` | `sync` waits for the result; `async` fires and continues |
| `on_miss` | none | Calls to run on a cache miss (`get` only) |
| `ttl_s` | none | For `set` |
| `query` | `point` | `point` (by key), `range` (rows: n), `scan` (full table) for datastore reads |

Workers (`role: worker`) add `consume: { from: <queue>, concurrency: 8 }` and run their `calls` once per message instead of per HTTP request.

### Datastores and infrastructure (`kind`)

| `kind` | Key fields | Notes |
| --- | --- | --- |
| `postgres` | `version` (default `"17"`), `config` (`max_connections` 100, `shared_buffers` 128MB, any postgresql.conf key), `dataset` | MVP |
| `redis` | `version`, `config` (`maxmemory`, `maxmemory-policy`) | MVP |
| `pgbouncer` | `target`, `pool_mode` (transaction), `default_pool_size` | MVP |
| `kafka`, `rabbitmq` | `partitions`, `retention` | Phase 2 |
| `postgres-replica` | `primary`, `lag_ms` emerges, not set | Phase 2 |

`dataset` (datastores):

```yaml
dataset: { rows: 5000000, row_bytes: 512, indexes: [id, user_id] }
```

There is deliberately no field for hit rate, capacity or failure point. Those emerge from the run.

## Workload

```yaml
workload:
  entry: api                    # service receiving external traffic
  profile:                      # steps run in order
    - ramp:  { from: 100, to: 5000, over: 5m }
    - hold:  { rps: 5000, for: 2m }
    - spike: { to: 20000, for: 30s }
  mix: { read: 0.9, write: 0.1 }          # default 0.9 / 0.1
  keys: { distribution: zipf, skew: 1.1 } # uniform | zipf (skew) | hotspot (share, fraction)
  payload_kb: 2                           # default 1
```

`hotspot` example: `{ distribution: hotspot, share: 0.8, fraction: 0.01 }` means 80% of requests hit 1% of keys.

Durations: `30s`, `5m`, `1h`.

## Scenario

A list of fault injections, each with `at`, `inject` and `target`. Optional `for` ends the fault after that duration; otherwise it lasts to the end of the run.

| `inject` | Fields | Mechanism |
| --- | --- | --- |
| `latency` | `add_ms`, `jitter_ms` | Toxiproxy |
| `loss` | `percent` | tc netem |
| `bandwidth` | `kbps` | Toxiproxy |
| `timeout` | | Toxiproxy (connections hang) |
| `reset` | | Toxiproxy (connections reset) |
| `kill` | | Container killed, not restarted |
| `pause` | `for` required | Container frozen |
| `restart` | | Container restarted (memory and caches cleared) |
| `partition` | `between: [a, b]` | tc netem |
| `disk_slow` | `add_ms` | Device throttling |

`target` is a service name, or `name[i]` for one replica.

```yaml
scenario:
  - { at: 3m, inject: latency, target: db, add_ms: 50, for: 2m }
  - { at: 6m, inject: restart, target: cache }
```

## Expect

One prediction per line: `<metric> <op> <value> [until|after <condition> | throughout]`.

Metrics: `p50_ms`, `p95_ms`, `p99_ms`, `error_rate`, `rps` (end to end), and per component `<service>.<metric>` such as `db.connections`, `db.cpu`, `cache.hit_rate`, `api.cpu`.

```yaml
expect:
  - p99_ms < 300 until rps >= 3000
  - error_rate < 1% throughout
  - cache.hit_rate > 80% after 1m
```

Write predictions in the user's words translated to this form. If the user's prediction can't be expressed, keep it as a quoted note: `- note: "the database will be the bottleneck"`.

## Assumptions

Written by the AI for every value the user did not state. Shown in every report.

```yaml
assumptions:
  - { field: "services.api.behavior.cpu_ms", value: 5, basis: guessed, why: "JSON API handler with light validation" }
  - { field: "services.api.behavior.calls[0].count", value: 3, basis: from-code, why: "src/orders/handler.go:GetOrder runs 3 queries" }
  - { field: "workload.mix", value: "0.9/0.1", basis: user }
```

Always quote `field` paths: brackets inside an unquoted flow mapping are invalid YAML.

`basis`: `user`, `from-code`, `measured`, `guessed`.

## Modes

```yaml
modes:
  local: { scale: 0.5 }
  cloud: { provider: aws, region: eu-west-3, budget_usd: 10, instance: { db: db.t4g.medium } }
```

`budget_usd` in the file never replaces the `--budget` flag; the CLI requires both to agree.
