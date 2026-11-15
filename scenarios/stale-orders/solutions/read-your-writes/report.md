# stale-orders-read-your-writes: run report

_run `stale-orders-read-your-writes-20261115-123901` · completed · 15 Nov 2026 12:39 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 1,800 rps offered, 1,754 rps served at best, worst 10-second p99 73 ms, at most 0% errors. But ro fell behind db, so reads from it returned old data (see replication lag).

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 2:50

Measured in the 10-second window from 2:50 to 3:00 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 1,800 rps<br/>offered 1,756 rps"])
    api["api<br/>2 CPU · 1g<br/>CPU 37%<br/>in flight 6"]
    ro[("ro<br/>postgres-replica 17<br/>0.75 CPU · 512m<br/>disk 1 ms · 3,000 IOPS<br/>CPU 18%<br/>conns 81/100<br/>lag 4.64 s")]
    db[("db<br/>postgres 17<br/>1 CPU · 2g<br/>CPU 19%<br/>conns 47/100")]
    load -->|"50% read · 50% write<br/>1,754 rps · p99 73 ms"| api
    api -->|"read on reads<br/>pool 80, min 40<br/>871 calls/s · p99 87 ms<br/>in use 3/80"| ro
    api -->|"write on writes<br/>pool 80, min 40<br/>1,742 calls/s · p99 4.7 ms<br/>in use 3/80"| db
    api -.->|"read on writes<br/>pool 80, min 40<br/>1,742 calls/s · p99 4.7 ms<br/>in use 3/80"| db
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
    line [238.723, 331.603, 420.422, 509.341, 598.199, 687.044, 776.019, 864.932, 953.745, 1042.733, 1130.659, 1221.303, 1309.119, 1397.996, 1487.69, 1575.84, 1664.805, 1753.785]
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
    y-axis "ms" 0 --> 80
    line [1.251, 0.958, 0.926, 0.872, 0.886, 0.86, 0.873, 0.89, 0.873, 0.874, 0.88, 0.886, 0.907, 0.931, 0.933, 0.929, 0.996, 1.01]
    line [2.786, 2.513, 2.347, 1.873, 2.064, 1.737, 1.836, 1.844, 1.812, 1.929, 1.928, 1.931, 2.542, 3.039, 4.142, 4.691, 8.516, 16.806]
    line [3.859, 2.75, 2.721, 2.572, 4.614, 2.53, 3.034, 2.996, 3.038, 3.386, 3.341, 3.493, 27.098, 41.337, 59.08, 61.878, 62.562, 72.863]
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
    line [5.985, 7.776, 9.703, 11.542, 13.654, 15.319, 17.241, 19.153, 20.854, 22.419, 24.383, 26.289, 28.195, 30.524, 31.784, 33.426, 35.601, 36.874]
    line [7.948, 5.425, 6.036, 6.7, 8.517, 8.471, 9.593, 10.239, 10.973, 11.152, 11.833, 12.947, 14.04, 15.622, 15.317, 15.203, 19.549, 19.152]
    line [6.81, 7.121, 8.065, 9.063, 10.846, 11.267, 12.407, 14.082, 14.092, 14.339, 15.498, 16.654, 17.219, 18.933, 17.922, 18.133, 17.915, 18.388]
