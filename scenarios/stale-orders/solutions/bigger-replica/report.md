# stale-orders-bigger-replica: run report

_run `stale-orders-bigger-replica-20261118-010714` · completed · 18 Nov 2026 01:07 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 1,800 rps offered, 1,754 rps served at best, worst 10-second p99 329 ms, at most 0% errors.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 2:50

Measured in the 10-second window from 2:50 to 3:00 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 1,800 rps<br/>offered 1,756 rps"])
    lb{{"lb<br/>load balancer<br/>0.4 CPU · 256m<br/>CPU 21%<br/>2/2 replicas up"}}
    api["api ×2<br/>1 CPU · 1g<br/>CPU 39%<br/>in flight 235"]
    ro[("ro<br/>postgres-replica 17<br/>1 CPU · 2g<br/>disk 1 ms · 10,000 IOPS<br/>CPU 18%<br/>conns 81/100<br/>lag 100 ms")]
    db[("db<br/>postgres 17<br/>1 CPU · 2g<br/>CPU 18%<br/>conns 81/100")]
    load -->|"50% read · 50% write<br/>1,754 rps · p99 329 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>1,728 rps · p99 319 ms"| api
    api -->|"read on reads<br/>pool 40, min 40<br/>1,752 calls/s · p99 16 ms<br/>in use 16/80"| ro
    api -->|"write on writes<br/>pool 40, min 40<br/>875 calls/s · p99 363 ms<br/>in use 80/80, 141 waiting"| db
    api -.->|"read on writes<br/>pool 40, min 40<br/>1,752 calls/s · p99 16 ms<br/>in use 16/80"| ro
    db -.->|"streams WAL"| ro
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb,ro,db ok
    class api hot
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
    line [237.686, 331.243, 420.187, 509.002, 597.795, 686.833, 775.635, 864.78, 953.342, 1042.251, 1131.391, 1219.96, 1309.025, 1397.912, 1486.709, 1575.473, 1664.523, 1753.716]
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
    line [2.281, 1.89, 1.799, 1.747, 1.703, 1.685, 1.659, 1.651, 1.622, 1.609, 1.591, 1.696, 2.355, 1.863, 2.071, 2.604, 2.113, 1.76]
    line [3.797, 3.372, 3.144, 2.8, 2.485, 2.424, 2.406, 2.408, 2.394, 2.351, 2.344, 5.561, 9.756, 7.887, 9.55, 17.274, 13.943, 47.388]
    line [4.505, 3.612, 3.516, 3.364, 2.974, 2.809, 2.695, 2.762, 2.953, 2.576, 2.677, 9.123, 143.764, 12.62, 38.749, 52.163, 44.469, 328.566]
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
    line [6.174, 8.002, 9.838, 11.781, 13.668, 15.556, 17.441, 19.432, 21.351, 23.068, 25.054, 26.804, 28.92, 30.821, 32.809, 34.732, 37.425, 39.436]
    line [4.291, 4.684, 4.905, 5.616, 5.983, 6.341, 6.863, 7.7, 7.84, 8.063, 8.834, 9.198, 10.516, 10.595, 11.175, 11.844, 16.685, 18.281]
    line [5.58, 5.688, 7.269, 8.162, 9.159, 10.469, 11.233, 11.919, 9.768, 12.724, 15.472, 15.378, 13.114, 13.891, 15.346, 16.371, 18.413, 20.931]
    line [4.823, 5.304, 6.038, 6.551, 7.186, 7.804, 8.511, 9.113, 9.928, 10.019, 10.854, 10.734, 10.963, 12.202, 12.578, 12.872, 15.402, 18.027]
