# featured-on-hacker-news-scale-on-inflight: run report

_run `featured-on-hacker-news-scale-on-inflight-20261118-092840` · completed · 18 Nov 2026 09:28 UTC · scale 1 · seed 1_

## Verdict

**Broke.** Broke 1:20 into the load, at 1,089 rps offered: p99 reached 1.15 s (3.1 ms at low load) and only 906 rps succeeded. Most saturated then: api CPU at 100% of its limit. After the break the load generator couldn't send 12% of the arrivals the profile asked for (20,250 arrivals from 1:30; k6_dropped_iterations_total (k6: arrivals it couldn't start)): slow responses held its 4,000 VUs, so served throughput from then on is partly the generator's limit, not only the system's. It recovered at 2:20, 1:00 after the break, and stayed healthy to the end.

Caveat: at 1:20 the load generator hit its CPU limit and was stalled in 1.0% of its scheduling periods (container_cpu_cfs_throttled_periods_total ÷ container_cpu_cfs_periods_total (cAdvisor)); work caught in a stall waits for the next 100 ms period, which can show up in p99.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); saturation: container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit. Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 1:20

Measured in the 10-second window from 1:20 to 1:30 (the break). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 1,800 rps<br/>offered 1,089 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 9.3%<br/>2/6 replicas up"}}
    api["api ×2<br/>0.5 CPU · 256m<br/>autoscale 2–6 on 10 in flight<br/>CPU 100%<br/>in flight 399"]
    db[("db<br/>postgres 17<br/>1 CPU · 1g<br/>disk gp3 · 3,000 IOPS<br/>CPU 5.0%<br/>conns 21/100")]
    load -->|"90% read · 10% write<br/>906 rps · p99 1.15 s"| lb
    lb -->|"round-robin<br/>checks every 1s, out after 3<br/>905 rps · p99 1.47 s"| api
    api -->|"read<br/>pool 10, min 10<br/>904 calls/s · p99 498 ms<br/>in use 18/20, 377 waiting"| db
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb,db ok
    class api hot
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
    y-axis "requests/s" 0 --> 2500
    line [0, 0, 0, 0, 0, 0, 0, 0, 2500, 2500, 0, 0, 2500, 0, 0, 0, 0, 0, 0]
    line [200, 200, 200, 200, 200, 200, 377.778, 733.333, 1088.889, 1444.444, 1755.556, 1800, 1800, 1800, 1800, 1800, 1800, 1800, 1800]
    line [186.072, 199.914, 200.15, 199.862, 200.053, 200.025, 353.903, 708.457, 906.16, 949.46, 958.874, 958.281, 1912.041, 1915.813, 1799.897, 1800.044, 1800.188, 1799.965, 1799.796]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 1:30 api 2 → 6 replicas (91.5 requests in flight per replica, target 10); 2:00 api-5 start; 2:00 api-2 start; 2:00 api-4 start; 2:00 api-3 start; 2:02 api-2 took traffic, 31.9 s after the decision; 2:02 api-3 took traffic, 31.9 s after the decision; 2:02 api-4 took traffic, 32.0 s after the decision; 2:02 api-5 took traffic, 32.1 s after the decision; 1:20 break.</sub>

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
    y-axis "ms" 0 --> 5000
    line [0, 0, 0, 0, 0, 0, 0, 0, 5000, 5000, 0, 0, 5000, 0, 0, 0, 0, 0, 0]
    line [2.975, 2.454, 2.115, 2.077, 2.082, 2.085, 2.053, 2.03, 504.284, 2306.569, 4170.92, 4181.015, 2783.939, 2.031, 1.977, 1.971, 1.979, 1.98, 1.976]
    line [4.153, 3.558, 2.937, 2.309, 2.307, 2.313, 2.292, 2.273, 1048.702, 3666.169, 4342.92, 4343.929, 4315.818, 1862.178, 2.32, 2.284, 2.296, 2.31, 2.292]
    line [5.023, 4.039, 3.696, 2.664, 2.447, 3.053, 2.362, 2.321, 1150.478, 3932.833, 4358.209, 4358.411, 4352.788, 2346.026, 3.572, 2.833, 2.981, 3.26, 2.979]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 1:30 api 2 → 6 replicas (91.5 requests in flight per replica, target 10); 2:00 api-5 start; 2:00 api-2 start; 2:00 api-4 start; 2:00 api-3 start; 2:02 api-2 took traffic, 31.9 s after the decision; 2:02 api-3 took traffic, 31.9 s after the decision; 2:02 api-4 took traffic, 32.0 s after the decision; 2:02 api-5 took traffic, 32.1 s after the decision; 1:20 break.</sub>

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
    line [0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0]
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 1:30 api 2 → 6 replicas (91.5 requests in flight per replica, target 10); 2:00 api-5 start; 2:00 api-2 start; 2:00 api-4 start; 2:00 api-3 start; 2:02 api-2 took traffic, 31.9 s after the decision; 2:02 api-3 took traffic, 31.9 s after the decision; 2:02 api-4 took traffic, 32.0 s after the decision; 2:02 api-5 took traffic, 32.1 s after the decision; 1:20 break.</sub>

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
    line [0, 0, 0, 0, 0, 0, 0, 0, 100, 100, 0, 0, 100, 0, 0, 0, 0, 0, 0]
    line [23.593, 24.819, 24.179, 24.149, 24.294, 24.348, 41.633, 81.683, 100.308, 99.805, 100.073, 100.301, 72.251, 73.516, 71.129, 70.837, 70.942, 71.088, 70.992]
    line [3.337, 3.243, 2.985, 3.459, 3.056, 3.161, 3.301, 4.113, 4.962, 4.94, 4.865, 5.021, 10.038, 7.937, 7.307, 6.982, 7.128, 7.239, 7.161]
    line [4.651, 5.101, 3.431, 3.363, 3.345, 3.348, 4.877, 8.578, 9.282, 9.886, 8.153, 8.215, 18.824, 18.267, 17.725, 13.698, 16.973, 18.473, 18.449]
