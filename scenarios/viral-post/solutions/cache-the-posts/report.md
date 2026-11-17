# viral-post-cache-the-posts: run report

_run `viral-post-cache-the-posts-20261117-142200` · completed · 17 Nov 2026 14:22 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 2,900 rps offered, 2,901 rps served at best, worst 10-second p99 521 ms, at most 0% errors.

Caveat: at 4:10 the load generator hit its CPU limit and was stalled in 3.0% of its scheduling periods (container_cpu_cfs_throttled_periods_total ÷ container_cpu_cfs_periods_total (cAdvisor)); work caught in a stall waits for the next 100 ms period, which can show up in p99.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 4:00

Measured in the 10-second window from 4:00 to 4:10 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 2,900 rps<br/>offered 2,900 rps"])
    lb{{"lb<br/>load balancer<br/>0.4 CPU · 256m<br/>CPU 33%<br/>2/2 replicas up"}}
    api["api ×2<br/>0.75 CPU · 512m<br/>CPU 44%<br/>in flight 6"]
    cache[("cache<br/>redis 8<br/>0.2 CPU · 256m<br/>CPU 26%<br/>hit rate 63%")]
    db0[("db0<br/>postgres 17<br/>0.4 CPU · 1g<br/>disk gp3 · 3,000 IOPS<br/>CPU 28%<br/>conns 41/100")]
    db1[("db1<br/>postgres 17<br/>0.4 CPU · 1g<br/>disk gp3 · 3,000 IOPS<br/>CPU 28%<br/>conns 41/100")]
    load -->|"100% read · 0% write<br/>2,901 rps · p99 4.2 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>2,900 rps · p99 4.5 ms"| api
    api -->|"get, set on miss<br/>pool 20<br/>3,977 calls/s · p99 0.5 ms<br/>in use 2/40"| cache
    api -->|"read ×7 on miss<br/>shard 1 of 2<br/>pool 20, min 20<br/>3,711 calls/s · p99 0.5 ms<br/>in use 2/40"| db0
    api -->|"read ×7 on miss<br/>shard 2 of 2<br/>pool 20, min 20<br/>3,827 calls/s · p99 0.5 ms<br/>in use 3/40"| db1
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb,api,cache,db0,db1 ok
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
      plotColorPalette: "#f4cccc, #9e9e9e, #1f77b4"
---
xychart-beta
    title "Throughput"
    x-axis "seconds" 0 --> 290
    y-axis "requests/s" 0 --> 4000
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4000, 0, 0, 0, 0]
    line [275, 425, 575, 725, 875, 1025, 1175, 1325, 1475, 1625, 1775, 1925, 2075, 2225, 2375, 2525, 2675, 2825, 2900, 2900, 2900, 2900, 2900, 2900, 2900, 2900, 2900, 2900, 2900, 2900]
    line [266.081, 421.2, 571.109, 721.165, 871.243, 1021.183, 1170.962, 1321.005, 1471.462, 1621.307, 1770.943, 1921.177, 2071.348, 2221.125, 2371.047, 2521.054, 2671.004, 2821.091, 2900.228, 2900.203, 2899.804, 2899.72, 2900.276, 2899.429, 2900.582, 2899.924, 2900.271, 2899.617, 2900.174, 2899.921]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 4:10 stall.</sub>

```mermaid
---
config:
  themeVariables:
    xyChart:
      backgroundColor: "transparent"
      plotColorPalette: "#f4cccc, #2ca02c, #ff7f0e, #d62728"
---
xychart-beta
    title "Latency, end to end"
    x-axis "seconds" 0 --> 290
    y-axis "ms" 0 --> 600
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 600, 0, 0, 0, 0]
    line [1.359, 1.102, 1.063, 1.053, 1.05, 1.043, 1.037, 1.034, 1.028, 1.026, 1.022, 1.019, 1.011, 1.008, 1.009, 1.009, 1.013, 1.014, 1.015, 1.019, 1.022, 1.024, 1.025, 1.024, 1.025, 1.191, 1.022, 1.027, 1.031, 1.034]
    line [6.246, 5.658, 4.979, 4.704, 4.61, 4.568, 4.495, 4.484, 4.431, 4.387, 4.382, 4.301, 4.221, 4.135, 4.019, 3.857, 3.782, 3.732, 3.646, 3.625, 3.624, 3.627, 3.63, 3.604, 3.613, 98.751, 3.681, 3.655, 3.701, 3.724]
    line [6.788, 5.966, 5.682, 5.039, 5, 4.986, 4.929, 4.941, 4.909, 4.79, 4.824, 4.761, 4.627, 4.598, 4.611, 4.521, 4.516, 4.53, 4.29, 4.233, 4.217, 4.181, 4.168, 4.197, 4.169, 521.484, 4.065, 4.014, 4.022, 4.09]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 4:10 stall.</sub>

```mermaid
---
config:
  themeVariables:
    xyChart:
      backgroundColor: "transparent"
      plotColorPalette: "#f4cccc, #d62728"
