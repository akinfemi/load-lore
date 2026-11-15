# stale-orders-bigger-replica: run report

_run `stale-orders-bigger-replica-20261115-123524` · completed · 15 Nov 2026 12:35 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 1,800 rps offered, 1,754 rps served at best, worst 10-second p99 6.4 ms, at most 0% errors.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 2:50

Measured in the 10-second window from 2:50 to 3:00 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 1,800 rps<br/>offered 1,756 rps"])
    api["api<br/>2 CPU · 1g<br/>CPU 37%<br/>in flight 3"]
    ro[("ro<br/>postgres-replica 17<br/>1 CPU · 2g<br/>disk 1 ms · 10,000 IOPS<br/>CPU 18%<br/>conns 41/100<br/>lag 24 ms")]
    db[("db<br/>postgres 17<br/>1 CPU · 2g<br/>CPU 17%<br/>conns 41/100")]
    load -->|"50% read · 50% write<br/>1,754 rps · p99 3.8 ms"| api
    api -->|"read on reads<br/>pool 80, min 40<br/>1,746 calls/s · p99 1.9 ms<br/>in use 2/80"| ro
    api -->|"write on writes<br/>pool 80, min 40<br/>867 calls/s · p99 4.1 ms<br/>in use 2/80"| db
    api -.->|"read on writes<br/>pool 80, min 40<br/>1,746 calls/s · p99 1.9 ms<br/>in use 2/80"| ro
    db -.->|"streams WAL"| ro
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class api,ro,db ok
```

<sub>Sources: offered from the load profile in plan.json (offered arrival rate); load edge from k6_http_reqs_total (k6: all responses) and k6_http_req_duration_seconds (k6 histogram, end to end); CPU from container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit; calls/s from loadlore_calls_total (the caller's outbound call attempts) and their p99 from loadlore_call_duration_seconds (the caller's histogram, pool wait included); pools from loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections and loadlore_pool_pending (callers waiting for a connection); conns from pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); a replica's lag from pg_stat_replication_pg_wal_lsn_diff ÷ the rate of pg_stat_replication_pg_current_wal_lsn_bytes (postgres_exporter, on the primary): the WAL the replica hadn't replayed, in seconds of the primary's writing; without them, pg_replication_lag_seconds on the replica.</sub>

## Charts

One point per 10-second window, at the window's start; x is seconds from the start of the load.

```mermaid
---
config:
  themeVariables:
    xyChart:
      backgroundColor: "transparent"
      plotColorPalette: "#9e9e9e, #1f77b4"
---
xychart-beta
    title "Throughput"
    x-axis "seconds" 0 --> 170
    y-axis "requests/s" 0 --> 2000
    line [244.444, 333.333, 422.222, 511.111, 600, 688.889, 777.778, 866.667, 955.556, 1044.444, 1133.333, 1222.222, 1311.111, 1400, 1488.889, 1577.778, 1666.667, 1755.556]
    line [239.34, 331.759, 420.753, 509.489, 598.35, 687.346, 776.213, 865.063, 953.997, 1042.929, 1131.701, 1220.552, 1309.565, 1398.371, 1487.495, 1576.161, 1664.936, 1754.088]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)).</sub>

```mermaid
---
config:
  themeVariables:
    xyChart:
      backgroundColor: "transparent"
      plotColorPalette: "#2ca02c, #ff7f0e, #d62728"
---
xychart-beta
    title "Latency, end to end"
    x-axis "seconds" 0 --> 170
    y-axis "ms" 0 --> 8
    line [1.213, 0.989, 0.935, 0.869, 0.85, 0.838, 0.833, 0.835, 0.818, 0.813, 0.794, 0.775, 0.761, 0.744, 0.736, 0.712, 0.75, 0.729]
    line [2.726, 2.539, 2.3, 1.873, 1.615, 1.609, 1.541, 1.529, 1.539, 1.544, 1.502, 1.501, 1.537, 1.578, 1.599, 1.629, 2.636, 1.989]
    line [3.141, 2.778, 2.704, 2.539, 2.057, 2.547, 2.062, 1.879, 1.909, 1.96, 1.779, 1.848, 1.959, 2.013, 2.218, 2.507, 6.404, 3.771]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)).</sub>

```mermaid
---
config:
  themeVariables:
    xyChart:
      backgroundColor: "transparent"
      plotColorPalette: "#d62728"
