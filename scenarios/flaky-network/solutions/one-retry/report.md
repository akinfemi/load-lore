# flaky-network-one-retry: run report

_run `flaky-network-one-retry-20261112-114959` · completed · 12 Nov 2026 11:50 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 300 rps offered, 300 rps served at best, worst 10-second p99 162 ms, at most 0.2% errors.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 3:00

Measured in the 10-second window from 3:00 to 3:10 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 300 rps<br/>offered 300 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 4.0%<br/>2/2 replicas up"}}
    api["api ×2<br/>0.5 CPU · 256m<br/>CPU 30%<br/>in flight 4"]
    inventory["inventory<br/>1 CPU · 256m<br/>CPU 49%<br/>in flight 0"]
    load -->|"100% read · 0% write<br/>301 rps · p99 156 ms · #lt;0.1% errors"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>300 rps · p99 151 ms · #lt;0.1% errors"| api
    api -->|"http ×10<br/>pool 50<br/>3,004 calls/s · p99 0.5 ms · 0.1% errors<br/>in use 4/100"| inventory
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
    line [0, 0, 0, 0, 0, 0, 400, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 400, 0, 0, 0, 0, 0]
    line [91.667, 175, 258.333, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300]
    line [90.6, 174.336, 257.669, 299.974, 300.001, 300.028, 299.701, 299.501, 299.414, 299.611, 300.176, 299.969, 299.628, 299.501, 299.201, 299.676, 299.824, 299.271, 300.499, 300.03, 299.98, 300.02, 300, 300]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 1:01 happy_robinson start; 1:01 happy_robinson die (exit code 0); 1:01 loss 1% of the packets inventory sends from port 8080 (its replies) for 2m0s; 3:01 priceless_kapitsa start; 3:01 priceless_kapitsa die (exit code 0).</sub>

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
    line [0, 0, 0, 0, 0, 0, 200, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 200, 0, 0, 0, 0, 0]
    line [3.137, 2.684, 2.672, 2.663, 2.663, 2.661, 2.686, 2.694, 2.693, 2.695, 2.695, 2.695, 2.699, 2.691, 2.695, 2.696, 2.694, 2.691, 2.671, 2.666, 2.664, 2.662, 2.661, 2.664]
    line [4.093, 3.029, 2.986, 2.962, 2.955, 2.944, 154.035, 155.378, 155.486, 155.38, 155.722, 155.007, 156.129, 155.604, 155.474, 155.816, 155.705, 154.601, 3.226, 2.979, 2.968, 2.963, 2.986, 2.972]
    line [5.603, 3.896, 4.026, 4.202, 3.972, 3.995, 161.205, 161.103, 161.373, 161.314, 161.272, 161.221, 161.484, 161.559, 161.548, 161.169, 161.566, 161.033, 155.669, 3.747, 3.741, 3.663, 4.088, 3.728]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 1:01 happy_robinson start; 1:01 happy_robinson die (exit code 0); 1:01 loss 1% of the packets inventory sends from port 8080 (its replies) for 2m0s; 3:01 priceless_kapitsa start; 3:01 priceless_kapitsa die (exit code 0).</sub>

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
    line [0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0]
    line [0, 0, 0, 0, 0, 0, 0.033, 0.067, 0.125, 0.075, 0.033, 0.067, 0.133, 0.164, 0.206, 0.164, 0.124, 0.133, 0.01, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 1:01 happy_robinson start; 1:01 happy_robinson die (exit code 0); 1:01 loss 1% of the packets inventory sends from port 8080 (its replies) for 2m0s; 3:01 priceless_kapitsa start; 3:01 priceless_kapitsa die (exit code 0).</sub>

| Service | Failed requests | Share of its failures | Cause | From calls to | First seen | What it means |
| --- | --- | --- | --- | --- | --- | --- |
| api | 40 | 100% | `timeout` | inventory | 1:00 | calls to inventory ran past their timeout |

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
    line [0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0]
    line [14.124, 18.26, 26.124, 30.063, 29.98, 30.015, 30.561, 30.687, 30.842, 30.787, 30.87, 30.659, 30.853, 30.632, 30.992, 30.811, 30.727, 30.68, 30.028, 30.109, 29.975, 29.9, 30.078, 29.166]
    line [18.328, 28.772, 41.785, 48.44, 48.341, 48.408, 48.714, 48.817, 48.888, 48.841, 48.979, 48.703, 48.892, 48.906, 48.904, 48.795, 48.834, 48.738, 48.516, 48.469, 48.324, 48.245, 48.221, 46.923]
    line [4.999, 3.836, 5.015, 5.51, 5.486, 5.437, 4.909, 4.553, 4.576, 4.496, 4.638, 4.632, 4.502, 4.412, 4.653, 4.561, 4.626, 4.716, 4.022, 5.354, 5.397, 5.307, 5.345, 4.794]
```

<sub>Blue: api. Orange: inventory. Green: lb (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 1:01 happy_robinson start; 1:01 happy_robinson die (exit code 0); 1:01 loss 1% of the packets inventory sends from port 8080 (its replies) for 2m0s; 3:01 priceless_kapitsa start; 3:01 priceless_kapitsa die (exit code 0).</sub>

## Diagnosis

No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version 8; thresholds below).

## Predictions

The design has no `expect` lines. Writing a prediction before a run turns it into a test of your mental model.

## Timeline

| Time | Event |
| --- | --- |
| 1:01 | container: happy_robinson start |
| 1:01 | container: happy_robinson die (exit code 0) |
| 1:01 | fault started: loss 1% of the packets inventory sends from port 8080 (its replies) for 2m0s |
| 3:01 | container: priceless_kapitsa start |
| 3:01 | container: priceless_kapitsa die (exit code 0) |
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
| resources.cpu_period_ms | 100 ms for api, inventory, lb | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |

### How the run was planned

- Loadlore's own containers get fixed budgets: load generator 1.5 CPU (peak 300 rps), Toxiproxy 1 CPU (about 3300 proxied calls/s at peak), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each

## Thresholds

- **Broke:** 2 windows in a row with at least 1.0% of requests failing or successful responses below 90% of the offered rate, or 3 windows in a row with any of those or p99 at least 10x the low-load p99 (the median p99 of windows offered at most 25% of the peak, before errors or a shortfall in 2 windows in a row) and at least the SLO, 500 ms (workload.slo.p99_ms): 500 ms in this run. A shorter episode is shown as a stall in the timeline, not a break.
- **Doubtful:** a break on p99 alone, when the load generator was stalled at its CPU limit in 1.0% or more of its scheduling periods in one of the break's windows and the entry service's own p99 (loadlore_request_duration_seconds (the service's own histogram)) was below the limit in one of them. The verdict stays what it was.
- **Inconclusive:** before the break (or anywhere, if nothing broke), the load generator dropped more than 1.0% of a window's arrivals (more than 5.0% in a window where the system itself failed 1.0% or more of requests; up to that it's a caveat that gives the share), or the load generator or Toxiproxy used 90% or more of its CPU budget, or was stalled at its CPU limit in 5.0% or more of its scheduling periods (from 1.0% it's noted as a caveat). The first 10 s of load are a warm-up and don't count: the load generator starts its VUs and connections open then. A cause is named only with evidence: on a Linux host, the hypervisor giving 2.0% or more of CPU time to other virtual machines (steal), or other programs using 10% or more of the host's CPUs (at least 0.5); inside a VM on this machine, only as a possibility.
- **Connection pool exhausted:** for two windows in a row, a pool at 95% or more of its size with callers waiting, while the target wasn't busy (busy: its CPU at 80% or more, or its time per call with a connection at least 3x the low-load value and 1 ms above it; CPU throttling alone doesn't count) and the caller's CPU stayed below 90%.
- **Queueing knee:** for 3 windows in a row, p99 at least 3x the low-load p99 and 10 ms above it, with a CPU or pool at 70% or more. When that is a pool whose target was busy in any of those windows, or a service's CPU whose requests spent 50% or more of their time in calls to a busy target, the knee is the target's.
- **Connection storm:** up to the break, a service's pools to a Postgres opening at least 50% of their combined size, and 10 or more connections, in one window, while p99 in that window or the next was at least 10x the low-load p99 and 10 ms above it. New connections are counted by loadlore_pool_connect_seconds (new connections the pools opened, and how long each took), or for runs from before it, from the rise in open connections.
- **Retry storm:** for two windows in a row, a caller that retries sending 1.5x or more of its low-load calls per request to a target, with retries 20% or more of them, while the target failed 1.0% or more of its calls or its p99 was 3x its low-load value.
- **Cold cache:** a cache's share of lookups that miss rising to 5x its level before or more, and to 5.0% of lookups at least, while its callers' reads of the database behind it reach 2x their rate before (counted as at least 1 a second, for a cache warm enough to send none).
- **Working set doesn't fit:** for two windows in a row, a Postgres answering less than 90% of block reads from shared_buffers and reading 100 blocks a second or more from outside them, while p99 was 3x the low-load p99 or more.
- **Hot rows:** for two windows in a row, 1 or more of a Postgres's backends waiting on locks on average, while a caller's writes to it took 3x their low-load time or more (and 2 ms more), its reads stayed within 1.5x of theirs and the database's CPU was below 80%. Windows with a disk_slow fault on the database don't count.

## Environment

- loadl dev, profile 4m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 985 (hash 944, memory 1018, syscalls 996): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (985) was within 5.0% of the run it's compared with (1008)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 68,249 requests, 0 dropped arrivals, <0.1% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-flaky-network-one-retry-20261112-114959-api-0-1 | loadlore/synthetic:dev |
| loadlore-flaky-network-one-retry-20261112-114959-api-1-1 | loadlore/synthetic:dev |
| loadlore-flaky-network-one-retry-20261112-114959-inventory-0-1 | loadlore/synthetic:dev |
| loadlore-flaky-network-one-retry-20261112-114959-lb-1 | haproxy:3.2.26 |
| loadlore-flaky-network-one-retry-20261112-114959-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-flaky-network-one-retry-20261112-114959-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-flaky-network-one-retry-20261112-114959-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-flaky-network-one-retry-20261112-114959-lore-toxiproxy-1 | ghcr.io/shopify/toxiproxy:2.13.0 |

</details>

## Per-window data

<details><summary>24 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | inventory CPU | lb CPU | lb up | api→inventory pool | wait | k6 CPU | Toxiproxy CPU | k6 / Toxiproxy stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 92 | 91 | 0 | 0% | 3.1 ms | 5.6 ms | 14% | 18% | 5.0% | 2 | 1/100 | 0.0 ms | 2.3% | 10% | 0% / 0% |
| 0:10 | 175 | 174 | 0 | 0% | 2.7 ms | 3.9 ms | 18% | 29% | 3.8% | 2 | 1/100 | 0.0 ms | 2.4% | 13% | 0% / 0% |
| 0:20 | 258 | 258 | 0 | 0% | 2.7 ms | 4.0 ms | 26% | 42% | 5.0% | 2 | 2/100 | 0.0 ms | 3.0% | 20% | 0% / 0% |
| 0:30 | 300 | 300 | 0 | 0% | 2.7 ms | 4.2 ms | 30% | 48% | 5.5% | 2 | 0/100 | 0.0 ms | 3.3% | 22% | 0% / 0% |
| 0:40 | 300 | 300 | 0 | 0% | 2.7 ms | 4.0 ms | 30% | 48% | 5.5% | 2 | 1/100 | 0.0 ms | 3.2% | 22% | 0% / 0% |
| 0:50 | 300 | 300 | 0 | 0% | 2.7 ms | 4.0 ms | 30% | 48% | 5.4% | 2 | 1/100 | 0.0 ms | 3.3% | 22% | 0% / 0% |
| 1:00 | 300 | 300 | 0 | <0.1% | 2.7 ms | 161 ms | 31% | 49% | 4.9% | 2 | 9/100 | 0.0 ms | 3.4% | 23% | 0% / 0% |
| 1:10 | 300 | 300 | 0 | <0.1% | 2.7 ms | 161 ms | 31% | 49% | 4.6% | 2 | 6/100 | 0.0 ms | 3.3% | 23% | 0% / 0% |
| 1:20 | 300 | 299 | 0 | 0.1% | 2.7 ms | 161 ms | 31% | 49% | 4.6% | 2 | 6/100 | 0.0 ms | 3.4% | 23% | 0% / 0% |
| 1:30 | 300 | 300 | 0 | <0.1% | 2.7 ms | 161 ms | 31% | 49% | 4.5% | 2 | 9/100 +1 | 0.0 ms | 3.4% | 23% | 0% / 0% |
| 1:40 | 300 | 300 | 0 | <0.1% | 2.7 ms | 161 ms | 31% | 49% | 4.6% | 2 | 9/100 | 0.0 ms | 3.4% | 23% | 0% / 0% |
| 1:50 | 300 | 300 | 0 | <0.1% | 2.7 ms | 161 ms | 31% | 49% | 4.6% | 2 | 8/100 | 0.0 ms | 3.3% | 23% | 0% / 0% |
| 2:00 | 300 | 300 | 0 | 0.1% | 2.7 ms | 161 ms | 31% | 49% | 4.5% | 2 | 7/100 | 0.0 ms | 3.4% | 23% | 0% / 0% |
| 2:10 | 300 | 300 | 0 | 0.2% | 2.7 ms | 162 ms | 31% | 49% | 4.4% | 2 | 7/100 | 0.0 ms | 3.4% | 23% | 0% / 0% |
| 2:20 | 300 | 299 | 0 | 0.2% | 2.7 ms | 162 ms | 31% | 49% | 4.7% | 2 | 6/100 +1 | 0.0 ms | 3.4% | 23% | 0% / 0% |
| 2:30 | 300 | 300 | 0 | 0.2% | 2.7 ms | 161 ms | 31% | 49% | 4.6% | 2 | 8/100 | 0.0 ms | 3.4% | 23% | 0% / 0% |
| 2:40 | 300 | 300 | 0 | 0.1% | 2.7 ms | 162 ms | 31% | 49% | 4.6% | 2 | 8/100 | 0.0 ms | 3.3% | 23% | 0% / 0% |
| 2:50 | 300 | 299 | 0 | 0.1% | 2.7 ms | 161 ms | 31% | 49% | 4.7% | 2 | 7/100 | 0.0 ms | 3.4% | 23% | 0% / 0% |
| 3:00 | 300 | 300 | 0 | <0.1% | 2.7 ms | 156 ms | 30% | 49% | 4.0% | 2 | 4/100 | 0.0 ms | 3.3% | 23% | 0% / 0% |
| 3:10 | 300 | 300 | 0 | 0% | 2.7 ms | 3.7 ms | 30% | 48% | 5.4% | 2 | 1/100 | 0.0 ms | 3.4% | 23% | 0% / 0% |
| 3:20 | 300 | 300 | 0 | 0% | 2.7 ms | 3.7 ms | 30% | 48% | 5.4% | 2 | 1/100 | 0.0 ms | 3.3% | 22% | 0% / 0% |
| 3:30 | 300 | 300 | 0 | 0% | 2.7 ms | 3.7 ms | 30% | 48% | 5.3% | 2 | 1/100 | 0.0 ms | 3.3% | 22% | 0% / 0% |
| 3:40 | 300 | 300 | 0 | 0% | 2.7 ms | 4.1 ms | 30% | 48% | 5.3% | 2 | 1/100 | 0.0 ms | 3.3% | 23% | 0% / 0% |
| 3:50 | 300 | 300 | 0 | 0% | 2.7 ms | 3.7 ms | 29% | 47% | 4.8% | 2 | 2/100 | 0.0 ms | 3.0% | 22% | 0% / 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 and Toxiproxy CPU are against their budgets.

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