```

<sub>Blue: api. Orange: db. Green: lb (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 1:30 api 2 → 6 replicas (91.5 requests in flight per replica, target 10); 2:00 api-5 start; 2:00 api-2 start; 2:00 api-4 start; 2:00 api-3 start; 2:02 api-2 took traffic, 31.9 s after the decision; 2:02 api-3 took traffic, 31.9 s after the decision; 2:02 api-4 took traffic, 32.0 s after the decision; 2:02 api-5 took traffic, 32.1 s after the decision; 1:20 break.</sub>

## Diagnosis

### 1. Autoscale lag (api), from 1:20

At the break, 1:20, api ran 2 of up to 6 replicas, their CPU at 100% of the limit: the load grew faster than the autoscaler added capacity. It reached 6 at 1:30.

Evidence:

- api CPU at 100% of its limit at 1:20, over 2 replicas <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- autoscale 1:30 api 2 → 6 replicas (91.5 requests in flight per replica, target 10) <sub>(events.jsonl (Loadlore's autoscaler))</sub>
- 2:02 api-2 took traffic, 31.9 s after the decision <sub>(events.jsonl (Loadlore's autoscaler))</sub>
- 2:02 api-3 took traffic, 31.9 s after the decision <sub>(events.jsonl (Loadlore's autoscaler))</sub>
- 2:02 api-4 took traffic, 32.0 s after the decision <sub>(events.jsonl (Loadlore's autoscaler))</sub>
- 2:02 api-5 took traffic, 32.1 s after the decision <sub>(events.jsonl (Loadlore's autoscaler))</sub>

**Try next:** Let api's autoscaler react sooner and in bigger steps: decide more often (interval_s), and keep more replicas running before a spike you expect (min).

### 2. Queueing knee (db, slower without running out of CPU), from 1:20

From 1:20, p99 stayed above 13 ms (3.1 ms at low load): db answered more slowly while its CPU stayed at most 5.0% of its limit, so callers held their connections longer and queued for them. The slowdown wasn't its CPU.

Evidence:

- 1:20 to 1:50: p99 1.15 s, 3.93 s, 4.36 s <sub>(k6_http_req_duration_seconds (k6 histogram, end to end))</sub>
- api CPU at 100% of its limit at 1:20: not where requests queued, since most of their time went to calls to db <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- api's requests spent 52% of their time in calls to db at 1:20: 1 calls per request × 270 ms each, against 515 ms per request <sub>(loadlore_calls_total (the caller's outbound call attempts); loadlore_call_duration_seconds (the caller's histogram, pool wait included); loadlore_request_duration_seconds (the service's own histogram))</sub>
- api's calls to db took 8.6 ms each once they had a connection at 1:20, against 0.4 ms at low load <sub>(loadlore_call_duration_seconds minus loadlore_pool_wait_seconds, per call (time with a connection))</sub>
- db CPU at most 5.0% of its limit from 1:20 to 1:50 <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>

**Try next:** Find what db waited on besides CPU, such as disk writes or locks, in its own logs and metrics, then re-run the ramp.

<sub>Rules version 23. Rules are deterministic and offline; their thresholds are listed below.</sub>

## Predictions

1 of 1 scored prediction held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 500 until load_rps >= 1000` | **held** | 5.0 ms | 0:00 to 1:20, when load_rps >= 1,000 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |

