# when-to-scale-your-database-one-query-per-page: run report

_run `when-to-scale-your-database-one-query-per-page-20261117-213423` · completed · 17 Nov 2026 21:34 UTC · scale 1 · seed 1_

## Verdict

**Broke.** Broke 5:10 into the load, at 3,500 rps offered: p99 reached 648 ms (2.5 ms at low load). Most saturated then: api → db pool 92 of 96 connections in use, waiting on db (CPU 84%, CPU-throttled 20.3 s, 8.8 ms per call with a connection, 0.4 ms at low load). After the break the load generator couldn't send 3.1% of the arrivals the profile asked for (10,745 arrivals from 5:20; k6_dropped_iterations_total (k6: arrivals it couldn't start)): slow responses held its 4,000 VUs, so served throughput from then on is partly the generator's limit, not only the system's. It hadn't recovered when the load ended.

Caveat: at 5:00 the load generator hit its CPU limit and was stalled in 3.0% of its scheduling periods (container_cpu_cfs_throttled_periods_total ÷ container_cpu_cfs_periods_total (cAdvisor)); work caught in a stall waits for the next 100 ms period, which can show up in p99.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); saturation: loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections. Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 5:10

Measured in the 10-second window from 5:10 to 5:20 (the break). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 3,500 rps<br/>offered 3,500 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 41%<br/>4/4 replicas up"}}
    api["api ×4<br/>0.8 CPU · 512m<br/>CPU 72%<br/>in flight 678"]
    db[("db<br/>postgres 17<br/>0.4 CPU · 1g<br/>CPU 84%<br/>conns 97/100")]
    load -->|"90% read · 10% write<br/>3,357 rps · p99 648 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>3,341 rps · p99 697 ms"| api
    api -->|"read batch 3, write on writes<br/>pool 24, min 24<br/>3,678 calls/s · p99 338 ms<br/>in use 92/96, 585 waiting"| db
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb ok
    class api hot
    class db warm
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
    x-axis "seconds" 0 --> 410
    y-axis "requests/s" 0 --> 4000
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [156.667, 270, 383.333, 496.667, 610, 723.333, 836.667, 950, 1063.333, 1176.667, 1290, 1403.333, 1516.667, 1630, 1743.333, 1856.667, 1970, 2083.333, 2196.667, 2310, 2423.333, 2536.667, 2650, 2763.333, 2876.667, 2990, 3103.333, 3216.667, 3330, 3443.333, 3500, 3500, 3500, 3500, 3500, 3500, 3500, 3500, 3500, 3500, 3500, 3500]
    line [150.42, 266.563, 379.943, 493.365, 606.558, 719.998, 833.229, 946.645, 1060.163, 1172.965, 1286.646, 1400.078, 1513.207, 1626.757, 1739.772, 1853.239, 1966.828, 2079.943, 2193.017, 2306.696, 2419.962, 2533.158, 2646.774, 2760.083, 2873.282, 2986.372, 3100.139, 3213.415, 3326.544, 3440.126, 3499.672, 3356.79, 3416.715, 3655.522, 3499.944, 3114.015, 3413.378, 3369.133, 3295.597, 3182.919, 3412.345, 3334.256]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 5:10 break.</sub>

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
    x-axis "seconds" 0 --> 410
    y-axis "ms" 0 --> 2000
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [1.973, 1.641, 1.592, 1.572, 1.55, 1.535, 1.411, 1.414, 1.413, 1.407, 1.403, 1.397, 1.391, 1.386, 1.384, 1.384, 1.379, 1.381, 1.381, 1.381, 1.388, 1.385, 1.393, 1.403, 1.417, 1.445, 1.474, 1.497, 1.521, 1.569, 1.583, 1.747, 538.594, 45.46, 1.591, 1.614, 721.31, 730.472, 928.208, 1211.519, 1125.975, 1161.405]
    line [2.724, 2.298, 2.119, 2.071, 2.054, 97.639, 1.887, 1.922, 1.912, 1.891, 1.871, 1.851, 1.854, 1.856, 1.87, 1.882, 1.893, 1.918, 1.891, 1.901, 1.922, 1.926, 1.946, 1.973, 2.005, 2.052, 2.132, 3.16, 2.294, 5.728, 3.34, 361.792, 887.122, 702.871, 11.621, 712.894, 945.738, 991.22, 1218.431, 1613.634, 1537.769, 1411.072]
    line [3.777, 2.716, 2.478, 2.441, 2.425, 360.771, 2.23, 2.443, 2.582, 2.289, 2.237, 2.207, 2.172, 2.194, 2.291, 2.424, 2.473, 2.746, 2.633, 2.477, 2.594, 2.769, 2.677, 2.795, 2.748, 2.821, 2.954, 28.235, 3.945, 29.951, 19.52, 647.599, 1015.781, 838.873, 49.813, 838.755, 1073.479, 1128.174, 1320.863, 1765.541, 1683.369, 1563.958]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 5:10 break.</sub>

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
    x-axis "seconds" 0 --> 410
    y-axis "% of requests" 0 --> 1
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 5:10 break.</sub>

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
    x-axis "seconds" 0 --> 410
    y-axis "% of limit" 0 --> 100
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [4.288, 6.445, 8.251, 10.437, 12.555, 16.324, 16.913, 19.204, 21.372, 23.66, 25.926, 28.042, 30.254, 32.64, 34.941, 37.275, 39.648, 42.084, 44.205, 46.575, 48.938, 51.218, 53.671, 56.146, 58.582, 61.968, 65.457, 67.4, 70.404, 73.194, 74.701, 72.059, 66.865, 72.392, 74.009, 68.512, 65.153, 64.513, 63.895, 61.98, 65.139, 64.513]
    line [9.351, 11.718, 11.476, 12.372, 13.672, 26.603, 17.31, 18.157, 18.98, 20.388, 21.266, 23.137, 23.899, 25.101, 26.96, 27.382, 29.142, 30.667, 32.132, 33.061, 34.879, 36.5, 38.118, 41.469, 43.725, 48.524, 54.327, 62.504, 66.444, 72.937, 75.957, 84.411, 100.159, 91.55, 76.064, 80.983, 99.933, 100.166, 99.729, 99.884, 99.884, 100.129]
    line [4.834, 4.181, 4.869, 6.034, 6.93, 10.286, 6.218, 6.985, 9.441, 12.532, 13.599, 14.461, 15.5, 16.598, 17.628, 18.478, 19.8, 19.802, 20.148, 22.026, 23.842, 22.138, 25.152, 25.192, 23.951, 31.845, 37.49, 31.714, 34.261, 37.347, 39.443, 41.382, 39.679, 39.039, 39.232, 44.409, 39.167, 38.855, 40.369, 39.459, 39.232, 38.732]
