# loadlore/synthetic

A configurable stand-in for an application service. It burns real CPU, allocates real memory, and calls Postgres, Redis and other services through real clients (pgx, go-redis, net/http) with real pools, timeouts and retries. Nothing sleeps in place of I/O, so saturation, queueing and pool exhaustion happen for real.

`loadl run` builds its config from the design file; you don't write it by hand. Build the image with `make image`.

## Config

JSON in `LOADLORE_SYNTHETIC_CONFIG`, or a file named by `LOADLORE_SYNTHETIC_CONFIG_FILE`. The shape is `synthetic.Config` in `internal/synthetic/config.go`: the design's `behavior`, `concurrency` and `resilience` with every default applied, plus a `targets` map giving each dependency's kind, address (usually a Toxiproxy listener) and pool size.

## Endpoints

| Path | What it does |
| --- | --- |
| `GET/POST /r?type=read\|write&key=N` | One request. The body, if any, is read and dropped. Errors return 500, 503 (breaker or bulkhead), 504 (timeout) or 499 (caller left), with the cause in `X-Loadlore-Cause`. |
| `GET /metrics` | Prometheus metrics |
| `GET /healthz` | The process is up |
| `GET /readyz` | Every dependency answers a ping |

## How behavior maps to real mechanisms

- **CPU (`cpu_ms`)** spins until the thread has used that much CPU time, measured with the thread CPU clock. Under a cgroup limit it takes longer in wall time, as a real handler would.
- **Concurrency**: `threads` and `workers` admit `max` requests at a time and queue the rest; `event-loop` runs CPU work one request at a time and I/O concurrently.
- **Pools**: pgxpool `MaxConns`, go-redis `PoolSize`, and `MaxConnsPerHost` for HTTP, all per replica.
- **Timeouts** apply per call attempt and include the wait for a pool connection. go-redis's built-in retries are off, so every retry is one the design asked for and shows in the metrics.
- **Queries** run against the seeded `items` table (`internal/dataset`).

## Metrics

Every series carries `service` and `replica` labels.

| Metric | Type | Labels |
| --- | --- | --- |
| `loadlore_requests_total` | counter | `type`, `code` |
| `loadlore_request_duration_seconds` | histogram | `type` |
| `loadlore_request_errors_total` | counter | `cause` |
| `loadlore_requests_inflight` | gauge | |
| `loadlore_requests_queued` | gauge | |
| `loadlore_queue_wait_seconds` | histogram | |
| `loadlore_cpu_burned_seconds_total` | counter | |
| `loadlore_calls_total` | counter | `target`, `op`, `outcome` |
| `loadlore_call_duration_seconds` | histogram | `target`, `op` |
| `loadlore_call_errors_total` | counter | `target`, `op`, `cause` |
| `loadlore_call_retries_total` | counter | `target`, `op` |
| `loadlore_cache_lookups_total` | counter | `target`, `result` (`hit`, `miss`) |
| `loadlore_pool_connections` | gauge | `target`, `state` (`in_use`, `idle`) |
| `loadlore_pool_max_connections` | gauge | `target` |
| `loadlore_pool_pending` | gauge | `target` |
| `loadlore_pool_wait_seconds` | histogram | `target` (postgres and http) |
| `loadlore_pool_waits_total`, `loadlore_pool_wait_seconds_total` | counter | `target` (redis, from client totals) |
| `loadlore_circuit_open` | gauge | `target` (with a breaker) |
| `loadlore_async_calls_failed_total` | counter | |

Error causes: `pool_timeout`, `timeout`, `canceled`, `conn_refused`, `conn_reset`, `too_many_conns`, `db_error`, `cache_error`, `upstream_error`, `circuit_open`, `bulkhead_full`, `other`.
