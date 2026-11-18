# featured-on-hacker-news-headroom: run report

_run `featured-on-hacker-news-headroom-20261117-214545` · completed · 17 Nov 2026 21:45 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 1,800 rps offered, 1,803 rps served at best, worst 10-second p99 62 ms, at most 0% errors.

Caveat: at 1:50 the load generator hit its CPU limit and was stalled in 2.0% of its scheduling periods (container_cpu_cfs_throttled_periods_total ÷ container_cpu_cfs_periods_total (cAdvisor)); work caught in a stall waits for the next 100 ms period, which can show up in p99.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 2:10

Measured in the 10-second window from 2:10 to 2:20 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 1,800 rps<br/>offered 1,800 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 17%<br/>4/6 replicas up"}}
    api["api ×4<br/>0.5 CPU · 256m<br/>autoscale 4–6 on CPU 70%<br/>CPU 82%<br/>in flight 49"]
    db[("db<br/>postgres 17<br/>1 CPU · 1g<br/>CPU 8.9%<br/>conns 51/100")]
    load -->|"90% read · 10% write<br/>1,803 rps · p99 61 ms"| lb
    lb -->|"round-robin<br/>checks every 1s, out after 3<br/>1,805 rps · p99 62 ms"| api
    api -->|"read<br/>pool 10, min 10<br/>1,802 calls/s · p99 14 ms<br/>in use 22/50, 10 waiting"| db
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb,db ok
    class api warm
```

<sub>Sources: offered from the load profile in plan.json (offered arrival rate); load edge from k6_http_reqs_total (k6: all responses) and k6_http_req_duration_seconds (k6 histogram, end to end); CPU from container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit; calls/s from loadlore_calls_total (the caller's outbound call attempts) and their p99 from loadlore_call_duration_seconds (the caller's histogram, pool wait included); pools from loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections and loadlore_pool_pending (callers waiting for a connection); lb traffic from haproxy_backend_http_requests_total (HAProxy), replicas up from haproxy_backend_active_servers (HAProxy), and the lb edge's p99 from loadlore_request_duration_seconds (the service's own histogram); conns from pg_stat_activity_count and pg_settings_max_connections (postgres_exporter).</sub>

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
    x-axis "seconds" 0 --> 180
    y-axis "requests/s" 0 --> 2000
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2000, 0, 2000, 2000, 0, 2000, 0, 0, 0]
    line [200, 200, 200, 200, 200, 200, 377.778, 733.333, 1088.889, 1444.444, 1755.556, 1800, 1800, 1800, 1800, 1800, 1800, 1800, 1800]
    line [188.168, 199.969, 200.045, 199.985, 199.925, 200.05, 357.379, 711.975, 1067.779, 1423.25, 1740.881, 1799.71, 1801.11, 1802.549, 1800.096, 1800.052, 1800.202, 1799.695, 1800.194]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 1:45 api 4 → 5 replicas (CPU 86% of its limit, target 70%); 2:00 api 5 → 6 replicas (CPU 96% of its limit, target 70%); 2:15 api-4 start; 2:17 api-4 took traffic, 31.9 s after the decision; 2:30 api-5 start; 2:32 api-5 took traffic, 31.8 s after the decision.</sub>

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
    x-axis "seconds" 0 --> 180
    y-axis "ms" 0 --> 80
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 80, 0, 80, 80, 0, 80, 0, 0, 0]
    line [2.396, 2.39, 2.126, 2.104, 2.102, 2.107, 2.056, 2.039, 2.016, 1.999, 2.077, 4.143, 5.663, 2.173, 1.987, 2.001, 1.99, 1.995, 1.996]
    line [2.678, 2.647, 2.429, 2.319, 2.316, 2.318, 2.302, 2.295, 2.274, 2.288, 39.369, 49.568, 51.964, 49.456, 2.292, 2.381, 2.284, 2.311, 2.304]
    line [3.261, 2.787, 2.66, 2.521, 2.528, 2.527, 2.515, 2.743, 2.518, 3.468, 52.213, 58.652, 61.862, 60.74, 3.108, 3.671, 2.786, 3.32, 3.094]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 1:45 api 4 → 5 replicas (CPU 86% of its limit, target 70%); 2:00 api 5 → 6 replicas (CPU 96% of its limit, target 70%); 2:15 api-4 start; 2:17 api-4 took traffic, 31.9 s after the decision; 2:30 api-5 start; 2:32 api-5 took traffic, 31.8 s after the decision.</sub>

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
    x-axis "seconds" 0 --> 180
    y-axis "% of requests" 0 --> 1
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 1, 1, 0, 1, 0, 0, 0]
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 1:45 api 4 → 5 replicas (CPU 86% of its limit, target 70%); 2:00 api 5 → 6 replicas (CPU 96% of its limit, target 70%); 2:15 api-4 start; 2:17 api-4 took traffic, 31.9 s after the decision; 2:30 api-5 start; 2:32 api-5 took traffic, 31.8 s after the decision.</sub>

```mermaid
---
config:
  themeVariables:
    xyChart:
      backgroundColor: "transparent"
      plotColorPalette: "#f4cccc, #1f77b4, #ff7f0e, #2ca02c"