```

<sub>Blue: api. Orange: db. Green: lb (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 5:10 break.</sub>

## Diagnosis

### 1. Queueing knee (db CPU), from 4:50

From 4:50, p99 stayed above 12 ms (2.5 ms at low load) with db CPU at 73%. Near saturation requests wait behind each other, so latency grows non-linearly, not in proportion to load.

Evidence:

- median end-to-end p99 by db CPU: below 50%: 2.5 ms (26 windows); 50–70%: 3.9 ms (3 windows); 70–80%: 30 ms (3 windows); 80–90%: 743 ms (2 windows); 90% and up: 1.22 s (8 windows) <sub>(k6_http_req_duration_seconds (k6 histogram, end to end); container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- 4:50 to 5:20: p99 30 ms, 20 ms, 648 ms with db CPU 73%, 76%, 84% <sub>(k6_http_req_duration_seconds (k6 histogram, end to end); container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- api CPU at 73% of its limit at 4:50: not where requests queued, since most of their time went to calls to db <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- api's requests spent 78% of their time in calls to db at 5:10: 1.1 calls per request × 45 ms each, against 63 ms per request <sub>(loadlore_calls_total (the caller's outbound call attempts); loadlore_call_duration_seconds (the caller's histogram, pool wait included); loadlore_request_duration_seconds (the service's own histogram))</sub>
- db CPU at 84% of its limit at 5:10 <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- api's calls to db took 8.8 ms each once they had a connection at 5:10, against 0.4 ms at low load <sub>(loadlore_call_duration_seconds minus loadlore_pool_wait_seconds, per call (time with a connection))</sub>
- db was CPU-throttled for 20.3 s (summed over its threads) in the window from 5:10, while its average CPU was 84% of its limit: bursts of work hit the 0.4-CPU limit and waited for the next period <sub>(container_cpu_cfs_throttled_seconds_total (cAdvisor))</sub>
- db was CPU-throttled for 3.1 s (summed over its threads) in the window from 4:50, while its average CPU was 73% of its limit: bursts of work hit the 0.4-CPU limit and waited for the next period <sub>(container_cpu_cfs_throttled_seconds_total (cAdvisor))</sub>

**Try next:** Add capacity or cut per-request work on db, then re-run the ramp.

<sub>Rules version 22. Rules are deterministic and offline; their thresholds are listed below.</sub>

## Predictions

1 of 2 scored predictions held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 300 until load_rps >= 3000` | **broken** at 0:50 | 361 ms | 0:00 to 4:20, when load_rps >= 3,000 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |
| `error_rate < 1% throughout` | **held** | 0% | the whole run | source: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6) |

