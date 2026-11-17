# stale-orders-read-your-writes: run report

_run `stale-orders-read-your-writes-20261117-145733` · completed · 17 Nov 2026 14:57 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 1,800 rps offered, 1,754 rps served at best, worst 10-second p99 331 ms, at most 0% errors. But ro fell behind db, so reads from it returned old data (see replication lag).

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 2:50

Measured in the 10-second window from 2:50 to 3:00 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 1,800 rps<br/>offered 1,756 rps"])
    lb{{"lb<br/>load balancer<br/>0.4 CPU · 256m<br/>CPU 26%<br/>2/2 replicas up"}}
    api["api ×2<br/>1 CPU · 1g<br/>CPU 43%<br/>in flight 13"]
    ro[("ro<br/>postgres-replica 17<br/>0.75 CPU · 512m<br/>disk 1 ms · 3,000 IOPS<br/>CPU 26%<br/>conns 81/100<br/>lag 1.92 s")]
    db[("db<br/>postgres 17<br/>1 CPU · 2g<br/>CPU 28%<br/>conns 81/100")]
    load -->|"50% read · 50% write<br/>1,754 rps · p99 40 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>1,728 rps · p99 44 ms"| api
    api -->|"read on reads<br/>pool 40, min 20<br/>870 calls/s · p99 61 ms<br/>in use 11/80"| ro
    api -->|"write on writes<br/>pool 40, min 20<br/>1,737 calls/s · p99 7.8 ms<br/>in use 6/80"| db
    api -.->|"read on writes<br/>pool 40, min 20<br/>1,737 calls/s · p99 7.8 ms<br/>in use 6/80"| db
    db -.->|"streams WAL"| ro
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb,api,ro,db ok
```

<sub>Sources: offered from the load profile in plan.json (offered arrival rate); load edge from k6_http_reqs_total (k6: all responses) and k6_http_req_duration_seconds (k6 histogram, end to end); CPU from container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit; calls/s from loadlore_calls_total (the caller's outbound call attempts) and their p99 from loadlore_call_duration_seconds (the caller's histogram, pool wait included); pools from loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections and loadlore_pool_pending (callers waiting for a connection); lb traffic from haproxy_backend_http_requests_total (HAProxy), replicas up from haproxy_backend_active_servers (HAProxy), and the lb edge's p99 from loadlore_request_duration_seconds (the service's own histogram); conns from pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); a replica's lag from pg_stat_replication_pg_wal_lsn_diff ÷ the rate of pg_stat_replication_pg_current_wal_lsn_bytes (postgres_exporter, on the primary): the WAL the replica hadn't replayed, in seconds of the primary's writing; without them, pg_replication_lag_seconds on the replica.</sub>

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
    line [238.344, 331.274, 420.252, 509.283, 597.974, 686.918, 775.871, 864.733, 953.613, 1042.584, 1131.608, 1220.089, 1309.126, 1398.106, 1486.96, 1575.561, 1664.915, 1753.69]
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
    y-axis "ms" 0 --> 400
    line [2.237, 1.82, 1.789, 1.758, 1.722, 1.685, 1.689, 1.699, 1.69, 1.696, 1.696, 1.691, 1.702, 1.728, 1.763, 1.874, 2.031, 2.129]
    line [3.716, 3.279, 3.055, 2.941, 2.507, 2.517, 2.607, 2.519, 2.706, 2.605, 2.963, 2.959, 3.012, 3.264, 4.541, 37.226, 73.616, 8.401]
    line [4.553, 3.554, 3.467, 3.833, 3.353, 3.418, 3.894, 3.405, 4.133, 3.705, 6.504, 6.263, 6.425, 34.42, 54.095, 218.986, 331.126, 39.974]
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
      plotColorPalette: "#1f77b4, #ff7f0e, #2ca02c, #9467bd"
---
xychart-beta
    title "CPU against each limit"
    x-axis "seconds" 0 --> 170
    y-axis "% of limit" 0 --> 100
    line [6.226, 7.97, 10.021, 12.006, 13.891, 15.735, 17.805, 19.651, 21.718, 23.558, 25.392, 27.37, 29.424, 31.618, 33.897, 37.334, 43.281, 43.414]
    line [4.694, 5.18, 5.98, 7.069, 7.177, 7.89, 8.607, 9.081, 10.257, 10.268, 10.738, 11.318, 12.262, 13.728, 14.812, 19.047, 33.985, 27.819]
    line [5.593, 5.866, 7.206, 8.664, 9.107, 10.716, 11.697, 12.709, 13.463, 10.997, 12.451, 11.811, 13.218, 14.405, 16.173, 19.225, 30.384, 26.487]
    line [5.887, 6.809, 7.455, 9.038, 9.066, 10.351, 11.205, 12.497, 12.991, 13.542, 13.969, 14.879, 15.776, 16.55, 18.748, 21.427, 29.292, 25.657]
```

