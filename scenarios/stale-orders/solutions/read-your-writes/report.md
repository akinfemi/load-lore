# stale-orders-read-your-writes: run report

_run `stale-orders-read-your-writes-20261118-011051` · completed · 18 Nov 2026 01:11 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 1,800 rps offered, 1,754 rps served at best, worst 10-second p99 69 ms, at most 0% errors. But ro fell behind db, so reads from it returned old data (see replication lag).

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 2:50

Measured in the 10-second window from 2:50 to 3:00 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 1,800 rps<br/>offered 1,756 rps"])
    lb{{"lb<br/>load balancer<br/>0.4 CPU · 256m<br/>CPU 23%<br/>2/2 replicas up"}}
    api["api ×2<br/>1 CPU · 1g<br/>CPU 41%<br/>in flight 6"]
    ro[("ro<br/>postgres-replica 17<br/>0.75 CPU · 512m<br/>disk 1 ms · 3,000 IOPS<br/>CPU 21%<br/>conns 81/100<br/>lag 4.39 s")]
    db[("db<br/>postgres 17<br/>1 CPU · 2g<br/>CPU 22%<br/>conns 81/100")]
    load -->|"50% read · 50% write<br/>1,754 rps · p99 69 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>1,736 rps · p99 72 ms"| api
    api -->|"read on reads<br/>pool 40, min 40<br/>867 calls/s · p99 84 ms<br/>in use 4/80"| ro
    api -->|"write on writes<br/>pool 40, min 40<br/>1,735 calls/s · p99 4.8 ms<br/>in use 4/80"| db
    api -.->|"read on writes<br/>pool 40, min 40<br/>1,735 calls/s · p99 4.8 ms<br/>in use 4/80"| db
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
    line [237.39, 330.957, 419.894, 508.928, 597.798, 686.502, 775.529, 864.505, 953.244, 1042.173, 1131.126, 1219.893, 1308.889, 1397.763, 1486.59, 1575.552, 1664.231, 1753.591]
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
    line [2.21, 1.868, 1.794, 1.748, 1.7, 1.671, 1.672, 1.678, 1.662, 1.664, 1.682, 1.662, 1.697, 1.685, 1.698, 1.728, 1.814, 1.894]
    line [3.752, 3.398, 3.126, 2.818, 2.508, 2.502, 2.615, 2.526, 2.743, 2.635, 2.905, 2.724, 3.111, 3.21, 3.928, 4.51, 7.163, 20.966]
    line [4.688, 3.589, 3.516, 3.438, 3.138, 3.178, 3.998, 3.356, 4.083, 3.617, 7.114, 4.948, 8.159, 35.878, 51.829, 57.383, 59.32, 68.564]
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
    line [6.159, 7.924, 9.857, 11.849, 13.707, 15.604, 17.645, 19.522, 21.54, 23.309, 25.225, 27.206, 29.137, 31.061, 33.202, 35.726, 38.742, 40.983]
    line [4.483, 5.069, 5.553, 6.385, 6.726, 7.522, 8.335, 8.943, 9.364, 9.815, 10.392, 11.149, 12.022, 12.566, 13.287, 15.105, 19.603, 21.698]
    line [5.612, 5.958, 7.131, 8.306, 9.341, 10.621, 10.88, 12.222, 10.797, 11.191, 12.173, 12.001, 13.098, 13.71, 15.184, 17.18, 19.468, 23.469]
    line [5.792, 6.809, 7.485, 8.302, 8.897, 10.078, 10.788, 12.008, 12.337, 13.134, 13.782, 15.144, 14.869, 15.633, 16.142, 18.595, 18.256, 20.803]
