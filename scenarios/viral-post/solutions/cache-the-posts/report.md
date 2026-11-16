# viral-post-cache-the-posts: run report

_run `viral-post-cache-the-posts-20261116-214141` · completed · 16 Nov 2026 21:41 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 2,900 rps offered, 2,900 rps served at best, worst 10-second p99 4.9 ms, at most 0% errors.

Caveat: at 4:50 the load generator hit its CPU limit and was stalled in 1.0% of its scheduling periods (container_cpu_cfs_throttled_periods_total ÷ container_cpu_cfs_periods_total (cAdvisor)); work caught in a stall waits for the next 100 ms period, which can show up in p99.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 3:40

Measured in the 10-second window from 3:40 to 3:50 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 2,900 rps<br/>offered 2,900 rps"])
    lb{{"lb<br/>load balancer<br/>0.4 CPU · 256m<br/>CPU 48%<br/>2/2 replicas up"}}
    api["api ×2<br/>0.75 CPU · 512m<br/>CPU 52%<br/>in flight 4"]
    cache[("cache<br/>redis 8<br/>0.2 CPU · 256m<br/>CPU 39%<br/>hit rate 63%")]
    db0[("db0<br/>postgres 17<br/>0.4 CPU · 1g<br/>disk gp3 · 3,000 IOPS<br/>CPU 36%<br/>conns 41/100")]
    db1[("db1<br/>postgres 17<br/>0.4 CPU · 1g<br/>disk gp3 · 3,000 IOPS<br/>CPU 36%<br/>conns 41/100")]
    load -->|"100% read · 0% write<br/>2,900 rps · p99 2.6 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>2,900 rps · p99 2.3 ms"| api
    api -->|"get, set on miss<br/>pool 20<br/>3,983 calls/s · p99 0.5 ms<br/>in use 3/40"| cache
    api -->|"read ×7 on miss<br/>shard 1 of 2<br/>pool 20, min 20<br/>3,771 calls/s · p99 0.5 ms<br/>in use 1/40"| db0
    api -->|"read ×7 on miss<br/>shard 2 of 2<br/>pool 20, min 20<br/>3,813 calls/s · p99 0.5 ms<br/>in use 2/40"| db1
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
      plotColorPalette: "#9e9e9e, #1f77b4"
---
xychart-beta
    title "Throughput"
    x-axis "seconds" 0 --> 290
    y-axis "requests/s" 0 --> 4000
    line [275, 425, 575, 725, 875, 1025, 1175, 1325, 1475, 1625, 1775, 1925, 2075, 2225, 2375, 2525, 2675, 2825, 2900, 2900, 2900, 2900, 2900, 2900, 2900, 2900, 2900, 2900, 2900, 2900]
    line [266.343, 421.243, 571.217, 721.203, 871.246, 1021.225, 1171.309, 1321.207, 1471.292, 1620.987, 1771.402, 1921.211, 2071.411, 2221.073, 2371.19, 2521.328, 2671.116, 2821.051, 2900.161, 2900.008, 2899.809, 2900.175, 2900.29, 2899.615, 2900.125, 2899.813, 2900.164, 2900.076, 2899.843, 2900.035]
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
    y-axis "ms" 0 --> 6
    line [0.525, 0.347, 0.34, 0.334, 0.336, 0.344, 0.357, 0.361, 0.373, 0.376, 0.389, 0.401, 0.405, 0.41, 0.411, 0.419, 0.422, 0.441, 0.431, 0.442, 0.44, 0.446, 0.465, 0.488, 0.501, 0.508, 0.516, 0.509, 0.516, 0.513]
    line [3.573, 3.065, 2.494, 2.217, 2.162, 2.191, 2.212, 2.213, 2.246, 2.21, 2.194, 2.169, 2.147, 2.136, 1.986, 1.947, 1.827, 1.937, 1.804, 1.829, 1.785, 1.782, 1.836, 1.931, 2.003, 2.016, 2.049, 2.032, 2.063, 2.051]
    line [4.923, 3.512, 3.205, 2.757, 2.718, 2.734, 2.78, 2.81, 2.888, 2.786, 2.75, 2.722, 2.81, 2.791, 2.685, 2.703, 2.637, 3.066, 2.74, 2.66, 2.658, 2.754, 2.602, 2.646, 2.732, 2.745, 2.675, 2.77, 2.949, 2.766]
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
      plotColorPalette: "#1f77b4, #ff7f0e, #2ca02c, #9467bd, #8c564b"