A broken prediction isn't a wrong answer: it shows where your picture of the system and the run differ, and that gap is what to look into. Loads and request rates depend on this machine: slower cores break the same design at a lower load, so a load guess can miss even when the reasoning behind it was right.

## Assumptions

### From the design

The design lists no assumptions.

### Chosen by Loadlore

| Field | Value | Why |
| --- | --- | --- |
| workload.timeout_ms | 10s | the default; k6 gives up on a request after this long and counts it as failed |
| workload.slo.p99_ms | 500 ms | the default; a break on p99 alone needs p99 at or above this as well as 10x the low-load p99, so latency users would still accept isn't called a break |
| network.profile | same-zone: 0.3 ms round trip | the default: every service in one availability zone. AWS says a round trip between two instances in the same zone is generally sub-millisecond with enhanced networking, and publishes no exact figure, so the 0.3 ms round trip Loadlore adds between any two containers is its own estimate, inside that bound. Applied between any two of the stack's containers, the load generator and the entry point included, as half the round trip on each side's outgoing packets; Loadlore's own tooling isn't slowed. Not scaled with --scale. Source: AWS Architecture Blog, Improving performance and reducing cost using Availability Zone Affinity: https://aws.amazon.com/blogs/architecture/improving-performance-and-reducing-cost-using-availability-zone-affinity/ |
| resources.cpu_period_ms | 100 ms for api, db, lb | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.db.disk | laptop: the Docker host's | no disk block: a read that misses db's memory goes to the Docker host's disk, which on a laptop mostly answers from the host's own memory, fast and uncapped, and a commit's fsync returns in a fraction of a millisecond. Add disk: { profile: gp3 } for a cloud volume's waits and caps |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |
| services.api.pools.db.min | 24 | set in the design: each replica opens 24 connections before the load starts and keeps them open |
| services.api.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- Loadlore's own containers get fixed budgets: load generator 1.5 CPU (peak 3500 rps), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each. No fault acts on a connection, so every connection is direct and there is no Toxiproxy

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

