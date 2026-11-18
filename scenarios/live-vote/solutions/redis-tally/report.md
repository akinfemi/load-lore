# live-vote-redis-tally: run report

_run `live-vote-redis-tally-20261117-234527` · completed · 17 Nov 2026 23:45 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 4,500 rps offered, 4,500 rps served at best, worst 10-second p99 23 ms, at most 0% errors.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 4:20

Measured in the 10-second window from 4:20 to 4:30 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 4,500 rps<br/>offered 4,500 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 33%<br/>2/2 replicas up"}}
    api["api ×2<br/>1 CPU · 512m<br/>CPU 87%<br/>in flight 4"]
    db[("db<br/>postgres 17<br/>1 CPU · 1g<br/>CPU 5.0%<br/>conns 81/100")]
    tally[("tally<br/>redis 8<br/>0.5 CPU · 256m<br/>CPU 11%")]
    load -->|"20% read · 80% write<br/>4,500 rps · p99 2.4 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>4,500 rps · p99 1.2 ms"| api
    api -->|"read on reads<br/>pool 40, min 40<br/>907 calls/s · p99 0.9 ms<br/>in use 2/80"| db
    api -->|"set on writes<br/>pool 40<br/>3,593 calls/s · p99 0.9 ms<br/>in use 3/80"| tally
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb,db,tally ok
    class api warm
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
    y-axis "requests/s" 0 --> 5000
    line [289.583, 468.75, 647.917, 827.083, 1006.25, 1185.417, 1364.583, 1543.75, 1722.917, 1902.083, 2081.25, 2260.417, 2439.583, 2618.75, 2797.917, 2977.083, 3156.25, 3335.417, 3514.583, 3693.75, 3872.917, 4052.083, 4231.25, 4410.417, 4500, 4500, 4500, 4500, 4500, 4500]
    line [279.319, 463.802, 642.926, 822.229, 1001.326, 1180.388, 1359.779, 1538.737, 1717.989, 1897.006, 2076.588, 2255.371, 2434.481, 2614.094, 2792.71, 2972.445, 3151.359, 3330.102, 3510.218, 3688.624, 3867.682, 4047.496, 4226.194, 4405.511, 4500.064, 4499.56, 4500.361, 4499.884, 4500.001, 4499.824]
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
    y-axis "ms" 0 --> 25
    line [1.602, 1.32, 1.308, 1.304, 1.297, 1.295, 1.29, 1.282, 1.273, 1.258, 1.25, 1.239, 1.237, 1.234, 1.234, 1.227, 1.224, 1.222, 1.221, 1.218, 1.216, 1.213, 1.21, 1.212, 1.211, 1.21, 1.212, 1.21, 1.211, 1.212]
    line [1.775, 1.441, 1.38, 1.38, 1.379, 1.378, 1.378, 1.375, 1.378, 1.373, 1.373, 1.373, 1.373, 1.374, 1.376, 1.373, 1.376, 1.377, 1.38, 1.383, 1.411, 1.413, 1.418, 1.488, 1.469, 1.458, 1.483, 1.442, 1.45, 1.476]
    line [2.264, 1.504, 1.484, 1.49, 1.492, 1.485, 1.486, 1.469, 23.04, 1.466, 1.476, 1.498, 1.529, 1.584, 1.581, 1.549, 1.582, 1.692, 1.806, 1.94, 1.934, 2.128, 2.264, 2.887, 2.36, 2.227, 2.411, 1.845, 1.919, 2.149]
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
    line [7.889, 10.02, 13.634, 17.201, 20.994, 24.804, 28.401, 31.944, 36.356, 38.903, 42.104, 45.37, 48.815, 51.909, 55.384, 58.614, 62.195, 65.575, 68.615, 72.048, 75.411, 78.787, 82.093, 85.499, 87.373, 87.371, 87.367, 87.339, 87.359, 86.022]
    line [5.89, 2.996, 2.806, 2.992, 3.107, 3.141, 3.321, 3.233, 3.87, 3.487, 3.517, 3.573, 3.652, 3.914, 3.9, 3.896, 4.087, 4.246, 4.369, 4.328, 4.54, 4.728, 4.703, 5.011, 4.973, 5.096, 4.998, 4.975, 5.059, 5.115]
    line [7.398, 5.986, 4.993, 9.467, 11.511, 12.867, 14.031, 15.247, 14.289, 12.503, 14.804, 18.912, 19.001, 18.762, 21.542, 24.389, 28.357, 27.395, 26.615, 26.693, 29.913, 31.478, 30.896, 32.634, 33.859, 33.396, 32.928, 35.742, 38.163, 33.346]
    line [4.28, 2.198, 2.51, 2.921, 3.219, 3.578, 3.848, 4.204, 4.619, 4.712, 4.988, 5.386, 5.817, 5.998, 6.322, 6.664, 7.159, 7.451, 7.969, 8.433, 8.891, 9.431, 9.958, 10.342, 10.655, 10.747, 10.808, 10.529, 10.691, 10.48]