---
xychart-beta
    title "CPU against each limit"
    x-axis "seconds" 0 --> 290
    y-axis "% of limit" 0 --> 100
    line [7.54, 8.304, 10.753, 12.846, 15.365, 17.831, 20.108, 22.268, 24.949, 27.087, 29.599, 31.826, 34.621, 37.025, 39.192, 41.488, 44.5, 47.489, 48.054, 49.393, 49.432, 50.184, 51.907, 53.796, 54.564, 55.325, 55.956, 55.624, 55.816, 54.793]
    line [12.501, 8.7, 10.798, 12.109, 13.57, 15.01, 16.583, 18.021, 19.233, 21.126, 22.887, 24.132, 26.22, 28.118, 29.757, 31.296, 33.432, 36.831, 36.48, 37.702, 37.181, 37.879, 39.236, 41.482, 42.125, 43.039, 43.664, 43.415, 43.547, 43.282]
    line [14.439, 10.879, 11.832, 13.152, 14.397, 16.076, 17.358, 18.927, 20.001, 22.305, 23.042, 24.409, 26.375, 27.414, 28.421, 29.288, 31.487, 34.049, 34.416, 34.317, 34.201, 35.134, 35.649, 37.725, 38.485, 40.402, 39.661, 39.737, 40.973, 39.512]
    line [14.192, 10.558, 11.907, 12.743, 14.17, 16.303, 17.232, 18.378, 20.534, 21.361, 22.72, 23.964, 25.671, 26.854, 28.324, 29.349, 31.072, 34.222, 33.999, 34.201, 34.656, 34.053, 35.936, 38.113, 38.917, 39.511, 40.344, 39.738, 39.974, 40.018]
    line [9.287, 8.604, 10.714, 10.789, 11.3, 14.607, 14.916, 16.032, 18.379, 19.903, 23.098, 23.174, 27.022, 33.508, 32.482, 36.98, 40.66, 40.172, 37.452, 41.289, 42.135, 43.518, 47.906, 49.63, 50.919, 46.138, 49.367, 49.584, 46.627, 46.003]
```

<sub>Blue: api. Orange: cache. Green: db0. Purple: db1. Brown: lb (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window).</sub>

## Diagnosis

No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version 15; thresholds below).

## Predictions

1 of 1 scored prediction held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 100 until load_rps >= 1000` | **held** | 4.9 ms | 0:00 to 0:50, when load_rps >= 1,000 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |

Loads and request rates depend on this machine: slower cores break the same design at a lower load, so a load guess can miss even when the reasoning behind it was right.

## Assumptions

### From the design

| Field | Value | Basis | Why |
| --- | --- | --- | --- |
| `services.api.behavior.cpu_ms` |  | guessed | a thin handler: the page is mostly its reads |
| `services.api.behavior.calls` |  | guessed | a post page reads the post, its author, its counts and four top comments |
| `workload.keys` |  | guessed | a celebrity's post going viral: six views in ten are of it. No published figure; the README shows how the answer moves with this share |
| `workload.mix` |  | guessed | page views only; likes and comments are left out to keep one lesson |

4 of these are guessed, not measured: the result depends on them.

### Chosen by Loadlore