Loads and request rates depend on this machine: slower cores break the same design at a lower load, so a load guess can miss even when the reasoning behind it was right.

## Timeline

| Time | Event |
| --- | --- |
| 1:30 | autoscale: api 2 → 6 replicas (91.5 requests in flight per replica, target 10) |
| 2:00 | container: api-5 start |
| 2:00 | container: api-2 start |
| 2:00 | container: api-4 start |
| 2:00 | container: api-3 start |
| 2:02 | replica ready: api-2 took traffic, 31.9 s after the decision |
| 2:02 | replica ready: api-3 took traffic, 31.9 s after the decision |
| 2:02 | replica ready: api-4 took traffic, 32.0 s after the decision |
| 2:02 | replica ready: api-5 took traffic, 32.1 s after the decision |

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
| services.api.autoscale | 10 requests in flight per replica, 2 to 6 replicas (starts with 2), every 15 s, scale-up cooldown 0 s, scale-down cooldown 300 s | as a Kubernetes HorizontalPodAutoscaler: it wants ceil(replicas taking traffic × metric ÷ target), ignores changes within 10%, and goes on deciding while new replicas start, adding more if it wants more; it scales down only once none are starting. Unset values are Kubernetes' defaults: 15 s, no wait to scale up, 300 s before scaling down |
| services.api.autoscale.startup_s | 30 s | set in the design |
| services.db.disk | gp3: 1 ms per read, 1 ms per write, 3000 IOPS, 125 MiB/s | a gp3 volume at its baseline: 3,000 IOPS and 125 MiB/s, included in its price, and up to 80,000 IOPS provisioned. AWS publishes no exact latency, only single-digit milliseconds, so the 1 ms Loadlore adds to every read, write and fsync is its own estimate: the low end of that range, so the disk is flattered rather than handicapped. Reads and writes are capped separately, so a mixed load can reach up to twice the IOPS. Provisioned throughput isn't modelled. Source: AWS, Amazon EBS General Purpose SSD volumes: https://docs.aws.amazon.com/ebs/latest/userguide/general-purpose.html |
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
- **Host disk stalls:** for 2 seconds or more, a Postgres read no blocks and committed at most 10% of its median over the 10 s before (50 a second or more), while 5 or more queries waited on disk reads or WAL flushes and its exporter answered; on a modelled disk it had also been reading 20 blocks a second or more (median), which its caps can't stop for seconds; not under a disk_slow fault on it. Near the break it makes the run inconclusive; anywhere else it's a caveat, and its windows are flagged and left out of exports.

## Environment

- loadl dev, profile 3m15s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 990 (hash 925, memory 1054, syscalls 994): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (990) was within 5.0% of the run it's compared with (980)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 198,720 requests, 20,250 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261118-092840-api-0-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261118-092840-api-1-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261118-092840-api-2-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261118-092840-api-3-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261118-092840-api-4-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261118-092840-api-5-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261118-092840-db-1 | postgres:17.11 |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261118-092840-lb-1 | haproxy:3.2.26 |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261118-092840-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261118-092840-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261118-092840-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261118-092840-lore-prometheus-1 | prom/prometheus:v3.16.0 |

</details>

## Per-window data

