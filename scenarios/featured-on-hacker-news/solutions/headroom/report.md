# featured-on-hacker-news-headroom: run report

_run `featured-on-hacker-news-headroom-20261117-152242` · completed · 17 Nov 2026 15:22 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 1,800 rps offered, 1,805 rps served at best, worst 10-second p99 62 ms, at most 0% errors.

Caveat: at 2:10 the load generator hit its CPU limit and was stalled in 2.0% of its scheduling periods (container_cpu_cfs_throttled_periods_total ÷ container_cpu_cfs_periods_total (cAdvisor)); work caught in a stall waits for the next 100 ms period, which can show up in p99.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 2:10

Measured in the 10-second window from 2:10 to 2:20 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 1,800 rps<br/>offered 1,800 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 18%<br/>4/6 replicas up"}}
    api["api ×4<br/>0.5 CPU · 256m<br/>autoscale 4–6 on CPU 70%<br/>CPU 68%<br/>in flight 39"]
    db[("db<br/>postgres 17<br/>1 CPU · 1g<br/>CPU 9.1%<br/>conns 44/100")]
    load -->|"90% read · 10% write<br/>1,805 rps · p99 62 ms"| lb
    lb -->|"round-robin<br/>checks every 1s, out after 3<br/>1,803 rps · p99 62 ms"| api
    api -->|"read<br/>pool 10<br/>1,803 calls/s · p99 14 ms<br/>in use 14/60, 1 waiting"| db
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb,api,db ok
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
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2000, 0, 0, 2000, 0, 0, 0, 0, 0]
    line [200, 200, 200, 200, 200, 200, 377.778, 733.333, 1088.889, 1444.444, 1755.556, 1800, 1800, 1800, 1800, 1800, 1800, 1800, 1800]
    line [189.323, 199.934, 199.984, 200.101, 200.004, 200.032, 359.253, 714.034, 1069.78, 1425.561, 1740.1, 1803.387, 1797.169, 1804.516, 1800.059, 1800.182, 1799.741, 1800.109, 1799.856]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 1:45 api 4 → 6 replicas (CPU 88% of its limit, target 70%); 2:15 api-5 start; 2:15 api-4 start; 2:17 api-4 took traffic, 32.0 s after the decision; 2:17 api-5 took traffic, 32.1 s after the decision.</sub>

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
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 80, 0, 0, 80, 0, 0, 0, 0, 0]
    line [2.366, 2.359, 2.104, 2.071, 2.072, 2.066, 2.04, 2.02, 1.994, 1.977, 2.052, 6.1, 5.801, 2.257, 1.969, 1.965, 1.968, 1.974, 1.967]
    line [2.657, 2.625, 2.465, 2.307, 2.306, 2.307, 2.288, 2.28, 2.249, 2.273, 41.159, 51.21, 51.462, 51.369, 2.285, 2.282, 2.294, 2.298, 2.272]
    line [3.25, 2.932, 2.751, 2.489, 2.492, 2.499, 2.44, 2.706, 2.46, 3.716, 55.66, 60.831, 61.494, 61.612, 3.003, 2.895, 3.199, 3.353, 2.82]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 1:45 api 4 → 6 replicas (CPU 88% of its limit, target 70%); 2:15 api-5 start; 2:15 api-4 start; 2:17 api-4 took traffic, 32.0 s after the decision; 2:17 api-5 took traffic, 32.1 s after the decision.</sub>

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
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 1, 0, 0, 0, 0, 0]
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 1:45 api 4 → 6 replicas (CPU 88% of its limit, target 70%); 2:15 api-5 start; 2:15 api-4 start; 2:17 api-4 took traffic, 32.0 s after the decision; 2:17 api-5 took traffic, 32.1 s after the decision.</sub>

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
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 100, 0, 0, 0, 0, 0]
    line [13.5, 13.417, 13.119, 12.983, 13.013, 13.038, 22.098, 42.438, 62.791, 83.673, 99.365, 99.999, 99.899, 68.409, 71.817, 71.691, 71.684, 71.882, 71.748]
    line [5.078, 3.427, 3.401, 3.532, 3.07, 3.103, 3.613, 4.35, 5.071, 6.336, 8.446, 9.631, 9.366, 9.134, 7.521, 7.319, 7.509, 7.375, 7.319]
    line [5.4, 5.834, 3.742, 3.485, 3.517, 3.488, 5.193, 9.136, 11.107, 13.411, 15.374, 17.405, 17.164, 17.793, 16.809, 15.453, 15.023, 15.804, 18.181]