<sub>Blue: api. Orange: db. Green: lb. Purple: ro (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window).</sub>

## Diagnosis

### 1. Replication lag (ro behind db), from 2:10

From 2:10, ro fell behind db, by up to 1.9 s at 2:50 (15 MB of WAL it hadn't replayed), so reads from it returned data from before the primary's latest writes. Its block reads took 2.4 ms each against 0.8 ms at low load: a replica replays its primary's writes in one process, and each change to a page it doesn't hold in memory waits for the disk, which its own queries also queue for.

Evidence:

- ro 1.9 s behind db at 2:50, 15 MB of WAL it hadn't replayed <sub>(pg_stat_replication_pg_wal_lsn_diff ÷ the rate of pg_stat_replication_pg_current_wal_lsn_bytes (postgres_exporter, on the primary): the WAL the replica hadn't replayed, in seconds of the primary's writing; without them, pg_replication_lag_seconds on the replica)</sub>
- 2.4 ms per block read on ro at 2:50, against 0.8 ms at low load <sub>(pg_stat_database_blk_read_time ÷ pg_stat_database_blks_read (postgres_exporter, track_io_timing on))</sub>
- ro CPU at 26% of its limit at 2:50 <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>

**Try next:** Send reads that must see a request's own writes, such as the page after a form post, to db (read-your-writes), or let ro keep up: memory for the rows its writes touch, or a faster disk. More replicas don't help: each one replays every write.

### 2. Working set doesn't fit in memory (ro), from 2:20

From 2:20, ro answered only 81% of its block reads from shared_buffers and read 1,028 blocks a second from outside them, each taking 2.4 ms against 0.8 ms at low load. The rows requests touch no longer fit in its memory, so reads wait for the disk.

Evidence:

- 81% of block reads from shared_buffers at 2:50 <sub>(pg_stat_database_blks_hit ÷ (blks_hit + blks_read) (postgres_exporter))</sub>
- 1,028 blocks read per second from outside them at 2:50 <sub>(pg_stat_database_blks_read (postgres_exporter): blocks read from outside shared_buffers)</sub>
- 2.4 ms per block read at 2:50, against 0.8 ms at low load <sub>(pg_stat_database_blk_read_time ÷ pg_stat_database_blks_read (postgres_exporter, track_io_timing on))</sub>
- p99 40 ms at 2:50, against 3.6 ms at low load <sub>(k6_http_req_duration_seconds (k6 histogram, end to end))</sub>
- ro CPU at 26% of its limit at 2:50 <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- ro has a dataset of about 977 MB, shared_buffers 128MB, a 512 MB memory limit <sub>(plan.json)</sub>

**Try next:** Make the hot rows fit: give ro more memory (its limit and shared_buffers), read fewer rows per request (an index that covers the query, or narrower rows), or split the data so each node's working set fits.

<sub>Rules version 19. Rules are deterministic and offline; their thresholds are listed below.</sub>

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
| network.profile | same-zone: 0.3 ms round trip | the default: every service in one availability zone. AWS says a round trip between two instances in the same zone is generally sub-millisecond with enhanced networking, and publishes no exact figure, so the 0.3 ms round trip Loadlore adds between any two containers is its own estimate, inside that bound. Applied between any two of the stack's containers, the load generator and the entry point included, as half the round trip on each side's outgoing packets; Loadlore's own tooling isn't slowed. Not scaled with --scale. Source: AWS Architecture Blog, Improving performance and reducing cost using Availability Zone Affinity: https://aws.amazon.com/blogs/architecture/improving-performance-and-reducing-cost-using-availability-zone-affinity/ |
| resources.cpu_period_ms | 100 ms for api, db, lb, ro | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.db.disk | laptop: the Docker host's | no disk block: a read that misses db's memory goes to the Docker host's disk, which on a laptop mostly answers from the host's own memory, fast and uncapped, and a commit's fsync returns in a fraction of a millisecond. Add disk: { profile: gp3 } for a cloud volume's waits and caps |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |
| services.ro.disk | no profile: 1 ms per read, 3000 IOPS | set in the design: every read that misses ro's memory waits this long, and reads and writes are each capped at this rate, as on a cloud volume (unset values default to about a gp3 volume's: 1 ms, 3000 IOPS) |
| services.ro replication | asynchronous streaming from db | the replica starts from a copy of db's seeded data and replays its WAL as it arrives, directly, not through Toxiproxy: a commit on db doesn't wait for it, so a read on ro can miss a write that just committed |
| services.db.config.wal_keep_size | 1GB | Loadlore's default: enough for a replica that falls behind under write load to catch up |
| services.api.pools.db.min | 20 | set in the design: each replica opens 20 connections before the load starts and keeps them open |
| services.api.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- Loadlore's own containers get fixed budgets: load generator 1.5 CPU (peak 1800 rps), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each. No fault acts on a connection, so every connection is direct and there is no Toxiproxy

