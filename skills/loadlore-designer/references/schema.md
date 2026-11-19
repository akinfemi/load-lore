# Loadlore design file schema (v1)

`loadl schema --json` is the authority for the installed version. This file explains the fields and their defaults. A CI check keeps the two in sync: every field documented here (outside **Planned**) exists in the schema, and every schema field is documented here.

## Top level

| Key | Required | Purpose |
| --- | --- | --- |
| `version` | yes | Always `1` for now |
| `name` | yes | Short kebab-case name, used in run ids and reports |
| `services` | yes | Map of service name to service definition |
| `workload` | yes | How traffic arrives |
| `network` | no | The network between the containers: `{ profile: same-zone }` by default (see Network) |
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
| `replicas` | `1` | Application services and `postgres-replica`; other datastores, pgbouncer and lb run exactly one |
| `resources.cpu` | `1` (`0.5` for `lb`) | CPU cores per replica (cgroup limit locally) |
| `resources.memory` | `512m` (`256m` for `lb`) | Per replica. Units `k`, `m`, `g` (binary), also `256MB`, `1Gi` |
| `resources.cpu_period_ms` | `100` (Docker's default) | Period over which the CPU limit is enforced, 1–1000. A 0.5-CPU container may run 50 ms in each 100 ms period, then waits for the next one: bursts of work stall, which shows in p99. Shorter periods make a small limit behave more like a slower CPU; `100` or more reproduces Kubernetes-style CPU throttling. Loadlore's own containers keep the default |

### Application services (`image: loadlore/synthetic`)

See `synthetic-service.md` for how to choose values.

```yaml
api:
  image: loadlore/synthetic        # or the user's own image (then no behavior)
  role: server                  # server (default) or worker
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

A pool is a size, or `{ size, min }`. `min` (default `0`, postgres and pgbouncer targets only) is how many connections each replica opens before the load starts and keeps open: a warm pool. With `0`, connections open only when callers need them, as pgxpool does by default, so a burst can make every replica open many Postgres connections at once (the connection storm rule names it). `pools: { db: { size: 24, min: 24 } }` opens all 24 up front. `loadl validate` rejects warm pools that add up, over every caller's replicas, to more connections than a Postgres's `max_connections` (100 unless `config` sets it) less one for Loadlore's exporter: they couldn't fill, and the run couldn't start. `hold` (default `call`, postgres and pgbouncer targets only) is how long a request keeps a connection: `call` takes one for each call and hands it back straight away; `request` keeps the connection from the request's first call to that target until it responds, through any slower calls in between, as Rails and Django check out a connection per request. A held connection sits idle while the request waits on something else, so a pool fills long before the database is busy. The report prints each pool's `min` and `hold` as assumptions.

`concurrency` approximates the real runtime:

| `model` | `max` means | Default `max` |
| --- | --- | --- |
| `threads` (Go, Java, .NET) | in-flight requests per replica | `200` |
| `event-loop` (Node.js) | in-flight requests per replica; CPU work is serialized on one core per replica, I/O is concurrent | `0` (unlimited) |
| `workers` (Python/Ruby: gunicorn, puma) | worker count, one request per worker at a time | `4` |

`autoscale` adds and removes replicas during the run, as a Kubernetes HorizontalPodAutoscaler does. `replicas` is how many it starts with, and, for a service that serves requests, an `lb` must sit in front: other services call the lb, not the service.

```yaml
api:
  image: loadlore/synthetic
  replicas: 2
  autoscale: { max: 6, metric: cpu, target: 0.7 }
```

| Field | Default | Meaning |
| --- | --- | --- |
| `min` | `replicas` | Fewest replicas |
| `max` | required | Most replicas. Loadlore budgets CPU and memory for all of them |
| `metric` | `cpu` | `cpu`: mean CPU of the running replicas against their limit, so it can't read above 100%. (Kubernetes measures against a pod's request; this matches a request equal to the limit.) `p99`: the service's own p99 in ms. `inflight`: mean requests in flight per replica. `queue_depth` (workers only): the jobs in the worker's queue, waiting or running, per replica taking them, as KEDA's Redis Streams scaler counts them |
| `target` | `0.7` for `cpu` | The value to hold: a fraction for `cpu`, milliseconds for `p99`, requests for `inflight`, jobs per replica for `queue_depth` (required for those three) |
| `interval_s` | `15` | Seconds between decisions; the metric is averaged over this window |
| `scale_up_cooldown_s` | `0` | Seconds after a change before it scales up again. Kubernetes' default is `0`; a cloud autoscaling group's cooldown applies here too |
| `scale_down_cooldown_s` | `300` | Seconds after a change before it scales down, Kubernetes' stabilization window |
| `startup_s` | `30` | Seconds a new replica waits before its container starts, for finding a node or VM and pulling the image, which this machine has already done. Its own start, pools and health checks then run for real. Kubernetes with the image on the node takes seconds; a new node or VM, one to several minutes. Printed as an assumption |

Every `interval_s` it wants `ceil(ready × value ÷ target)` replicas, where `ready` is the replicas taking traffic, within `min` and `max`, and ignores a change within 10% of the target. It goes on deciding while new replicas start, asking for more if it wants more, and scales down only once none are starting. A new replica is a real container: it starts, its pools open their connections (all `min` of them, at once), it passes its health check, and only then does the lb send it traffic. A leaving replica is drained first: the lb sends it no new requests, and it stops once its requests finish. The report shows each decision and how long each new replica took to take traffic.

A worker (`role: worker`) can autoscale too, on `cpu` or `queue_depth`, with no lb: a new replica takes jobs as soon as it starts, and a leaving one finishes the jobs it holds before it stops. Scaling on `queue_depth` flaps when the workers keep up: a queue they empty reads near 0, so the autoscaler wants `min`, and the backlog comes back once it gets there. Kubernetes' 300 s scale-down window is what holds the replicas in practice (KEDA scales the same way); a short `scale_down_cooldown_s` shows the flap.

Call fields:

| Field | Default | Values |
| --- | --- | --- |
| `target` | required, or `shards` | A service name |
| `shards` | none | Instead of `target`, for data split across postgres services: the shards, two or more, in order. See [Sharding](#sharding) |
| `shard_by` | `key` | With `shards`: `key` sends each call to the one shard that holds its key; `all` sends it to every shard at once and waits for the slowest (a fan-out read; `read` only) |
| `op` | required | `read`, `write` (postgres, pgbouncer) · `get`, `set`, `del` (redis) · `publish` (queue: adds one job) · `http` (application services and lb) |
| `count` | `1` | Times per request |
| `when` | `always` | `always`, `read`, `write` (from the workload mix) |
| `mode` | `sync` | `sync` waits for the result; `async` fires and continues |
| `after_ms` | none | `mode: async` only: milliseconds the call waits before it runs, while the request responds without it. Models a follow-up the client makes later, such as the page that loads after a form post: `{ target: ro, op: read, when: write, mode: async, after_ms: 50 }`. When the target has read replicas, a point read of a key the request wrote is checked for the write, including in a follow-up |
| `on_miss` | none | Calls to run on a cache miss (`get` only) |
| `coalesce` | `false` | `get` with `on_miss`: concurrent misses for the same key in one replica wait for one run of `on_miss` (request coalescing, single-flight). Each waiter still gives up at its own deadline |
| `ttl_s` | none | For `set` only |
| `query` | `point` | `point` (one row by key), `range` (`rows` rows in key order from the key: `id >= key ORDER BY id LIMIT rows`), `batch` (the key and the `rows - 1` keys after it in one statement: `id = ANY(keys)`), `scan` (filter on `user_id`: a full table scan unless `user_id` is indexed); `read` only |
| `rows` | `10` | Rows per `range` read, or keys per `batch` read |

### Sharding

A call to data split across several postgres services names them as `shards` instead of a `target`:

```yaml
api:
  behavior:
    calls:
      - { shards: [db0, db1, db2, db3], op: read, when: read }    # one shard: the key's
      - { shards: [db0, db1, db2, db3], op: write, when: write }
      - { shards: [db0, db1, db2, db3], shard_by: all, op: read, query: range, rows: 10 }   # every shard
  pools: { db0: 20, db1: 20, db2: 20, db3: 20 }   # a pool per shard, per replica
```

A key goes to shard `(key - 1) mod N` of the N shards, as that shard's own row `(key - 1) div N + 1`: hash sharding on a numeric key, where consecutive keys land on different shards. So a shard seeded with `rows` rows holds its `rows` keys, and the keyspace (`workload.keys.count`, by default) is N × `rows`. Give each shard the same dataset. A hot key stays on one shard however many there are: a `hotspot` or `zipf` key distribution sends its popular keys' traffic to whichever shards hold them.

`shard_by: all` is a fan-out read: one call to every shard at once, done when the slowest answers, failed if any fails, as a query that can't be routed by key (a search, a listing across all users) is. It costs every shard a query, so it grows with the shard count. Each shard has its own pool, and the report shows each shard as its own component.

### Workers (`role: worker`)

A worker takes jobs from a `kind: queue` service and runs its `behavior` once for each, instead of for each HTTP request:

```yaml
api:
  image: loadlore/synthetic
  behavior:
    calls:
      - { target: db, op: write, when: write }
      - { target: emails, op: publish, when: write }   # one job per order
emails:
  kind: queue
sender:
  image: loadlore/synthetic
  role: worker
  replicas: 2
  consume: { from: emails, concurrency: 5 }
  behavior:
    cpu_ms: 20
    calls:
      - { target: db, op: read }
  pools: { db: 5 }
```

| Field | Default | Notes |
| --- | --- | --- |
| `consume.from` | required | The `kind: queue` service it takes jobs from |
| `consume.concurrency` | `5` | Jobs each replica runs at once, as Sidekiq's concurrency or a Celery worker's. Sidekiq's default is 5. Printed as an assumption when not set |
| `consume.retries.max` | none | Retries of a failed job before it is dead. Without `retries`, a failed job is dropped. Sidekiq's default is 25 |
| `consume.retries.base_ms` | `15000` | The wait before the first retry, doubling for each one after it, give or take a quarter: 15 s, 30 s, 60 s... close to Sidekiq's first waits (about 16 s, 31 s and 96 s) |

A job carries the request that published it: its type (read or write) and key, so a worker's calls with `when` and its key-based reads and writes act on the same rows the request would have. Each job also carries `workload.payload_kb` of payload, which is what fills the queue. A worker finishes a job, then acknowledges and deletes it. A job whose calls fail, after per-call `resilience` (timeouts, retries), is a failed run. With `consume.retries` it waits in the queue's retry set (a Redis sorted set, `<queue>:retry`) for its backoff, and each worker replica moves due retries back onto the queue every 250 ms, as Sidekiq's poller does; after its last retry it goes on the dead list (`<queue>:dead`, the newest 10,000 kept) and counts as dead. The verdict says how many failed runs went to retry, how many jobs are dead, and how many still waited when the load ended, and consumer lag counts retries apart from new jobs. Without `retries` a failed job is dropped, as dead; real job systems retry (Sidekiq up to 25 times), so when 1% or more of a window's jobs fail, the verdict says how many were dropped and that the backlog would really have been bigger. A job a runner took and never acknowledged, as a worker killed mid-job leaves it, is picked up again by another replica once it has been idle 30 s (BullMQ's default stalled-job interval; each replica checks every 5 s), and the verdict says how many were. A worker serves no requests: it can't be the workload's `entry`, an lb's `target` or the target of an `http` call. `concurrency.model` still applies (`event-loop` runs one job's CPU work at a time); `concurrency.max` doesn't, `consume.concurrency` sets it.

Resilience fields apply to every outbound call of the service:

| Field | Default | Notes |
| --- | --- | --- |
| `timeout_ms` | `5000` | `0` means no timeout |
| `retries.max` | required with `retries` | Retries after the first attempt. No `retries` block = no retries |
| `retries.backoff` | `none` | `none` (retry immediately), `constant`, `exponential` |
| `retries.base_ms` | `100` | First delay for `constant` and `exponential` |
| `retries.jitter` | `false` | Randomize each delay (full jitter); not with `none` |
| `circuit_breaker` | none | Per target. `error_rate` (0–1), `window_s`, `open_s`, all required. Opens once the window holds at least 20 calls; after `open_s`, one trial call decides |
| `bulkhead` | none | Per target. `max_concurrent`, required; calls beyond it are rejected, not queued |

### Datastores and infrastructure (`kind`)

| `kind` | Key fields | Notes |
| --- | --- | --- |
| `postgres` | `version` (default `"17"`), `config` (any postgresql.conf key; Postgres's own defaults apply, such as `max_connections` 100 and `shared_buffers` 128MB), `dataset`, `disk` | |
| `postgres-replica` | `primary` (required, a postgres service), `resources`, `config` | A streaming read replica of `primary`: it starts from a copy of the primary's data and replays its WAL as it arrives, asynchronously, so a read can miss a write that just committed. It serves reads only: a call with `op: write` to it is an error. It runs the primary's `version` and has no `dataset` of its own. Loadlore sets `primary_conninfo` and keeps `hot_standby` on; the primary keeps `wal_keep_size` 1GB for its replicas unless its `config` sets it. `replicas: N` runs N replicas (`ro-0`, `ro-1`, …), each streaming from the primary with its own copy of the data, and a caller spreads its calls across them in turn, with a pool of the given size to each, as a client configured with every replica's host does: no health checks, so a dead replica keeps getting its share. Its metrics are the replicas' together; `replication_lag_ms` is the worst one's. A `pgbouncer` can't target a replica service with several replicas |
| `redis` | `version` (default `"8"`), `config` (any redis.conf key) | Unless `config` sets them, `maxmemory` is 75% of the container's memory limit and `maxmemory-policy` is `allkeys-lru`, as managed Redis usually does. Both are printed as assumptions in every report, and `--scale` scales `maxmemory`. `maxmemory: 0` restores stock Redis, including being OOM-killed when the cache outgrows its container |
| `queue` | `version` (default `"8"`), `config` (any redis.conf key) | A job queue on Redis Streams: callers add jobs with `op: publish`, and workers with `consume: { from: <it> }` take them in order, each job once, through one consumer group. Unless `config` sets them, `maxmemory` is 75% of the container's memory limit, as for `redis`, and `maxmemory-policy` is `noeviction`: a queue never drops jobs to make room, so once it is full, publishing fails with an error until workers make space. Jobs live in memory only, without persistence, so a restart loses them |
| `pgbouncer` | `target` (required, a postgres service), `pool_mode` (`transaction`; also `session`, `statement`), `default_pool_size` (`20`), `config` (pgbouncer.ini keys) | PgBouncer 1.26. Clients beyond `default_pool_size` server connections wait in PgBouncer. `max_client_conn` defaults to PgBouncer's 100, and the plan notes when the callers' pools can open more. Loadlore sets the listen, auth and admin keys itself, and `pool_mode` and `default_pool_size` are fields, so `config` can't set those |
| `lb` | `target` (required, an application service), `algorithm`, `health_check` | A load balancer (HAProxy) in front of the target's replicas. See below |

`lb` spreads requests across the healthy replicas of its target. Call it like an application service (`op: http`), or make it the workload `entry`:

```yaml
lb:
  kind: lb
  target: api
  algorithm: round-robin        # round-robin (default) | least-conn | random
  health_check: { path: /healthz, interval_ms: 2000, timeout_ms: 1000, fall: 3, rise: 2 }
```

| Field | Default | Notes |
| --- | --- | --- |
| `algorithm` | `round-robin` | `round-robin`, `least-conn` (fewest open requests), `random` |
| `health_check.path` | `/healthz` | HTTP GET on each replica |
| `health_check.interval_ms` | `2000` | Time between checks of one replica (minimum 100) |
| `health_check.timeout_ms` | `1000` | A slower check fails |
| `health_check.fall` | `3` | Failed checks in a row that take a replica out |
| `health_check.rise` | `2` | Passed checks in a row that put it back |

A dead replica keeps getting requests until it fails `fall` checks, about `interval_ms × fall`; that lag is part of what a run shows. Traffic sent to a service with more than one replica without an `lb` still validates, with a `no-lb` warning: the caller (or the load generator) spreads requests across the replicas itself, with no health checks, and the report says so.

`dataset` (postgres only) is seeded before the run into a table with columns `id` (primary key), `user_id` and a payload:

```yaml
dataset: { rows: 5000000, row_bytes: 512, indexes: [id, user_id] }
```

`rows` is required; `row_bytes` defaults to `512`; `indexes` lists extra indexed columns (`id` is always indexed).

`disk` (postgres and postgres-replica) picks the data disk from a named profile, so every design stands on the same published disks instead of waits and caps tuned until a result shows. Without it, the data sits on the Docker host's disk (the `laptop` profile): on a laptop a read that misses Postgres's memory comes back from the host's own memory in about a tenth of a millisecond, with no limit on how many, and a commit's fsync returns in a fraction of a millisecond. So a working set bigger than the database's memory, or a row that takes one commit at a time, shows its cost only on a cloud profile.

```yaml
disk: { profile: gp3 }               # a gp3 volume at its baseline
disk: { profile: gp3, iops: 10000 }  # with provisioned IOPS
disk: { profile: io2, iops: 20000 }
```

| Profile | Waits added | IOPS | Throughput | Source |
|---|---|---|---|---|
| `laptop` | none | no cap | no cap | the Docker host's own disk |
| `gp3` | 2 ms asked per read, and per write and flush: about 1.4 ms measured | `3000` (the baseline), or provisioned from 3000 to 80000 | 125 MiB/s (the baseline) | [AWS: General Purpose SSD volumes](https://docs.aws.amazon.com/ebs/latest/userguide/general-purpose.html): 3,000 IOPS and 125 MiB/s included, up to 80,000 IOPS, "single-digit millisecond latency" |
| `io2` | 1 ms asked per read, and per write and flush: about 0.45 ms measured | required: provisioned, 100 to 256000 | 0.256 MiB/s per IOPS, up to 4,000 | [AWS: Provisioned IOPS SSD volumes](https://docs.aws.amazon.com/ebs/latest/userguide/provisioned-iops.html): io2 Block Express, average latency under 500 microseconds |

How the numbers are read from the sources: the IOPS and throughput are AWS's published figures, but the waits are estimates. The waits are the kernel's dm-delay, which takes whole milliseconds and counts in 1 ms ticks, so a request waits between one tick less than asked and what was asked: about half a millisecond less on average. AWS publishes no exact latency for gp3, only "single-digit millisecond", so gp3 asks for 2 ms, which comes out at 1 to 2 ms, about 1.4 ms on average (measured 19 Nov; 1 ms asked gave about 0.45 ms, below AWS's range): the low end of the range, which flatters the disk rather than handicapping it. io2's stated average is under 500 microseconds, so it asks for 1 ms, about 0.45 ms. A WAL fsync is a write and a flush, each delayed, so a commit waits about twice the per-request wait. Each run measures its disks before the stack starts, with the wait off and on, records asked and measured in `run.json` (`disks`), and the report gives the measured wait next to the profile. The caps are the kernel's cgroup I/O throttle, which gives a capped disk a tail a real volume shouldn't have, under any profile: in a probe at a third of the cap, about 1 read in 1,000 waited about 100 ms, and an fsync waited up to 3 s about once a minute. Until that is replaced, `loadl export` marks runs on a modelled disk as not usable for fitting. IOPS and throughput are capped separately for reads and for writes, so a mixed load can reach up to twice them, and both are scaled with `--scale`; the waits aren't. gp3's provisioned throughput isn't modelled. Every report lists the profile, its numbers and its source under assumptions.

Reads that miss shared_buffers and the database container's own page cache wait the profile's read wait and queue at its IOPS; every write and fsync waits its write wait, from the moment the dataset is copied in. Loadlore empties the Docker host's page cache after copying the dataset in, so the container's memory limit holds, and turns on `track_io_timing` (unless `config` sets it) so the report can tell reads that waited for the disk from ones memory answered. A `disk_slow` fault on the same database adds its write delay on top of the profile's.

The older form without a profile, `disk: {}` or `disk: { read_ms: 1, iops: 3000 }`, still works: reads wait `read_ms` (default 2, as gp3 asks, 0 to 100) and reads and writes are each capped at `iops` (default 3000, at least 100), with no write wait. Scenarios don't use it except where `meta.yaml` marks them as moving to a profile at their next re-measure.

There is deliberately no field for hit rate, capacity or failure point. Those emerge from the run, and `loadl validate` rejects such fields with an explanation.

## Workload

```yaml
workload:
  entry: lb                     # application service or lb receiving external traffic
  profile:                      # steps run in order
    - ramp:  { from: 100, to: 5000, over: 5m }
    - hold:  { rps: 5000, for: 2m }
    - spike: { to: 20000, for: 30s }       # then back to the rate before the spike
  mix: { read: 0.9, write: 0.1 }          # default 0.9 / 0.1; give one and the other is the rest
  keys: { distribution: zipf, skew: 1.1, count: 5000000 }
  payload_kb: 2                           # default 1
  timeout_ms: 10000                       # default 10000: the load generator's request timeout
  slo: { p99_ms: 500 }                    # default 500: the p99 users accept
```

Each profile step has exactly one of `ramp`, `hold` or `spike`. Load is open-model: the rate is what arrives, whether or not the system keeps up.

`slo.p99_ms` is the end-to-end p99 users accept. The verdict calls a break on latency alone only when p99 stays at or above both this and 10x the low-load p99 for 30 seconds; errors and a shortfall in served requests break regardless. Set it to what the system's users would tolerate, and keep it when comparing runs.

`keys`:

| Field | Default | Notes |
| --- | --- | --- |
| `distribution` | `uniform` | `uniform`, `zipf`, `hotspot` |
| `skew` | `1.1` | `zipf` only: the exponent |
| `share`, `fraction` | `0.8`, `0.01` | `hotspot` only: `share` of requests hit `fraction` of keys |
| `count` | see below | Distinct keys the workload touches |

`hotspot` example: `{ distribution: hotspot, share: 0.8, fraction: 0.01 }` means 80% of requests hit 1% of keys.

Client timeout: `timeout_ms` is how long the load generator waits for a response before counting the request as failed. It decides how long requests to a dead or overloaded service hang, so it shapes every failure. Every report prints it as an assumption, next to any lb's own timeouts.

Keyspace: `count` defaults to the `dataset.rows` of the datastore the entry path reads. Validation requires it when that is ambiguous (several datasets of different sizes, or none), and it can't exceed that row count.

Durations: `30s`, `5m`, `1h`, `1m30s`, `500ms`. A unit is required.

## Network

`network` picks the network between the stack's containers from a named profile, so every design stands on the same published network instead of the Docker bridge, where two containers reach each other in tens of microseconds, faster than any two machines in a data center.

```yaml
network: { profile: same-zone }    # the default
network: { profile: cross-zone }   # every call crosses availability zones
network: { profile: local }        # nothing added
```

| Profile | Round trip added | Source |
|---|---|---|
| `local` | none | the Docker bridge on one machine |
| `same-zone` (default) | 0.3 ms | [AWS Architecture Blog: Availability Zone Affinity](https://aws.amazon.com/blogs/architecture/improving-performance-and-reducing-cost-using-availability-zone-affinity/): a round trip in one zone is "generally sub-millisecond" with enhanced networking |
| `cross-zone` | 1.5 ms | [AWS Fault Isolation Boundaries: Availability Zones](https://docs.aws.amazon.com/whitepapers/latest/aws-fault-isolation-boundaries/availability-zones.html): zones in a region are close enough for synchronous replication "with single-digit millisecond latency" |

How the numbers are read from the sources: AWS publishes bounds, not figures, so both round trips are Loadlore's own estimates: same-zone's 0.3 ms sits inside "sub-millisecond", and cross-zone's 1.5 ms is at the low end of "single-digit milliseconds". Reports say so next to the profile. The round trip is added between any two of the stack's containers (services, datastores, PgBouncers, lbs) and between the load generator and the entry point, as half of it on each side's outgoing packets (tc netem); it is fixed, with no jitter, and isn't scaled with `--scale`. Loadlore's own tooling (Prometheus, exporters, Toxiproxy) isn't slowed, so a connection a fault sends through Toxiproxy has the same round trip as a direct one. A container that restarts, or an autoscaler's new replica, gets the delay about a second after it starts. One profile covers the whole design: a replica in another zone, or one cross-zone call, isn't expressible yet.

## Scenario

A list of fault injections, each with `at` and `inject`, and a `target` (except `partition`). Optional `for` ends the fault after that duration; otherwise it lasts to the end of the run. A fault must start before the workload ends.

| `inject` | Fields | Mechanism |
| --- | --- | --- |
| `latency` | `add_ms` (required), `jitter_ms` | Toxiproxy |
| `loss` | `percent` (required) | tc netem in the target's network namespace: drops that share of the packets it sends from its service port, so the replies on connections into it. TCP retransmits them, so loss shows as slow calls (about 200 ms per lost segment), not errors |
| `bandwidth` | `kbps` (required) | Toxiproxy |
| `timeout` | | Toxiproxy (connections hang) |
| `reset` | | Toxiproxy (connections reset) |
| `kill` | | Container killed, not restarted |
| `pause` | `for` required | Container frozen |
| `restart` | | Container restarted (memory and caches cleared) |
| `partition` | `between: [a, b]` instead of `target`; each side a service or `name[i]` for one replica | tc netem in each side's network namespace: every packet it sends toward the other side is dropped, handshakes included, so calls hang until their timeouts. Health checks that don't touch the other side keep passing |
| `disk_slow` | `add_ms` (required) | The kernel's dm-delay under the datastore's data: every write and fsync waits `add_ms` longer; reads don't. Postgres only in local runs |

`target` is a service name, or `name[i]` for one replica. Connection faults (`latency`, `loss`, `bandwidth`, `timeout`, `reset`) act on connections into the target, so something must call it (or it is the workload entry).

Only connections that a `latency`, `bandwidth`, `timeout` or `reset` fault acts on go through Toxiproxy, from the start of the run: every other connection goes straight to its container, and a run with none of those faults has no Toxiproxy at all. Toxiproxy relaying a call costs CPU (about 67 µs per call, more than a Postgres statement), which would otherwise be a tax on every design. `loss` and `partition` act on the containers themselves, with tc netem, and keep the network profile's delay on everything they don't drop.

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
<run metric> within <N>% of <value>
<run metric> none
```

- `op` is `<`, `<=`, `>` or `>=`.
- A line with no qualifier means `throughout`.
- `until <condition>` checks the prediction from the start until the condition first holds. Conditions usually use `load_rps`, the offered load (what k6 sends), so the line can still be scored when the system saturates below it. `rps` is achieved throughput.
- `after <time>` checks it from that time (from the start of the workload) to the end.
- Percentiles and rates are computed over 10-second windows. A `throughout` or `until` line breaks if any window in its range breaks.

End-to-end metrics: `p50_ms`, `p95_ms`, `p99_ms`, `error_rate`, `rps`, and `load_rps` (conditions only).

Whole-run metrics, for "where will it break?" predictions: `break_load_rps`, the offered load where the verdict's break starts, and `errors_load_rps`, the offered load where 1% or more of requests first fail, two windows in a row. Predict them as `within <N>% of <value>`, which holds when the measured load is within N% of the guess either side, so a guess too high and one too low both break; as `none`, which holds when it never happened; or as `any`, which holds when it happened at any load. They take no qualifier. Prefer them to `until load_rps >= X` lines for a load guess: an `until` line only breaks when the guess is too high.

Per-component metrics, written `<service>.<metric>`:

| Component | Metrics |
| --- | --- |
| postgres | `connections`, `cpu`, `memory` |
| postgres-replica | `connections`, `cpu`, `memory`, `replication_lag_ms`: how far behind its primary it is: the WAL it hadn't replayed, in milliseconds of the primary's writing; `stale_read_rate`: of the point reads of a key the same request had just written to the primary, the share that didn't see the write |
| redis | `hit_rate`, `memory`, `cpu` |
| queue | `depth`: jobs published and not yet finished, waiting or running; `oldest_job_ms`: how long the oldest of them had waited; `job_wait_p99_ms`: the p99 wait, publish to start, of the jobs workers took; `retrying`: jobs in the retry set, waiting for their retry; `dead_jobs`: jobs given up on so far (out of retries, or dropped without them); `memory`, `cpu` |
| application services | `cpu`, `memory`, `p99_ms`, `error_rate`, `inflight` |

Values: latencies in milliseconds (`300`, `300ms` or `0.3s`); `error_rate`, `hit_rate`, `cpu` and `memory` as a percentage (`5%`) or fraction (`0.05`), where `cpu` and `memory` are utilization of the container's limit; rates and counts as plain numbers.

```yaml
expect:
  - break_load_rps within 25% of 3000
  - errors_load_rps none
  - break_load_rps any        # it breaks somewhere in the profile
  - p99_ms < 300 until load_rps >= 2000
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

`scale` (default `1`) shrinks resources and load together in local mode. The `--scale` flag overrides it. It multiplies CPU, memory, memory settings, dataset rows, the keyspace and request rates; replicas, pool sizes, connection limits and per-request work stay as written. Because connection counts don't shrink, memory doesn't shrink below what they need: a Postgres keeps its scaled `shared_buffers` plus about 2 MiB per allowed connection and 64 MiB, and an `lb` keeps about 40 KiB per possible client connection plus 32 MiB, never more than the design's own value. The run's notes say when this applies.

## Planned

Documented for later phases. `loadl validate` rejects these with a "planned" error, and the schema sync check skips this section.

- **Other queues** (phase 2): `kind: kafka` or `kind: rabbitmq` with `partitions` and `retention`. `kind: queue` (Redis Streams) is built.
- **Cloud mode** (phase 2):

```yaml
modes:
  cloud: { provider: aws, region: eu-west-3, budget_usd: 10, instance: { db: db.t4g.medium } }
```

`budget_usd` in the file never replaces the `--budget` flag; the CLI requires both to agree.
