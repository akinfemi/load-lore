# live-vote-redis-tally: run report

_run `live-vote-redis-tally-20261114-150111` · completed · 14 Nov 2026 15:01 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 4,500 rps offered, 4,407 rps served at best, worst 10-second p99 5.2 ms, at most 0% errors.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 3:50

Measured in the 10-second window from 3:50 to 4:00 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 4,500 rps<br/>offered 4,410 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 48%<br/>2/2 replicas up"}}
    api["api ×2<br/>1 CPU · 512m<br/>CPU 89%<br/>in flight 4"]
    db[("db<br/>postgres 17<br/>1 CPU · 1g<br/>CPU 6.1%<br/>conns 81/100")]
    tally[("tally<br/>redis 8<br/>0.5 CPU · 256m<br/>CPU 15%")]
    load -->|"20% read · 80% write<br/>4,407 rps · p99 2.7 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>4,362 rps · p99 1.5 ms"| api
    api -->|"read on reads<br/>pool 40, min 40<br/>865 calls/s · p99 1.1 ms<br/>in use 1/80, 1 waiting"| db
    api -->|"set on writes<br/>pool 40<br/>3,520 calls/s · p99 0.9 ms<br/>in use 2/80"| tally
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
    x-axis "seconds" 0 --> 230
    y-axis "requests/s" 0 --> 5000
    line [289.583, 468.75, 647.917, 827.083, 1006.25, 1185.417, 1364.583, 1543.75, 1722.917, 1902.083, 2081.25, 2260.417, 2439.583, 2618.75, 2797.917, 2977.083, 3156.25, 3335.417, 3514.583, 3693.75, 3872.917, 4052.083, 4231.25, 4410.417]
    line [282.229, 464.928, 644.224, 823.662, 1002.397, 1181.704, 1361.004, 1540.194, 1719.317, 1898.34, 2077.716, 2256.754, 2435.958, 2615.093, 2794.518, 2973.335, 3152.371, 3331.984, 3510.612, 3690.409, 3869.304, 4048.466, 4227.299, 4407.07]
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
    x-axis "seconds" 0 --> 230
    y-axis "ms" 0 --> 6
    line [0.961, 0.642, 0.594, 0.595, 0.596, 0.595, 0.583, 0.579, 0.576, 0.576, 0.575, 0.576, 0.576, 0.577, 0.579, 0.581, 0.582, 0.584, 0.593, 0.595, 0.601, 0.606, 0.625, 0.661]
    line [1.639, 0.893, 0.696, 0.69, 0.699, 0.797, 0.688, 0.689, 0.695, 0.689, 0.688, 0.718, 0.726, 0.73, 0.743, 0.744, 0.751, 0.774, 0.809, 0.815, 0.871, 0.917, 1.234, 1.226]
    line [2.44, 1.214, 1.081, 1.132, 1.142, 1.336, 1.048, 0.936, 1.041, 0.964, 1.052, 1.252, 0.991, 1.142, 1.052, 1.112, 1.14, 1.093, 1.183, 1.257, 1.415, 1.817, 5.175, 2.652]
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
    x-axis "seconds" 0 --> 230
    y-axis "% of requests" 0 --> 1
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
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
    x-axis "seconds" 0 --> 230
    y-axis "% of limit" 0 --> 100
    line [10.294, 11.409, 14.317, 18.103, 22.07, 26.075, 29.253, 32.685, 36.404, 40.153, 43.934, 47.805, 51.438, 55.073, 58.797, 62.599, 66.254, 69.959, 73.491, 76.63, 80.005, 83.285, 87.455, 88.81]
    line [3.679, 3.769, 2.929, 3.146, 3.023, 3.494, 3.32, 3.484, 3.509, 3.626, 3.549, 3.831, 3.92, 3.854, 4.097, 4.099, 4.292, 4.32, 4.564, 4.556, 4.929, 5.265, 5.616, 6.094]
    line [14.383, 10.031, 9.145, 9.893, 10.245, 12.072, 13.099, 15.005, 19.386, 20.408, 20.793, 20.729, 23.217, 24.168, 24.559, 26.351, 27.455, 29.813, 34.661, 33.902, 36.667, 37.838, 43.689, 47.978]
    line [5.968, 3.575, 3.288, 3.829, 4.057, 4.732, 4.953, 5.376, 6.045, 6.291, 6.578, 7.072, 7.38, 7.685, 7.969, 8.476, 8.934, 9.488, 9.745, 10.252, 10.878, 11.886, 13.415, 14.731]
