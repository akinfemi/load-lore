# Loadlore design file schema (v1)

`loadl schema --json` is the authority for the installed version. This file explains the fields and their defaults. A CI check keeps the two in sync: every field documented here (outside **Planned**) exists in the schema, and every schema field is documented here.

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

Service names: lowercase letters, digits and `-`. Connections between services are inferred from `calls`; there is no separate edges list.

## Services

Every service has either `image` (an application service) or `kind` (a built-in datastore or infrastructure component), never both.

### Common fields

| Field | Default | Notes |
| --- | --- | --- |
| `replicas` | `1` | Application services only; datastores and pgbouncer run exactly one |
| `resources.cpu` | `1` | CPU cores per replica (cgroup limit locally) |
| `resources.memory` | `512m` | Per replica. Units `k`, `m`, `g` (binary), also `256MB`, `1Gi` |

### Application services (`image: loadlore/synthetic`)

See `synthetic-service.md` for how to choose values.

```yaml
api:
  image: loadlore/synthetic        # or the user's own image (then no behavior)
  role: server                  # the only role for now (default)
  replicas: 2
  resources: { cpu: 1, memory: 512m }
  behavior:
    cpu_ms: 5                   # mean CPU time per request
    cpu_jitter: 0.2             # optional, fraction of cpu_ms, default 0.1
    alloc_kb: 64                # optional, memory allocated per request
    calls:                      # executed in order for each request
      - { target: cache, op: get, on_miss: [ { target: db, op: read }, { target: cache, op: set, ttl_s: 300 } ] }
      - { target: db, op: read, count: 3 }
      - { target: db, op: read, query: range, rows: 50 }
      - { target: db, op: write, count: 1, when: write }
      - { target: payments, op: http, mode: sync }
      - { target: audit, op: http, mode: async, when: write }
    resilience:
      timeout_ms: 1000          # per outbound call, default 5000; 0 = no timeout
      retries: { max: 3, backoff: exponential, base_ms: 50, jitter: true }   # default: no retries
      circuit_breaker: { error_rate: 0.5, window_s: 10, open_s: 30 }         # default: none
      bulkhead: { max_concurrent: 50 }                                       # default: none
  pools: { db: 20, cache: 50 }  # connections per replica, per target; default 10
  concurrency: { model: threads, max: 200 }   # threads (default) | event-loop | workers
```

`concurrency` approximates the real runtime:

| `model` | `max` means | Default `max` |
| --- | --- | --- |
| `threads` (Go, Java, .NET) | in-flight requests per replica | `200` |
| `event-loop` (Node.js) | in-flight requests per replica; CPU work is serialized on one core per replica, I/O is concurrent | `0` (unlimited) |
| `workers` (Python/Ruby: gunicorn, puma) | worker count, one request per worker at a time | `4` |

Call fields:

| Field | Default | Values |
| --- | --- | --- |
| `target` | required | A service name |
| `op` | required | `read`, `write` (postgres, pgbouncer) · `get`, `set`, `del` (redis) · `http` (application services) |
| `count` | `1` | Times per request |
| `when` | `always` | `always`, `read`, `write` (from the workload mix) |
| `mode` | `sync` | `sync` waits for the result; `async` fires and continues |
| `on_miss` | none | Calls to run on a cache miss (`get` only) |
| `ttl_s` | none | For `set` only |
| `query` | `point` | `point` (by key), `range` (`rows` rows), `scan` (full table); `read` only |
| `rows` | `10` | Rows per `range` read |

Resilience fields apply to every outbound call of the service:

| Field | Default | Notes |
| --- | --- | --- |
| `timeout_ms` | `5000` | `0` means no timeout |
| `retries.max` | required with `retries` | Retries after the first attempt. No `retries` block = no retries |
| `retries.backoff` | `none` | `none` (retry immediately), `constant`, `exponential` |
| `retries.base_ms` | `100` | First delay for `constant` and `exponential` |
| `retries.jitter` | `false` | Randomize each delay (full jitter); not with `none` |
| `circuit_breaker` | none | `error_rate` (0–1), `window_s`, `open_s`, all required |
| `bulkhead` | none | `max_concurrent`, required |

### Datastores and infrastructure (`kind`)

