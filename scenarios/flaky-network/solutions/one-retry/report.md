# flaky-network-one-retry: run report

_run `flaky-network-one-retry-20261117-230527` · completed · 17 Nov 2026 23:05 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 300 rps offered, 301 rps served at best, worst 10-second p99 162 ms, at most 0.1% errors.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 3:00

Measured in the 10-second window from 3:00 to 3:10 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 300 rps<br/>offered 300 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 3.1%<br/>2/2 replicas up"}}
    api["api ×2<br/>0.5 CPU · 256m<br/>CPU 27%<br/>in flight 7"]
    inventory["inventory<br/>1 CPU · 256m<br/>CPU 46%<br/>in flight 0"]
    load -->|"100% read · 0% write<br/>301 rps · p99 152 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>301 rps · p99 158 ms"| api
    api -->|"http ×10<br/>pool 50<br/>3,006 calls/s · p99 0.8 ms · 0.1% errors<br/>in use 7/100"| inventory
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb,api,inventory ok
```

<sub>Sources: offered from the load profile in plan.json (offered arrival rate); load edge from k6_http_reqs_total (k6: all responses) and k6_http_req_duration_seconds (k6 histogram, end to end); CPU from container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit; calls/s from loadlore_calls_total (the caller's outbound call attempts) and their p99 from loadlore_call_duration_seconds (the caller's histogram, pool wait included); pools from loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections and loadlore_pool_pending (callers waiting for a connection); lb traffic from haproxy_backend_http_requests_total (HAProxy), replicas up from haproxy_backend_active_servers (HAProxy), and the lb edge's p99 from loadlore_request_duration_seconds (the service's own histogram).</sub>

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
    x-axis "seconds" 0 --> 230
    y-axis "requests/s" 0 --> 400
    line [0, 0, 0, 0, 0, 0, 400, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [91.667, 175, 258.333, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300]
    line [91.437, 174.883, 258.15, 300, 300, 300.03, 299.767, 299.332, 299.801, 299.983, 299.775, 299.719, 299.821, 299.781, 300.185, 299.904, 299.794, 299.33, 300.678, 300.028, 299.972, 300, 300, 300.008]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 1:01 loss 1% of the packets inventory sends from port 8080 (its replies) for 2m0s.</sub>

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
    x-axis "seconds" 0 --> 230
    y-axis "ms" 0 --> 200
    line [0, 0, 0, 0, 0, 0, 200, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [5.543, 5.358, 5.332, 5.319, 5.323, 5.32, 5.348, 5.345, 5.351, 5.348, 5.345, 5.352, 5.345, 5.343, 5.344, 5.349, 5.341, 5.346, 5.313, 5.316, 5.316, 5.316, 5.319, 5.316]
    line [6.009, 5.936, 5.883, 5.805, 5.832, 5.8, 153.797, 154.597, 155.08, 155.67, 155.004, 155.874, 155.297, 155.368, 155.404, 155.703, 155.448, 155.383, 5.75, 5.733, 5.739, 5.742, 5.777, 5.747]
    line [6.673, 6.255, 6.302, 6.214, 6.381, 6.029, 160.816, 161.036, 161.275, 161.35, 161.363, 161.538, 161.172, 161.063, 161.105, 161.31, 161.414, 161.315, 152.207, 5.971, 5.98, 5.974, 5.98, 5.985]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 1:01 loss 1% of the packets inventory sends from port 8080 (its replies) for 2m0s.</sub>

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
    x-axis "seconds" 0 --> 230
    y-axis "% of requests" 0 --> 1
    line [0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [0, 0, 0, 0, 0, 0, 0, 0.033, 0.1, 0.067, 0.12, 0.08, 0, 0.064, 0.036, 0.067, 0, 0.133, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 1:01 loss 1% of the packets inventory sends from port 8080 (its replies) for 2m0s.</sub>

| Service | Failed requests | Share of its failures | Cause | From calls to | First seen | What it means |
| --- | --- | --- | --- | --- | --- | --- |
| api | 21 | 100% | `timeout` | inventory | 1:10 | calls to inventory ran past their timeout |

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
    x-axis "seconds" 0 --> 230
    y-axis "% of limit" 0 --> 100
    line [0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [9.387, 15.955, 23.711, 27.398, 27.431, 27.53, 27.986, 27.93, 27.894, 27.858, 27.825, 27.814, 27.741, 27.706, 27.682, 27.861, 27.601, 27.728, 27.406, 27.223, 27.212, 26.868, 27.163, 26.342]
    line [14.587, 27.362, 39.925, 45.966, 45.888, 45.922, 46.219, 46.213, 46.224, 46.182, 46.126, 46.142, 46.079, 46.135, 46.136, 46.162, 46.157, 46.155, 45.802, 45.829, 45.829, 45.69, 45.812, 44.798]
    line [2.171, 3.096, 4.079, 4.509, 4.511, 4.445, 3.932, 3.72, 3.655, 3.524, 3.562, 3.609, 3.605, 3.631, 3.577, 3.664, 3.535, 3.627, 3.12, 3.955, 4.31, 4.395, 4.339, 4.11]
```