- loadl dev, profile 7m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 974 (hash 934, memory 1011, syscalls 979): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (974) was within 5.0% of the run it's compared with (993)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 949,224 requests, 10,746 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-when-to-scale-your-database-one-query-per-page-20261117-213423-api-0-1 | loadlore/synthetic:dev |
| loadlore-when-to-scale-your-database-one-query-per-page-20261117-213423-api-1-1 | loadlore/synthetic:dev |
| loadlore-when-to-scale-your-database-one-query-per-page-20261117-213423-api-2-1 | loadlore/synthetic:dev |
| loadlore-when-to-scale-your-database-one-query-per-page-20261117-213423-api-3-1 | loadlore/synthetic:dev |
| loadlore-when-to-scale-your-database-one-query-per-page-20261117-213423-db-1 | postgres:17.11 |
| loadlore-when-to-scale-your-database-one-query-per-page-20261117-213423-lb-1 | haproxy:3.2.26 |
| loadlore-when-to-scale-your-database-one-query-per-page-20261117-213423-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-when-to-scale-your-database-one-query-per-page-20261117-213423-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-when-to-scale-your-database-one-query-per-page-20261117-213423-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-when-to-scale-your-database-one-query-per-page-20261117-213423-lore-prometheus-1 | prom/prometheus:v3.16.0 |

</details>

## Per-window data