```

<sub>Blue: api. Orange: db. Green: lb (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 1:45 api 4 → 6 replicas (CPU 88% of its limit, target 70%); 2:15 api-5 start; 2:15 api-4 start; 2:17 api-4 took traffic, 32.0 s after the decision; 2:17 api-5 took traffic, 32.1 s after the decision.</sub>

## Diagnosis

### 1. Connection storm (api → db), from 1:30

At 1:30, api's pools opened 27 connections to db within 10 seconds, at 1,444 rps offered, and p99 reached 56 ms (2.5 ms at low load). The pools open connections only when callers need them, so when a short slowdown leaves requests waiting, every replica opens more at once. Each new Postgres connection starts a server process and authenticates, which takes db's CPU from queries just when it is short of it.

Evidence:

- 27 new connections to db from 1:30 to 1:40: from 12 open to 39 of 40 <sub>(loadlore_pool_connect_seconds (new connections the pools opened, and how long each took))</sub>
- p99 56 ms at 1:40, against 2.5 ms at low load <sub>(k6_http_req_duration_seconds (k6 histogram, end to end))</sub>
- each new connection took 14 ms to open at 1:30, against 3.7 ms for the ones opened before <sub>(loadlore_pool_connect_seconds (new connections the pools opened, and how long each took))</sub>
- up to 6 requests waiting for a connection at 1:30 <sub>(loadlore_pool_pending (callers waiting for a connection))</sub>
- db CPU at 6.3% of its limit at 1:30 <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>

**Try next:** Open the connections before the load arrives: give the pool a minimum (pools: { db: { size: 10, min: 10 } }) and re-run to compare a warm pool with this cold one. PgBouncer in front of db also keeps server connections open.

### 2. Queueing knee (api CPU), from 1:40

From 1:40, p99 stayed above 12 ms (2.5 ms at low load) with api CPU at 99%. Near saturation requests wait behind each other, so latency grows non-linearly, not in proportion to load.

Evidence:

- median end-to-end p99 by api CPU: below 50%: 2.6 ms (8 windows); 50–70%: 32 ms (2 windows); 70–80%: 3.0 ms (5 windows); 80–90%: 3.7 ms (1 window); 90% and up: 61 ms (3 windows) <sub>(k6_http_req_duration_seconds (k6 histogram, end to end); container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- 1:40 to 2:10: p99 56 ms, 61 ms, 61 ms with api CPU 99%, 100%, 100% <sub>(k6_http_req_duration_seconds (k6 histogram, end to end); container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- api was CPU-throttled for 23.1 s (summed over its threads) in the window from 1:40, while its average CPU was 99% of its limit: bursts of work hit the 0.5-CPU limit and waited for the next period <sub>(container_cpu_cfs_throttled_seconds_total (cAdvisor))</sub>

**Try next:** Add capacity or cut per-request work on api, then re-run the ramp.

<sub>Rules version 19. Rules are deterministic and offline; their thresholds are listed below.</sub>

## Predictions

1 of 1 scored prediction held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 500 until load_rps >= 1000` | **held** | 3.3 ms | 0:00 to 1:20, when load_rps >= 1,000 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |

Loads and request rates depend on this machine: slower cores break the same design at a lower load, so a load guess can miss even when the reasoning behind it was right.

## Timeline

| Time | Event |
| --- | --- |
| 1:45 | autoscale: api 4 → 6 replicas (CPU 88% of its limit, target 70%) |
| 2:15 | container: api-5 start |
| 2:15 | container: api-4 start |
| 2:17 | replica ready: api-4 took traffic, 32.0 s after the decision |
| 2:17 | replica ready: api-5 took traffic, 32.1 s after the decision |

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
| services.api.pools.db.min | 0 | the default, as pgxpool's: each replica opens connections only when callers need them, so a pool fills as load rises, and a burst opens many at once |
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

## Environment

- loadl dev, profile 3m15s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 987 (hash 932, memory 1016, syscalls 1015): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (987) was within 5.0% of the run it's compared with (983)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 218,973 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-featured-on-hacker-news-headroom-20261117-152242-api-0-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-headroom-20261117-152242-api-1-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-headroom-20261117-152242-api-2-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-headroom-20261117-152242-api-3-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-headroom-20261117-152242-api-4-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-headroom-20261117-152242-api-5-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-headroom-20261117-152242-db-1 | postgres:17.11 |
| loadlore-featured-on-hacker-news-headroom-20261117-152242-lb-1 | haproxy:3.2.26 |
| loadlore-featured-on-hacker-news-headroom-20261117-152242-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-featured-on-hacker-news-headroom-20261117-152242-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-featured-on-hacker-news-headroom-20261117-152242-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-featured-on-hacker-news-headroom-20261117-152242-lore-prometheus-1 | prom/prometheus:v3.16.0 |