```

<sub>Blue: api. Orange: db. Green: lb. Purple: ro (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window).</sub>

## Diagnosis

No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version 22; thresholds below).

## Predictions

1 of 1 scored prediction held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 500 until load_rps >= 1000` | **held** | 4.5 ms | 0:00 to 1:30, when load_rps >= 1,000 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |

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
| services.ro.disk | no profile: 1 ms per read, 10000 IOPS | set in the design: every read that misses ro's memory waits this long, and reads and writes are each capped at this rate, as on a cloud volume (unset values default to about a gp3 volume's: 1 ms, 3000 IOPS) |
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
- **CPU bursts at the limit:** a stall no other rule covers and no fault made, in which a component was CPU-throttled for 3 s or more in a window (summed over its threads) while its average CPU stayed under 90% of its limit: bursts of work used up a CFS period's quota early and waited for the next period.
- **Host disk stalls:** for 2 seconds or more, a Postgres on the Docker host's own disk read no blocks and committed at most 10% of its median over the 10 s before (50 a second or more), while 5 or more queries waited on disk reads or WAL flushes and its exporter answered. Near the break it makes the run inconclusive; anywhere else it's a caveat, and its windows are flagged and left out of exports.

## Environment

- loadl dev, profile 3m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 976 (hash 919, memory 1037, syscalls 974): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (976) was within 5.0% of the run it's compared with (995)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 179,989 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-stale-orders-bigger-replica-20261118-010714-api-0-1 | loadlore/synthetic:dev |
| loadlore-stale-orders-bigger-replica-20261118-010714-api-1-1 | loadlore/synthetic:dev |
| loadlore-stale-orders-bigger-replica-20261118-010714-db-1 | postgres:17.11 |
| loadlore-stale-orders-bigger-replica-20261118-010714-lb-1 | haproxy:3.2.26 |
| loadlore-stale-orders-bigger-replica-20261118-010714-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-stale-orders-bigger-replica-20261118-010714-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-stale-orders-bigger-replica-20261118-010714-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-stale-orders-bigger-replica-20261118-010714-lore-pgexp-ro-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-stale-orders-bigger-replica-20261118-010714-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-stale-orders-bigger-replica-20261118-010714-ro-1 | postgres:17.11 |

</details>

Warnings:

- the run's memory limits add up to 9984 MiB but Docker has 9921 MiB; the host may run out of memory before a container reaches its limit

## Per-window data