<details><summary>42 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | lb CPU | lb up | api→db pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 157 | 150 | 0 | 0% | 2.0 ms | 3.8 ms | 4.3% | 9.4% | 97 | 4.8% | 4 | 1/96 | 0.0 ms | 6.2% | 2.0% |
| 0:10 | 270 | 267 | 0 | 0% | 1.6 ms | 2.7 ms | 6.4% | 12% | 97 | 4.2% | 4 | 1/96 | 0.0 ms | 3.3% | 0% |
| 0:20 | 383 | 380 | 0 | 0% | 1.6 ms | 2.5 ms | 8.3% | 11% | 97 | 4.9% | 4 | 1/96 | 0.0 ms | 4.2% | 0% |
| 0:30 | 497 | 493 | 0 | 0% | 1.6 ms | 2.4 ms | 10% | 12% | 97 | 6.0% | 4 | 1/96 | 0.0 ms | 4.9% | 0% |
| 0:40 | 610 | 607 | 0 | 0% | 1.5 ms | 2.4 ms | 13% | 14% | 97 | 6.9% | 4 | 1/96 | 0.0 ms | 4.6% | 0% |
| 0:50 | 723 | 720 | 0 | 0% | 1.5 ms | 361 ms | 16% | 27% | 97 | 10% | 4 | 49/96 +55 | 7.5 ms | 9.2% | 0% |
| 1:00 | 837 | 833 | 0 | 0% | 1.4 ms | 2.2 ms | 17% | 17% | 97 | 6.2% | 4 | 1/96 | 0.0 ms | 7.6% | 0% |
| 1:10 | 950 | 947 | 0 | 0% | 1.4 ms | 2.4 ms | 19% | 18% | 97 | 7.0% | 4 | 2/96 | 0.0 ms | 7.9% | 0% |
| 1:20 | 1,063 | 1,060 | 0 | 0% | 1.4 ms | 2.6 ms | 21% | 19% | 97 | 9.4% | 4 | 2/96 | 0.0 ms | 8.5% | 0% |
| 1:30 | 1,177 | 1,173 | 0 | 0% | 1.4 ms | 2.3 ms | 24% | 20% | 97 | 13% | 4 | 2/96 | 0.0 ms | 8.9% | 0% |
| 1:40 | 1,290 | 1,287 | 0 | 0% | 1.4 ms | 2.2 ms | 26% | 21% | 97 | 14% | 4 | 2/96 | 0.0 ms | 9.5% | 0% |
| 1:50 | 1,403 | 1,400 | 0 | 0% | 1.4 ms | 2.2 ms | 28% | 23% | 97 | 14% | 4 | 2/96 | 0.0 ms | 9.3% | 0% |
| 2:00 | 1,517 | 1,513 | 0 | 0% | 1.4 ms | 2.2 ms | 30% | 24% | 97 | 15% | 4 | 2/96 | 0.0 ms | 11% | 0% |
| 2:10 | 1,630 | 1,627 | 0 | 0% | 1.4 ms | 2.2 ms | 33% | 25% | 97 | 17% | 4 | 2/96 | 0.0 ms | 10% | 0% |
| 2:20 | 1,743 | 1,740 | 0 | 0% | 1.4 ms | 2.3 ms | 35% | 27% | 97 | 18% | 4 | 2/96 | 0.0 ms | 12% | 0% |
| 2:30 | 1,857 | 1,853 | 0 | 0% | 1.4 ms | 2.4 ms | 37% | 27% | 97 | 18% | 4 | 2/96 | 0.0 ms | 12% | 0% |
| 2:40 | 1,970 | 1,967 | 0 | 0% | 1.4 ms | 2.5 ms | 40% | 29% | 97 | 20% | 4 | 1/96 | 0.0 ms | 13% | 0% |
| 2:50 | 2,083 | 2,080 | 0 | 0% | 1.4 ms | 2.7 ms | 42% | 31% | 97 | 20% | 4 | 2/96 | 0.0 ms | 13% | 0% |
| 3:00 | 2,197 | 2,193 | 0 | 0% | 1.4 ms | 2.6 ms | 44% | 32% | 97 | 20% | 4 | 2/96 | 0.0 ms | 14% | 0% |
| 3:10 | 2,310 | 2,307 | 0 | 0% | 1.4 ms | 2.5 ms | 47% | 33% | 97 | 22% | 4 | 2/96 | 0.0 ms | 14% | 1.0% |
| 3:20 | 2,423 | 2,420 | 0 | 0% | 1.4 ms | 2.6 ms | 49% | 35% | 97 | 24% | 4 | 3/96 | 0.0 ms | 15% | 0% |
| 3:30 | 2,537 | 2,533 | 0 | 0% | 1.4 ms | 2.8 ms | 51% | 37% | 97 | 22% | 4 | 2/96 | 0.0 ms | 17% | 0% |
| 3:40 | 2,650 | 2,647 | 0 | 0% | 1.4 ms | 2.7 ms | 54% | 38% | 97 | 25% | 4 | 3/96 | 0.0 ms | 17% | 0% |
| 3:50 | 2,763 | 2,760 | 0 | 0% | 1.4 ms | 2.8 ms | 56% | 41% | 97 | 25% | 4 | 3/96 | 0.0 ms | 19% | 1.0% |
| 4:00 | 2,877 | 2,873 | 0 | 0% | 1.4 ms | 2.7 ms | 59% | 44% | 97 | 24% | 4 | 4/96 | 0.0 ms | 20% | 0% |
| 4:10 | 2,990 | 2,986 | 0 | 0% | 1.4 ms | 2.8 ms | 62% | 49% | 97 | 32% | 4 | 2/96 | 0.0 ms | 21% | 0% |
| 4:20 | 3,103 | 3,100 | 0 | 0% | 1.5 ms | 3.0 ms | 65% | 54% | 97 | 37% | 4 | 2/96 | 0.0 ms | 25% | 0.8% |
| 4:30 | 3,217 | 3,213 | 0 | 0% | 1.5 ms | 28 ms | 67% | 63% | 97 | 32% | 4 | 36/96 +3 | 0.1 ms | 26% | 1.2% |
| 4:40 | 3,330 | 3,327 | 0 | 0% | 1.5 ms | 3.9 ms | 70% | 66% | 97 | 34% | 4 | 4/96 | 0.0 ms | 28% | 0.5% |
| 4:50 | 3,443 | 3,440 | 0 | 0% | 1.6 ms | 30 ms | 73% | 73% | 97 | 37% | 4 | 22/96 +5 | 0.1 ms | 30% | 2.5% |
| 5:00 | 3,500 | 3,500 | 0 | 0% | 1.6 ms | 20 ms | 75% | 76% | 97 | 39% | 4 | 10/96 +2 | 0.1 ms | 33% | 3.0% |
| 5:10 | 3,500 | 3,357 | 0 | 0% | 1.7 ms | 648 ms | 72% | 84% | 97 | 41% | 4 | 92/96 +585 | 36 ms | 29% | 2.0% |
| 5:20 | 3,500 | 3,417 | 706 | 0% | 539 ms | 1.02 s | 67% | 100% | 97 | 40% | 4 | 96/96 +551 | 142 ms | 27% | 2.0% |
| 5:30 | 3,500 | 3,656 | 0 | 0% | 45 ms | 839 ms | 72% | 92% | 97 | 39% | 4 | 96/96 +528 | 51 ms | 27% | 1.0% |
| 5:40 | 3,500 | 3,500 | 0 | 0% | 1.6 ms | 50 ms | 74% | 76% | 97 | 39% | 4 | 39/96 +28 | 0.6 ms | 31% | 1.3% |
| 5:50 | 3,500 | 3,114 | 1,667 | 0% | 1.6 ms | 839 ms | 69% | 81% | 97 | 44% | 4 | 92/96 +703 | 51 ms | 36% | 6.7% |
| 6:00 | 3,500 | 3,413 | 797 | 0% | 721 ms | 1.07 s | 65% | 100% | 97 | 39% | 4 | 95/96 +706 | 188 ms | 29% | 3.1% |
| 6:10 | 3,500 | 3,369 | 760 | 0% | 730 ms | 1.13 s | 65% | 100% | 97 | 39% | 4 | 96/96 +704 | 188 ms | 28% | 2.9% |
| 6:20 | 3,500 | 3,296 | 1,540 | 0% | 928 ms | 1.32 s | 64% | 100% | 97 | 40% | 4 | 96/96 +704 | 193 ms | 31% | 5.0% |
| 6:30 | 3,500 | 3,183 | 2,831 | 0% | 1.21 s | 1.77 s | 62% | 100% | 97 | 39% | 4 | 95/96 +705 | 201 ms | 29% | 3.0% |
| 6:40 | 3,500 | 3,412 | 1,273 | 0% | 1.13 s | 1.68 s | 65% | 100% | 97 | 39% | 4 | 95/96 +705 | 187 ms | 29% | 4.0% |
| 6:50 | 3,500 | 3,334 | 1,170 | 0% | 1.16 s | 1.56 s | 65% | 100% | 97 | 39% | 4 | 92/96 +705 | 190 ms | 28% | 3.0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 CPU is against its budget.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: when-to-scale-your-database-one-query-per-page