</details>

## Per-window data

<details><summary>19 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | api replicas | db CPU | db conns | lb CPU | lb up | api→db pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 200 | 189 | 0 | 0% | 2.4 ms | 3.3 ms | 14% | 4 | 5.1% | 6 | 5.4% | 4 | 0/40 | 0.0 ms | 8.9% | 4.0% |
| 0:10 | 200 | 200 | 0 | 0% | 2.4 ms | 2.9 ms | 13% | 4 | 3.4% | 7 | 5.8% | 4 | 1/40 | 0.0 ms | 3.7% | 0% |
| 0:20 | 200 | 200 | 0 | 0% | 2.1 ms | 2.8 ms | 13% | 4 | 3.4% | 9 | 3.7% | 4 | 1/40 | 0.0 ms | 4.5% | 0% |
| 0:30 | 200 | 200 | 0 | 0% | 2.1 ms | 2.5 ms | 13% | 4 | 3.5% | 9 | 3.5% | 4 | 1/40 | 0.0 ms | 2.5% | 0% |
| 0:40 | 200 | 200 | 0 | 0% | 2.1 ms | 2.5 ms | 13% | 4 | 3.1% | 9 | 3.5% | 4 | 1/40 | 0.0 ms | 2.3% | 0% |
| 0:50 | 200 | 200 | 0 | 0% | 2.1 ms | 2.5 ms | 13% | 4 | 3.1% | 9 | 3.5% | 4 | 1/40 | 0.0 ms | 2.3% | 0% |
| 1:00 | 378 | 359 | 0 | 0% | 2.0 ms | 2.4 ms | 22% | 4 | 3.6% | 9 | 5.2% | 4 | 1/40 | 0.0 ms | 3.5% | 0% |
| 1:10 | 733 | 714 | 0 | 0% | 2.0 ms | 2.7 ms | 42% | 4 | 4.4% | 9 | 9.1% | 4 | 0/40 | 0.0 ms | 8.2% | 1.9% |
| 1:20 | 1,089 | 1,070 | 0 | 0% | 2.0 ms | 2.5 ms | 63% | 4 | 5.1% | 13 | 11% | 4 | 1/40 | 0.0 ms | 7.4% | <0.1% |
| 1:30 | 1,444 | 1,426 | 0 | 0% | 2.0 ms | 3.7 ms | 84% | 4 | 6.3% | 40 | 13% | 4 | 3/40 +6 | 0.0 ms | 11% | 1.0% |
| 1:40 | 1,756 | 1,740 | 0 | 0% | 2.1 ms | 56 ms | 99% | 4 | 8.4% | 41 | 15% | 4 | 19/40 | 0.4 ms | 13% | 1.0% |
| 1:50 | 1,800 | 1,803 | 0 | 0% | 6.1 ms | 61 ms | 100% | 4 | 9.6% | 41 | 17% | 4 | 18/40 +8 | 0.9 ms | 14% | 1.0% |
| 2:00 | 1,800 | 1,797 | 0 | 0% | 5.8 ms | 61 ms | 100% | 4 | 9.4% | 41 | 17% | 4 | 23/40 +8 | 0.9 ms | 12% | 0% |
| 2:10 | 1,800 | 1,805 | 0 | 0% | 2.3 ms | 62 ms | 68% | 6 | 9.1% | 44 | 18% | 4 | 14/60 +1 | 0.7 ms | 15% | 2.0% |
| 2:20 | 1,800 | 1,800 | 0 | 0% | 2.0 ms | 3.0 ms | 72% | 6 | 7.5% | 46 | 17% | 6 | 5/60 | 0.0 ms | 14% | 1.0% |
| 2:30 | 1,800 | 1,800 | 0 | 0% | 2.0 ms | 2.9 ms | 72% | 6 | 7.3% | 46 | 15% | 6 | 2/60 | 0.0 ms | 12% | 0% |
| 2:40 | 1,800 | 1,800 | 0 | 0% | 2.0 ms | 3.2 ms | 72% | 6 | 7.5% | 51 | 15% | 6 | 2/60 | 0.0 ms | 14% | 1.0% |
| 2:50 | 1,800 | 1,800 | 0 | 0% | 2.0 ms | 3.4 ms | 72% | 6 | 7.4% | 51 | 16% | 6 | 4/60 | 0.0 ms | 14% | 1.0% |
| 3:00 | 1,800 | 1,800 | 0 | 0% | 2.0 ms | 2.8 ms | 72% | 6 | 7.3% | 51 | 18% | 6 | 3/60 +1 | 0.0 ms | 12% | <0.1% |

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
    pools: { db: 10 }
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