```

<sub>Blue: api. Orange: db. Green: lb. Purple: tally (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window).</sub>

## Diagnosis

No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version 9; thresholds below).

## Predictions

1 of 1 scored prediction held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 100 until load_rps >= 1000` | **held** | 2.4 ms | 0:00 to 0:40, when load_rps >= 1,000 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |

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
| resources.cpu_period_ms | 100 ms for api, db, lb, tally | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |
| services.api.pools.db.min | 40 | set in the design: each replica opens 40 connections before the load starts and keeps them open |
| services.api.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- Loadlore's own containers get fixed budgets: load generator 2 CPU (peak 4500 rps), Toxiproxy 1 CPU (about 9000 proxied calls/s at peak), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each

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
- **Working set doesn't fit:** for two windows in a row, a Postgres answering less than 90% of block reads from shared_buffers and reading 100 blocks a second or more from outside them, while p99 was 3x the low-load p99 or more.
- **Hot rows:** for two windows in a row, 2 or more of a Postgres's backends waiting on locks on average, while a caller's writes to it took 3x their low-load time or more (and 2 ms more). Reads of the same rows may slow too, and the summary says when the database's CPU per request reached 3x its low-load value: the contention burning CPU. A queueing knee on that database's CPU at the same time then points at the rows. Windows with a disk_slow fault on the database don't count.

## Environment

- loadl dev, profile 4m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 987 (hash 966, memory 991, syscalls 1003): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (987) was within 5.0% of the run it's compared with (1021)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 563,967 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-live-vote-redis-tally-20261114-150111-api-0-1 | loadlore/synthetic:dev |
| loadlore-live-vote-redis-tally-20261114-150111-api-1-1 | loadlore/synthetic:dev |
| loadlore-live-vote-redis-tally-20261114-150111-db-1 | postgres:17.11 |
| loadlore-live-vote-redis-tally-20261114-150111-lb-1 | haproxy:3.2.26 |
| loadlore-live-vote-redis-tally-20261114-150111-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-live-vote-redis-tally-20261114-150111-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-live-vote-redis-tally-20261114-150111-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-live-vote-redis-tally-20261114-150111-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-live-vote-redis-tally-20261114-150111-lore-redisexp-tally-1 | oliver006/redis_exporter:v1.97.0 |
| loadlore-live-vote-redis-tally-20261114-150111-lore-toxiproxy-1 | ghcr.io/shopify/toxiproxy:2.13.0 |
| loadlore-live-vote-redis-tally-20261114-150111-tally-1 | redis:8.2 |

</details>

## Per-window data

