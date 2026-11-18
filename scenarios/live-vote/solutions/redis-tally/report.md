# live-vote-redis-tally: run report

_run `live-vote-redis-tally-20261118-104206` · completed · 18 Nov 2026 10:42 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 2,000 rps offered, 2,000 rps served at best, worst 10-second p99 2.2 ms, at most 0% errors.

Caveat: at 3:50 the load generator hit its CPU limit and was stalled in 1.2% of its scheduling periods (container_cpu_cfs_throttled_periods_total ÷ container_cpu_cfs_periods_total (cAdvisor)); work caught in a stall waits for the next 100 ms period, which can show up in p99.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 4:30

Measured in the 10-second window from 4:30 to 4:40 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 2,000 rps<br/>offered 2,000 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 14%<br/>2/2 replicas up"}}
    api["api ×2<br/>1 CPU · 512m<br/>CPU 40%<br/>in flight 2"]
    db[("db<br/>postgres 17<br/>1 CPU · 1g<br/>disk gp3 · 3,000 IOPS<br/>CPU 3.4%<br/>conns 81/100")]
    tally[("tally<br/>redis 8<br/>0.5 CPU · 256m<br/>CPU 4.8%")]
    load -->|"20% read · 80% write<br/>2,000 rps · p99 1.7 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>2,000 rps · p99 1.0 ms"| api
    api -->|"read on reads<br/>pool 40, min 40<br/>401 calls/s · p99 0.5 ms<br/>in use 1/80"| db
    api -->|"set on writes<br/>pool 40<br/>1,599 calls/s · p99 0.5 ms<br/>in use 2/80"| tally
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb,api,db,tally ok
```

<sub>Sources: offered from the load profile in plan.json (offered arrival rate); load edge from k6_http_reqs_total (k6: all responses) and k6_http_req_duration_seconds (k6 histogram, end to end); CPU from container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit; calls/s from loadlore_calls_total (the caller's outbound call attempts) and their p99 from loadlore_call_duration_seconds (the caller's histogram, pool wait included); pools from loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections and loadlore_pool_pending (callers waiting for a connection); lb traffic from haproxy_backend_http_requests_total (HAProxy), replicas up from haproxy_backend_active_servers (HAProxy), and the lb edge's p99 from loadlore_request_duration_seconds (the service's own histogram); conns from pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter).</sub>

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
    x-axis "seconds" 0 --> 290
    y-axis "requests/s" 0 --> 2500
    line [237.5, 312.5, 387.5, 462.5, 537.5, 612.5, 687.5, 762.5, 837.5, 912.5, 987.5, 1062.5, 1137.5, 1212.5, 1287.5, 1362.5, 1437.5, 1512.5, 1587.5, 1662.5, 1737.5, 1812.5, 1887.5, 1962.5, 2000, 2000, 2000, 2000, 2000, 2000]
    line [230.493, 310.573, 385.479, 460.751, 535.424, 610.675, 685.485, 760.664, 835.453, 910.549, 985.595, 1060.553, 1135.563, 1210.475, 1285.607, 1360.556, 1435.494, 1510.582, 1585.505, 1660.791, 1735.333, 1810.703, 1885.38, 1960.614, 2000.002, 2000.037, 1999.906, 2000.175, 2000.078, 1999.837]
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
    x-axis "seconds" 0 --> 290
    y-axis "ms" 0 --> 2.5
    line [1.59, 1.329, 1.318, 1.311, 1.311, 1.304, 1.304, 1.299, 1.301, 1.304, 1.301, 1.301, 1.297, 1.297, 1.295, 1.292, 1.288, 1.3, 1.358, 1.355, 1.349, 1.346, 1.343, 1.341, 1.34, 1.341, 1.345, 1.342, 1.342, 1.34]
    line [1.764, 1.544, 1.396, 1.38, 1.38, 1.378, 1.379, 1.377, 1.377, 1.378, 1.378, 1.378, 1.377, 1.377, 1.377, 1.377, 1.376, 1.446, 1.496, 1.497, 1.493, 1.494, 1.491, 1.493, 1.491, 1.492, 1.496, 1.494, 1.495, 1.493]
    line [2.188, 1.64, 1.495, 1.525, 1.5, 1.491, 1.495, 1.486, 1.469, 1.484, 1.484, 1.492, 1.471, 1.481, 1.491, 1.498, 1.47, 1.579, 1.708, 1.661, 1.601, 1.742, 1.616, 1.637, 1.618, 1.651, 1.77, 1.703, 1.839, 1.753]
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
    x-axis "seconds" 0 --> 290
    y-axis "% of requests" 0 --> 1
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
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
    x-axis "seconds" 0 --> 290
    y-axis "% of limit" 0 --> 100
    line [5.684, 6.996, 8.397, 9.87, 11.38, 12.888, 14.388, 15.821, 17.364, 18.98, 20.516, 22.092, 23.625, 25.159, 26.665, 28.262, 29.683, 31.165, 32.765, 34.157, 35.63, 36.827, 38.221, 39.629, 40.259, 40.364, 40.264, 40.219, 40.375, 39.837]
    line [2.696, 2.831, 2.599, 2.739, 2.748, 2.916, 2.79, 2.87, 2.887, 2.95, 2.955, 2.99, 3.042, 3.087, 3.061, 3.077, 3.22, 3.2, 3.264, 3.305, 3.386, 3.439, 3.248, 3.517, 3.42, 3.445, 3.485, 3.401, 3.521, 3.484]
    line [5.655, 4.357, 5.008, 5.357, 6.455, 6.469, 7.293, 7.716, 8.868, 10.458, 11.075, 11.717, 11.942, 12.415, 12.966, 10.333, 11.221, 14.443, 12.514, 14.9, 14.143, 13.404, 14.215, 16.399, 14.002, 16.211, 13.702, 14.083, 15.37, 15.226]
    line [1.806, 1.999, 2.092, 2.217, 2.251, 2.461, 2.553, 2.742, 2.907, 3.034, 3.226, 3.327, 3.487, 3.424, 3.688, 3.766, 3.845, 3.989, 4.262, 4.192, 4.384, 4.556, 4.655, 4.671, 4.695, 4.715, 4.765, 4.819, 4.73, 4.993]
```

