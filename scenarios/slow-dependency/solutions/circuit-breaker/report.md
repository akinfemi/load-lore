# slow-dependency-circuit-breaker: run report

_run `slow-dependency-circuit-breaker-20261117-224638` · completed · 17 Nov 2026 22:46 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 150 rps offered, 150 rps served at best, worst 10-second p99 1.07 s, at most 35% errors.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 3:10

Measured in the 10-second window from 3:10 to 3:20 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 150 rps<br/>offered 150 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 3.3%<br/>3/3 replicas up"}}
    api["api ×3<br/>0.5 CPU · 256m<br/>CPU 6.6%<br/>in flight 2"]
    pricing["pricing<br/>1 CPU · 256m<br/>CPU 41%<br/>in flight 1"]
    load -->|"100% read · 0% write<br/>150 rps · p99 7.8 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>150 rps · p99 5.0 ms"| api
    api -->|"http<br/>pool 100<br/>150 calls/s · p99 5.0 ms<br/>in use 2/300"| pricing
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb,api,pricing ok
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
    x-axis "seconds" 0 --> 260
    y-axis "requests/s" 0 --> 200
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 200, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [30.833, 52.5, 74.167, 95.833, 117.5, 139.167, 150, 150, 150, 150, 150, 150, 150, 150, 150, 150, 150, 150, 150, 150, 150, 150, 150, 150, 150, 150, 150]
    line [30.672, 52.413, 74.07, 95.755, 117.41, 139.105, 149.999, 149.995, 149.99, 150.015, 149.995, 150.005, 98.181, 150.004, 150.015, 150, 149.985, 150, 150, 150.024, 150.006, 149.99, 149.995, 149.985, 150.015, 149.99, 150.01]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 2:00 stall; 2:01 pricing-0 pause; 2:01 pause pricing-0 for 2s; 2:03 pricing-0 unpause.</sub>

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
    x-axis "seconds" 0 --> 260
    y-axis "ms" 0 --> 1200
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1200, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [5.051, 4.538, 4.399, 4.231, 4.151, 4.054, 3.953, 3.948, 3.935, 3.956, 3.962, 3.962, 3.907, 3.947, 3.922, 3.935, 3.918, 3.771, 3.781, 3.802, 3.768, 3.769, 3.778, 3.765, 3.771, 3.764, 3.78]
    line [5.996, 5.412, 5.365, 4.934, 4.812, 4.615, 4.505, 4.502, 4.498, 4.513, 4.493, 4.516, 1010.559, 4.446, 4.485, 4.475, 4.504, 4.245, 4.254, 4.468, 4.247, 4.243, 4.234, 4.243, 4.243, 4.235, 4.241]
    line [6.954, 5.964, 6.351, 5.421, 5.428, 4.99, 4.786, 4.638, 4.644, 4.919, 4.636, 4.644, 1074.518, 4.627, 4.68, 4.637, 4.936, 4.592, 4.6, 7.767, 4.561, 4.557, 4.526, 4.571, 4.575, 4.55, 4.593]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 2:00 stall; 2:01 pricing-0 pause; 2:01 pause pricing-0 for 2s; 2:03 pricing-0 unpause.</sub>

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
    x-axis "seconds" 0 --> 260
    y-axis "% of requests" 0 --> 40
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 40, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 34.538, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 2:00 stall; 2:01 pricing-0 pause; 2:01 pause pricing-0 for 2s; 2:03 pricing-0 unpause.</sub>

| Service | Failed requests | Share of its failures | Cause | From calls to | First seen | What it means |
| --- | --- | --- | --- | --- | --- | --- |
| api | 425 | 82% | `circuit_open` | pricing | 2:00 | the circuit breaker to pricing was open |
| api | 93 | 18% | `timeout` | pricing | 2:00 | calls to pricing ran past their timeout |

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
    x-axis "seconds" 0 --> 260
    y-axis "% of limit" 0 --> 100
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [5.879, 5.984, 6.131, 6.487, 6.777, 6.598, 6.332, 6.251, 6.115, 6.556, 6.2, 6.354, 7.122, 6.333, 6.044, 6.024, 6.307, 5.972, 5.957, 6.627, 5.804, 5.832, 5.856, 5.872, 5.885, 5.987, 5.864]
    line [5.098, 5.185, 4.678, 4.902, 4.486, 3.821, 3.03, 3.024, 2.917, 3.145, 2.982, 3.029, 2.877, 2.448, 2.535, 2.869, 2.976, 2.674, 2.733, 3.283, 2.689, 2.697, 2.637, 2.581, 2.679, 2.542, 2.634]
    line [10.532, 16.826, 22.526, 27.731, 33.615, 38.684, 40.686, 40.737, 40.573, 40.729, 40.711, 40.952, 41.961, 40.873, 40.576, 40.536, 40.636, 40.415, 40.477, 40.975, 40.536, 40.444, 40.518, 40.433, 40.448, 40.293, 38.97]