<details><summary>24 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | lb CPU | lb up | tally CPU | tally hit rate | api→db pool | wait | api→tally pool | wait | k6 CPU | Toxiproxy CPU | k6 / Toxiproxy stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 290 | 282 | 0 | 0% | 1.0 ms | 2.4 ms | 10% | 3.7% | 81 | 14% | 2 | 6.0% | – | 0/80 | 0.0 ms | 0/80 | – | 9.0% | 17% | 0% / 0% |
| 0:10 | 469 | 465 | 0 | 0% | 0.6 ms | 1.2 ms | 11% | 3.8% | 81 | 10% | 2 | 3.6% | – | 0/80 | 0.0 ms | 1/80 | – | 4.9% | 9.0% | 0% / 0% |
| 0:20 | 648 | 644 | 0 | 0% | 0.6 ms | 1.1 ms | 14% | 2.9% | 81 | 9.1% | 2 | 3.3% | – | 0/80 | 0.0 ms | 1/80 | – | 5.7% | 9.7% | 0% / 0% |
| 0:30 | 827 | 824 | 0 | 0% | 0.6 ms | 1.1 ms | 18% | 3.1% | 81 | 9.9% | 2 | 3.8% | – | 0/80 | 0.0 ms | 1/80 | – | 6.6% | 12% | 0% / 0% |
| 0:40 | 1,006 | 1,002 | 0 | 0% | 0.6 ms | 1.1 ms | 22% | 3.0% | 81 | 10% | 2 | 4.1% | – | 0/80 | 0.0 ms | 1/80 | – | 7.2% | 13% | 0% / 0% |
| 0:50 | 1,185 | 1,182 | 0 | 0% | 0.6 ms | 1.3 ms | 26% | 3.5% | 81 | 12% | 2 | 4.7% | – | 0/80 | 0.0 ms | 1/80 | – | 8.1% | 15% | 0% / 0% |
| 1:00 | 1,365 | 1,361 | 0 | 0% | 0.6 ms | 1.0 ms | 29% | 3.3% | 81 | 13% | 2 | 5.0% | – | 1/80 | 0.0 ms | 1/80 | – | 8.7% | 18% | 0% / 0% |
| 1:10 | 1,544 | 1,540 | 0 | 0% | 0.6 ms | 0.9 ms | 33% | 3.5% | 81 | 15% | 2 | 5.4% | – | 0/80 | 0.0 ms | 1/80 | – | 9.2% | 20% | 0% / 0% |
| 1:20 | 1,723 | 1,719 | 0 | 0% | 0.6 ms | 1.0 ms | 36% | 3.5% | 81 | 19% | 2 | 6.0% | – | 1/80 | 0.0 ms | 1/80 | – | 10% | 23% | 0% / 0% |
| 1:30 | 1,902 | 1,898 | 0 | 0% | 0.6 ms | 1.0 ms | 40% | 3.6% | 81 | 20% | 2 | 6.3% | – | 0/80 | 0.0 ms | 0/80 | – | 10% | 25% | 0% / 0% |
| 1:40 | 2,081 | 2,078 | 0 | 0% | 0.6 ms | 1.1 ms | 44% | 3.5% | 81 | 21% | 2 | 6.6% | – | 1/80 | 0.0 ms | 2/80 | – | 10% | 26% | 0% / 0% |
| 1:50 | 2,260 | 2,257 | 0 | 0% | 0.6 ms | 1.3 ms | 48% | 3.8% | 81 | 21% | 2 | 7.1% | – | 0/80 | 0.0 ms | 2/80 | – | 12% | 27% | 0% / 0% |
| 2:00 | 2,440 | 2,436 | 0 | 0% | 0.6 ms | 1.0 ms | 51% | 3.9% | 81 | 23% | 2 | 7.4% | – | 1/80 | 0.0 ms | 1/80 | – | 12% | 29% | 0% / 0% |
| 2:10 | 2,619 | 2,615 | 0 | 0% | 0.6 ms | 1.1 ms | 55% | 3.9% | 81 | 24% | 2 | 7.7% | – | 1/80 | 0.0 ms | 1/80 | – | 12% | 30% | 0% / 0% |
| 2:20 | 2,798 | 2,795 | 0 | 0% | 0.6 ms | 1.1 ms | 59% | 4.1% | 81 | 25% | 2 | 8.0% | – | 0/80 | 0.0 ms | 1/80 | – | 13% | 32% | 0% / 0% |
| 2:30 | 2,977 | 2,973 | 0 | 0% | 0.6 ms | 1.1 ms | 63% | 4.1% | 81 | 26% | 2 | 8.5% | – | 1/80 | 0.0 ms | 2/80 | – | 14% | 33% | 0% / 0% |
| 2:40 | 3,156 | 3,152 | 0 | 0% | 0.6 ms | 1.1 ms | 66% | 4.3% | 81 | 27% | 2 | 8.9% | – | 1/80 | 0.0 ms | 3/80 | – | 15% | 35% | 0% / 0% |
| 2:50 | 3,335 | 3,332 | 0 | 0% | 0.6 ms | 1.1 ms | 70% | 4.3% | 81 | 30% | 2 | 9.5% | – | 1/80 | 0.0 ms | 2/80 | – | 16% | 37% | 0% / 0% |
| 3:00 | 3,515 | 3,511 | 0 | 0% | 0.6 ms | 1.2 ms | 73% | 4.6% | 81 | 35% | 2 | 9.7% | – | 1/80 | 0.0 ms | 2/80 | – | 16% | 38% | 0% / 0% |
| 3:10 | 3,694 | 3,690 | 0 | 0% | 0.6 ms | 1.3 ms | 77% | 4.6% | 81 | 34% | 2 | 10% | – | 1/80 | 0.0 ms | 3/80 | – | 17% | 39% | 0% / 0% |
| 3:20 | 3,873 | 3,869 | 0 | 0% | 0.6 ms | 1.4 ms | 80% | 4.9% | 81 | 37% | 2 | 11% | – | 1/80 | 0.0 ms | 2/80 | – | 18% | 41% | 0% / 0% |
| 3:30 | 4,052 | 4,048 | 0 | 0% | 0.6 ms | 1.8 ms | 83% | 5.3% | 81 | 38% | 2 | 12% | – | 1/80 | 0.0 ms | 2/80 +1 | – | 20% | 43% | 0% / 0% |
| 3:40 | 4,231 | 4,227 | 0 | 0% | 0.6 ms | 5.2 ms | 87% | 5.6% | 81 | 44% | 2 | 13% | – | 0/80 | 0.0 ms | 2/80 | – | 23% | 48% | 0% / 0% |
| 3:50 | 4,410 | 4,407 | 0 | 0% | 0.7 ms | 2.7 ms | 89% | 6.1% | 81 | 48% | 2 | 15% | – | 1/80 +1 | 0.0 ms | 2/80 | – | 26% | 51% | 0% / 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 and Toxiproxy CPU are against their budgets.

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
