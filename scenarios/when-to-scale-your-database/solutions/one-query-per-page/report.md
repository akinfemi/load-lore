# when-to-scale-your-database-one-query-per-page: run report

_run `when-to-scale-your-database-one-query-per-page-20261118-072056` · completed · 18 Nov 2026 07:21 UTC · scale 1 · seed 1_

## Verdict

**Broke.** Broke 5:10 into the load, at 3,500 rps offered: p99 reached 940 ms (5.7 ms at low load). Most saturated then: db CPU at 100% of its limit. After the break the load generator couldn't send 2.7% of the arrivals the profile asked for (9,452 arrivals from 5:20; k6_dropped_iterations_total (k6: arrivals it couldn't start)): slow responses held its 4,000 VUs, so served throughput from then on is partly the generator's limit, not only the system's. It hadn't recovered when the load ended.

Caveat: at 4:30 the load generator hit its CPU limit and was stalled in 3.0% of its scheduling periods (container_cpu_cfs_throttled_periods_total ÷ container_cpu_cfs_periods_total (cAdvisor)); work caught in a stall waits for the next 100 ms period, which can show up in p99.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); saturation: container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit. Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 5:10

Measured in the 10-second window from 5:10 to 5:20 (the break). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 3,500 rps<br/>offered 3,500 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 40%<br/>4/4 replicas up"}}
    api["api ×4<br/>0.8 CPU · 512m<br/>CPU 64%<br/>in flight 799"]
    db[("db<br/>postgres 17<br/>0.4 CPU · 1g<br/>disk gp3 · 3,000 IOPS<br/>CPU 100%<br/>conns 97/100")]
    load -->|"90% read · 10% write<br/>3,298 rps · p99 940 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>3,301 rps · p99 979 ms"| api
    api -->|"read batch 3, write on writes<br/>pool 24, min 24<br/>3,628 calls/s · p99 382 ms<br/>in use 92/96, 703 waiting"| db
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb ok
    class api,db hot
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
    line [0, 0, 0, 0, 0, 0, 0, 4000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [156.667, 270, 383.333, 496.667, 610, 723.333, 836.667, 950, 1063.333, 1176.667, 1290, 1403.333, 1516.667, 1630, 1743.333, 1856.667, 1970, 2083.333, 2196.667, 2310, 2423.333, 2536.667, 2650, 2763.333, 2876.667, 2990, 3103.333, 3216.667, 3330, 3443.333, 3500, 3500, 3500, 3500, 3500, 3500, 3500, 3500, 3500, 3500, 3500, 3500]
    line [150.274, 266.606, 379.795, 493.34, 606.349, 719.929, 833.158, 946.643, 1059.684, 1173.2, 1286.698, 1399.891, 1513.13, 1626.527, 1740.08, 1852.931, 1966.669, 2079.884, 2193.089, 2306.533, 2419.991, 2533.159, 2646.736, 2759.45, 2873.463, 2986.549, 3097.242, 3215.811, 3326.549, 3439.886, 3390.529, 3298.381, 3581.156, 3410.704, 3029.284, 3037.437, 3580.251, 3592.844, 3526.514, 3619.326, 3085.939, 3418.421]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 1:10 stall; 5:10 break.</sub>

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
    y-axis "ms" 0 --> 2500
    line [0, 0, 0, 0, 0, 0, 0, 2500, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2500, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [2.409, 1.807, 1.596, 1.581, 1.55, 1.517, 1.501, 1.537, 1.757, 1.452, 1.413, 1.474, 1.464, 1.4, 1.398, 1.417, 1.408, 1.427, 1.457, 1.445, 1.465, 1.487, 1.493, 1.594, 1.615, 1.594, 1.738, 1.705, 1.773, 1.842, 2.151, 515.501, 538.681, 79.203, 879.194, 1250.566, 1159.158, 424.149, 471.222, 52.022, 83.464, 1420.204]
    line [5.409, 4.538, 4.204, 4.244, 4.14, 4.156, 4.124, 223.699, 110.077, 4.187, 4.029, 4.403, 30.365, 3.959, 4.046, 5.955, 4.22, 4.128, 5.109, 4.446, 4.458, 4.545, 4.599, 6.562, 38.059, 5.438, 44.558, 8.14, 25.099, 30.472, 57.278, 779.454, 805.986, 555.227, 1735.822, 2177.891, 2006.118, 1248.874, 1063.847, 823.27, 1470.246, 1770.178]
    line [7.157, 5.776, 5.852, 5.675, 5.386, 5.377, 5.456, 513.878, 335.752, 5.912, 5.364, 7.022, 224.755, 5.313, 5.446, 116.363, 5.73, 5.706, 66.01, 6.366, 6.011, 6.403, 6.583, 45.633, 111.874, 12.961, 87.933, 36.54, 54.1, 56.242, 100.02, 939.832, 934.93, 714.093, 1918.3, 2378.344, 2292.249, 1398.951, 1247.144, 967.458, 1666.484, 1942.761]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 1:10 stall; 5:10 break.</sub>

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
    line [0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0.017, 0.024, 0.016, 0, 0, 0, 0, 0.013, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 1:10 stall; 5:10 break.</sub>

| Service | Failed requests | Share of its failures | Cause | From calls to | First seen | What it means |
| --- | --- | --- | --- | --- | --- | --- |
| api | 22 | 100% | `timeout` | db | 5:30 | calls to db ran past their timeout |

<sub>Why requests failed, over the whole run (loadlore_request_errors_total and loadlore_call_errors_total, by cause). A request that failed after retries counts once, by its last attempt's cause.</sub>

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
    line [0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [4.153, 6.175, 8.27, 10.473, 12.565, 14.766, 16.867, 21.319, 25.29, 24.418, 26.189, 29.982, 32.09, 32.832, 35.201, 37.683, 39.695, 42.429, 45.462, 47.107, 49.929, 52.817, 55.223, 59.356, 60.838, 62.731, 65.615, 68.107, 70.143, 71.752, 71.975, 63.691, 67.418, 67.671, 59.151, 58.241, 67.945, 68.923, 67.632, 70.704, 63.571, 65.5]
    line [9.212, 10.326, 10.842, 12.457, 13.723, 15.168, 16.612, 36.68, 43.793, 26.452, 24.447, 34.17, 40.874, 29.995, 34.281, 35.705, 34.539, 37.758, 43.885, 41.832, 45.603, 48.361, 52.121, 65.587, 67.404, 65.369, 78.897, 80.223, 84.828, 86.285, 92.572, 99.745, 99.956, 100.034, 99.887, 100.31, 100.045, 99.585, 100.43, 96.297, 100.249, 99.766]
    line [4.289, 3.671, 4.896, 5.935, 6.944, 7.867, 8.957, 12.807, 14.674, 9.698, 10.068, 15.245, 14.654, 11.821, 13.427, 14.413, 14.591, 16.742, 21.176, 18.969, 20.655, 25.337, 24.454, 31.385, 31.176, 31.326, 35.847, 38.239, 39.846, 40.376, 43.171, 39.718, 37.282, 37.516, 37.179, 33.655, 37.244, 39.218, 36.703, 39.554, 40.432, 36.449]
```

<sub>Blue: api. Orange: db. Green: lb (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 1:10 stall; 5:10 break.</sub>

## Diagnosis

### 1. CPU bursts at the limit (db), from 1:10

At 1:10, db was CPU-throttled for 12.6 s (summed over its threads) in 10 seconds while its average CPU was 37% of its 0.4-CPU limit. Docker enforces the limit as 40 ms of CPU in every 100 ms period: work that came in a burst used up a period's share early and waited for the next one, and requests piled up behind it until the burst cleared.

Evidence:

- db throttled 12.6 s in the window from 1:10, at 37% of its CPU limit on average <sub>(container_cpu_cfs_throttled_seconds_total (cAdvisor); container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- 1:10: p99 reached 514 ms (5.7 ms at low load) for 10 s, then recovered; too short to count as a break <sub>(k6_http_req_duration_seconds (k6 histogram, end to end))</sub>

**Try next:** This is how a container's CPU limit behaves (Kubernetes throttles the same way); a VM with a small CPU slows down evenly instead. A bigger CPU limit leaves each period more room; resources.cpu_period_ms on db shortens the period, which spreads the wait but runs the CPU closer to its limit.

### 2. Queueing knee (db CPU), from 4:20

From 4:20, p99 stayed above 17 ms (5.7 ms at low load) with db CPU at 79%. Near saturation requests wait behind each other, so latency grows non-linearly, not in proportion to load.

Evidence:

- median end-to-end p99 by db CPU: below 50%: 5.9 ms (22 windows); 50–70%: 29 ms (4 windows); 70–80%: 88 ms (1 window); 80–90%: 54 ms (3 windows); 90% and up: 1.32 s (12 windows) <sub>(k6_http_req_duration_seconds (k6 histogram, end to end); container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- 4:20 to 4:50: p99 88 ms, 37 ms, 54 ms with db CPU 79%, 80%, 85% <sub>(k6_http_req_duration_seconds (k6 histogram, end to end); container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- db was CPU-throttled for 10.4 s (summed over its threads) in the window from 4:20, while its average CPU was 79% of its limit: bursts of work hit the 0.4-CPU limit and waited for the next period <sub>(container_cpu_cfs_throttled_seconds_total (cAdvisor))</sub>
- Little's law on api at 4:20: 3,098 rps × 7.1 ms mean time in the service = 22 in flight; measured 22 <sub>(loadlore_requests_inflight (summed over replicas); rate and mean from loadlore_request_duration_seconds (the service's own histogram))</sub>

**Try next:** Add capacity or cut per-request work on db, then re-run the ramp.

### 3. Hot rows (api → db), from 5:10

From 5:10, api's writes to db took 307 ms each, 136.3x their 2.3 ms at low load, its reads, many of them of the same rows, slowed to 180 ms from 0.8 ms, and 24.7 backends of db waited on locks. Writes to the same popular rows wait for each other's row locks and go through one at a time. Where hot rows tip over moves from run to run: the queue for a row can stay short and then collapse within seconds, so the same design can break at loads a quarter apart. Judge a fix by whether it takes the writes off the row, not by how much later it breaks.

Evidence:

- 24.7 backends waiting on locks at 5:40, on average <sub>(pg_lock_waits_waiting: pg_stat_activity backends with wait_event_type Lock (postgres_exporter, Loadlore's query))</sub>
- a write took 307 ms at 5:40, against 2.3 ms at low load <sub>(loadlore_call_duration_seconds by op (the caller's own timings, pool wait included))</sub>
- a read took 180 ms at 5:40, against 0.8 ms at low load <sub>(loadlore_call_duration_seconds by op (the caller's own timings, pool wait included))</sub>
- db CPU at 100% of its limit at 5:40: not what writes wait for <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit; calls from loadlore_call_duration_seconds by op (the caller's own timings, pool wait included))</sub>

**Try next:** Spread the writes over more rows: change the key design so popular items don't share a row, batch updates to the same row, or shard a hot counter into several rows summed on read.

<sub>Rules version 23. Rules are deterministic and offline; their thresholds are listed below.</sub>

## Predictions

1 of 2 scored predictions held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 300 until load_rps >= 3000` | **broken** at 1:10 | 514 ms | 0:00 to 4:20, when load_rps >= 3,000 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |
| `error_rate < 1% throughout` | **held** | <0.1% | the whole run | source: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6) |

A broken prediction isn't a wrong answer: it shows where your picture of the system and the run differ, and that gap is what to look into. Loads and request rates depend on this machine: slower cores break the same design at a lower load, so a load guess can miss even when the reasoning behind it was right.

## Timeline

| Time | Event |
| --- | --- |
| 1:10 | stall: p99 reached 514 ms (5.7 ms at low load) for 10 s, then recovered; too short to count as a break; diagnosed as CPU bursts at the limit (db) |

<sub>From events.jsonl: the fault timeline and Docker container events. Stalls are from the per-window numbers below, judged by the thresholds; each names the diagnoses that cover it, counting one that ends as the stall starts.</sub>

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
| services.db.disk | gp3: 1 ms per read, 1 ms per write, 3000 IOPS, 125 MiB/s | a gp3 volume at its baseline: 3,000 IOPS and 125 MiB/s, included in its price, and up to 80,000 IOPS provisioned. AWS publishes no exact latency, only single-digit milliseconds, so the 1 ms Loadlore adds to every read, write and fsync is its own estimate: the low end of that range, so the disk is flattered rather than handicapped. Reads and writes are capped separately, so a mixed load can reach up to twice the IOPS. Provisioned throughput isn't modelled. Source: AWS, Amazon EBS General Purpose SSD volumes: https://docs.aws.amazon.com/ebs/latest/userguide/general-purpose.html |
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
- **Host disk stalls:** for 2 seconds or more, a Postgres read no blocks and committed at most 10% of its median over the 10 s before (50 a second or more), while 5 or more queries waited on disk reads or WAL flushes and its exporter answered; on a modelled disk it had also been reading 20 blocks a second or more (median), which its caps can't stop for seconds; not under a disk_slow fault on it. Near the break it makes the run inconclusive; anywhere else it's a caveat, and its windows are flagged and left out of exports.

## Environment

- loadl dev, profile 7m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 984 (hash 940, memory 1020, syscalls 994): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (984) was within 5.0% of the run it's compared with (1033)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 949,321 requests, 10,647 dropped arrivals, <0.1% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-when-to-scale-your-database-one-query-per-page-20261118-072056-api-0-1 | loadlore/synthetic:dev |
| loadlore-when-to-scale-your-database-one-query-per-page-20261118-072056-api-1-1 | loadlore/synthetic:dev |
| loadlore-when-to-scale-your-database-one-query-per-page-20261118-072056-api-2-1 | loadlore/synthetic:dev |
| loadlore-when-to-scale-your-database-one-query-per-page-20261118-072056-api-3-1 | loadlore/synthetic:dev |
| loadlore-when-to-scale-your-database-one-query-per-page-20261118-072056-db-1 | postgres:17.11 |
| loadlore-when-to-scale-your-database-one-query-per-page-20261118-072056-lb-1 | haproxy:3.2.26 |
| loadlore-when-to-scale-your-database-one-query-per-page-20261118-072056-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-when-to-scale-your-database-one-query-per-page-20261118-072056-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-when-to-scale-your-database-one-query-per-page-20261118-072056-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-when-to-scale-your-database-one-query-per-page-20261118-072056-lore-prometheus-1 | prom/prometheus:v3.16.0 |

</details>

## Per-window data

<details><summary>42 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | lb CPU | lb up | api→db pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 157 | 150 | 0 | 0% | 2.4 ms | 7.2 ms | 4.2% | 9.2% | 97 | 4.3% | 4 | 1/96 | 0.0 ms | 5.7% | 2.0% |
| 0:10 | 270 | 267 | 0 | 0% | 1.8 ms | 5.8 ms | 6.2% | 10% | 97 | 3.7% | 4 | 2/96 | 0.0 ms | 2.8% | 0% |
| 0:20 | 383 | 380 | 0 | 0% | 1.6 ms | 5.9 ms | 8.3% | 11% | 97 | 4.9% | 4 | 1/96 | 0.0 ms | 4.2% | 0% |
| 0:30 | 497 | 493 | 0 | 0% | 1.6 ms | 5.7 ms | 10% | 12% | 97 | 5.9% | 4 | 1/96 | 0.0 ms | 4.7% | 0% |
| 0:40 | 610 | 606 | 0 | 0% | 1.6 ms | 5.4 ms | 13% | 14% | 97 | 6.9% | 4 | 2/96 | 0.0 ms | 4.6% | 0% |
| 0:50 | 723 | 720 | 0 | 0% | 1.5 ms | 5.4 ms | 15% | 15% | 97 | 7.9% | 4 | 2/96 | 0.0 ms | 6.0% | 0% |
| 1:00 | 837 | 833 | 0 | 0% | 1.5 ms | 5.5 ms | 17% | 17% | 97 | 9.0% | 4 | 3/96 | 0.0 ms | 6.9% | 0% |
| 1:10 | 950 | 947 | 0 | 0% | 1.5 ms | 514 ms | 21% | 37% | 97 | 13% | 4 | 72/96 +109 | 16 ms | 14% | 0% |
| 1:20 | 1,063 | 1,060 | 0 | 0% | 1.8 ms | 336 ms | 25% | 44% | 97 | 15% | 4 | 48/96 +62 | 4.8 ms | 17% | 1.0% |
| 1:30 | 1,177 | 1,173 | 0 | 0% | 1.5 ms | 5.9 ms | 24% | 26% | 97 | 9.7% | 4 | 3/96 | 0.0 ms | 12% | 1.0% |
| 1:40 | 1,290 | 1,287 | 0 | 0% | 1.4 ms | 5.4 ms | 26% | 24% | 97 | 10% | 4 | 4/96 | 0.0 ms | 10% | 0% |
| 1:50 | 1,403 | 1,400 | 0 | 0% | 1.5 ms | 7.0 ms | 30% | 34% | 97 | 15% | 4 | 3/96 | 0.0 ms | 15% | 0% |
| 2:00 | 1,517 | 1,513 | 0 | 0% | 1.5 ms | 225 ms | 32% | 41% | 97 | 15% | 4 | 48/96 +63 | 4.8 ms | 15% | 0% |
| 2:10 | 1,630 | 1,627 | 0 | 0% | 1.4 ms | 5.3 ms | 33% | 30% | 97 | 12% | 4 | 3/96 | 0.0 ms | 13% | 0% |
| 2:20 | 1,743 | 1,740 | 0 | 0% | 1.4 ms | 5.4 ms | 35% | 34% | 97 | 13% | 4 | 4/96 | 0.1 ms | 14% | 0% |
| 2:30 | 1,857 | 1,853 | 0 | 0% | 1.4 ms | 116 ms | 38% | 36% | 97 | 14% | 4 | 12/96 | 1.3 ms | 14% | 0% |
| 2:40 | 1,970 | 1,967 | 0 | 0% | 1.4 ms | 5.7 ms | 40% | 35% | 97 | 15% | 4 | 3/96 | 0.0 ms | 14% | 1.0% |
| 2:50 | 2,083 | 2,080 | 0 | 0% | 1.4 ms | 5.7 ms | 42% | 38% | 97 | 17% | 4 | 3/96 | 0.0 ms | 16% | 1.0% |
| 3:00 | 2,197 | 2,193 | 0 | 0% | 1.5 ms | 66 ms | 45% | 44% | 97 | 21% | 4 | 26/96 +22 | 0.9 ms | 17% | 1.0% |
| 3:10 | 2,310 | 2,307 | 0 | 0% | 1.4 ms | 6.4 ms | 47% | 42% | 97 | 19% | 4 | 45/96 +30 | 0.0 ms | 17% | 1.4% |
| 3:20 | 2,423 | 2,420 | 0 | 0% | 1.5 ms | 6.0 ms | 50% | 46% | 97 | 21% | 4 | 4/96 | 0.0 ms | 18% | 1.6% |
| 3:30 | 2,537 | 2,533 | 0 | 0% | 1.5 ms | 6.4 ms | 53% | 48% | 97 | 25% | 4 | 4/96 | 0.0 ms | 21% | 0.5% |
| 3:40 | 2,650 | 2,647 | 0 | 0% | 1.5 ms | 6.6 ms | 55% | 52% | 97 | 24% | 4 | 5/96 | 0.0 ms | 22% | 1.5% |
| 3:50 | 2,763 | 2,759 | 0 | 0% | 1.6 ms | 46 ms | 59% | 66% | 97 | 31% | 4 | 27/96 +20 | 0.3 ms | 26% | 2.0% |
| 4:00 | 2,877 | 2,873 | 0 | 0% | 1.6 ms | 112 ms | 61% | 67% | 97 | 31% | 4 | 51/96 +89 | 2.0 ms | 25% | 0% |
| 4:10 | 2,990 | 2,987 | 0 | 0% | 1.6 ms | 13 ms | 63% | 65% | 97 | 31% | 4 | 7/96 | 0.0 ms | 27% | 1.0% |
| 4:20 | 3,103 | 3,097 | 0 | 0% | 1.7 ms | 88 ms | 66% | 79% | 97 | 36% | 4 | 71/96 +30 | 1.7 ms | 28% | 1.0% |
| 4:30 | 3,217 | 3,216 | 0 | 0% | 1.7 ms | 37 ms | 68% | 80% | 97 | 38% | 4 | 49/96 | 0.1 ms | 31% | 3.0% |
| 4:40 | 3,330 | 3,327 | 0 | 0% | 1.8 ms | 54 ms | 70% | 85% | 98 | 40% | 4 | 47/96 +9 | 0.5 ms | 30% | 1.0% |
| 4:50 | 3,443 | 3,440 | 0 | 0% | 1.8 ms | 56 ms | 72% | 86% | 97 | 40% | 4 | 46/96 +9 | 0.6 ms | 31% | 1.0% |
| 5:00 | 3,500 | 3,391 | 0 | 0% | 2.2 ms | 100 ms | 72% | 93% | 97 | 43% | 4 | 93/96 +41 | 14 ms | 31% | 2.0% |
| 5:10 | 3,500 | 3,298 | 1,195 | 0% | 516 ms | 940 ms | 64% | 100% | 97 | 40% | 4 | 92/96 +703 | 193 ms | 25% | 2.0% |
| 5:20 | 3,500 | 3,581 | 214 | 0% | 539 ms | 935 ms | 67% | 100% | 97 | 37% | 4 | 93/96 +704 | 154 ms | 26% | 2.0% |
| 5:30 | 3,500 | 3,411 | 0 | <0.1% | 79 ms | 714 ms | 68% | 100% | 98 | 38% | 4 | 95/96 +527 | 87 ms | 24% | 0.5% |
| 5:40 | 3,500 | 3,029 | 3,134 | <0.1% | 879 ms | 1.92 s | 59% | 100% | 97 | 37% | 4 | 95/96 +528 | 164 ms | 24% | 2.5% |
| 5:50 | 3,500 | 3,037 | 4,102 | <0.1% | 1.25 s | 2.38 s | 58% | 100% | 97 | 34% | 4 | 96/96 +587 | 165 ms | 26% | 5.0% |
| 6:00 | 3,500 | 3,580 | 641 | 0% | 1.16 s | 2.29 s | 68% | 100% | 98 | 37% | 4 | 83/96 +528 | 136 ms | 25% | 4.0% |
| 6:10 | 3,500 | 3,593 | 0 | 0% | 424 ms | 1.40 s | 69% | 100% | 97 | 39% | 4 | 89/96 +529 | 113 ms | 25% | 3.0% |
| 6:20 | 3,500 | 3,527 | 0 | 0% | 471 ms | 1.25 s | 68% | 100% | 97 | 37% | 4 | 94/96 +396 | 98 ms | 24% | 3.0% |
| 6:30 | 3,500 | 3,619 | 0 | 0% | 52 ms | 967 ms | 71% | 96% | 97 | 40% | 4 | 92/96 +431 | 61 ms | 28% | 5.2% |
| 6:40 | 3,500 | 3,086 | 348 | <0.1% | 83 ms | 1.67 s | 64% | 100% | 97 | 40% | 4 | 96/96 +563 | 120 ms | 28% | 4.8% |
| 6:50 | 3,500 | 3,418 | 1,013 | 0% | 1.42 s | 1.94 s | 65% | 100% | 97 | 36% | 4 | 83/96 +528 | 143 ms | 24% | 3.0% |

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
    disk: { profile: gp3 }

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