---
xychart-beta
    title "Errors"
    x-axis "seconds" 0 --> 170
    y-axis "% of requests" 0 --> 1
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included.</sub>

```mermaid
---
config:
  themeVariables:
    xyChart:
      backgroundColor: "transparent"
      plotColorPalette: "#1f77b4, #ff7f0e, #2ca02c"
---
xychart-beta
    title "CPU against each limit"
    x-axis "seconds" 0 --> 170
    y-axis "% of limit" 0 --> 100
    line [6.633, 7.789, 9.616, 11.515, 13.342, 15.252, 16.937, 18.769, 20.516, 22.248, 24.08, 25.826, 27.751, 29.597, 31.443, 33.334, 35.551, 37.43]
    line [7.269, 5.178, 5.303, 5.835, 6.333, 7.151, 7.423, 8.191, 8.445, 8.943, 9.466, 9.831, 10.494, 11.214, 11.866, 12.699, 17.175, 16.996]
    line [7.631, 5.87, 6.456, 7.114, 7.752, 8.894, 9.365, 9.96, 10.498, 11.479, 12.112, 12.584, 13.614, 14.559, 16.369, 17.364, 17.438, 18.084]
```

<sub>Blue: api. Orange: db. Green: ro (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window).</sub>

## Diagnosis

No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version 11; thresholds below).

## Predictions