| Field | Value | Why |
| --- | --- | --- |
| services.cache.config.maxmemory | 192m | 75% of the container's 256m memory limit, as managed Redis usually does; the rest is for fragmentation and client buffers. Set maxmemory: 0 for stock Redis (no cap) |
| services.cache.config.maxmemory-policy | allkeys-lru | evict the least recently used keys when the cache is full, as managed Redis usually does |
| workload.timeout_ms | 10s | the default; k6 gives up on a request after this long and counts it as failed |
| workload.slo.p99_ms | 500 ms | the default; a break on p99 alone needs p99 at or above this as well as 10x the low-load p99, so latency users would still accept isn't called a break |
| resources.cpu_period_ms | 100 ms for api, cache, db0, db1, lb | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.db0.disk | gp3: 1 ms per read, 1 ms per write, 3000 IOPS, 125 MiB/s | a gp3 volume at its baseline: 3,000 IOPS and 125 MiB/s, included in its price, and up to 80,000 IOPS provisioned. AWS publishes no exact latency, only single-digit milliseconds, so the 1 ms Loadlore adds to every read, write and fsync is its own estimate: the low end of that range, so the disk is flattered rather than handicapped. Reads and writes are capped separately, so a mixed load can reach up to twice the IOPS. Provisioned throughput isn't modelled. Source: AWS, Amazon EBS General Purpose SSD volumes: https://docs.aws.amazon.com/ebs/latest/userguide/general-purpose.html |
| services.db1.disk | gp3: 1 ms per read, 1 ms per write, 3000 IOPS, 125 MiB/s | a gp3 volume at its baseline: 3,000 IOPS and 125 MiB/s, included in its price, and up to 80,000 IOPS provisioned. AWS publishes no exact latency, only single-digit milliseconds, so the 1 ms Loadlore adds to every read, write and fsync is its own estimate: the low end of that range, so the disk is flattered rather than handicapped. Reads and writes are capped separately, so a mixed load can reach up to twice the IOPS. Provisioned throughput isn't modelled. Source: AWS, Amazon EBS General Purpose SSD volumes: https://docs.aws.amazon.com/ebs/latest/userguide/general-purpose.html |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |
| services.api.pools.db0.min | 20 | set in the design: each replica opens 20 connections before the load starts and keeps them open |
| services.api.pools.db0.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |
| services.api.pools.db1.min | 20 | set in the design: each replica opens 20 connections before the load starts and keeps them open |
| services.api.pools.db1.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- Loadlore's own containers get fixed budgets: load generator 1.5 CPU (peak 2900 rps), Toxiproxy 3 CPU (about 29000 proxied calls/s at peak), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each

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
- **Consumer lag:** for two windows in a row, a queue's oldest unfinished job had waited 5 s or more, or the queue held more jobs than its workers run at once and grew, with jobs published 5.0% or more faster than they were finished. Named a full queue once its memory reached 90% of its maxmemory or it refused jobs.
- **Autoscale lag:** the run broke while an autoscaled service's replicas were at 90% or more of their CPU limit: below its max, the autoscaler hadn't caught up; at its max (autoscale ceiling), it had nothing left to add.
- **Hot rows:** for two windows in a row, 2 or more of a Postgres's backends waiting on locks on average, while a caller's writes to it took 3x their low-load time or more (and 2 ms more). Reads of the same rows may slow too, and the summary says when the database's CPU per request reached 3x its low-load value: the contention burning CPU. A queueing knee on that database's CPU at the same time then points at the rows. Windows with a disk_slow fault on the database don't count.
- **Hot shard:** for two windows in a row, a caller sending calls to shards by key, one shard taking 1.5x the calls of the next busiest shard or more while it was busy (CPU 80% or more, or its calls 3x their low-load time and 1 ms more) and another shard's CPU under 50%. A queueing knee on that shard at the same time then points at its popular keys.

## Environment