```

<sub>Blue: api. Orange: db. Green: lb. Purple: ro (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window).</sub>

## Diagnosis

### 1. Replication lag (ro behind db), from 2:20

From 2:20, ro fell behind db, by up to 4.4 s at 2:50 (34 MB of WAL it hadn't replayed), so reads from it returned data from before the primary's latest writes. Its block reads took 5.0 ms each against 0.8 ms at low load: a replica replays its primary's writes in one process, and each change to a page it doesn't hold in memory waits for the disk, which its own queries also queue for.

Evidence:

- ro 4.4 s behind db at 2:50, 34 MB of WAL it hadn't replayed <sub>(pg_stat_replication_pg_wal_lsn_diff ÷ the rate of pg_stat_replication_pg_current_wal_lsn_bytes (postgres_exporter, on the primary): the WAL the replica hadn't replayed, in seconds of the primary's writing; without them, pg_replication_lag_seconds on the replica)</sub>
- 5.0 ms per block read on ro at 2:50, against 0.8 ms at low load <sub>(pg_stat_database_blk_read_time ÷ pg_stat_database_blks_read (postgres_exporter, track_io_timing on))</sub>
- ro CPU at 21% of its limit at 2:50 <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>

**Try next:** Send reads that must see a request's own writes, such as the page after a form post, to db (read-your-writes), or let ro keep up: memory for the rows its writes touch, or a faster disk. More replicas don't help: each one replays every write.

### 2. Working set doesn't fit in memory (ro), from 2:30

From 2:30, ro answered only 81% of its block reads from shared_buffers and read 1,014 blocks a second from outside them, each taking 3.0 ms against 0.8 ms at low load. The rows requests touch no longer fit in its memory, so reads wait for the disk.

Evidence:

- 81% of block reads from shared_buffers at 2:30 <sub>(pg_stat_database_blks_hit ÷ (blks_hit + blks_read) (postgres_exporter))</sub>
- 1,014 blocks read per second from outside them at 2:30 <sub>(pg_stat_database_blks_read (postgres_exporter): blocks read from outside shared_buffers)</sub>
- 3.0 ms per block read at 2:30, against 0.8 ms at low load <sub>(pg_stat_database_blk_read_time ÷ pg_stat_database_blks_read (postgres_exporter, track_io_timing on))</sub>
- p99 57 ms at 2:30, against 3.6 ms at low load <sub>(k6_http_req_duration_seconds (k6 histogram, end to end))</sub>
- ro CPU at 19% of its limit at 2:30 <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- ro has a dataset of about 977 MB, shared_buffers 128MB, a 512 MB memory limit <sub>(plan.json)</sub>

**Try next:** Make the hot rows fit: give ro more memory (its limit and shared_buffers), read fewer rows per request (an index that covers the query, or narrower rows), or split the data so each node's working set fits.

<sub>Rules version 21. Rules are deterministic and offline; their thresholds are listed below.</sub>

## Predictions

1 of 1 scored prediction held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 500 until load_rps >= 1000` | **held** | 4.7 ms | 0:00 to 1:30, when load_rps >= 1,000 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |

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
| services.api.pools.db.min | 40 | set in the design: each replica opens 40 connections before the load starts and keeps them open |
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
- Host speed index 980 (hash 926, memory 1035, syscalls 983): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (980) was within 5.0% of the run it's compared with (995)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 179,994 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-stale-orders-read-your-writes-20261118-011051-api-0-1 | loadlore/synthetic:dev |
| loadlore-stale-orders-read-your-writes-20261118-011051-api-1-1 | loadlore/synthetic:dev |
| loadlore-stale-orders-read-your-writes-20261118-011051-db-1 | postgres:17.11 |
| loadlore-stale-orders-read-your-writes-20261118-011051-lb-1 | haproxy:3.2.26 |
| loadlore-stale-orders-read-your-writes-20261118-011051-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-stale-orders-read-your-writes-20261118-011051-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-stale-orders-read-your-writes-20261118-011051-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-stale-orders-read-your-writes-20261118-011051-lore-pgexp-ro-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-stale-orders-read-your-writes-20261118-011051-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-stale-orders-read-your-writes-20261118-011051-ro-1 | postgres:17.11 |

</details>

## Per-window data