---
xychart-beta
    title "Errors"
    x-axis "seconds" 0 --> 290
    y-axis "% of requests" 0 --> 1
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0]
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 4:10 stall.</sub>

```mermaid
---
config:
  themeVariables:
    xyChart:
      backgroundColor: "transparent"
      plotColorPalette: "#f4cccc, #1f77b4, #ff7f0e, #2ca02c, #9467bd, #8c564b"
---
xychart-beta
    title "CPU against each limit"
    x-axis "seconds" 0 --> 290
    y-axis "% of limit" 0 --> 100
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0]
    line [6.049, 7.45, 9.547, 11.701, 14.069, 16.244, 18.176, 20.635, 22.853, 25.142, 27.312, 29.283, 31.11, 33.103, 34.993, 37.12, 39.218, 41.866, 42.998, 43.352, 43.655, 43.831, 44.165, 44.166, 43.999, 53.038, 42.557, 42.197, 44.454, 45.885]
    line [11.716, 7.097, 7.929, 9.416, 10.509, 11.521, 12.354, 13.426, 14.579, 15.592, 16.882, 17.822, 18.703, 19.78, 21.018, 22.324, 23.344, 25.299, 25.838, 25.68, 26.027, 25.722, 26.516, 26.338, 26.295, 40.036, 27.982, 26.615, 27.502, 27.917]
    line [8.149, 10.211, 10.586, 11.919, 12.908, 14.472, 15.487, 16.635, 17.853, 19.467, 21.045, 21.071, 22.611, 23.863, 24.436, 25.331, 27.059, 28.417, 28.713, 28.755, 28.19, 28.886, 28.927, 28.841, 27.884, 49.817, 30.582, 29.193, 30.396, 31.359]
    line [8.687, 9.636, 10.864, 11.498, 13.117, 14.316, 15.157, 16.887, 18.159, 18.824, 20.079, 21.65, 22.063, 22.766, 24.387, 25.853, 26.281, 28.44, 28.707, 28.447, 29.108, 28.371, 28.652, 28.89, 28.341, 45.721, 30.963, 29.963, 29.825, 30.844]
    line [7.617, 6.522, 7.863, 9.886, 11.721, 12.725, 14.366, 16.231, 17.409, 18.436, 20.992, 21.891, 22.749, 24.976, 22.33, 25.657, 29.622, 31.705, 32.564, 31.421, 31.755, 33.75, 34.487, 32.668, 32.666, 41.641, 26.952, 26.214, 27.285, 31.966]
```

<sub>Blue: api. Orange: cache. Green: db0. Purple: db1. Brown: lb (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 4:10 stall.</sub>

## Diagnosis

No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version 19; thresholds below).

## Predictions