## Thresholds

- **Broke:** 2 windows in a row with at least 1.0% of requests failing or successful responses below 90% of the offered rate, or 3 windows in a row with any of those or p99 at least 10x the low-load p99 (the median p99 of windows offered at most 25% of the peak, before errors or a shortfall in 2 windows in a row) and at least the SLO, 500 ms (workload.slo.p99_ms): 500 ms in this run. A shorter episode is shown as a stall in the timeline, not a break.
- **Doubtful:** a break on p99 alone, when the load generator was stalled at its CPU limit in 1.0% or more of its scheduling periods in one of the break's windows and the entry service's own p99 (loadlore_request_duration_seconds (the service's own histogram)) was below the limit in one of them. The verdict stays what it was.
- **Inconclusive:** before the break (or anywhere, if nothing broke), the load generator dropped more than 1.0% of a window's arrivals (more than 5.0% in a window where the system itself failed 1.0% or more of requests; up to that it's a caveat that gives the share), or the load generator used 90% or more of its CPU budget, or was stalled at its CPU limit in 5.0% or more of its scheduling periods (from 1.0% it's noted as a caveat). The first 10 s of load are a warm-up and don't count: the load generator starts its VUs and connections open then. A cause is named only with evidence: on a Linux host, the hypervisor giving 2.0% or more of CPU time to other virtual machines (steal), or other programs using 10% or more of the host's CPUs (at least 0.5); inside a VM on this machine, only as a possibility. A fault that couldn't be injected, or an autoscaler decision that couldn't be carried out, also makes a run inconclusive: it measured a different design.
- **Connection pool exhausted:** for two windows in a row, a pool at 95% or more of its size with callers waiting, while the target wasn't busy (busy: its CPU at 80% or more, or its time per call with a connection at least 3x the low-load value and 1 ms above it; CPU throttling alone doesn't count) and the caller's CPU stayed below 90%.
- **Queueing knee:** for 3 windows in a row, p99 at least 3x the low-load p99 and 10 ms above it, with a CPU or pool at 70% or more. When that is a pool whose target was busy in any of those windows, or a service's CPU whose requests spent 50% or more of their time in calls to a busy target, the knee is the target's.
- **Connection storm:** up to the break, a service's pools to a Postgres opening at least 50% of their combined size, and 10 or more connections, in one window, while p99 in that window or the next was at least 10x the low-load p99 and 10 ms above it. New connections are counted by loadlore_pool_connect_seconds (new connections the pools opened, and how long each took), or for runs from before it, from the rise in open connections.
- **Retry storm:** for two windows in a row, a caller that retries sending 1.5x or more of its low-load calls per request to a target, with retries 20% or more of them, while the target failed 1.0% or more of its calls or its p99 was 3x its low-load value.
- **Cold cache:** a cache's share of lookups that miss rising to 5x its level before or more, and to 5.0% of lookups at least, while its callers' reads of the database behind it reach 2x their rate before (counted as at least 1 a second, for a cache warm enough to send none).
- **Working set doesn't fit:** for two windows in a row, a Postgres answering less than 90% of block reads from shared_buffers and reading 100 blocks a second or more from outside them, while p99 was 3x the low-load p99 or more. When Postgres times its reads (track_io_timing, on with a disk block), each block read must also take 3x its low-load time or more, and 1 ms more.
- **Replication lag:** for two windows in a row, a read replica 1 s or more behind its primary, or 5.0% or more of the reads after a write it served missing the write.
- **Consumer lag:** for two windows in a row, a queue's oldest unfinished job had waited 5 s or more, or the queue held more jobs than its workers run at once and grew, with jobs published 5.0% or more faster than they were finished. Named a full queue once its memory reached 90% of its maxmemory or it refused jobs.
- **Autoscale lag:** the run broke while an autoscaled service's replicas were at 90% or more of their CPU limit: below its max, the autoscaler hadn't caught up; at its max (autoscale ceiling), it had nothing left to add.
- **Hot rows:** for two windows in a row, 2 or more of a Postgres's backends waiting on locks on average, while a caller's writes to it took 3x their low-load time or more (and 2 ms more). Reads of the same rows may slow too, and the summary says when the database's CPU per request reached 3x its low-load value: the contention burning CPU. A queueing knee on that database's CPU at the same time then points at the rows. Windows with a disk_slow fault on the database don't count.
- **Hot shard:** for two windows in a row, a caller sending calls to shards by key, one shard taking 1.5x the calls of the next busiest shard or more while it was busy (CPU 80% or more, or its calls 3x their low-load time and 1 ms more) and another shard's CPU under 50%. A queueing knee on that shard at the same time then points at its popular keys.

## Environment

- loadl dev, profile 3m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 982 (hash 927, memory 1020, syscalls 1003): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (982) was within 5.0% of the run it's compared with (1019)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 179,990 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-stale-orders-read-your-writes-20261117-145733-api-0-1 | loadlore/synthetic:dev |
| loadlore-stale-orders-read-your-writes-20261117-145733-api-1-1 | loadlore/synthetic:dev |
| loadlore-stale-orders-read-your-writes-20261117-145733-db-1 | postgres:17.11 |
| loadlore-stale-orders-read-your-writes-20261117-145733-lb-1 | haproxy:3.2.26 |
| loadlore-stale-orders-read-your-writes-20261117-145733-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-stale-orders-read-your-writes-20261117-145733-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-stale-orders-read-your-writes-20261117-145733-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-stale-orders-read-your-writes-20261117-145733-lore-pgexp-ro-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-stale-orders-read-your-writes-20261117-145733-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-stale-orders-read-your-writes-20261117-145733-ro-1 | postgres:17.11 |

</details>

## Per-window data

<details><summary>18 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | lb CPU | lb up | ro CPU | ro conns | ro lag | api→db pool | wait | api→ro pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 244 | 238 | 0 | 0% | 2.2 ms | 4.6 ms | 6.2% | 4.7% | 41 | 5.6% | 2 | 5.9% | 41 | 9.1 ms | 2/80 +1 | 0.0 ms | 1/80 | 0.0 ms | 5.3% | 1.0% |
| 0:10 | 333 | 331 | 0 | 0% | 1.8 ms | 3.6 ms | 8.0% | 5.2% | 41 | 5.9% | 2 | 6.8% | 41 | 7.2 ms | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 3.2% | 0% |
| 0:20 | 422 | 420 | 0 | 0% | 1.8 ms | 3.5 ms | 10% | 6.0% | 41 | 7.2% | 2 | 7.5% | 41 | 6.4 ms | 2/80 | 0.0 ms | 1/80 | 0.0 ms | 4.5% | 0% |
| 0:30 | 511 | 509 | 0 | 0% | 1.8 ms | 3.8 ms | 12% | 7.1% | 41 | 8.7% | 2 | 9.0% | 41 | 5.8 ms | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 5.2% | 0% |
| 0:40 | 600 | 598 | 0 | 0% | 1.7 ms | 3.4 ms | 14% | 7.2% | 41 | 9.1% | 2 | 9.1% | 41 | 5.6 ms | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 5.3% | 0% |
| 0:50 | 689 | 687 | 0 | 0% | 1.7 ms | 3.4 ms | 16% | 7.9% | 41 | 11% | 2 | 10% | 41 | 5.3 ms | 2/80 | 0.0 ms | 1/80 | 0.0 ms | 6.2% | 0% |
| 1:00 | 778 | 776 | 0 | 0% | 1.7 ms | 3.9 ms | 18% | 8.6% | 41 | 12% | 2 | 11% | 41 | 7.8 ms | 2/80 | 0.0 ms | 1/80 | 0.0 ms | 7.6% | 0% |
| 1:10 | 867 | 865 | 0 | 0% | 1.7 ms | 3.4 ms | 20% | 9.1% | 41 | 13% | 2 | 12% | 41 | 2.7 ms | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 7.7% | 0% |
| 1:20 | 956 | 954 | 0 | 0% | 1.7 ms | 4.1 ms | 22% | 10% | 41 | 13% | 2 | 13% | 41 | 369 ms | 1/80 | 0.0 ms | 2/80 | 0.0 ms | 8.5% | 0% |
| 1:30 | 1,044 | 1,043 | 0 | 0% | 1.7 ms | 3.7 ms | 24% | 10% | 41 | 11% | 2 | 14% | 41 | 289 ms | 2/80 | 0.0 ms | 1/80 | 0.0 ms | 9.1% | 0% |
| 1:40 | 1,133 | 1,132 | 0 | 0% | 1.7 ms | 6.5 ms | 25% | 11% | 41 | 12% | 2 | 14% | 41 | 1.09 s | 4/80 | 0.0 ms | 2/80 | 0.0 ms | 9.3% | 0% |
| 1:50 | 1,222 | 1,220 | 0 | 0% | 1.7 ms | 6.3 ms | 27% | 11% | 41 | 12% | 2 | 15% | 49 | 819 ms | 2/80 | 0.0 ms | 1/80 | 0.0 ms | 9.9% | 0% |
| 2:00 | 1,311 | 1,309 | 0 | 0% | 1.7 ms | 6.4 ms | 29% | 12% | 41 | 13% | 2 | 16% | 54 | 609 ms | 3/80 +1 | 0.0 ms | 2/80 | 0.0 ms | 11% | 0% |
| 2:10 | 1,400 | 1,398 | 0 | 0% | 1.7 ms | 34 ms | 32% | 14% | 41 | 14% | 2 | 17% | 63 | 1.35 s | 3/80 | 0.0 ms | 20/80 | 0.0 ms | 12% | 0% |
| 2:20 | 1,489 | 1,487 | 0 | 0% | 1.8 ms | 54 ms | 34% | 15% | 41 | 16% | 2 | 19% | 63 | 1.69 s | 1/80 | 0.0 ms | 8/80 | 0.0 ms | 13% | 0% |
| 2:30 | 1,578 | 1,576 | 0 | 0% | 1.9 ms | 219 ms | 37% | 19% | 81 | 19% | 2 | 21% | 81 | 1.78 s | 42/80 +86 | 2.2 ms | 46/80 +19 | 1.4 ms | 14% | 0% |
| 2:40 | 1,667 | 1,665 | 0 | 0% | 2.0 ms | 331 ms | 43% | 34% | 81 | 30% | 2 | 29% | 81 | 1.79 s | 80/80 +176 | 6.4 ms | 20/80 +1 | 0.0 ms | 22% | 1.0% |
| 2:50 | 1,756 | 1,754 | 0 | 0% | 2.1 ms | 40 ms | 43% | 28% | 81 | 26% | 2 | 26% | 81 | 1.92 s | 6/80 | 0.0 ms | 11/80 | 0.0 ms | 18% | 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); a replica's lag is from pg_stat_replication_pg_wal_lsn_diff ÷ the rate of pg_stat_replication_pg_current_wal_lsn_bytes (postgres_exporter, on the primary): the WAL the replica hadn't replayed, in seconds of the primary's writing; without them, pg_replication_lag_seconds on the replica, and its stale share of reads after writes from loadlore_read_after_write_total (the synthetic service): point reads of a key the same request had just written, stale when they didn't see the write; hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 CPU is against its budget.

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
  lb:
    kind: lb
    target: api
    resources: { cpu: 0.4, memory: 256m }
    health_check: { interval_ms: 2000, fall: 3, rise: 2 }

  api:
    image: loadlore/synthetic
    replicas: 2
    resources: { cpu: 1, memory: 1g }
    behavior:
      cpu_ms: 0.3
      calls:
        - { target: ro, op: read, when: read }                                # browsing
        - { target: db, op: write, when: write }                              # place an order
        - { target: db, op: read, when: write, mode: async, after_ms: 200 }   # the confirmation page reads the primary (was: ro)
      resilience:
        timeout_ms: 2000
    pools:   # per replica: 2 x 40 stays under the primary's max_connections
      db: { size: 40, min: 20 }
      ro: { size: 40, min: 20 }

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
  entry: lb
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