- loadl dev, profile 5m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 1005 (hash 961, memory 1051, syscalls 1004): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (1005) was within 5.0% of the run it's compared with (986)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 626,985 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-viral-post-cache-the-posts-20261116-214141-api-0-1 | loadlore/synthetic:dev |
| loadlore-viral-post-cache-the-posts-20261116-214141-api-1-1 | loadlore/synthetic:dev |
| loadlore-viral-post-cache-the-posts-20261116-214141-cache-1 | redis:8.2 |
| loadlore-viral-post-cache-the-posts-20261116-214141-db0-1 | postgres:17.11 |
| loadlore-viral-post-cache-the-posts-20261116-214141-db1-1 | postgres:17.11 |
| loadlore-viral-post-cache-the-posts-20261116-214141-lb-1 | haproxy:3.2.26 |
| loadlore-viral-post-cache-the-posts-20261116-214141-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-viral-post-cache-the-posts-20261116-214141-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-viral-post-cache-the-posts-20261116-214141-lore-pgexp-db0-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-viral-post-cache-the-posts-20261116-214141-lore-pgexp-db1-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-viral-post-cache-the-posts-20261116-214141-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-viral-post-cache-the-posts-20261116-214141-lore-redisexp-cache-1 | oliver006/redis_exporter:v1.97.0 |
| loadlore-viral-post-cache-the-posts-20261116-214141-lore-toxiproxy-1 | ghcr.io/shopify/toxiproxy:2.13.0 |

</details>

## Per-window data