```

<sub>Blue: api. Orange: db. Green: lb. Purple: tally (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window).</sub>

## Diagnosis

No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version 22; thresholds below).

## Predictions

1 of 1 scored prediction held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 100 until load_rps >= 1000` | **held** | 2.3 ms | 0:00 to 0:40, when load_rps >= 1,000 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |

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
| services.db.disk | laptop: the Docker host's | no disk block: a read that misses db's memory goes to the Docker host's disk, which on a laptop mostly answers from the host's own memory, fast and uncapped, and a commit's fsync returns in a fraction of a millisecond. Add disk: { profile: gp3 } for a cloud volume's waits and caps |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |
| services.api.pools.db.min | 40 | set in the design: each replica opens 40 connections before the load starts and keeps them open |
| services.api.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- Loadlore's own containers get fixed budgets: load generator 2 CPU (peak 4500 rps), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each. No fault acts on a connection, so every connection is direct and there is no Toxiproxy

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

- loadl dev, profile 5m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 1003 (hash 977, memory 1037, syscalls 996): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (1003) was within 5.0% of the run it's compared with (1023)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 833,958 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-live-vote-redis-tally-20261117-234527-api-0-1 | loadlore/synthetic:dev |
| loadlore-live-vote-redis-tally-20261117-234527-api-1-1 | loadlore/synthetic:dev |
| loadlore-live-vote-redis-tally-20261117-234527-db-1 | postgres:17.11 |
| loadlore-live-vote-redis-tally-20261117-234527-lb-1 | haproxy:3.2.26 |
| loadlore-live-vote-redis-tally-20261117-234527-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-live-vote-redis-tally-20261117-234527-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-live-vote-redis-tally-20261117-234527-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-live-vote-redis-tally-20261117-234527-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-live-vote-redis-tally-20261117-234527-lore-redisexp-tally-1 | oliver006/redis_exporter:v1.97.0 |
| loadlore-live-vote-redis-tally-20261117-234527-tally-1 | redis:8.2 |

</details>

## Per-window data