| `kind` | Key fields | Notes |
| --- | --- | --- |
| `postgres` | `version` (default `"17"`), `config` (any postgresql.conf key; Postgres's own defaults apply, such as `max_connections` 100 and `shared_buffers` 128MB), `dataset` | |
| `redis` | `version` (default `"8"`), `config` (any redis.conf key, such as `maxmemory` and `maxmemory-policy`; Redis's own defaults apply) | |
| `pgbouncer` | `target` (required, a postgres service), `pool_mode` (`transaction`; also `session`, `statement`), `default_pool_size` (`20`), `config` (any pgbouncer.ini key) | |

`dataset` (postgres only) is seeded before the run into a table with columns `id` (primary key), `user_id` and a payload:

```yaml
dataset: { rows: 5000000, row_bytes: 512, indexes: [id, user_id] }
```

`rows` is required; `row_bytes` defaults to `512`; `indexes` lists extra indexed columns (`id` is always indexed).

There is deliberately no field for hit rate, capacity or failure point. Those emerge from the run, and `loadl validate` rejects such fields with an explanation.

## Workload

```yaml
workload:
  entry: api                    # application service receiving external traffic
  profile:                      # steps run in order
    - ramp:  { from: 100, to: 5000, over: 5m }
    - hold:  { rps: 5000, for: 2m }
    - spike: { to: 20000, for: 30s }       # then back to the rate before the spike
  mix: { read: 0.9, write: 0.1 }          # default 0.9 / 0.1; give one and the other is the rest
  keys: { distribution: zipf, skew: 1.1, count: 5000000 }
  payload_kb: 2                           # default 1
```

Each profile step has exactly one of `ramp`, `hold` or `spike`. Load is open-model: the rate is what arrives, whether or not the system keeps up.

`keys`:

| Field | Default | Notes |
| --- | --- | --- |
| `distribution` | `uniform` | `uniform`, `zipf`, `hotspot` |
| `skew` | `1.1` | `zipf` only: the exponent |
| `share`, `fraction` | `0.8`, `0.01` | `hotspot` only: `share` of requests hit `fraction` of keys |
| `count` | see below | Distinct keys the workload touches |

`hotspot` example: `{ distribution: hotspot, share: 0.8, fraction: 0.01 }` means 80% of requests hit 1% of keys.

Keyspace: `count` defaults to the `dataset.rows` of the datastore the entry path reads. Validation requires it when that is ambiguous (several datasets of different sizes, or none), and it can't exceed that row count.

Durations: `30s`, `5m`, `1h`, `1m30s`, `500ms`. A unit is required.

## Scenario

A list of fault injections, each with `at` and `inject`, and a `target` (except `partition`). Optional `for` ends the fault after that duration; otherwise it lasts to the end of the run. A fault must start before the workload ends.

| `inject` | Fields | Mechanism |
| --- | --- | --- |
| `latency` | `add_ms` (required), `jitter_ms` | Toxiproxy |
| `loss` | `percent` (required) | tc netem |
| `bandwidth` | `kbps` (required) | Toxiproxy |
| `timeout` | | Toxiproxy (connections hang) |
| `reset` | | Toxiproxy (connections reset) |
| `kill` | | Container killed, not restarted |
| `pause` | `for` required | Container frozen |
| `restart` | | Container restarted (memory and caches cleared) |
| `partition` | `between: [a, b]` instead of `target` | tc netem |
| `disk_slow` | `add_ms` (required) | Device throttling; datastores only |

`target` is a service name, or `name[i]` for one replica. Connection faults (`latency`, `loss`, `bandwidth`, `timeout`, `reset`) act on connections into the target, so something must call it (or it is the workload entry).

```yaml
scenario:
  - { at: 3m, inject: latency, target: db, add_ms: 50, for: 2m }
  - { at: 6m, inject: restart, target: cache }
  - { at: 7m, inject: partition, between: [api, cache], for: 30s }
```

## Expect

One prediction per line:

```
<metric> <op> <value> [until <condition> | after <time> | throughout]
```

- `op` is `<`, `<=`, `>` or `>=`.
- A line with no qualifier means `throughout`.
- `until <condition>` checks the prediction from the start until the condition first holds. Conditions usually use `load_rps`, the offered load (what k6 sends), so the line can still be scored when the system saturates below it. `rps` is achieved throughput.
- `after <time>` checks it from that time (from the start of the workload) to the end.
- Percentiles and rates are computed over 10-second windows. A `throughout` or `until` line breaks if any window in its range breaks.

End-to-end metrics: `p50_ms`, `p95_ms`, `p99_ms`, `error_rate`, `rps`, and `load_rps` (conditions only).

Per-component metrics, written `<service>.<metric>`:

| Component | Metrics |
| --- | --- |
| postgres | `connections`, `cpu`, `memory` |
| redis | `hit_rate`, `memory`, `cpu` |
| application services | `cpu`, `memory`, `p99_ms`, `error_rate`, `inflight` |

Values: latencies in milliseconds (`300`, `300ms` or `0.3s`); `error_rate`, `hit_rate`, `cpu` and `memory` as a percentage (`5%`) or fraction (`0.05`), where `cpu` and `memory` are utilization of the container's limit; rates and counts as plain numbers.

```yaml
expect:
  - p99_ms < 300 until load_rps >= 3000
  - error_rate < 1% throughout
  - cache.hit_rate > 80% after 1m
  - db.connections <= 96
  - note: "the database will be the bottleneck"
```

Write predictions in the user's words translated to this form. If the user's prediction can't be expressed, keep it as a quoted `note`; notes are shown in the report but not scored.

## Assumptions

Written by the AI for every value the user did not state. Shown in every report.

```yaml
assumptions:
  - { field: "services.api.behavior.cpu_ms", value: 5, basis: guessed, why: "JSON API handler with light validation" }
  - { field: "services.api.behavior.calls[0].count", value: 3, basis: from-code, why: "src/orders/handler.go:GetOrder runs 3 queries" }
  - { field: "workload.mix", value: "0.9/0.1", basis: user }
```

Always quote `field` paths: brackets inside an unquoted flow mapping are invalid YAML. `loadl validate` warns when a `field` path doesn't point at a field of the design.

`basis`: `user`, `from-code`, `measured`, `guessed`.

## Modes

```yaml
modes:
  local: { scale: 0.5 }
```

`scale` (default `1`) shrinks resources and load together in local mode. The `--scale` flag overrides it.

## Planned

Documented for later phases. `loadl validate` rejects these with a "planned" error, and the schema sync check skips this section.

- **Workers** (phase 2): `role: worker` with `consume: { from: <queue>, concurrency: 8 }`, running `calls` once per message instead of per HTTP request.
- **Queues** (phase 2): `kind: kafka` or `kind: rabbitmq` with `partitions` and `retention`, and the call op `publish`, e.g. `{ target: orders, op: publish, mode: async, when: write }`.
- **Read replicas** (phase 2): `kind: postgres-replica` with `primary`. Replication lag emerges; it is never set.
- **Cloud mode** (phase 2):

```yaml
modes:
  cloud: { provider: aws, region: eu-west-3, budget_usd: 10, instance: { db: db.t4g.medium } }
```

`budget_usd` in the file never replaces the `--budget` flag; the CLI requires both to agree.