<details><summary>30 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | cache CPU | cache hit rate | db0 CPU | db0 conns | db1 CPU | db1 conns | lb CPU | lb up | api→cache pool | wait | api→db0 pool | wait | api→db1 pool | wait | k6 CPU | Toxiproxy CPU | k6 / Toxiproxy stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 275 | 266 | 0 | 0% | 0.5 ms | 4.9 ms | 7.5% | 13% | 61% | 14% | 41 | 14% | 41 | 9.3% | 2 | 1/40 | – | 1/40 | 0.0 ms | 1/40 | 0.0 ms | 6.7% | 3.9% | 1.0% / 0% |
| 0:10 | 425 | 421 | 0 | 0% | 0.3 ms | 3.5 ms | 8.3% | 8.7% | 59% | 11% | 41 | 11% | 41 | 8.6% | 2 | 1/40 | – | 0/40 | 0.0 ms | 1/40 | 0.0 ms | 4.4% | 4.0% | 0% / 0% |
| 0:20 | 575 | 571 | 0 | 0% | 0.3 ms | 3.2 ms | 11% | 11% | 59% | 12% | 41 | 12% | 41 | 11% | 2 | 1/40 | – | 0/40 | 0.0 ms | 0/40 | 0.0 ms | 6.2% | 5.4% | 0% / 0% |
| 0:30 | 725 | 721 | 0 | 0% | 0.3 ms | 2.8 ms | 13% | 12% | 61% | 13% | 41 | 13% | 41 | 11% | 2 | 1/40 | – | 1/40 | 0.0 ms | 1/40 | 0.0 ms | 7.1% | 6.4% | 0% / 0% |
| 0:40 | 875 | 871 | 0 | 0% | 0.3 ms | 2.7 ms | 15% | 14% | 60% | 14% | 41 | 14% | 41 | 11% | 2 | 1/40 | – | 0/40 | 0.0 ms | 2/40 | 0.0 ms | 8.3% | 7.7% | 0% / 0% |
| 0:50 | 1,025 | 1,021 | 0 | 0% | 0.3 ms | 2.7 ms | 18% | 15% | 61% | 16% | 41 | 16% | 41 | 15% | 2 | 1/40 | – | 1/40 | 0.0 ms | 1/40 | 0.0 ms | 8.9% | 8.9% | 0% / 0% |
| 1:00 | 1,175 | 1,171 | 0 | 0% | 0.4 ms | 2.8 ms | 20% | 17% | 61% | 17% | 41 | 17% | 41 | 15% | 2 | 0/40 | – | 1/40 | 0.0 ms | 2/40 | 0.0 ms | 10.0% | 10.0% | 0% / 0% |
| 1:10 | 1,325 | 1,321 | 0 | 0% | 0.4 ms | 2.8 ms | 22% | 18% | 61% | 19% | 41 | 18% | 41 | 16% | 2 | 0/40 | – | 1/40 | 0.0 ms | 1/40 | 0.0 ms | 10% | 11% | 0% / 0% |
| 1:20 | 1,475 | 1,471 | 0 | 0% | 0.4 ms | 2.9 ms | 25% | 19% | 61% | 20% | 41 | 21% | 41 | 18% | 2 | 1/40 | – | 1/40 | 0.0 ms | 2/40 | 0.0 ms | 12% | 12% | 0% / 0% |
| 1:30 | 1,625 | 1,621 | 0 | 0% | 0.4 ms | 2.8 ms | 27% | 21% | 61% | 22% | 41 | 21% | 41 | 20% | 2 | 2/40 | – | 1/40 | 0.0 ms | 1/40 | 0.0 ms | 12% | 14% | 0% / 0% |
| 1:40 | 1,775 | 1,771 | 0 | 0% | 0.4 ms | 2.8 ms | 30% | 23% | 61% | 23% | 41 | 23% | 41 | 23% | 2 | 2/40 | – | 1/40 | 0.0 ms | 1/40 | 0.0 ms | 14% | 15% | 0% / 0% |
| 1:50 | 1,925 | 1,921 | 0 | 0% | 0.4 ms | 2.7 ms | 32% | 24% | 61% | 24% | 41 | 24% | 41 | 23% | 2 | 2/40 | – | 1/40 | 0.0 ms | 1/40 | 0.0 ms | 15% | 16% | 0% / 0% |
| 2:00 | 2,075 | 2,071 | 0 | 0% | 0.4 ms | 2.8 ms | 35% | 26% | 61% | 26% | 41 | 26% | 41 | 27% | 2 | 1/40 | – | 2/40 | 0.0 ms | 2/40 | 0.0 ms | 16% | 17% | 0% / 0% |
| 2:10 | 2,225 | 2,221 | 0 | 0% | 0.4 ms | 2.8 ms | 37% | 28% | 62% | 27% | 41 | 27% | 41 | 34% | 2 | 2/40 | – | 2/40 | 0.0 ms | 1/40 | 0.0 ms | 17% | 18% | 0% / 0% |
| 2:20 | 2,375 | 2,371 | 0 | 0% | 0.4 ms | 2.7 ms | 39% | 30% | 62% | 28% | 41 | 28% | 41 | 32% | 2 | 2/40 | – | 1/40 | 0.0 ms | 1/40 | 0.0 ms | 18% | 19% | 0% / 0% |
| 2:30 | 2,525 | 2,521 | 0 | 0% | 0.4 ms | 2.7 ms | 41% | 31% | 62% | 29% | 41 | 29% | 41 | 37% | 2 | 2/40 | – | 1/40 | 0.0 ms | 1/40 | 0.0 ms | 19% | 21% | 0% / 0% |
| 2:40 | 2,675 | 2,671 | 0 | 0% | 0.4 ms | 2.6 ms | 45% | 33% | 62% | 31% | 41 | 31% | 41 | 41% | 2 | 2/40 | – | 1/40 | 0.0 ms | 2/40 | 0.0 ms | 21% | 22% | 0% / 0% |
| 2:50 | 2,825 | 2,821 | 0 | 0% | 0.4 ms | 3.1 ms | 47% | 37% | 62% | 34% | 41 | 34% | 41 | 40% | 2 | 2/40 | – | 1/40 | 0.0 ms | 1/40 | 0.0 ms | 21% | 24% | 0% / 0% |
| 3:00 | 2,900 | 2,900 | 0 | 0% | 0.4 ms | 2.7 ms | 48% | 36% | 62% | 34% | 41 | 34% | 41 | 37% | 2 | 2/40 | – | 2/40 | 0.0 ms | 2/40 | 0.0 ms | 23% | 24% | 0% / 0% |
| 3:10 | 2,900 | 2,900 | 0 | 0% | 0.4 ms | 2.7 ms | 49% | 38% | 63% | 34% | 41 | 34% | 41 | 41% | 2 | 2/40 | – | 1/40 | 0.0 ms | 2/40 | 0.0 ms | 23% | 25% | 0% / 0% |
| 3:20 | 2,900 | 2,900 | 0 | 0% | 0.4 ms | 2.7 ms | 49% | 37% | 63% | 34% | 41 | 35% | 41 | 42% | 2 | 3/40 | – | 2/40 | 0.0 ms | 2/40 | 0.0 ms | 23% | 25% | 0% / 0% |
| 3:30 | 2,900 | 2,900 | 0 | 0% | 0.4 ms | 2.8 ms | 50% | 38% | 62% | 35% | 41 | 34% | 41 | 44% | 2 | 4/40 | – | 3/40 | 0.0 ms | 1/40 | 0.0 ms | 23% | 25% | 0% / 0% |
| 3:40 | 2,900 | 2,900 | 0 | 0% | 0.5 ms | 2.6 ms | 52% | 39% | 63% | 36% | 41 | 36% | 41 | 48% | 2 | 3/40 | – | 1/40 | 0.0 ms | 2/40 | 0.0 ms | 24% | 26% | 0% / 0% |
| 3:50 | 2,900 | 2,900 | 0 | 0% | 0.5 ms | 2.6 ms | 54% | 41% | 62% | 38% | 41 | 38% | 41 | 50% | 2 | 2/40 | – | 1/40 | 0.0 ms | 3/40 | 0.0 ms | 25% | 27% | 0% / 0% |
| 4:00 | 2,900 | 2,900 | 0 | 0% | 0.5 ms | 2.7 ms | 55% | 42% | 63% | 38% | 41 | 39% | 41 | 51% | 2 | 1/40 | – | 4/40 | 0.0 ms | 2/40 | 0.0 ms | 25% | 27% | 0% / 0% |
| 4:10 | 2,900 | 2,900 | 0 | 0% | 0.5 ms | 2.7 ms | 55% | 43% | 62% | 40% | 41 | 40% | 41 | 46% | 2 | 4/40 | – | 2/40 | 0.0 ms | 2/40 | 0.0 ms | 25% | 28% | 0% / 0% |
| 4:20 | 2,900 | 2,900 | 0 | 0% | 0.5 ms | 2.7 ms | 56% | 44% | 63% | 40% | 41 | 40% | 41 | 49% | 2 | 2/40 | – | 3/40 | 0.0 ms | 2/40 | 0.0 ms | 26% | 28% | 0% / 0% |
| 4:30 | 2,900 | 2,900 | 0 | 0% | 0.5 ms | 2.8 ms | 56% | 43% | 63% | 40% | 41 | 40% | 41 | 50% | 2 | 3/40 | – | 2/40 | 0.0 ms | 2/40 | 0.0 ms | 26% | 28% | 0% / 0% |
| 4:40 | 2,900 | 2,900 | 0 | 0% | 0.5 ms | 2.9 ms | 56% | 44% | 63% | 41% | 41 | 40% | 41 | 47% | 2 | 2/40 | – | 3/40 | 0.0 ms | 1/40 | 0.0 ms | 25% | 28% | 0% / 0% |
| 4:50 | 2,900 | 2,900 | 0 | 0% | 0.5 ms | 2.8 ms | 55% | 43% | 63% | 40% | 41 | 40% | 41 | 46% | 2 | 2/40 | – | 2/40 | 0.0 ms | 2/40 | 0.0 ms | 24% | 27% | 1.0% / 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 and Toxiproxy CPU are against their budgets.

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
  - { field: workload.keys, basis: guessed, why: "a celebrity's post going viral: six views in ten are of it. No published figure; the README shows how the answer moves with this share" }
  - { field: workload.mix, basis: guessed, why: "page views only; likes and comments are left out to keep one lesson" }

# Your predictions, scored in the report. The line here is an example;
# loadl predict design.yaml replaces it with your answers.
expect:
  - p99_ms < 100 until load_rps >= 1000
```