<details><summary>18 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | lb CPU | lb up | ro CPU | ro conns | ro lag | ro stale | api→db pool | wait | api→ro pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 244 | 238 | 0 | 0% | 2.3 ms | 4.5 ms | 6.2% | 4.3% | 81 | 5.6% | 2 | 4.8% | 81 | 9.2 ms | 0% | 0/80 | 0.0 ms | 1/80 | 0.0 ms | 5.2% | 0% |
| 0:10 | 333 | 331 | 0 | 0% | 1.9 ms | 3.6 ms | 8.0% | 4.7% | 81 | 5.7% | 2 | 5.3% | 81 | 7.2 ms | 0% | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 3.1% | 0% |
| 0:20 | 422 | 420 | 0 | 0% | 1.8 ms | 3.5 ms | 9.8% | 4.9% | 81 | 7.3% | 2 | 6.0% | 81 | 9.9 ms | 0% | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 4.1% | 0% |
| 0:30 | 511 | 509 | 0 | 0% | 1.7 ms | 3.4 ms | 12% | 5.6% | 81 | 8.2% | 2 | 6.6% | 81 | 3.1 ms | 0% | 1/80 | 0.0 ms | 2/80 | 0.0 ms | 5.0% | 0% |
| 0:40 | 600 | 598 | 0 | 0% | 1.7 ms | 3.0 ms | 14% | 6.0% | 81 | 9.2% | 2 | 7.2% | 81 | 5.6 ms | 0% | 1/80 | 0.0 ms | 1/80 | 0.0 ms | 5.1% | 0% |
| 0:50 | 689 | 687 | 0 | 0% | 1.7 ms | 2.8 ms | 16% | 6.3% | 81 | 10% | 2 | 7.8% | 81 | 5.3 ms | <0.1% | 2/80 | 0.0 ms | 2/80 | 0.0 ms | 5.8% | 0% |
| 1:00 | 778 | 776 | 0 | 0% | 1.7 ms | 2.7 ms | 17% | 6.9% | 81 | 11% | 2 | 8.5% | 81 | 2.7 ms | <0.1% | 1/80 | 0.0 ms | 3/80 | 0.0 ms | 6.8% | 0% |
| 1:10 | 867 | 865 | 0 | 0% | 1.7 ms | 2.8 ms | 19% | 7.7% | 81 | 12% | 2 | 9.1% | 81 | 2.6 ms | 0% | 1/80 | 0.0 ms | 2/80 | 0.0 ms | 7.2% | 0% |
| 1:20 | 956 | 953 | 0 | 0% | 1.6 ms | 3.0 ms | 21% | 7.8% | 81 | 9.8% | 2 | 9.9% | 81 | 4.9 ms | 0% | 1/80 | 0.0 ms | 2/80 | 0.0 ms | 8.2% | 0% |
| 1:30 | 1,044 | 1,042 | 0 | 0% | 1.6 ms | 2.6 ms | 23% | 8.1% | 81 | 13% | 2 | 10% | 81 | 2.4 ms | 0% | 2/80 | 0.0 ms | 2/80 | 0.0 ms | 8.1% | 0% |
| 1:40 | 1,133 | 1,131 | 0 | 0% | 1.6 ms | 2.7 ms | 25% | 8.8% | 81 | 15% | 2 | 11% | 81 | 16 ms | 0% | 2/80 | 0.0 ms | 3/80 | 0.0 ms | 9.0% | 0% |
| 1:50 | 1,222 | 1,220 | 0 | 0% | 1.7 ms | 9.1 ms | 27% | 9.2% | 81 | 15% | 2 | 11% | 81 | 11 ms | 0% | 4/80 | 0.0 ms | 2/80 | 0.0 ms | 9.3% | 0% |
| 2:00 | 1,311 | 1,309 | 0 | 0% | 2.4 ms | 144 ms | 29% | 11% | 81 | 13% | 2 | 11% | 81 | 46 ms | <0.1% | 6/80 | 1.2 ms | 2/80 | 0.0 ms | 10% | 0% |
| 2:10 | 1,400 | 1,398 | 0 | 0% | 1.9 ms | 13 ms | 31% | 11% | 81 | 14% | 2 | 12% | 81 | 71 ms | <0.1% | 2/80 | 0.0 ms | 3/80 | 0.0 ms | 11% | 0% |
| 2:20 | 1,489 | 1,487 | 0 | 0% | 2.1 ms | 39 ms | 33% | 11% | 81 | 15% | 2 | 13% | 81 | 33 ms | <0.1% | 15/80 | 0.0 ms | 3/80 | 0.0 ms | 11% | 0% |
| 2:30 | 1,578 | 1,575 | 0 | 0% | 2.6 ms | 52 ms | 35% | 12% | 81 | 16% | 2 | 13% | 81 | 61 ms | 0% | 17/80 | 0.0 ms | 3/80 | 0.0 ms | 12% | 0% |
| 2:40 | 1,667 | 1,665 | 0 | 0% | 2.1 ms | 44 ms | 37% | 17% | 81 | 18% | 2 | 15% | 81 | 108 ms | <0.1% | 8/80 | 0.0 ms | 5/80 | 0.0 ms | 14% | 0% |
| 2:50 | 1,756 | 1,754 | 0 | 0% | 1.8 ms | 329 ms | 39% | 18% | 81 | 21% | 2 | 18% | 81 | 100 ms | 2.0% | 80/80 +141 | 10 ms | 16/80 | 0.0 ms | 13% | 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); a replica's lag is from pg_stat_replication_pg_wal_lsn_diff ÷ the rate of pg_stat_replication_pg_current_wal_lsn_bytes (postgres_exporter, on the primary): the WAL the replica hadn't replayed, in seconds of the primary's writing; without them, pg_replication_lag_seconds on the replica, and its stale share of reads after writes from loadlore_read_after_write_total (the synthetic service): point reads of a key the same request had just written, stale when they didn't see the write; hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 CPU is against its budget.

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
        - { target: ro, op: read, when: write, mode: async, after_ms: 200 }   # the confirmation page loads
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
    resources: { cpu: 1, memory: 2g }   # was 0.75 CPU and 512m: the primary's size
    config: { max_connections: 100, shared_buffers: 512MB }   # was 128MB
    disk: { iops: 10000 }   # was {}: provisioned IOPS, as a primary's volume has

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