# A feed service: every request returns a page of three items from Postgres,
# and one request in ten also updates an item.

services:
  lb:
    kind: lb
    target: api
    health_check: { interval_ms: 2000, fall: 3, rise: 2 }

  api:
    image: loadlore/synthetic
    replicas: 4
    resources: { cpu: 0.8, memory: 512m }
    behavior:
      cpu_ms: 0.5
      calls:
        - { target: db, op: read,  query: batch, rows: 3 }  # the page's three items in one query
        - { target: db, op: write, count: 1, when: write }
    pools: { db: { size: 24, min: 24 } }   # opened at start; 4 replicas x 24 = 96 connections, under max_connections

  db:
    kind: postgres
    version: "17"
    resources: { cpu: 0.4, memory: 1g }
    config: { max_connections: 100, shared_buffers: 256MB }
    dataset: { rows: 5000000, row_bytes: 512 }

workload:
  entry: lb
  profile:
    - ramp: { from: 100, to: 3500, over: 5m }
    - hold: { rps: 3500, for: 2m }
  mix: { read: 0.9, write: 0.1 }
  keys: { distribution: zipf, skew: 1.1, count: 5000000 }

# The starting design's predictions, kept so the two reports compare.
expect:
  - p99_ms < 300 until load_rps >= 3000
  - error_rate < 1% throughout
```