<details><summary>19 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | api replicas | db CPU | db conns | lb CPU | lb up | api→db pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 200 | 186 | 0 | 0% | 3.0 ms | 5.0 ms | 24% | 2 | 3.3% | 21 | 4.7% | 2 | 1/20 | 0.0 ms | 9.3% | 5.0% |
| 0:10 | 200 | 200 | 0 | 0% | 2.5 ms | 4.0 ms | 25% | 2 | 3.2% | 21 | 5.1% | 2 | 1/20 | 0.0 ms | 3.3% | 0% |
| 0:20 | 200 | 200 | 0 | 0% | 2.1 ms | 3.7 ms | 24% | 2 | 3.0% | 21 | 3.4% | 2 | 0/20 | 0.0 ms | 2.3% | 0% |
| 0:30 | 200 | 200 | 0 | 0% | 2.1 ms | 2.7 ms | 24% | 2 | 3.5% | 21 | 3.4% | 2 | 0/20 | 0.0 ms | 2.2% | 0% |
| 0:40 | 200 | 200 | 0 | 0% | 2.1 ms | 2.4 ms | 24% | 2 | 3.1% | 21 | 3.3% | 2 | 0/20 | 0.0 ms | 2.3% | 0% |
| 0:50 | 200 | 200 | 0 | 0% | 2.1 ms | 3.1 ms | 24% | 2 | 3.2% | 21 | 3.3% | 2 | 0/20 | 0.0 ms | 4.2% | 1.0% |
| 1:00 | 378 | 354 | 0 | 0% | 2.1 ms | 2.4 ms | 42% | 2 | 3.3% | 21 | 4.9% | 2 | 0/20 | 0.0 ms | 3.4% | 0% |
| 1:10 | 733 | 708 | 0 | 0% | 2.0 ms | 2.3 ms | 82% | 2 | 4.1% | 21 | 8.6% | 2 | 2/20 | 0.0 ms | 5.6% | 0% |
| 1:20 | 1,089 | 906 | 0 | 0% | 504 ms | 1.15 s | 100% | 2 | 5.0% | 21 | 9.3% | 2 | 18/20 +377 | 261 ms | 8.0% | 1.0% |
| 1:30 | 1,444 | 949 | 2,306 | 0% | 2.31 s | 3.93 s | 100% | 2 | 4.9% | 21 | 9.9% | 2 | 20/20 +378 | 391 ms | 5.9% | 0% |
| 1:40 | 1,756 | 959 | 7,833 | 0% | 4.17 s | 4.36 s | 100% | 2 | 4.9% | 21 | 8.2% | 2 | 19/20 +378 | 389 ms | 7.3% | 2.0% |
| 1:50 | 1,800 | 958 | 8,402 | 0% | 4.18 s | 4.36 s | 100% | 2 | 5.0% | 21 | 8.2% | 2 | 19/20 +378 | 390 ms | 5.1% | 0% |
| 2:00 | 1,800 | 1,912 | 1,709 | 0% | 2.78 s | 4.35 s | 72% | 6 | 10% | 61 | 19% | 2 | 17/60 +378 | 195 ms | 15% | 2.0% |
| 2:10 | 1,800 | 1,916 | 0 | 0% | 2.0 ms | 2.35 s | 74% | 6 | 7.9% | 61 | 18% | 6 | 19/60 +378 | 47 ms | 14% | 2.0% |
| 2:20 | 1,800 | 1,800 | 0 | 0% | 2.0 ms | 3.6 ms | 71% | 6 | 7.3% | 61 | 18% | 6 | 2/60 | 0.0 ms | 14% | 1.0% |
| 2:30 | 1,800 | 1,800 | 0 | 0% | 2.0 ms | 2.8 ms | 71% | 6 | 7.0% | 61 | 14% | 6 | 2/60 | 0.0 ms | 11% | 0% |
| 2:40 | 1,800 | 1,800 | 0 | 0% | 2.0 ms | 3.0 ms | 71% | 6 | 7.1% | 61 | 17% | 6 | 3/60 | 0.0 ms | 13% | 2.0% |
| 2:50 | 1,800 | 1,800 | 0 | 0% | 2.0 ms | 3.3 ms | 71% | 6 | 7.2% | 61 | 18% | 6 | 3/60 | 0.0 ms | 14% | 1.0% |
| 3:00 | 1,800 | 1,800 | 0 | 0% | 2.0 ms | 3.0 ms | 71% | 6 | 7.2% | 61 | 18% | 6 | 3/60 | 0.0 ms | 11% | 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 CPU is against its budget.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: featured-on-hacker-news-scale-on-inflight

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
    replicas: 2
    resources: { cpu: 0.5, memory: 256m }
    behavior:
      cpu_ms: 1
      calls:
        - { target: db, op: read }
      resilience:
        timeout_ms: 2000
    pools: { db: { size: 10, min: 10 } }   # opened when a replica starts
    autoscale: { max: 6, metric: inflight, target: 10 }   # was metric: cpu, target: 0.7

  db:
    kind: postgres
    version: "17"
    resources: { cpu: 1, memory: 1g }
    config: { max_connections: 100 }
    dataset: { rows: 100000, row_bytes: 256 }
    disk: { profile: gp3 }

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