```

<sub>Blue: api. Orange: db. Green: ro (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window).</sub>

## Diagnosis

### 1. Working set doesn't fit in memory (ro), from 2:10

From 2:10, ro answered only 82% of its block reads from shared_buffers and read 972 blocks a second from outside them, each taking 3.6 ms against 0.8 ms at low load. The rows requests touch no longer fit in its memory, so reads wait for the disk.

Evidence:

- 82% of block reads from shared_buffers at 2:40 <sub>(pg_stat_database_blks_hit ÷ (blks_hit + blks_read) (postgres_exporter))</sub>
- 972 blocks read per second from outside them at 2:40 <sub>(pg_stat_database_blks_read (postgres_exporter): blocks read from outside shared_buffers)</sub>
- 3.6 ms per block read at 2:40, against 0.8 ms at low load <sub>(pg_stat_database_blk_read_time ÷ pg_stat_database_blks_read (postgres_exporter, track_io_timing on))</sub>
- p99 63 ms at 2:40, against 2.8 ms at low load <sub>(k6_http_req_duration_seconds (k6 histogram, end to end))</sub>
- ro CPU at 18% of its limit at 2:40 <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- ro has a dataset of about 977 MB, shared_buffers 128MB, a 512 MB memory limit <sub>(plan.json)</sub>

**Try next:** Make the hot rows fit: give ro more memory (its limit and shared_buffers), read fewer rows per request (an index that covers the query, or narrower rows), or split the data so each node's working set fits.

### 2. Replication lag (ro behind db), from 2:20

From 2:20, ro fell behind db, by up to 4.6 s at 2:50 (35 MB of WAL it hadn't replayed), so reads from it returned data from before the primary's latest writes. Its block reads took 4.8 ms each against 0.8 ms at low load: a replica replays its primary's writes in one process, and each change to a page it doesn't hold in memory waits for the disk, which its own queries also queue for.

Evidence:

- ro 4.6 s behind db at 2:50, 35 MB of WAL it hadn't replayed <sub>(pg_stat_replication_pg_wal_lsn_diff ÷ the rate of pg_stat_replication_pg_current_wal_lsn_bytes (postgres_exporter, on the primary): the WAL the replica hadn't replayed, in seconds of the primary's writing; without them, pg_replication_lag_seconds on the replica)</sub>
- 4.8 ms per block read on ro at 2:50, against 0.8 ms at low load <sub>(pg_stat_database_blk_read_time ÷ pg_stat_database_blks_read (postgres_exporter, track_io_timing on))</sub>
- ro CPU at 18% of its limit at 2:50 <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>

**Try next:** Send reads that must see a request's own writes, such as the page after a form post, to db (read-your-writes), or let ro keep up: memory for the rows its writes touch, or a faster disk. More replicas don't help: each one replays every write.

<sub>Rules version 11. Rules are deterministic and offline; their thresholds are listed below.</sub>

## Predictions

1 of 1 scored prediction held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 500 until load_rps >= 1000` | **held** | 4.6 ms | 0:00 to 1:30, when load_rps >= 1,000 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |

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
| services.ro.disk | 1 ms per read, 3000 IOPS | set in the design: every read that misses ro's memory waits this long, and reads and writes are each capped at this rate, as on a cloud volume (unset values default to about a gp3 volume's: 1 ms, 3000 IOPS) |
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
- Host speed index 973 (hash 913, memory 1011, syscalls 998): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (973) was within 5.0% of the run it's compared with (996)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 179,992 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-stale-orders-read-your-writes-20261115-123901-api-0-1 | loadlore/synthetic:dev |
| loadlore-stale-orders-read-your-writes-20261115-123901-db-1 | postgres:17.11 |
| loadlore-stale-orders-read-your-writes-20261115-123901-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-stale-orders-read-your-writes-20261115-123901-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-stale-orders-read-your-writes-20261115-123901-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-stale-orders-read-your-writes-20261115-123901-lore-pgexp-ro-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-stale-orders-read-your-writes-20261115-123901-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-stale-orders-read-your-writes-20261115-123901-lore-toxiproxy-1 | ghcr.io/shopify/toxiproxy:2.13.0 |
| loadlore-stale-orders-read-your-writes-20261115-123901-ro-1 | postgres:17.11 |

</details>

## Per-window data