<details><summary>18 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | lb CPU | lb up | ro CPU | ro conns | ro lag | api→db pool | wait | api→ro pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 244 | 237 | 0 | 0% | 2.2 ms | 4.7 ms | 6.2% | 4.5% | 81 | 5.6% | 2 | 5.8% | 81 | 9.2 ms | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 5.2% | 1.0% |
| 0:10 | 333 | 331 | 0 | 0% | 1.9 ms | 3.6 ms | 7.9% | 5.1% | 81 | 6.0% | 2 | 6.8% | 81 | 7.2 ms | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 3.2% | 0% |
| 0:20 | 422 | 420 | 0 | 0% | 1.8 ms | 3.5 ms | 9.9% | 5.6% | 81 | 7.1% | 2 | 7.5% | 81 | 3.4 ms | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 4.3% | 0% |
| 0:30 | 511 | 509 | 0 | 0% | 1.7 ms | 3.4 ms | 12% | 6.4% | 81 | 8.3% | 2 | 8.3% | 81 | 3.2 ms | 2/80 | 0.0 ms | 2/80 | 0.0 ms | 4.9% | 0% |
| 0:40 | 600 | 598 | 0 | 0% | 1.7 ms | 3.1 ms | 14% | 6.7% | 81 | 9.3% | 2 | 8.9% | 81 | 5.6 ms | 2/80 | 0.0 ms | 1/80 | 0.0 ms | 5.2% | 0% |
| 0:50 | 689 | 687 | 0 | 0% | 1.7 ms | 3.2 ms | 16% | 7.5% | 81 | 11% | 2 | 10% | 81 | 5.3 ms | 2/80 | 0.0 ms | 2/80 | 0.0 ms | 5.9% | 0% |
| 1:00 | 778 | 776 | 0 | 0% | 1.7 ms | 4.0 ms | 18% | 8.3% | 81 | 11% | 2 | 11% | 81 | 354 ms | 2/80 | 0.0 ms | 1/80 | 0.0 ms | 7.1% | 0% |
| 1:10 | 867 | 865 | 0 | 0% | 1.7 ms | 3.4 ms | 20% | 8.9% | 81 | 12% | 2 | 12% | 81 | 2.8 ms | 2/80 | 0.0 ms | 2/80 | 0.0 ms | 7.6% | 0% |
| 1:20 | 956 | 953 | 0 | 0% | 1.7 ms | 4.1 ms | 22% | 9.4% | 81 | 11% | 2 | 12% | 81 | 1.01 s | 2/80 | 0.0 ms | 2/80 | 0.0 ms | 8.4% | 0% |
| 1:30 | 1,044 | 1,042 | 0 | 0% | 1.7 ms | 3.6 ms | 23% | 9.8% | 81 | 11% | 2 | 13% | 81 | 199 ms | 2/80 | 0.0 ms | 2/80 | 0.0 ms | 8.8% | 0% |
| 1:40 | 1,133 | 1,131 | 0 | 0% | 1.7 ms | 7.1 ms | 25% | 10% | 81 | 12% | 2 | 14% | 81 | 502 ms | 2/80 | 0.0 ms | 11/80 | 0.0 ms | 9.1% | 0% |
| 1:50 | 1,222 | 1,220 | 0 | 0% | 1.7 ms | 4.9 ms | 27% | 11% | 81 | 12% | 2 | 15% | 81 | 187 ms | 2/80 | 0.0 ms | 2/80 | 0.0 ms | 9.5% | 0% |
| 2:00 | 1,311 | 1,309 | 0 | 0% | 1.7 ms | 8.2 ms | 29% | 12% | 81 | 13% | 2 | 15% | 81 | 1.06 s | 3/80 | 0.0 ms | 7/80 | 0.0 ms | 11% | 0% |
| 2:10 | 1,400 | 1,398 | 0 | 0% | 1.7 ms | 36 ms | 31% | 13% | 81 | 14% | 2 | 16% | 81 | 749 ms | 3/80 | 0.0 ms | 10/80 | 0.0 ms | 11% | 0% |
| 2:20 | 1,489 | 1,487 | 0 | 0% | 1.7 ms | 52 ms | 33% | 13% | 81 | 15% | 2 | 16% | 81 | 1.45 s | 2/80 | 0.0 ms | 13/80 | 0.0 ms | 12% | 0% |
| 2:30 | 1,578 | 1,576 | 0 | 0% | 1.7 ms | 57 ms | 36% | 15% | 81 | 17% | 2 | 19% | 81 | 1.56 s | 3/80 | 0.0 ms | 14/80 | 0.0 ms | 13% | 0% |
| 2:40 | 1,667 | 1,664 | 0 | 0% | 1.8 ms | 59 ms | 39% | 20% | 81 | 19% | 2 | 18% | 81 | 2.55 s | 4/80 | 0.0 ms | 15/80 | 0.0 ms | 14% | 0% |
| 2:50 | 1,756 | 1,754 | 0 | 0% | 1.9 ms | 69 ms | 41% | 22% | 81 | 23% | 2 | 21% | 81 | 4.39 s | 4/80 | 0.0 ms | 4/80 | 0.0 ms | 15% | 0% |

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
      db: { size: 40, min: 40 }
      ro: { size: 40, min: 40 }

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