<sub>Blue: api. Orange: inventory. Green: lb (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 1:01 loss 1% of the packets inventory sends from port 8080 (its replies) for 2m0s.</sub>

## Diagnosis

No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version 22; thresholds below).

## Predictions

The design has no `expect` lines. Writing a prediction before a run turns it into a test of your mental model.

## Timeline

| Time | Event |
| --- | --- |
| 1:01 | fault started: loss 1% of the packets inventory sends from port 8080 (its replies) for 2m0s |
| 3:01 | fault ended: loss 1% of the packets inventory sends from port 8080 (its replies) for 2m0s |

<sub>From events.jsonl: the fault timeline and Docker container events.</sub>

## Assumptions

### From the design

The design lists no assumptions.

### Chosen by Loadlore

| Field | Value | Why |
| --- | --- | --- |
| workload.timeout_ms | 10s | the default; k6 gives up on a request after this long and counts it as failed |
| workload.slo.p99_ms | 500 ms | the default; a break on p99 alone needs p99 at or above this as well as 10x the low-load p99, so latency users would still accept isn't called a break |
| network.profile | same-zone: 0.3 ms round trip | the default: every service in one availability zone. AWS says a round trip between two instances in the same zone is generally sub-millisecond with enhanced networking, and publishes no exact figure, so the 0.3 ms round trip Loadlore adds between any two containers is its own estimate, inside that bound. Applied between any two of the stack's containers, the load generator and the entry point included, as half the round trip on each side's outgoing packets; Loadlore's own tooling isn't slowed. Not scaled with --scale. Source: AWS Architecture Blog, Improving performance and reducing cost using Availability Zone Affinity: https://aws.amazon.com/blogs/architecture/improving-performance-and-reducing-cost-using-availability-zone-affinity/ |
| resources.cpu_period_ms | 100 ms for api, inventory, lb | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |

### How the run was planned

- Loadlore's own containers get fixed budgets: load generator 1.5 CPU (peak 300 rps), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each. No fault acts on a connection, so every connection is direct and there is no Toxiproxy

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

## Environment

- loadl dev, profile 4m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 1009 (hash 988, memory 1054, syscalls 986): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (1009) was within 5.0% of the run it's compared with (984)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 68,249 requests, 0 dropped arrivals, <0.1% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-flaky-network-one-retry-20261117-230527-api-0-1 | loadlore/synthetic:dev |
| loadlore-flaky-network-one-retry-20261117-230527-api-1-1 | loadlore/synthetic:dev |
| loadlore-flaky-network-one-retry-20261117-230527-inventory-0-1 | loadlore/synthetic:dev |
| loadlore-flaky-network-one-retry-20261117-230527-lb-1 | haproxy:3.2.26 |
| loadlore-flaky-network-one-retry-20261117-230527-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-flaky-network-one-retry-20261117-230527-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-flaky-network-one-retry-20261117-230527-lore-prometheus-1 | prom/prometheus:v3.16.0 |

</details>

## Per-window data