---
xychart-beta
    title "CPU against each limit"
    x-axis "seconds" 0 --> 180
    y-axis "% of limit" 0 --> 100
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, 0, 100, 100, 0, 100, 0, 0, 0]
    line [13.737, 13.357, 13.217, 13.001, 13.041, 13.132, 22.017, 42.295, 62.797, 83.512, 99.344, 99.928, 100.023, 81.772, 85.399, 71.722, 71.762, 71.755, 71.853]
    line [3.069, 3.302, 3.34, 3.648, 3.197, 3.29, 3.629, 4.309, 5.051, 5.935, 8.221, 8.893, 9.027, 8.855, 7.364, 7.474, 7.065, 7.472, 7.251]
    line [5.343, 5.633, 3.845, 3.568, 3.543, 3.547, 5.177, 8.103, 11.425, 11.946, 15.273, 16.489, 16.711, 16.827, 15.127, 15.811, 14.174, 15.83, 15.858]
```

<sub>Blue: api. Orange: db. Green: lb (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 1:45 api 4 → 5 replicas (CPU 86% of its limit, target 70%); 2:00 api 5 → 6 replicas (CPU 96% of its limit, target 70%); 2:15 api-4 start; 2:17 api-4 took traffic, 31.9 s after the decision; 2:30 api-5 start; 2:32 api-5 took traffic, 31.8 s after the decision.</sub>

## Diagnosis

### 1. Queueing knee (api CPU), from 1:40

From 1:40, p99 stayed above 13 ms (2.5 ms at low load) with api CPU at 99%. Near saturation requests wait behind each other, so latency grows non-linearly, not in proportion to load.

Evidence:

- median end-to-end p99 by api CPU: below 50%: 2.6 ms (8 windows); 50–70%: 2.5 ms (1 window); 70–80%: 3.2 ms (4 windows); 80–90%: 3.5 ms (3 windows); 90% and up: 59 ms (3 windows) <sub>(k6_http_req_duration_seconds (k6 histogram, end to end); container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- 1:40 to 2:10: p99 52 ms, 59 ms, 62 ms with api CPU 99%, 100%, 100% <sub>(k6_http_req_duration_seconds (k6 histogram, end to end); container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- api was CPU-throttled for 22.4 s (summed over its threads) in the window from 1:40, while its average CPU was 99% of its limit: bursts of work hit the 0.5-CPU limit and waited for the next period <sub>(container_cpu_cfs_throttled_seconds_total (cAdvisor))</sub>

**Try next:** Add capacity or cut per-request work on api, then re-run the ramp.

<sub>Rules version 22. Rules are deterministic and offline; their thresholds are listed below.</sub>

## Predictions

1 of 1 scored prediction held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 500 until load_rps >= 1000` | **held** | 3.3 ms | 0:00 to 1:20, when load_rps >= 1,000 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |

Loads and request rates depend on this machine: slower cores break the same design at a lower load, so a load guess can miss even when the reasoning behind it was right.

## Timeline

| Time | Event |
| --- | --- |
| 1:45 | autoscale: api 4 → 5 replicas (CPU 86% of its limit, target 70%) |
| 2:00 | autoscale: api 5 → 6 replicas (CPU 96% of its limit, target 70%) |
| 2:15 | container: api-4 start |
| 2:17 | replica ready: api-4 took traffic, 31.9 s after the decision |
| 2:30 | container: api-5 start |
| 2:32 | replica ready: api-5 took traffic, 31.8 s after the decision |

<sub>From events.jsonl: the fault timeline and Docker container events.</sub>

## Assumptions

### From the design

| Field | Value | Basis | Why |
| --- | --- | --- | --- |
| `services.api.behavior.cpu_ms` |  | guessed | a JSON handler with some work in it: a replica of 0.5 CPU serves about 470 requests a second |
| `workload.profile` |  | guessed | a front-page surge: nine times the usual traffic in under a minute |
| `services.api.autoscale.startup_s` |  | guessed | a new pod waits about 30 s to be scheduled and pull its image |

3 of these are guessed, not measured: the result depends on them.

### Chosen by Loadlore