<sub>Blue: api. Orange: db. Green: lb. Purple: tally (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window).</sub>

## Diagnosis

No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version 23; thresholds below).

## Predictions

1 of 1 scored prediction held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 100 until load_rps >= 1000` | **held** | 2.2 ms | 0:00 to 1:50, when load_rps >= 1,000 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |

Loads and request rates depend on this machine: slower cores break the same design at a lower load, so a load guess can miss even when the reasoning behind it was right.

## Assumptions

### From the design

| Field | Value | Basis | Why |
| --- | --- | --- | --- |
| `services.api.behavior.cpu_ms` |  | guessed | a small JSON handler |
| `workload.mix` |  | guessed | during the vote, most requests are votes |
| `workload.keys` |  | guessed | a clear favourite takes nine votes in ten |

3 of these are guessed, not measured: the result depends on them.

### Chosen by Loadlore

| Field | Value | Why |
| --- | --- | --- |
| services.tally.config.maxmemory | 192m | 75% of the container's 256m memory limit, as managed Redis usually does; the rest is for fragmentation and client buffers. Set maxmemory: 0 for stock Redis (no cap) |
| services.tally.config.maxmemory-policy | allkeys-lru | evict the least recently used keys when the cache is full, as managed Redis usually does |
| workload.timeout_ms | 10s | the default; k6 gives up on a request after this long and counts it as failed |
| workload.slo.p99_ms | 500 ms | the default; a break on p99 alone needs p99 at or above this as well as 10x the low-load p99, so latency users would still accept isn't called a break |
| network.profile | same-zone: 0.3 ms round trip | the default: every service in one availability zone. AWS says a round trip between two instances in the same zone is generally sub-millisecond with enhanced networking, and publishes no exact figure, so the 0.3 ms round trip Loadlore adds between any two containers is its own estimate, inside that bound. Applied between any two of the stack's containers, the load generator and the entry point included, as half the round trip on each side's outgoing packets; Loadlore's own tooling isn't slowed. Not scaled with --scale. Source: AWS Architecture Blog, Improving performance and reducing cost using Availability Zone Affinity: https://aws.amazon.com/blogs/architecture/improving-performance-and-reducing-cost-using-availability-zone-affinity/ |
| resources.cpu_period_ms | 100 ms for api, db, lb, tally | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.db.disk | gp3: 1 ms per read, 1 ms per write, 3000 IOPS, 125 MiB/s | a gp3 volume at its baseline: 3,000 IOPS and 125 MiB/s, included in its price, and up to 80,000 IOPS provisioned. AWS publishes no exact latency, only single-digit milliseconds, so the 1 ms Loadlore adds to every read, write and fsync is its own estimate: the low end of that range, so the disk is flattered rather than handicapped. Reads and writes are capped separately, so a mixed load can reach up to twice the IOPS. Provisioned throughput isn't modelled. Source: AWS, Amazon EBS General Purpose SSD volumes: https://docs.aws.amazon.com/ebs/latest/userguide/general-purpose.html |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |
| services.api.pools.db.min | 40 | set in the design: each replica opens 40 connections before the load starts and keeps them open |
| services.api.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- load generator: 2250 VUs started before the load, the scenario's measured prealloc_vus 2250 at --scale 1, more than 2000 rps × the SLO's 500 ms
- Loadlore's own containers get fixed budgets: load generator 1.5 CPU (peak 2000 rps), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each. No fault acts on a connection, so every connection is direct and there is no Toxiproxy

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
- **Host disk stalls:** for 2 seconds or more, a Postgres read no blocks and committed at most 10% of its median over the 10 s before (50 a second or more), while 5 or more queries waited on disk reads or WAL flushes and its exporter answered; on a modelled disk it had also been reading 20 blocks a second or more (median), which its caps can't stop for seconds; not under a disk_slow fault on it. Near the break it makes the run inconclusive; anywhere else it's a caveat, and its windows are flagged and left out of exports.

## Environment

- loadl dev, profile 5m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 983 (hash 926, memory 1021, syscalls 1006): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (983) was within 5.0% of the run it's compared with (1033)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 383,985 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-live-vote-redis-tally-20261118-104206-api-0-1 | loadlore/synthetic:dev |
| loadlore-live-vote-redis-tally-20261118-104206-api-1-1 | loadlore/synthetic:dev |
| loadlore-live-vote-redis-tally-20261118-104206-db-1 | postgres:17.11 |
| loadlore-live-vote-redis-tally-20261118-104206-lb-1 | haproxy:3.2.26 |
| loadlore-live-vote-redis-tally-20261118-104206-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-live-vote-redis-tally-20261118-104206-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-live-vote-redis-tally-20261118-104206-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-live-vote-redis-tally-20261118-104206-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-live-vote-redis-tally-20261118-104206-lore-redisexp-tally-1 | oliver006/redis_exporter:v1.97.0 |
| loadlore-live-vote-redis-tally-20261118-104206-tally-1 | redis:8.2 |

</details>

## Per-window data

<details><summary>30 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | lb CPU | lb up | tally CPU | tally hit rate | api→db pool | wait | api→tally pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 238 | 230 | 0 | 0% | 1.6 ms | 2.2 ms | 5.7% | 2.7% | 81 | 5.7% | 2 | 1.8% | – | 0/80 | 0.0 ms | 1/80 | – | 7.6% | 1.0% |
| 0:10 | 313 | 311 | 0 | 0% | 1.3 ms | 1.6 ms | 7.0% | 2.8% | 82 | 4.4% | 2 | 2.0% | – | 0/80 | 0.0 ms | 2/80 | – | 3.4% | 0% |
| 0:20 | 388 | 385 | 0 | 0% | 1.3 ms | 1.5 ms | 8.4% | 2.6% | 81 | 5.0% | 2 | 2.1% | – | 0/80 | 0.0 ms | 0/80 | – | 4.6% | 0% |
| 0:30 | 463 | 461 | 0 | 0% | 1.3 ms | 1.5 ms | 9.9% | 2.7% | 81 | 5.4% | 2 | 2.2% | – | 0/80 | 0.0 ms | 1/80 | – | 4.7% | 0% |
| 0:40 | 538 | 535 | 0 | 0% | 1.3 ms | 1.5 ms | 11% | 2.7% | 81 | 6.5% | 2 | 2.3% | – | 1/80 | 0.0 ms | 1/80 | – | 5.7% | 1.0% |
| 0:50 | 613 | 611 | 0 | 0% | 1.3 ms | 1.5 ms | 13% | 2.9% | 81 | 6.5% | 2 | 2.5% | – | 0/80 | 0.0 ms | 2/80 | – | 5.2% | 0% |
| 1:00 | 688 | 685 | 0 | 0% | 1.3 ms | 1.5 ms | 14% | 2.8% | 81 | 7.3% | 2 | 2.6% | – | 0/80 | 0.0 ms | 1/80 | – | 6.7% | 0% |
| 1:10 | 763 | 761 | 0 | 0% | 1.3 ms | 1.5 ms | 16% | 2.9% | 81 | 7.7% | 2 | 2.7% | – | 0/80 | 0.0 ms | 1/80 | – | 6.7% | 1.0% |
| 1:20 | 838 | 835 | 0 | 0% | 1.3 ms | 1.5 ms | 17% | 2.9% | 81 | 8.9% | 2 | 2.9% | – | 1/80 | 0.0 ms | 1/80 | – | 6.4% | 0% |
| 1:30 | 913 | 911 | 0 | 0% | 1.3 ms | 1.5 ms | 19% | 3.0% | 81 | 10% | 2 | 3.0% | – | 0/80 | 0.0 ms | 1/80 | – | 8.2% | 1.0% |
| 1:40 | 988 | 986 | 0 | 0% | 1.3 ms | 1.5 ms | 21% | 3.0% | 81 | 11% | 2 | 3.2% | – | 1/80 | 0.0 ms | 2/80 | – | 8.5% | 0% |
| 1:50 | 1,063 | 1,061 | 0 | 0% | 1.3 ms | 1.5 ms | 22% | 3.0% | 81 | 12% | 2 | 3.3% | – | 1/80 | 0.0 ms | 1/80 | – | 8.8% | 0% |
| 2:00 | 1,138 | 1,136 | 0 | 0% | 1.3 ms | 1.5 ms | 24% | 3.0% | 81 | 12% | 2 | 3.5% | – | 0/80 | 0.0 ms | 1/80 | – | 8.2% | 0% |
| 2:10 | 1,213 | 1,210 | 0 | 0% | 1.3 ms | 1.5 ms | 25% | 3.1% | 81 | 12% | 2 | 3.4% | – | 1/80 | 0.0 ms | 1/80 | – | 9.2% | 0% |
| 2:20 | 1,288 | 1,286 | 0 | 0% | 1.3 ms | 1.5 ms | 27% | 3.1% | 81 | 13% | 2 | 3.7% | – | 0/80 | 0.0 ms | 1/80 | – | 9.8% | 0% |
| 2:30 | 1,363 | 1,361 | 0 | 0% | 1.3 ms | 1.5 ms | 28% | 3.1% | 81 | 10% | 2 | 3.8% | – | 1/80 | 0.0 ms | 1/80 | – | 9.6% | 1.0% |
| 2:40 | 1,438 | 1,435 | 0 | 0% | 1.3 ms | 1.5 ms | 30% | 3.2% | 81 | 11% | 2 | 3.8% | – | 1/80 | 0.0 ms | 1/80 | – | 9.9% | 0% |
| 2:50 | 1,513 | 1,511 | 0 | 0% | 1.3 ms | 1.6 ms | 31% | 3.2% | 81 | 14% | 2 | 4.0% | – | 1/80 | 0.0 ms | 2/80 | – | 10% | 0% |
| 3:00 | 1,588 | 1,586 | 0 | 0% | 1.4 ms | 1.7 ms | 33% | 3.3% | 81 | 13% | 2 | 4.3% | – | 0/80 | 0.0 ms | 2/80 | – | 12% | 0% |
| 3:10 | 1,663 | 1,661 | 0 | 0% | 1.4 ms | 1.7 ms | 34% | 3.3% | 81 | 15% | 2 | 4.2% | – | 0/80 | 0.0 ms | 2/80 | – | 11% | 0% |
| 3:20 | 1,738 | 1,735 | 0 | 0% | 1.3 ms | 1.6 ms | 36% | 3.4% | 81 | 14% | 2 | 4.4% | – | 1/80 | 0.0 ms | 1/80 | – | 11% | 0% |
| 3:30 | 1,813 | 1,811 | 0 | 0% | 1.3 ms | 1.7 ms | 37% | 3.4% | 81 | 13% | 2 | 4.6% | – | 1/80 | 0.0 ms | 1/80 | – | 11% | 1.0% |
| 3:40 | 1,888 | 1,885 | 0 | 0% | 1.3 ms | 1.6 ms | 38% | 3.2% | 81 | 14% | 2 | 4.7% | – | 1/80 | 0.0 ms | 2/80 | – | 12% | 0.8% |
| 3:50 | 1,963 | 1,961 | 0 | 0% | 1.3 ms | 1.6 ms | 40% | 3.5% | 81 | 16% | 2 | 4.7% | – | 2/80 | 0.0 ms | 3/80 | – | 12% | 1.2% |
| 4:00 | 2,000 | 2,000 | 0 | 0% | 1.3 ms | 1.6 ms | 40% | 3.4% | 81 | 14% | 2 | 4.7% | – | 1/80 | 0.0 ms | 3/80 | – | 12% | 0% |
| 4:10 | 2,000 | 2,000 | 0 | 0% | 1.3 ms | 1.7 ms | 40% | 3.4% | 81 | 16% | 2 | 4.7% | – | 1/80 | 0.0 ms | 3/80 | – | 13% | 1.0% |
| 4:20 | 2,000 | 2,000 | 0 | 0% | 1.3 ms | 1.8 ms | 40% | 3.5% | 81 | 14% | 2 | 4.8% | – | 1/80 | 0.0 ms | 2/80 | – | 11% | 1.0% |
| 4:30 | 2,000 | 2,000 | 0 | 0% | 1.3 ms | 1.7 ms | 40% | 3.4% | 81 | 14% | 2 | 4.8% | – | 1/80 | 0.0 ms | 2/80 | – | 12% | 1.0% |
| 4:40 | 2,000 | 2,000 | 0 | 0% | 1.3 ms | 1.8 ms | 40% | 3.5% | 81 | 15% | 2 | 4.7% | – | 1/80 | 0.0 ms | 3/80 | – | 12% | 1.0% |
| 4:50 | 2,000 | 2,000 | 0 | 0% | 1.3 ms | 1.8 ms | 40% | 3.5% | 81 | 15% | 2 | 5.0% | – | 2/80 | 0.0 ms | 2/80 | – | 11% | 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 CPU is against its budget.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: live-vote-redis-tally

# The votes are counted in Redis: one INCR per vote on the contestant's
# key. A job copies the totals into Postgres every few seconds, which this
# design leaves out (ten writes every few seconds).
#
# A TV talent show's live vote. During the final, viewers vote in the app:
# each vote adds one to the contestant's tally, a row in Postgres, and the
# results screen reads the tallies back. Ten contestants, and nine votes in
# ten go to the favourite.

services:
  lb:
    kind: lb
    target: api
    health_check: { interval_ms: 2000, fall: 3, rise: 2 }

  api:
    image: loadlore/synthetic
    replicas: 2
    resources: { cpu: 1, memory: 512m }
    behavior:
      cpu_ms: 0.3
      calls:
        - { target: db, op: read, when: read }    # the results screen reads a tally
        - { target: tally, op: set, when: write }  # a vote: INCR on the contestant's key in Redis
      resilience:
        timeout_ms: 2000
    pools:
      tally: 40
      db: { size: 40, min: 40 }   # opened at start; 2 replicas x 40 = 80 connections, under max_connections

  tally:
    kind: redis
    resources: { cpu: 0.5, memory: 256m }

  db:
    kind: postgres
    version: "17"
    resources: { cpu: 1, memory: 1g }
    config: { max_connections: 100, shared_buffers: 256MB }
    dataset: { rows: 1000, row_bytes: 256 }
    disk: { profile: gp3 }

workload:
  entry: lb
  profile:
    - ramp: { from: 200, to: 2000, over: 4m }
    - hold: { rps: 2000, for: 1m }  # the vote stays open at its peak
  mix: { read: 0.2, write: 0.8 }
  keys: { distribution: hotspot, share: 0.9, fraction: 0.1, count: 10 }

assumptions:
  - { field: services.api.behavior.cpu_ms, basis: guessed, why: "a small JSON handler" }
  - { field: workload.mix, basis: guessed, why: "during the vote, most requests are votes" }
  - { field: workload.keys, basis: guessed, why: "a clear favourite takes nine votes in ten" }

# Your predictions, scored in the report. The line here is an example;
# loadl predict design.yaml replaces it with your answers.
expect:
  - p99_ms < 100 until load_rps >= 1000
```