<details><summary>24 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | inventory CPU | lb CPU | lb up | api→inventory pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 92 | 91 | 0 | 0% | 5.5 ms | 6.7 ms | 9.4% | 15% | 2.2% | 2 | 1/100 | 0.0 ms | 1.7% | 0% |
| 0:10 | 175 | 175 | 0 | 0% | 5.4 ms | 6.3 ms | 16% | 27% | 3.1% | 2 | 2/100 | 0.0 ms | 1.9% | 0% |
| 0:20 | 258 | 258 | 0 | 0% | 5.3 ms | 6.3 ms | 24% | 40% | 4.1% | 2 | 2/100 | 0.0 ms | 2.8% | 0% |
| 0:30 | 300 | 300 | 0 | 0% | 5.3 ms | 6.2 ms | 27% | 46% | 4.5% | 2 | 2/100 | 0.0 ms | 3.1% | 0% |
| 0:40 | 300 | 300 | 0 | 0% | 5.3 ms | 6.4 ms | 27% | 46% | 4.5% | 2 | 2/100 | 0.0 ms | 3.1% | 0% |
| 0:50 | 300 | 300 | 0 | 0% | 5.3 ms | 6.0 ms | 28% | 46% | 4.4% | 2 | 2/100 | 0.0 ms | 3.2% | 0% |
| 1:00 | 300 | 300 | 0 | 0% | 5.3 ms | 161 ms | 28% | 46% | 3.9% | 2 | 9/100 | 0.0 ms | 3.2% | 0% |
| 1:10 | 300 | 299 | 0 | <0.1% | 5.3 ms | 161 ms | 28% | 46% | 3.7% | 2 | 9/100 | 0.0 ms | 3.1% | 0% |
| 1:20 | 300 | 300 | 0 | <0.1% | 5.4 ms | 161 ms | 28% | 46% | 3.7% | 2 | 9/100 | 0.0 ms | 3.2% | 0% |
| 1:30 | 300 | 300 | 0 | <0.1% | 5.3 ms | 161 ms | 28% | 46% | 3.5% | 2 | 13/100 | 0.0 ms | 3.2% | 0% |
| 1:40 | 300 | 300 | 0 | 0.1% | 5.3 ms | 161 ms | 28% | 46% | 3.6% | 2 | 12/100 | 0.0 ms | 3.2% | 0% |
| 1:50 | 300 | 300 | 0 | <0.1% | 5.4 ms | 162 ms | 28% | 46% | 3.6% | 2 | 12/100 +1 | 0.0 ms | 3.1% | 0% |
| 2:00 | 300 | 300 | 0 | 0% | 5.3 ms | 161 ms | 28% | 46% | 3.6% | 2 | 11/100 | 0.0 ms | 3.1% | 0% |
| 2:10 | 300 | 300 | 0 | <0.1% | 5.3 ms | 161 ms | 28% | 46% | 3.6% | 2 | 9/100 | 0.0 ms | 3.1% | 0% |
| 2:20 | 300 | 300 | 0 | <0.1% | 5.3 ms | 161 ms | 28% | 46% | 3.6% | 2 | 13/100 | 0.0 ms | 3.1% | 0% |
| 2:30 | 300 | 300 | 0 | <0.1% | 5.3 ms | 161 ms | 28% | 46% | 3.7% | 2 | 11/100 | 0.0 ms | 3.1% | 0% |
| 2:40 | 300 | 300 | 0 | 0% | 5.3 ms | 161 ms | 28% | 46% | 3.5% | 2 | 9/100 | 0.0 ms | 3.1% | 0% |
| 2:50 | 300 | 299 | 0 | 0.1% | 5.3 ms | 161 ms | 28% | 46% | 3.6% | 2 | 8/100 | 0.0 ms | 3.2% | 0% |
| 3:00 | 300 | 301 | 0 | 0% | 5.3 ms | 152 ms | 27% | 46% | 3.1% | 2 | 7/100 | 0.0 ms | 3.1% | 0% |
| 3:10 | 300 | 300 | 0 | 0% | 5.3 ms | 6.0 ms | 27% | 46% | 4.0% | 2 | 2/100 | 0.0 ms | 3.1% | 0% |
| 3:20 | 300 | 300 | 0 | 0% | 5.3 ms | 6.0 ms | 27% | 46% | 4.3% | 2 | 2/100 | 0.0 ms | 3.0% | 0% |
| 3:30 | 300 | 300 | 0 | 0% | 5.3 ms | 6.0 ms | 27% | 46% | 4.4% | 2 | 2/100 | 0.0 ms | 3.1% | 0% |
| 3:40 | 300 | 300 | 0 | 0% | 5.3 ms | 6.0 ms | 27% | 46% | 4.3% | 2 | 2/100 | 0.0 ms | 3.1% | 0% |
| 3:50 | 300 | 300 | 0 | 0% | 5.3 ms | 6.0 ms | 26% | 45% | 4.1% | 2 | 2/100 | 0.0 ms | 2.3% | 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 CPU is against its budget.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: flaky-network-one-retry

# Solution: keep the 150 ms timeout, retry a failed call once. A call
# whose reply lost a packet times out and is sent again on a new
# connection, where its reply is very likely not lost too.

services:
  lb:
    kind: lb
    target: api
    health_check: { interval_ms: 2000, fall: 3, rise: 2 }

  api:
    image: loadlore/synthetic
    replicas: 2
    resources: { cpu: 0.5, memory: 256m }
    behavior:
      cpu_ms: 0.3
      calls:
        - { target: inventory, op: http, count: 10 }   # one per product on the page
      resilience:
        timeout_ms: 150
        retries: { max: 1 }
    pools:
      inventory: 50

  inventory:
    image: loadlore/synthetic
    resources: { cpu: 1, memory: 256m }
    behavior:
      cpu_ms: 0.1

workload:
  entry: lb
  profile:
    - ramp: { from: 50, to: 300, over: 30s }
    - hold: { rps: 300, for: 3m30s }
  mix: { read: 1.0 }

# From 1:01, inventory's network loses 1% of the packets it sends, for two
# minutes: a bad cable, a busy switch, a noisy neighbour on the host.
scenario:
  - { at: 1m1s, inject: loss, target: inventory, percent: 1, for: 2m }
```