| Field | Value | Why |
| --- | --- | --- |
| workload.timeout_ms | 10s | the default; k6 gives up on a request after this long and counts it as failed |
| workload.slo.p99_ms | 500 ms | the default; a break on p99 alone needs p99 at or above this as well as 10x the low-load p99, so latency users would still accept isn't called a break |
| network.profile | same-zone: 0.3 ms round trip | the default: every service in one availability zone. AWS says a round trip between two instances in the same zone is generally sub-millisecond with enhanced networking, and publishes no exact figure, so the 0.3 ms round trip Loadlore adds between any two containers is its own estimate, inside that bound. Applied between any two of the stack's containers, the load generator and the entry point included, as half the round trip on each side's outgoing packets; Loadlore's own tooling isn't slowed. Not scaled with --scale. Source: AWS Architecture Blog, Improving performance and reducing cost using Availability Zone Affinity: https://aws.amazon.com/blogs/architecture/improving-performance-and-reducing-cost-using-availability-zone-affinity/ |
| resources.cpu_period_ms | 100 ms for api, db, lb | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.api.autoscale | 70% CPU, 4 to 6 replicas (starts with 4), every 15 s, scale-up cooldown 0 s, scale-down cooldown 300 s | as a Kubernetes HorizontalPodAutoscaler: it wants ceil(replicas taking traffic × metric ÷ target), ignores changes within 10%, and goes on deciding while new replicas start, adding more if it wants more; it scales down only once none are starting. Unset values are Kubernetes' defaults: 15 s, no wait to scale up, 300 s before scaling down |
| services.api.autoscale.startup_s | 30 s | set in the design |
| services.db.disk | laptop: the Docker host's | no disk block: a read that misses db's memory goes to the Docker host's disk, which on a laptop mostly answers from the host's own memory, fast and uncapped, and a commit's fsync returns in a fraction of a millisecond. Add disk: { profile: gp3 } for a cloud volume's waits and caps |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |
| services.api.pools.db.min | 10 | set in the design: each replica opens 10 connections before the load starts and keeps them open |
| services.api.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- load generator: 4000 VUs started before the load, the scenario's measured prealloc_vus 4000 at --scale 1, more than 1800 rps × the SLO's 500 ms
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

- loadl dev, profile 3m15s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 966 (hash 928, memory 981, syscalls 989): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (966) was within 5.0% of the run it's compared with (970)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 218,974 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-featured-on-hacker-news-headroom-20261117-214545-api-0-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-headroom-20261117-214545-api-1-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-headroom-20261117-214545-api-2-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-headroom-20261117-214545-api-3-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-headroom-20261117-214545-api-4-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-headroom-20261117-214545-api-5-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-headroom-20261117-214545-db-1 | postgres:17.11 |
| loadlore-featured-on-hacker-news-headroom-20261117-214545-lb-1 | haproxy:3.2.26 |
| loadlore-featured-on-hacker-news-headroom-20261117-214545-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-featured-on-hacker-news-headroom-20261117-214545-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-featured-on-hacker-news-headroom-20261117-214545-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-featured-on-hacker-news-headroom-20261117-214545-lore-prometheus-1 | prom/prometheus:v3.16.0 |

</details>

## Per-window data