```

<sub>Blue: api. Orange: lb. Green: pricing (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 2:00 stall; 2:01 pricing-0 pause; 2:01 pause pricing-0 for 2s; 2:03 pricing-0 unpause.</sub>

## Diagnosis

No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version 21; thresholds below).

## Predictions

The design has no `expect` lines. Writing a prediction before a run turns it into a test of your mental model.

## Timeline

| Time | Event |
| --- | --- |
| 2:00 | stall: p99 reached 1.07 s (7.0 ms at low load), 35% of requests failed, only 65% of the offered rate was served for 10 s, then recovered; too short to count as a break; no diagnosis covers it |
| 2:01 | container: pricing-0 pause |
| 2:01 | fault started: pause pricing-0 for 2s |
| 2:03 | container: pricing-0 unpause |
| 2:03 | fault ended: pause pricing-0 for 2s |

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
| resources.cpu_period_ms | 100 ms for api, lb, pricing | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |

### How the run was planned

- load generator: 227 VUs started before the load, the scenario's measured prealloc_vus 227 at --scale 1, more than 150 rps × the SLO's 500 ms
- Loadlore's own containers get fixed budgets: load generator 1.5 CPU (peak 150 rps), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each. No fault acts on a connection, so every connection is direct and there is no Toxiproxy

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

- loadl dev, profile 4m30s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 977 (hash 946, memory 1011, syscalls 976): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (977) was within 5.0% of the run it's compared with (966)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 36,599 requests, 0 dropped arrivals, 1.4% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-slow-dependency-circuit-breaker-20261117-224638-api-0-1 | loadlore/synthetic:dev |
| loadlore-slow-dependency-circuit-breaker-20261117-224638-api-1-1 | loadlore/synthetic:dev |
| loadlore-slow-dependency-circuit-breaker-20261117-224638-api-2-1 | loadlore/synthetic:dev |
| loadlore-slow-dependency-circuit-breaker-20261117-224638-lb-1 | haproxy:3.2.26 |
| loadlore-slow-dependency-circuit-breaker-20261117-224638-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-slow-dependency-circuit-breaker-20261117-224638-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-slow-dependency-circuit-breaker-20261117-224638-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-slow-dependency-circuit-breaker-20261117-224638-pricing-0-1 | loadlore/synthetic:dev |

</details>

## Per-window data

<details><summary>27 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | lb CPU | lb up | pricing CPU | api→pricing pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 31 | 31 | 0 | 0% | 5.1 ms | 7.0 ms | 5.9% | 5.1% | 3 | 11% | 0/300 | 0.0 ms | 3.2% | 0% |
| 0:10 | 53 | 52 | 0 | 0% | 4.5 ms | 6.0 ms | 6.0% | 5.2% | 3 | 17% | 2/300 | 0.0 ms | 2.6% | 0% |
| 0:20 | 74 | 74 | 0 | 0% | 4.4 ms | 6.4 ms | 6.1% | 4.7% | 3 | 23% | 0/300 | 0.0 ms | 2.8% | 0% |
| 0:30 | 96 | 96 | 0 | 0% | 4.2 ms | 5.4 ms | 6.5% | 4.9% | 3 | 28% | 1/300 | 0.0 ms | 2.7% | 0% |
| 0:40 | 118 | 117 | 0 | 0% | 4.2 ms | 5.4 ms | 6.8% | 4.5% | 3 | 34% | 2/300 | 0.0 ms | 2.8% | 0% |
| 0:50 | 139 | 139 | 0 | 0% | 4.1 ms | 5.0 ms | 6.6% | 3.8% | 3 | 39% | 1/300 | 0.0 ms | 2.3% | 0% |
| 1:00 | 150 | 150 | 0 | 0% | 4.0 ms | 4.8 ms | 6.3% | 3.0% | 3 | 41% | 2/300 | 0.0 ms | 2.0% | 0% |
| 1:10 | 150 | 150 | 0 | 0% | 3.9 ms | 4.6 ms | 6.3% | 3.0% | 3 | 41% | 2/300 | 0.0 ms | 2.0% | 0% |
| 1:20 | 150 | 150 | 0 | 0% | 3.9 ms | 4.6 ms | 6.1% | 2.9% | 3 | 41% | 2/300 | 0.0 ms | 1.9% | 0% |
| 1:30 | 150 | 150 | 0 | 0% | 4.0 ms | 4.9 ms | 6.6% | 3.1% | 3 | 41% | 2/300 | 0.0 ms | 2.1% | 0% |
| 1:40 | 150 | 150 | 0 | 0% | 4.0 ms | 4.6 ms | 6.2% | 3.0% | 3 | 41% | 2/300 | 0.0 ms | 2.0% | 0% |
| 1:50 | 150 | 150 | 0 | 0% | 4.0 ms | 4.6 ms | 6.4% | 3.0% | 3 | 41% | 2/300 | 0.0 ms | 2.1% | 0% |
| 2:00 | 150 | 98 | 0 | 35% | 3.9 ms | 1.07 s | 7.1% | 2.9% | 3 | 42% | 88/300 +1 | 0.2 ms | 2.1% | 0% |
| 2:10 | 150 | 150 | 0 | 0% | 3.9 ms | 4.6 ms | 6.3% | 2.4% | 3 | 41% | 2/300 | 0.0 ms | 2.1% | 0% |
| 2:20 | 150 | 150 | 0 | 0% | 3.9 ms | 4.7 ms | 6.0% | 2.5% | 3 | 41% | 2/300 | 0.0 ms | 2.0% | 0% |
| 2:30 | 150 | 150 | 0 | 0% | 3.9 ms | 4.6 ms | 6.0% | 2.9% | 3 | 41% | 2/300 | 0.0 ms | 2.0% | 0% |
| 2:40 | 150 | 150 | 0 | 0% | 3.9 ms | 4.9 ms | 6.3% | 3.0% | 3 | 41% | 2/300 | 0.0 ms | 2.3% | 0% |
| 2:50 | 150 | 150 | 0 | 0% | 3.8 ms | 4.6 ms | 6.0% | 2.7% | 3 | 40% | 2/300 | 0.0 ms | 2.0% | 0% |
| 3:00 | 150 | 150 | 0 | 0% | 3.8 ms | 4.6 ms | 6.0% | 2.7% | 3 | 40% | 2/300 | 0.0 ms | 1.9% | 0% |
| 3:10 | 150 | 150 | 0 | 0% | 3.8 ms | 7.8 ms | 6.6% | 3.3% | 3 | 41% | 2/300 | 0.0 ms | 2.9% | 0% |
| 3:20 | 150 | 150 | 0 | 0% | 3.8 ms | 4.6 ms | 5.8% | 2.7% | 3 | 41% | 2/300 | 0.0 ms | 2.0% | 0% |
| 3:30 | 150 | 150 | 0 | 0% | 3.8 ms | 4.6 ms | 5.8% | 2.7% | 3 | 40% | 2/300 | 0.0 ms | 1.9% | 0% |
| 3:40 | 150 | 150 | 0 | 0% | 3.8 ms | 4.5 ms | 5.9% | 2.6% | 3 | 41% | 2/300 | 0.0 ms | 1.9% | 0% |
| 3:50 | 150 | 150 | 0 | 0% | 3.8 ms | 4.6 ms | 5.9% | 2.6% | 3 | 40% | 2/300 | 0.0 ms | 1.9% | 0% |
| 4:00 | 150 | 150 | 0 | 0% | 3.8 ms | 4.6 ms | 5.9% | 2.7% | 3 | 40% | 2/300 | 0.0 ms | 1.9% | 0% |
| 4:10 | 150 | 150 | 0 | 0% | 3.8 ms | 4.6 ms | 6.0% | 2.5% | 3 | 40% | 2/300 | 0.0 ms | 1.9% | 0% |
| 4:20 | 150 | 150 | 0 | 0% | 3.8 ms | 4.6 ms | 5.9% | 2.6% | 3 | 39% | 2/300 | 0.0 ms | 1.5% | 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 CPU is against its budget.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: slow-dependency-circuit-breaker

# Solution: a circuit breaker on each api replica. When half its calls to
# pricing fail over 5 seconds, it stops calling for 2, failing those
# requests at once; pricing gets no traffic and drains its backlog, and a
# trial call closes the breaker again. The retries stay as they were.

services:
  lb:
    kind: lb
    target: api
    health_check: { interval_ms: 2000, fall: 3, rise: 2 }

  api:
    image: loadlore/synthetic
    replicas: 3
    resources: { cpu: 0.5, memory: 256m }
    behavior:
      cpu_ms: 0.3
      calls:
        - { target: pricing, op: http }
      resilience:
        timeout_ms: 250
        retries: { max: 3 }
        circuit_breaker: { error_rate: 0.5, window_s: 5, open_s: 2 }
    pools:
      pricing: 100

  pricing:
    image: loadlore/synthetic
    resources: { cpu: 1, memory: 256m }
    behavior:
      cpu_ms: 2.5   # about 400 requests a second on its one CPU

workload:
  entry: lb
  profile:
    - ramp: { from: 20, to: 150, over: 1m }
    - hold: { rps: 150, for: 3m30s }
  mix: { read: 1.0 }

# pricing stops for two seconds, as a garbage-collection pause or a node
# hiccup would stop it.
scenario:
  - { at: 2m1s, inject: pause, target: pricing, for: 2s }
```