1 of 1 scored prediction held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 500 until load_rps >= 1000` | **held** | 3.1 ms | 0:00 to 1:30, when load_rps >= 1,000 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |

Loads and request rates depend on this machine: slower cores break the same design at a lower load, so a load guess can miss even when the reasoning behind it was right.

## Assumptions

### From the design

| Field | Value | Basis | Why |
| --- | --- | --- | --- |
| `services.api.behavior.cpu_ms` |  | guessed | a small JSON handler |
| `workload.mix` |  | guessed | during the sale, half the requests place an order |
| `workload.keys` |  | guessed | orders and pages spread over all 5 million customers |
| `services.api.behavior.calls` |  | guessed | the confirmation page loads about 200 ms after the order is placed |

4 of these are guessed, not measured: the result depends on them.

### Chosen by Loadlore

| Field | Value | Why |
| --- | --- | --- |
| workload.timeout_ms | 10s | the default; k6 gives up on a request after this long and counts it as failed |
| workload.slo.p99_ms | 500 ms | the default; a break on p99 alone needs p99 at or above this as well as 10x the low-load p99, so latency users would still accept isn't called a break |
| resources.cpu_period_ms | 100 ms for api, db, ro | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.db.disk | the Docker host's | no disk block: a read that misses db's memory goes to the Docker host's disk, which on a laptop mostly answers from the host's own memory, fast and uncapped. Add disk: {} for a cloud volume's wait and IOPS cap |
| services.ro.disk | 1 ms per read, 10000 IOPS | set in the design: every read that misses ro's memory waits this long, and reads and writes are each capped at this rate, as on a cloud volume (unset values default to about a gp3 volume's: 1 ms, 3000 IOPS) |
| services.ro replication | asynchronous streaming from db | the replica starts from a copy of db's seeded data and replays its WAL as it arrives, directly, not through Toxiproxy: a commit on db doesn't wait for it, so a read on ro can miss a write that just committed |
| services.db.config.wal_keep_size | 1GB | Loadlore's default: enough for a replica that falls behind under write load to catch up |
| services.api.pools.db.min | 40 | set in the design: each replica opens 40 connections before the load starts and keeps them open |
| services.api.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- Loadlore's own containers get fixed budgets: load generator 1.5 CPU (peak 1800 rps), Toxiproxy 1 CPU (about 2700 proxied calls/s at peak), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each

## Thresholds

- **Broke:** 2 windows in a row with at least 1.0% of requests failing or successful responses below 90% of the offered rate, or 3 windows in a row with any of those or p99 at least 10x the low-load p99 (the median p99 of windows offered at most 25% of the peak, before errors or a shortfall in 2 windows in a row) and at least the SLO, 500 ms (workload.slo.p99_ms): 500 ms in this run. A shorter episode is shown as a stall in the timeline, not a break.
- **Doubtful:** a break on p99 alone, when the load generator was stalled at its CPU limit in 1.0% or more of its scheduling periods in one of the break's windows and the entry service's own p99 (loadlore_request_duration_seconds (the service's own histogram)) was below the limit in one of them. The verdict stays what it was.
- **Inconclusive:** before the break (or anywhere, if nothing broke), the load generator dropped more than 1.0% of a window's arrivals (more than 5.0% in a window where the system itself failed 1.0% or more of requests; up to that it's a caveat that gives the share), or the load generator or Toxiproxy used 90% or more of its CPU budget, or was stalled at its CPU limit in 5.0% or more of its scheduling periods (from 1.0% it's noted as a caveat). The first 10 s of load are a warm-up and don't count: the load generator starts its VUs and connections open then. A cause is named only with evidence: on a Linux host, the hypervisor giving 2.0% or more of CPU time to other virtual machines (steal), or other programs using 10% or more of the host's CPUs (at least 0.5); inside a VM on this machine, only as a possibility.
- **Toxiproxy stalls:** no new connection accepted on one of Toxiproxy's edges for 1 s or more, where it had accepted 10 a second or more over the 5 s before, then 10 or more accepted within 100 ms with a median spacing under 0.5 ms: connections queued behind one upstream dial, which a real network doesn't do. Up to the end of the break's first window it makes the run inconclusive; after a break the system didn't recover from, it's a doubt on the recovery, and `loadl scenario check` doesn't judge claims that rest on not recovering; in a run that held, or recovered, it's a caveat: a stall only adds failure.
- **Connection pool exhausted:** for two windows in a row, a pool at 95% or more of its size with callers waiting, while the target wasn't busy (busy: its CPU at 80% or more, or its time per call with a connection at least 3x the low-load value and 1 ms above it; CPU throttling alone doesn't count) and the caller's CPU stayed below 90%.
- **Queueing knee:** for 3 windows in a row, p99 at least 3x the low-load p99 and 10 ms above it, with a CPU or pool at 70% or more. When that is a pool whose target was busy in any of those windows, or a service's CPU whose requests spent 50% or more of their time in calls to a busy target, the knee is the target's.
- **Connection storm:** up to the break, a service's pools to a Postgres opening at least 50% of their combined size, and 10 or more connections, in one window, while p99 in that window or the next was at least 10x the low-load p99 and 10 ms above it. New connections are counted by loadlore_pool_connect_seconds (new connections the pools opened, and how long each took), or for runs from before it, from the rise in open connections.
- **Retry storm:** for two windows in a row, a caller that retries sending 1.5x or more of its low-load calls per request to a target, with retries 20% or more of them, while the target failed 1.0% or more of its calls or its p99 was 3x its low-load value.
- **Cold cache:** a cache's share of lookups that miss rising to 5x its level before or more, and to 5.0% of lookups at least, while its callers' reads of the database behind it reach 2x their rate before (counted as at least 1 a second, for a cache warm enough to send none).
- **Working set doesn't fit:** for two windows in a row, a Postgres answering less than 90% of block reads from shared_buffers and reading 100 blocks a second or more from outside them, while p99 was 3x the low-load p99 or more. When Postgres times its reads (track_io_timing, on with a disk block), each block read must also take 3x its low-load time or more, and 1 ms more.
- **Replication lag:** for two windows in a row, a read replica 1 s or more behind its primary, or 5.0% or more of the reads after a write it served missing the write.
- **Hot rows:** for two windows in a row, 2 or more of a Postgres's backends waiting on locks on average, while a caller's writes to it took 3x their low-load time or more (and 2 ms more). Reads of the same rows may slow too, and the summary says when the database's CPU per request reached 3x its low-load value: the contention burning CPU. A queueing knee on that database's CPU at the same time then points at the rows. Windows with a disk_slow fault on the database don't count.

## Environment

- loadl dev, profile 3m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 975 (hash 920, memory 1015, syscalls 994): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (975) was within 5.0% of the run it's compared with (996)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 179,993 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-stale-orders-bigger-replica-20261115-123524-api-0-1 | loadlore/synthetic:dev |
| loadlore-stale-orders-bigger-replica-20261115-123524-db-1 | postgres:17.11 |
| loadlore-stale-orders-bigger-replica-20261115-123524-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-stale-orders-bigger-replica-20261115-123524-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-stale-orders-bigger-replica-20261115-123524-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-stale-orders-bigger-replica-20261115-123524-lore-pgexp-ro-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-stale-orders-bigger-replica-20261115-123524-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-stale-orders-bigger-replica-20261115-123524-lore-toxiproxy-1 | ghcr.io/shopify/toxiproxy:2.13.0 |
| loadlore-stale-orders-bigger-replica-20261115-123524-ro-1 | postgres:17.11 |

</details>

## Per-window data

<details><summary>18 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | ro CPU | ro conns | ro lag | ro stale | api→db pool | wait | api→ro pool | wait | k6 CPU | Toxiproxy CPU | k6 / Toxiproxy stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 244 | 239 | 0 | 0% | 1.2 ms | 3.1 ms | 6.6% | 7.3% | 41 | 7.6% | 41 | 0.0 ms | 0% | 0/80 | 0.0 ms | 1/80 | 0.0 ms | 5.5% | 3.2% | 1.0% / 0% |
| 0:10 | 333 | 332 | 0 | 0% | 1.0 ms | 2.8 ms | 7.8% | 5.2% | 41 | 5.9% | 41 | 7.2 ms | 0% | 0/80 | 0.0 ms | 2/80 | 0.0 ms | 3.7% | 3.5% | 0% / 0% |
| 0:20 | 422 | 421 | 0 | 0% | 0.9 ms | 2.7 ms | 9.6% | 5.3% | 41 | 6.5% | 41 | 3.5 ms | 0% | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 4.8% | 4.3% | 0% / 0% |
| 0:30 | 511 | 509 | 0 | 0% | 0.9 ms | 2.5 ms | 12% | 5.8% | 41 | 7.1% | 41 | 5.8 ms | 0% | 1/80 | 0.0 ms | 2/80 | 0.0 ms | 5.5% | 5.3% | 0% / 0% |
| 0:40 | 600 | 598 | 0 | 0% | 0.8 ms | 2.1 ms | 13% | 6.3% | 41 | 7.8% | 41 | 5.6 ms | 0% | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 5.8% | 6.1% | 0% / 0% |
| 0:50 | 689 | 687 | 0 | 0% | 0.8 ms | 2.5 ms | 15% | 7.2% | 41 | 8.9% | 41 | 2.9 ms | <0.1% | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 6.7% | 7.1% | 0% / 0% |
| 1:00 | 778 | 776 | 0 | 0% | 0.8 ms | 2.1 ms | 17% | 7.4% | 41 | 9.4% | 41 | 2.7 ms | <0.1% | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 7.3% | 7.8% | 0% / 0% |
| 1:10 | 867 | 865 | 0 | 0% | 0.8 ms | 1.9 ms | 19% | 8.2% | 41 | 10.0% | 41 | 13 ms | 0% | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 7.6% | 8.6% | 0% / 0% |
| 1:20 | 956 | 954 | 0 | 0% | 0.8 ms | 1.9 ms | 21% | 8.4% | 41 | 10% | 41 | 2.4 ms | 0% | 1/80 | 0.0 ms | 2/80 | 0.0 ms | 8.5% | 9.3% | 0% / 0% |
| 1:30 | 1,044 | 1,043 | 0 | 0% | 0.8 ms | 2.0 ms | 22% | 8.9% | 41 | 11% | 41 | 7.1 ms | 0% | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 9.0% | 10% | 0% / 0% |
| 1:40 | 1,133 | 1,132 | 0 | 0% | 0.8 ms | 1.8 ms | 24% | 9.5% | 41 | 12% | 41 | 4.6 ms | 0% | 1/80 | 0.0 ms | 3/80 | 0.0 ms | 9.7% | 11% | 0% / 0% |
| 1:50 | 1,222 | 1,221 | 0 | 0% | 0.8 ms | 1.8 ms | 26% | 9.8% | 41 | 13% | 41 | 2.4 ms | 0% | 1/80 | 0.0 ms | 2/80 | 0.0 ms | 10% | 12% | 0% / 0% |
| 2:00 | 1,311 | 1,310 | 0 | 0% | 0.8 ms | 2.0 ms | 28% | 10% | 41 | 14% | 41 | 2.3 ms | <0.1% | 1/80 | 0.0 ms | 3/80 | 0.0 ms | 11% | 13% | 0% / 0% |
| 2:10 | 1,400 | 1,398 | 0 | 0% | 0.7 ms | 2.0 ms | 30% | 11% | 41 | 15% | 41 | 9.0 ms | <0.1% | 1/80 | 0.0 ms | 2/80 | 0.0 ms | 12% | 14% | 0% / 0% |
| 2:20 | 1,489 | 1,487 | 0 | 0% | 0.7 ms | 2.2 ms | 31% | 12% | 41 | 16% | 41 | 8.1 ms | <0.1% | 1/80 | 0.0 ms | 2/80 | 0.0 ms | 12% | 15% | 0% / 0% |
| 2:30 | 1,578 | 1,576 | 0 | 0% | 0.7 ms | 2.5 ms | 33% | 13% | 41 | 17% | 41 | 4.0 ms | 0% | 5/80 | 0.0 ms | 3/80 | 0.0 ms | 13% | 16% | 0% / 0% |
| 2:40 | 1,667 | 1,665 | 0 | 0% | 0.7 ms | 6.4 ms | 36% | 17% | 41 | 17% | 41 | 23 ms | <0.1% | 2/80 | 0.0 ms | 2/80 | 0.0 ms | 14% | 17% | 0% / 0% |
| 2:50 | 1,756 | 1,754 | 0 | 0% | 0.7 ms | 3.8 ms | 37% | 17% | 41 | 18% | 41 | 24 ms | <0.1% | 2/80 | 0.0 ms | 2/80 | 0.0 ms | 15% | 18% | 0% / 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); a replica's lag is from pg_stat_replication_pg_wal_lsn_diff ÷ the rate of pg_stat_replication_pg_current_wal_lsn_bytes (postgres_exporter, on the primary): the WAL the replica hadn't replayed, in seconds of the primary's writing; without them, pg_replication_lag_seconds on the replica, and its stale share of reads after writes from loadlore_read_after_write_total (the synthetic service): point reads of a key the same request had just written, stale when they didn't see the write; hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 and Toxiproxy CPU are against their budgets.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: stale-orders-bigger-replica

# An online shop's orders api. To take load off the database, the team
# added a read replica and moved the reads to it: browsing, and the order
# confirmation page that loads after a customer places an order. The
# replica was bought as a smaller instance than the primary, on the same
# kind of cloud disk.

services:
  api:
    image: loadlore/synthetic
    resources: { cpu: 2, memory: 1g }
    behavior:
      cpu_ms: 0.3
      calls:
        - { target: ro, op: read, when: read }                                # browsing
        - { target: db, op: write, when: write }                              # place an order
        - { target: ro, op: read, when: write, mode: async, after_ms: 200 }   # the confirmation page loads
      resilience:
        timeout_ms: 2000
    pools:
      db: { size: 80, min: 40 }   # under the primary's max_connections
      ro: { size: 80, min: 40 }

  db:
    kind: postgres
    version: "17"
    resources: { cpu: 1, memory: 2g }
    config: { max_connections: 100, shared_buffers: 512MB }
    dataset: { rows: 2000000, row_bytes: 512 }   # about 1.1 GB with its index
    # No disk block: its data fits in its memory, so its disk isn't the point.

  ro:
    kind: postgres-replica
    primary: db
    resources: { cpu: 1, memory: 2g }   # was 0.75 CPU and 512m: the primary's size
    config: { max_connections: 100, shared_buffers: 512MB }   # was 128MB
    disk: { iops: 10000 }   # was {}: provisioned IOPS, as a primary's volume has

workload:
  entry: api
  profile:
    - ramp: { from: 200, to: 1800, over: 3m }
  mix: { read: 0.5, write: 0.5 }

assumptions:
  - { field: services.api.behavior.cpu_ms, basis: guessed, why: "a small JSON handler" }
  - { field: workload.mix, basis: guessed, why: "during the sale, half the requests place an order" }
  - { field: workload.keys, basis: guessed, why: "orders and pages spread over all 5 million customers" }
  - { field: services.api.behavior.calls, basis: guessed, why: "the confirmation page loads about 200 ms after the order is placed" }

# Your predictions, scored in the report. The line here is an example;
# loadl predict design.yaml replaces it with your answers.
expect:
  - p99_ms < 500 until load_rps >= 1000
```