<details><summary>19 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | api replicas | db CPU | db conns | lb CPU | lb up | api→db pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 200 | 188 | 0 | 0% | 2.4 ms | 3.3 ms | 14% | 4 | 3.1% | 41 | 5.3% | 4 | 0/40 | 0.0 ms | 9.3% | 5.0% |
| 0:10 | 200 | 200 | 0 | 0% | 2.4 ms | 2.8 ms | 13% | 4 | 3.3% | 41 | 5.6% | 4 | 1/40 | 0.0 ms | 5.2% | 0% |
| 0:20 | 200 | 200 | 0 | 0% | 2.1 ms | 2.7 ms | 13% | 4 | 3.3% | 41 | 3.8% | 4 | 1/40 | 0.0 ms | 3.0% | 0% |
| 0:30 | 200 | 200 | 0 | 0% | 2.1 ms | 2.5 ms | 13% | 4 | 3.6% | 42 | 3.6% | 4 | 1/40 | 0.0 ms | 2.8% | 0% |
| 0:40 | 200 | 200 | 0 | 0% | 2.1 ms | 2.5 ms | 13% | 4 | 3.2% | 41 | 3.5% | 4 | 1/40 | 0.0 ms | 2.5% | 0% |
| 0:50 | 200 | 200 | 0 | 0% | 2.1 ms | 2.5 ms | 13% | 4 | 3.3% | 41 | 3.5% | 4 | 1/40 | 0.0 ms | 2.4% | 0% |
| 1:00 | 378 | 357 | 0 | 0% | 2.1 ms | 2.5 ms | 22% | 4 | 3.6% | 41 | 5.2% | 4 | 1/40 | 0.0 ms | 3.4% | 0% |
| 1:10 | 733 | 712 | 0 | 0% | 2.0 ms | 2.7 ms | 42% | 4 | 4.3% | 41 | 8.1% | 4 | 2/40 | 0.0 ms | 8.2% | 1.0% |
| 1:20 | 1,089 | 1,068 | 0 | 0% | 2.0 ms | 2.5 ms | 63% | 4 | 5.1% | 41 | 11% | 4 | 3/40 | 0.0 ms | 7.2% | 0% |
| 1:30 | 1,444 | 1,423 | 0 | 0% | 2.0 ms | 3.5 ms | 84% | 4 | 5.9% | 41 | 12% | 4 | 1/40 | 0.0 ms | 11% | 1.0% |
| 1:40 | 1,756 | 1,741 | 0 | 0% | 2.1 ms | 52 ms | 99% | 4 | 8.2% | 41 | 15% | 4 | 16/40 +10 | 0.3 ms | 13% | 1.0% |
| 1:50 | 1,800 | 1,800 | 0 | 0% | 4.1 ms | 59 ms | 100% | 4 | 8.9% | 41 | 16% | 4 | 21/40 +11 | 0.8 ms | 13% | 2.0% |
| 2:00 | 1,800 | 1,801 | 0 | 0% | 5.7 ms | 62 ms | 100% | 4 | 9.0% | 41 | 17% | 4 | 21/40 +15 | 0.9 ms | 13% | 1.9% |
| 2:10 | 1,800 | 1,803 | 0 | 0% | 2.2 ms | 61 ms | 82% | 5 | 8.9% | 51 | 17% | 4 | 22/50 +10 | 0.6 ms | 12% | 0.1% |
| 2:20 | 1,800 | 1,800 | 0 | 0% | 2.0 ms | 3.1 ms | 85% | 5 | 7.4% | 51 | 15% | 5 | 2/50 | 0.0 ms | 14% | 2.0% |
| 2:30 | 1,800 | 1,800 | 0 | 0% | 2.0 ms | 3.7 ms | 72% | 6 | 7.5% | 61 | 16% | 5 | 1/60 | 0.0 ms | 14% | 2.0% |
| 2:40 | 1,800 | 1,800 | 0 | 0% | 2.0 ms | 2.8 ms | 72% | 6 | 7.1% | 61 | 14% | 6 | 2/60 | 0.0 ms | 12% | 0% |
| 2:50 | 1,800 | 1,800 | 0 | 0% | 2.0 ms | 3.3 ms | 72% | 6 | 7.5% | 61 | 16% | 6 | 1/60 | 0.0 ms | 14% | 1.0% |
| 3:00 | 1,800 | 1,800 | 0 | 0% | 2.0 ms | 3.1 ms | 72% | 6 | 7.3% | 61 | 16% | 6 | 2/60 | 0.0 ms | 14% | 1.0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 CPU is against its budget.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: featured-on-hacker-news-headroom

# A small product's api, autoscaled on CPU as the platform's defaults
# suggest. It runs two replicas for its usual traffic. Then a post about
# the product reaches the front page of Hacker News.

services:
  lb:
    kind: lb
    target: api
    health_check: { interval_ms: 1000, fall: 3, rise: 2 }

  api:
    image: loadlore/synthetic
    replicas: 4   # was 2
    resources: { cpu: 0.5, memory: 256m }
    behavior:
      cpu_ms: 1
      calls:
        - { target: db, op: read }
      resilience:
        timeout_ms: 2000
    pools: { db: { size: 10, min: 10 } }   # opened when a replica starts
    autoscale: { min: 4, max: 6, metric: cpu, target: 0.7 }   # was min 2: room for a surge before it scales

  db:
    kind: postgres
    version: "17"
    resources: { cpu: 1, memory: 1g }
    config: { max_connections: 100 }
    dataset: { rows: 100000, row_bytes: 256 }

workload:
  entry: lb
  profile:
    - hold: { rps: 200, for: 1m }                  # a normal afternoon
    - ramp: { from: 200, to: 1800, over: 45s }    # the post reaches the front page
    - hold: { rps: 1800, for: 1m30s }

assumptions:
  - { field: services.api.behavior.cpu_ms, basis: guessed, why: "a JSON handler with some work in it: a replica of 0.5 CPU serves about 470 requests a second" }
  - { field: workload.profile, basis: guessed, why: "a front-page surge: nine times the usual traffic in under a minute" }
  - { field: services.api.autoscale.startup_s, basis: guessed, why: "a new pod waits about 30 s to be scheduled and pull its image" }

# Your predictions, scored in the report. The line here is an example;
# loadl predict design.yaml replaces it with your answers.
expect:
  - p99_ms < 500 until load_rps >= 1000
```