<details><summary>18 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | ro CPU | ro conns | ro lag | api→db pool | wait | api→ro pool | wait | k6 CPU | Toxiproxy CPU | k6 / Toxiproxy stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 244 | 239 | 0 | 0% | 1.3 ms | 3.9 ms | 6.0% | 7.9% | 41 | 6.8% | 41 | 0.1 ms | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 5.8% | 3.4% | 1.0% / 0% |
| 0:10 | 333 | 332 | 0 | 0% | 1.0 ms | 2.8 ms | 7.8% | 5.4% | 41 | 7.1% | 41 | 7.2 ms | 0/80 | 0.0 ms | 1/80 | 0.0 ms | 3.5% | 3.4% | 0% / 0% |
| 0:20 | 422 | 420 | 0 | 0% | 0.9 ms | 2.7 ms | 9.7% | 6.0% | 41 | 8.1% | 41 | 3.5 ms | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 5.1% | 4.4% | 0% / 0% |
| 0:30 | 511 | 509 | 0 | 0% | 0.9 ms | 2.6 ms | 12% | 6.7% | 41 | 9.1% | 41 | 5.8 ms | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 5.4% | 5.3% | 0% / 0% |
| 0:40 | 600 | 598 | 0 | 0% | 0.9 ms | 4.6 ms | 14% | 8.5% | 41 | 11% | 41 | 15 ms | 1/80 | 0.0 ms | 2/80 | 0.0 ms | 6.4% | 6.5% | 0% / 0% |
| 0:50 | 689 | 687 | 0 | 0% | 0.9 ms | 2.5 ms | 15% | 8.5% | 41 | 11% | 41 | 5.3 ms | 3/80 | 0.0 ms | 1/80 | 0.0 ms | 6.8% | 7.2% | 0% / 0% |
| 1:00 | 778 | 776 | 0 | 0% | 0.9 ms | 3.0 ms | 17% | 9.6% | 41 | 12% | 41 | 396 ms | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 7.9% | 8.2% | 0% / 0% |
| 1:10 | 867 | 865 | 0 | 0% | 0.9 ms | 3.0 ms | 19% | 10% | 41 | 14% | 41 | 5.3 ms | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 8.3% | 9.2% | 0% / 0% |
| 1:20 | 956 | 954 | 0 | 0% | 0.9 ms | 3.0 ms | 21% | 11% | 41 | 14% | 41 | 2.6 ms | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 8.8% | 9.9% | 0% / 0% |
| 1:30 | 1,044 | 1,043 | 0 | 0% | 0.9 ms | 3.4 ms | 22% | 11% | 41 | 14% | 41 | 555 ms | 2/80 | 0.0 ms | 1/80 | 0.0 ms | 9.4% | 10% | 0% / 0% |
| 1:40 | 1,133 | 1,131 | 0 | 0% | 0.9 ms | 3.3 ms | 24% | 12% | 41 | 15% | 41 | 423 ms | 2/80 | 0.0 ms | 2/80 | 0.0 ms | 9.9% | 12% | 0% / 0% |
| 1:50 | 1,222 | 1,221 | 0 | 0% | 0.9 ms | 3.5 ms | 26% | 13% | 41 | 17% | 41 | 75 ms | 2/80 | 0.0 ms | 2/80 | 0.0 ms | 11% | 13% | 0% / 0% |
| 2:00 | 1,311 | 1,309 | 0 | 0% | 0.9 ms | 27 ms | 28% | 14% | 41 | 17% | 49 | 1.09 s | 2/80 | 0.0 ms | 1/80 | 0.0 ms | 11% | 14% | 0% / 0% |
| 2:10 | 1,400 | 1,398 | 0 | 0% | 0.9 ms | 41 ms | 31% | 16% | 41 | 19% | 52 | 832 ms | 2/80 | 0.0 ms | 1/80 | 0.0 ms | 13% | 15% | 0% / 0% |
| 2:20 | 1,489 | 1,488 | 0 | 0% | 0.9 ms | 59 ms | 32% | 15% | 47 | 18% | 55 | 1.46 s | 2/80 | 0.0 ms | 20/80 | 0.0 ms | 13% | 16% | 0% / 0% |
| 2:30 | 1,578 | 1,576 | 0 | 0% | 0.9 ms | 62 ms | 33% | 15% | 47 | 18% | 65 | 2.33 s | 3/80 | 0.0 ms | 15/80 | 0.0 ms | 13% | 16% | 0% / 0% |
| 2:40 | 1,667 | 1,665 | 0 | 0% | 1.0 ms | 63 ms | 36% | 20% | 47 | 18% | 67 | 3.65 s | 5/80 | 0.0 ms | 23/80 +2 | 0.0 ms | 14% | 18% | 0% / 0% |
| 2:50 | 1,756 | 1,754 | 0 | 0% | 1.0 ms | 73 ms | 37% | 19% | 47 | 18% | 81 | 4.64 s | 3/80 | 0.0 ms | 3/80 | 0.0 ms | 14% | 19% | 0% / 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); a replica's lag is from pg_stat_replication_pg_wal_lsn_diff ÷ the rate of pg_stat_replication_pg_current_wal_lsn_bytes (postgres_exporter, on the primary): the WAL the replica hadn't replayed, in seconds of the primary's writing; without them, pg_replication_lag_seconds on the replica, and its stale share of reads after writes from loadlore_read_after_write_total (the synthetic service): point reads of a key the same request had just written, stale when they didn't see the write; hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 and Toxiproxy CPU are against their budgets.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: stale-orders-read-your-writes

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
        - { target: db, op: read, when: write, mode: async, after_ms: 200 }   # the confirmation page reads the primary (was: ro)
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
    resources: { cpu: 0.75, memory: 512m }
    config: { max_connections: 100, shared_buffers: 128MB }
    disk: {}   # a general-purpose volume at its baseline: about 1 ms a read, 3,000 IOPS

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