<details><summary>30 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | lb CPU | lb up | tally CPU | tally hit rate | api→db pool | wait | api→tally pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 290 | 279 | 0 | 0% | 1.6 ms | 2.3 ms | 7.9% | 5.9% | 81 | 7.4% | 2 | 4.3% | – | 0/80 | 0.0 ms | 1/80 | – | 6.3% | 1.0% |
| 0:10 | 469 | 464 | 0 | 0% | 1.3 ms | 1.5 ms | 10% | 3.0% | 81 | 6.0% | 2 | 2.2% | – | 0/80 | 0.0 ms | 1/80 | – | 3.9% | 0% |
| 0:20 | 648 | 643 | 0 | 0% | 1.3 ms | 1.5 ms | 14% | 2.8% | 81 | 5.0% | 2 | 2.5% | – | 0/80 | 0.0 ms | 1/80 | – | 4.4% | 0% |
| 0:30 | 827 | 822 | 0 | 0% | 1.3 ms | 1.5 ms | 17% | 3.0% | 81 | 9.5% | 2 | 2.9% | – | 0/80 | 0.0 ms | 2/80 | – | 5.7% | 0% |
| 0:40 | 1,006 | 1,001 | 0 | 0% | 1.3 ms | 1.5 ms | 21% | 3.1% | 81 | 12% | 2 | 3.2% | – | 1/80 | 0.0 ms | 2/80 | – | 6.4% | 0% |
| 0:50 | 1,185 | 1,180 | 0 | 0% | 1.3 ms | 1.5 ms | 25% | 3.1% | 81 | 13% | 2 | 3.6% | – | 0/80 | 0.0 ms | 2/80 | – | 7.0% | 0% |
| 1:00 | 1,365 | 1,360 | 0 | 0% | 1.3 ms | 1.5 ms | 28% | 3.3% | 81 | 14% | 2 | 3.8% | – | 1/80 | 0.0 ms | 2/80 | – | 7.4% | 0% |
| 1:10 | 1,544 | 1,539 | 0 | 0% | 1.3 ms | 1.5 ms | 32% | 3.2% | 81 | 15% | 2 | 4.2% | – | 1/80 | 0.0 ms | 1/80 | – | 7.8% | 0% |
| 1:20 | 1,723 | 1,718 | 0 | 0% | 1.3 ms | 23 ms | 36% | 3.9% | 81 | 14% | 2 | 4.6% | – | 1/80 | 0.0 ms | 1/80 | – | 8.6% | 0% |
| 1:30 | 1,902 | 1,897 | 0 | 0% | 1.3 ms | 1.5 ms | 39% | 3.5% | 81 | 13% | 2 | 4.7% | – | 1/80 | 0.0 ms | 2/80 | – | 8.7% | 0% |
| 1:40 | 2,081 | 2,077 | 0 | 0% | 1.2 ms | 1.5 ms | 42% | 3.5% | 81 | 15% | 2 | 5.0% | – | 0/80 | 0.0 ms | 2/80 | – | 9.0% | 0% |
| 1:50 | 2,260 | 2,255 | 0 | 0% | 1.2 ms | 1.5 ms | 45% | 3.6% | 81 | 19% | 2 | 5.4% | – | 1/80 | 0.0 ms | 3/80 | – | 9.7% | 0% |
| 2:00 | 2,440 | 2,434 | 0 | 0% | 1.2 ms | 1.5 ms | 49% | 3.7% | 81 | 19% | 2 | 5.8% | – | 1/80 | 0.0 ms | 3/80 | – | 10% | 0% |
| 2:10 | 2,619 | 2,614 | 0 | 0% | 1.2 ms | 1.6 ms | 52% | 3.9% | 81 | 19% | 2 | 6.0% | – | 1/80 | 0.0 ms | 3/80 | – | 11% | 0% |
| 2:20 | 2,798 | 2,793 | 0 | 0% | 1.2 ms | 1.6 ms | 55% | 3.9% | 81 | 22% | 2 | 6.3% | – | 1/80 | 0.0 ms | 2/80 | – | 12% | 0% |
| 2:30 | 2,977 | 2,972 | 0 | 0% | 1.2 ms | 1.5 ms | 59% | 3.9% | 81 | 24% | 2 | 6.7% | – | 1/80 | 0.0 ms | 3/80 | – | 13% | 0% |
| 2:40 | 3,156 | 3,151 | 0 | 0% | 1.2 ms | 1.6 ms | 62% | 4.1% | 81 | 28% | 2 | 7.2% | – | 1/80 | 0.0 ms | 2/80 | – | 14% | 0% |
| 2:50 | 3,335 | 3,330 | 0 | 0% | 1.2 ms | 1.7 ms | 66% | 4.2% | 81 | 27% | 2 | 7.5% | – | 1/80 | 0.0 ms | 2/80 | – | 15% | 0% |
| 3:00 | 3,515 | 3,510 | 0 | 0% | 1.2 ms | 1.8 ms | 69% | 4.4% | 81 | 27% | 2 | 8.0% | – | 1/80 | 0.0 ms | 3/80 | – | 16% | 0% |
| 3:10 | 3,694 | 3,689 | 0 | 0% | 1.2 ms | 1.9 ms | 72% | 4.3% | 81 | 27% | 2 | 8.4% | – | 2/80 | 0.0 ms | 2/80 | – | 17% | 0% |
| 3:20 | 3,873 | 3,868 | 0 | 0% | 1.2 ms | 1.9 ms | 75% | 4.5% | 81 | 30% | 2 | 8.9% | – | 2/80 | 0.0 ms | 3/80 | – | 19% | 0% |
| 3:30 | 4,052 | 4,047 | 0 | 0% | 1.2 ms | 2.1 ms | 79% | 4.7% | 81 | 31% | 2 | 9.4% | – | 1/80 | 0.0 ms | 3/80 | – | 20% | 0% |
| 3:40 | 4,231 | 4,226 | 0 | 0% | 1.2 ms | 2.3 ms | 82% | 4.7% | 81 | 31% | 2 | 10.0% | – | 2/80 | 0.0 ms | 2/80 | – | 21% | 0% |
| 3:50 | 4,410 | 4,406 | 0 | 0% | 1.2 ms | 2.9 ms | 85% | 5.0% | 81 | 33% | 2 | 10% | – | 1/80 | 0.0 ms | 3/80 | – | 21% | 0% |
| 4:00 | 4,500 | 4,500 | 0 | 0% | 1.2 ms | 2.4 ms | 87% | 5.0% | 81 | 34% | 2 | 11% | – | 2/80 | 0.0 ms | 3/80 | – | 23% | 0% |
| 4:10 | 4,500 | 4,500 | 0 | 0% | 1.2 ms | 2.2 ms | 87% | 5.1% | 81 | 33% | 2 | 11% | – | 2/80 | 0.0 ms | 3/80 | – | 23% | 0% |
| 4:20 | 4,500 | 4,500 | 0 | 0% | 1.2 ms | 2.4 ms | 87% | 5.0% | 81 | 33% | 2 | 11% | – | 2/80 | 0.0 ms | 3/80 | – | 23% | 0% |
| 4:30 | 4,500 | 4,500 | 0 | 0% | 1.2 ms | 1.8 ms | 87% | 5.0% | 81 | 36% | 2 | 11% | – | 1/80 | 0.0 ms | 4/80 | – | 22% | 0% |
| 4:40 | 4,500 | 4,500 | 0 | 0% | 1.2 ms | 1.9 ms | 87% | 5.1% | 81 | 38% | 2 | 11% | – | 1/80 | 0.0 ms | 3/80 | – | 23% | 0% |
| 4:50 | 4,500 | 4,500 | 0 | 0% | 1.2 ms | 2.1 ms | 86% | 5.1% | 81 | 33% | 2 | 10% | – | 3/80 +1 | 0.0 ms | 3/80 | – | 18% | 0% |

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

workload:
  entry: lb
  profile:
    - ramp: { from: 200, to: 4500, over: 4m }
    - hold: { rps: 4500, for: 1m }  # the vote stays open at its peak
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