1 of 1 scored prediction held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 100 until load_rps >= 1000` | **held** | 6.8 ms | 0:00 to 0:50, when load_rps >= 1,000 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |

Loads and request rates depend on this machine: slower cores break the same design at a lower load, so a load guess can miss even when the reasoning behind it was right.

## Timeline

| Time | Event |
| --- | --- |
| 4:10 | stall: p99 reached 521 ms (5.8 ms at low load) for 10 s, then recovered; too short to count as a break |

<sub>From events.jsonl: the fault timeline and Docker container events. Stalls are from the per-window numbers below, judged by the thresholds.</sub>

## Assumptions

### From the design

| Field | Value | Basis | Why |
| --- | --- | --- | --- |
| `services.api.behavior.cpu_ms` |  | guessed | a thin handler: the page is mostly its reads |
| `services.api.behavior.calls` |  | guessed | a post page reads the post, its author, its counts and four top comments |
| `workload.keys` |  | guessed | a celebrity's post going viral: six views in ten are of it. No published figure; the hints and the variant's README show how the answer moves with this share |
| `workload.mix` |  | guessed | page views only; likes and comments are left out to keep one lesson |

4 of these are guessed, not measured: the result depends on them.

### Chosen by Loadlore

| Field | Value | Why |
| --- | --- | --- |
| services.cache.config.maxmemory | 192m | 75% of the container's 256m memory limit, as managed Redis usually does; the rest is for fragmentation and client buffers. Set maxmemory: 0 for stock Redis (no cap) |
| services.cache.config.maxmemory-policy | allkeys-lru | evict the least recently used keys when the cache is full, as managed Redis usually does |
| workload.timeout_ms | 10s | the default; k6 gives up on a request after this long and counts it as failed |
| workload.slo.p99_ms | 500 ms | the default; a break on p99 alone needs p99 at or above this as well as 10x the low-load p99, so latency users would still accept isn't called a break |
| network.profile | same-zone: 0.3 ms round trip | the default: every service in one availability zone. AWS says a round trip between two instances in the same zone is generally sub-millisecond with enhanced networking, and publishes no exact figure, so the 0.3 ms round trip Loadlore adds between any two containers is its own estimate, inside that bound. Applied between any two of the stack's containers, the load generator and the entry point included, as half the round trip on each side's outgoing packets; Loadlore's own tooling isn't slowed. Not scaled with --scale. Source: AWS Architecture Blog, Improving performance and reducing cost using Availability Zone Affinity: https://aws.amazon.com/blogs/architecture/improving-performance-and-reducing-cost-using-availability-zone-affinity/ |
| resources.cpu_period_ms | 100 ms for api, cache, db0, db1, lb | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.db0.disk | gp3: 1 ms per read, 1 ms per write, 3000 IOPS, 125 MiB/s | a gp3 volume at its baseline: 3,000 IOPS and 125 MiB/s, included in its price, and up to 80,000 IOPS provisioned. AWS publishes no exact latency, only single-digit milliseconds, so the 1 ms Loadlore adds to every read, write and fsync is its own estimate: the low end of that range, so the disk is flattered rather than handicapped. Reads and writes are capped separately, so a mixed load can reach up to twice the IOPS. Provisioned throughput isn't modelled. Source: AWS, Amazon EBS General Purpose SSD volumes: https://docs.aws.amazon.com/ebs/latest/userguide/general-purpose.html |
| services.db1.disk | gp3: 1 ms per read, 1 ms per write, 3000 IOPS, 125 MiB/s | a gp3 volume at its baseline: 3,000 IOPS and 125 MiB/s, included in its price, and up to 80,000 IOPS provisioned. AWS publishes no exact latency, only single-digit milliseconds, so the 1 ms Loadlore adds to every read, write and fsync is its own estimate: the low end of that range, so the disk is flattered rather than handicapped. Reads and writes are capped separately, so a mixed load can reach up to twice the IOPS. Provisioned throughput isn't modelled. Source: AWS, Amazon EBS General Purpose SSD volumes: https://docs.aws.amazon.com/ebs/latest/userguide/general-purpose.html |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |
| services.api.pools.db0.min | 20 | set in the design: each replica opens 20 connections before the load starts and keeps them open |
| services.api.pools.db0.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |
| services.api.pools.db1.min | 20 | set in the design: each replica opens 20 connections before the load starts and keeps them open |
| services.api.pools.db1.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- Loadlore's own containers get fixed budgets: load generator 1.5 CPU (peak 2900 rps), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each. No fault acts on a connection, so every connection is direct and there is no Toxiproxy

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

- loadl dev, profile 5m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 997 (hash 957, memory 1040, syscalls 996): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (997) was within 5.0% of the run it's compared with (959)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 626,986 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-viral-post-cache-the-posts-20261117-142200-api-0-1 | loadlore/synthetic:dev |
| loadlore-viral-post-cache-the-posts-20261117-142200-api-1-1 | loadlore/synthetic:dev |
| loadlore-viral-post-cache-the-posts-20261117-142200-cache-1 | redis:8.2 |
| loadlore-viral-post-cache-the-posts-20261117-142200-db0-1 | postgres:17.11 |
| loadlore-viral-post-cache-the-posts-20261117-142200-db1-1 | postgres:17.11 |
| loadlore-viral-post-cache-the-posts-20261117-142200-lb-1 | haproxy:3.2.26 |
| loadlore-viral-post-cache-the-posts-20261117-142200-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-viral-post-cache-the-posts-20261117-142200-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-viral-post-cache-the-posts-20261117-142200-lore-pgexp-db0-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-viral-post-cache-the-posts-20261117-142200-lore-pgexp-db1-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-viral-post-cache-the-posts-20261117-142200-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-viral-post-cache-the-posts-20261117-142200-lore-redisexp-cache-1 | oliver006/redis_exporter:v1.97.0 |

</details>

## Per-window data

<details><summary>30 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | cache CPU | cache hit rate | db0 CPU | db0 conns | db1 CPU | db1 conns | lb CPU | lb up | api→cache pool | wait | api→db0 pool | wait | api→db1 pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 275 | 266 | 0 | 0% | 1.4 ms | 6.8 ms | 6.0% | 12% | 61% | 8.1% | 41 | 8.7% | 41 | 7.6% | 2 | 1/40 | – | 1/40 | 0.0 ms | 1/40 | 0.0 ms | 6.2% | 1.0% |
| 0:10 | 425 | 421 | 0 | 0% | 1.1 ms | 6.0 ms | 7.4% | 7.1% | 59% | 10% | 41 | 9.6% | 42 | 6.5% | 2 | 0/40 | – | 1/40 | 0.0 ms | 2/40 | 0.0 ms | 3.7% | 0% |
| 0:20 | 575 | 571 | 0 | 0% | 1.1 ms | 5.7 ms | 9.5% | 7.9% | 59% | 11% | 41 | 11% | 41 | 7.9% | 2 | 1/40 | – | 0/40 | 0.0 ms | 1/40 | 0.0 ms | 5.3% | 0% |
| 0:30 | 725 | 721 | 0 | 0% | 1.1 ms | 5.0 ms | 12% | 9.4% | 61% | 12% | 41 | 11% | 41 | 9.9% | 2 | 2/40 | – | 2/40 | 0.0 ms | 1/40 | 0.0 ms | 6.1% | 0% |
| 0:40 | 875 | 871 | 0 | 0% | 1.1 ms | 5.0 ms | 14% | 11% | 60% | 13% | 41 | 13% | 41 | 12% | 2 | 1/40 | – | 1/40 | 0.0 ms | 2/40 | 0.0 ms | 7.6% | 0% |
| 0:50 | 1,025 | 1,021 | 0 | 0% | 1.0 ms | 5.0 ms | 16% | 12% | 61% | 14% | 41 | 14% | 41 | 13% | 2 | 2/40 | – | 1/40 | 0.0 ms | 2/40 | 0.0 ms | 7.2% | 0% |
| 1:00 | 1,175 | 1,171 | 0 | 0% | 1.0 ms | 4.9 ms | 18% | 12% | 61% | 15% | 41 | 15% | 41 | 14% | 2 | 1/40 | – | 2/40 | 0.0 ms | 2/40 | 0.0 ms | 7.5% | 0% |
| 1:10 | 1,325 | 1,321 | 0 | 0% | 1.0 ms | 4.9 ms | 21% | 13% | 61% | 17% | 41 | 17% | 41 | 16% | 2 | 1/40 | – | 2/40 | 0.0 ms | 2/40 | 0.0 ms | 8.3% | 0% |
| 1:20 | 1,475 | 1,471 | 0 | 0% | 1.0 ms | 4.9 ms | 23% | 15% | 61% | 18% | 41 | 18% | 41 | 17% | 2 | 2/40 | – | 1/40 | 0.0 ms | 3/40 | 0.0 ms | 9.6% | 0% |
| 1:30 | 1,625 | 1,621 | 0 | 0% | 1.0 ms | 4.8 ms | 25% | 16% | 61% | 19% | 41 | 19% | 41 | 18% | 2 | 2/40 | – | 3/40 | 0.0 ms | 2/40 | 0.0 ms | 10% | 0% |
| 1:40 | 1,775 | 1,771 | 0 | 0% | 1.0 ms | 4.8 ms | 27% | 17% | 61% | 21% | 41 | 20% | 41 | 21% | 2 | 2/40 | – | 5/40 | 0.0 ms | 2/40 | 0.0 ms | 11% | 0% |
| 1:50 | 1,925 | 1,921 | 0 | 0% | 1.0 ms | 4.8 ms | 29% | 18% | 61% | 21% | 41 | 22% | 41 | 22% | 2 | 2/40 | – | 2/40 | 0.0 ms | 2/40 | 0.0 ms | 12% | 0% |
| 2:00 | 2,075 | 2,071 | 0 | 0% | 1.0 ms | 4.6 ms | 31% | 19% | 61% | 23% | 41 | 22% | 41 | 23% | 2 | 2/40 | – | 3/40 | 0.0 ms | 2/40 | 0.0 ms | 13% | 0% |
| 2:10 | 2,225 | 2,221 | 0 | 0% | 1.0 ms | 4.6 ms | 33% | 20% | 62% | 24% | 41 | 23% | 41 | 25% | 2 | 2/40 | – | 4/40 | 0.0 ms | 2/40 | 0.0 ms | 13% | 0% |
| 2:20 | 2,375 | 2,371 | 0 | 0% | 1.0 ms | 4.6 ms | 35% | 21% | 62% | 24% | 41 | 24% | 41 | 22% | 2 | 2/40 | – | 2/40 | 0.0 ms | 2/40 | 0.0 ms | 15% | 0% |
| 2:30 | 2,525 | 2,521 | 0 | 0% | 1.0 ms | 4.5 ms | 37% | 22% | 62% | 25% | 41 | 26% | 41 | 26% | 2 | 3/40 | – | 4/40 | 0.0 ms | 2/40 | 0.0 ms | 15% | 0% |
| 2:40 | 2,675 | 2,671 | 0 | 0% | 1.0 ms | 4.5 ms | 39% | 23% | 62% | 27% | 41 | 26% | 41 | 30% | 2 | 2/40 +1 | – | 3/40 | 0.0 ms | 2/40 | 0.0 ms | 16% | 0% |
| 2:50 | 2,825 | 2,821 | 0 | 0% | 1.0 ms | 4.5 ms | 42% | 25% | 62% | 28% | 41 | 28% | 41 | 32% | 2 | 3/40 | – | 2/40 | 0.0 ms | 3/40 | 0.0 ms | 17% | 0% |
| 3:00 | 2,900 | 2,900 | 0 | 0% | 1.0 ms | 4.3 ms | 43% | 26% | 62% | 29% | 41 | 29% | 41 | 33% | 2 | 3/40 | – | 2/40 | 0.0 ms | 1/40 | 0.0 ms | 18% | 0% |
| 3:10 | 2,900 | 2,900 | 0 | 0% | 1.0 ms | 4.2 ms | 43% | 26% | 63% | 29% | 41 | 28% | 41 | 31% | 2 | 2/40 | – | 3/40 | 0.0 ms | 2/40 | 0.0 ms | 18% | 0% |
| 3:20 | 2,900 | 2,900 | 0 | 0% | 1.0 ms | 4.2 ms | 44% | 26% | 63% | 28% | 41 | 29% | 41 | 32% | 2 | 3/40 | – | 3/40 | 0.0 ms | 2/40 | 0.0 ms | 18% | 0% |
| 3:30 | 2,900 | 2,900 | 0 | 0% | 1.0 ms | 4.2 ms | 44% | 26% | 62% | 29% | 41 | 28% | 41 | 34% | 2 | 2/40 | – | 3/40 | 0.0 ms | 3/40 | 0.0 ms | 17% | 0% |
| 3:40 | 2,900 | 2,900 | 0 | 0% | 1.0 ms | 4.2 ms | 44% | 27% | 63% | 29% | 41 | 29% | 41 | 34% | 2 | 3/40 | – | 2/40 | 0.0 ms | 3/40 | 0.0 ms | 19% | 0% |
| 3:50 | 2,900 | 2,899 | 0 | 0% | 1.0 ms | 4.2 ms | 44% | 26% | 62% | 29% | 41 | 29% | 41 | 33% | 2 | 3/40 | – | 3/40 | 0.0 ms | 3/40 | 0.0 ms | 19% | 0% |
| 4:00 | 2,900 | 2,901 | 0 | 0% | 1.0 ms | 4.2 ms | 44% | 26% | 63% | 28% | 41 | 28% | 41 | 33% | 2 | 2/40 | – | 2/40 | 0.0 ms | 3/40 | 0.0 ms | 18% | 0% |
| 4:10 | 2,900 | 2,900 | 0 | 0% | 1.2 ms | 521 ms | 53% | 40% | 62% | 50% | 41 | 46% | 41 | 42% | 2 | 3/40 | 8.4 ms | 40/40 +178 | 7.1 ms | 23/40 +3 | 0.1 ms | 27% | 3.0% |
| 4:20 | 2,900 | 2,900 | 0 | 0% | 1.0 ms | 4.1 ms | 43% | 28% | 63% | 31% | 41 | 31% | 41 | 27% | 2 | 3/40 | – | 3/40 | 0.0 ms | 4/40 | 0.0 ms | 19% | 0% |
| 4:30 | 2,900 | 2,900 | 0 | 0% | 1.0 ms | 4.0 ms | 42% | 27% | 63% | 29% | 41 | 30% | 41 | 26% | 2 | 3/40 | – | 3/40 | 0.0 ms | 4/40 | 0.0 ms | 18% | 0% |
| 4:40 | 2,900 | 2,900 | 0 | 0% | 1.0 ms | 4.0 ms | 44% | 28% | 63% | 30% | 41 | 30% | 41 | 27% | 2 | 2/40 | – | 3/40 | 0.0 ms | 2/40 +1 | 0.0 ms | 19% | 0% |
| 4:50 | 2,900 | 2,900 | 0 | 0% | 1.0 ms | 4.1 ms | 46% | 28% | 63% | 31% | 41 | 31% | 41 | 32% | 2 | 3/40 | – | 3/40 | 0.0 ms | 3/40 | 0.0 ms | 16% | 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 CPU is against its budget.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: viral-post-cache-the-posts

# The posts api of a social app. The posts table outgrew one database, so
# it is split across two Postgres shards by post id; a post's comments and
# counts live on its shard. Each post page reads seven rows from it. Tonight
# a celebrity's post goes viral: six page views in ten are of that one post.

services:
  lb:
    kind: lb
    target: api
    resources: { cpu: 0.4, memory: 256m }
    health_check: { interval_ms: 2000, fall: 3, rise: 2 }

  api:
    image: loadlore/synthetic
    replicas: 2
    resources: { cpu: 0.75, memory: 512m }
    behavior:
      cpu_ms: 0.05
      calls:
        # A post page: the post, its author, its counts and the top
        # comments, seven rows read from the post's shard.
        # Changed: look the page up in a cache first, and read the shard
        # only on a miss, keeping the page for a minute.
        - target: cache
          op: get
          on_miss:
            - { shards: [db0, db1], op: read, count: 7 }
            - { target: cache, op: set, ttl_s: 60 }
      resilience:
        timeout_ms: 2000
    pools:
      cache: 20
      db0: { size: 20, min: 20 }   # opened at start, per replica
      db1: { size: 20, min: 20 }

  cache:   # added
    kind: redis
    resources: { cpu: 0.2, memory: 256m }

  db0:
    kind: postgres
    version: "17"
    resources: { cpu: 0.4, memory: 1g }
    config: { shared_buffers: 256MB }
    dataset: { rows: 500000, row_bytes: 512 }   # half of the million posts
    disk: { profile: gp3 }
  db1:
    kind: postgres
    version: "17"
    resources: { cpu: 0.4, memory: 1g }
    config: { shared_buffers: 256MB }
    dataset: { rows: 500000, row_bytes: 512 }
    disk: { profile: gp3 }

workload:
  entry: lb
  mix: { read: 1, write: 0 }
  # One post in a million takes six page views in ten.
  keys: { distribution: hotspot, share: 0.6, fraction: 0.000001, count: 1000000 }
  profile:
    - ramp: { from: 200, to: 2900, over: 3m }   # the post spreads
    - hold: { rps: 2900, for: 2m }

assumptions:
  - { field: services.api.behavior.cpu_ms, basis: guessed, why: "a thin handler: the page is mostly its reads" }
  - { field: services.api.behavior.calls, basis: guessed, why: "a post page reads the post, its author, its counts and four top comments" }
  - { field: workload.keys, basis: guessed, why: "a celebrity's post going viral: six views in ten are of it. No published figure; the hints and the variant's README show how the answer moves with this share" }
  - { field: workload.mix, basis: guessed, why: "page views only; likes and comments are left out to keep one lesson" }

# Your predictions, scored in the report. The line here is an example;
# loadl predict design.yaml replaces it with your answers.
expect:
  - p99_ms < 100 until load_rps >= 1000
```
