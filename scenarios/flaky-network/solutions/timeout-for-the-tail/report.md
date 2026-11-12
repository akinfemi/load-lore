# flaky-network-timeout-for-the-tail: run report

_run `flaky-network-timeout-for-the-tail-20261112-115418` · completed · 12 Nov 2026 11:54 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 300 rps offered, 301 rps served at best, worst 10-second p99 226 ms, at most 0% errors.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 2:50

Measured in the 10-second window from 2:50 to 3:00 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 300 rps<br/>offered 300 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 4.5%<br/>2/2 replicas up"}}
    api["api ×2<br/>0.5 CPU · 256m<br/>CPU 30%<br/>in flight 16"]
    inventory["inventory<br/>1 CPU · 256m<br/>CPU 48%<br/>in flight 0"]
    load -->|"100% read · 0% write<br/>301 rps · p99 225 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>300 rps · p99 247 ms"| api
    api -->|"http ×10<br/>pool 50<br/>3,002 calls/s · p99 200 ms<br/>in use 15/100"| inventory
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
    line [90.837, 174.484, 257.81, 300, 299.999, 299.971, 299.398, 300.127, 300.123, 299.882, 299.899, 300.069, 299.832, 300.272, 300.321, 299.382, 299.541, 300.597, 300.558, 300, 300.001, 299.999, 300.03, 299.995]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 1:01 naughty_sanderson start; 1:01 naughty_sanderson die (exit code 0); 1:01 loss 1% of the packets inventory sends from port 8080 (its replies) for 2m0s; 3:01 charming_bardeen start; 3:01 charming_bardeen die (exit code 0).</sub>

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
    y-axis "ms" 0 --> 250
    line [0, 0, 0, 0, 0, 0, 250, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 250, 0, 0, 0, 0, 0]
    line [3.123, 2.69, 2.673, 2.662, 2.663, 2.662, 2.681, 2.682, 2.683, 2.684, 2.681, 2.681, 2.684, 2.685, 2.683, 2.681, 2.683, 2.685, 2.668, 2.664, 2.663, 2.662, 2.663, 2.664]
    line [4.013, 3.01, 2.985, 2.95, 2.957, 2.957, 205.093, 203.478, 205.183, 206.213, 203.231, 205.061, 205.52, 204.909, 204.71, 202.598, 205.352, 204.972, 3.045, 2.963, 2.961, 2.963, 2.97, 2.97]
    line [4.531, 3.645, 3.622, 4.044, 4.054, 3.858, 225.698, 225.443, 224.771, 225.448, 223.574, 225.987, 225.223, 225.759, 225.739, 223.931, 225.196, 225.055, 206.535, 3.568, 3.699, 3.711, 3.698, 3.684]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 1:01 naughty_sanderson start; 1:01 naughty_sanderson die (exit code 0); 1:01 loss 1% of the packets inventory sends from port 8080 (its replies) for 2m0s; 3:01 charming_bardeen start; 3:01 charming_bardeen die (exit code 0).</sub>

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
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 1:01 naughty_sanderson start; 1:01 naughty_sanderson die (exit code 0); 1:01 loss 1% of the packets inventory sends from port 8080 (its replies) for 2m0s; 3:01 charming_bardeen start; 3:01 charming_bardeen die (exit code 0).</sub>

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
    line [13.813, 18.291, 26.043, 29.815, 30.04, 29.918, 29.86, 29.831, 29.847, 29.994, 30.012, 29.983, 29.79, 29.884, 29.926, 29.787, 29.816, 29.914, 29.808, 29.765, 29.821, 29.878, 29.844, 29.467]
    line [17.238, 28.845, 41.69, 48.275, 48.296, 48.352, 48.077, 48.279, 48.22, 48.128, 48.311, 48.222, 48.148, 48.219, 48.145, 48.18, 48.223, 48.254, 48.362, 48.313, 48.272, 48.306, 48.259, 46.084]
    line [4.923, 3.785, 4.957, 5.417, 5.306, 5.433, 4.697, 4.516, 4.447, 4.534, 4.471, 4.605, 4.498, 4.513, 4.424, 4.626, 4.601, 4.515, 3.981, 4.908, 5.225, 5.195, 5.146, 5.111]
```

<sub>Blue: api. Orange: inventory. Green: lb (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 1:01 naughty_sanderson start; 1:01 naughty_sanderson die (exit code 0); 1:01 loss 1% of the packets inventory sends from port 8080 (its replies) for 2m0s; 3:01 charming_bardeen start; 3:01 charming_bardeen die (exit code 0).</sub>

## Diagnosis

No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version 8; thresholds below).

## Predictions

The design has no `expect` lines. Writing a prediction before a run turns it into a test of your mental model.

## Timeline

| Time | Event |
| --- | --- |
| 1:01 | container: naughty_sanderson start |
| 1:01 | container: naughty_sanderson die (exit code 0) |
| 1:01 | fault started: loss 1% of the packets inventory sends from port 8080 (its replies) for 2m0s |
| 3:01 | container: charming_bardeen start |
| 3:01 | container: charming_bardeen die (exit code 0) |
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
- Host speed index 987 (hash 956, memory 1008, syscalls 997): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (987) was within 5.0% of the run it's compared with (1008)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 68,249 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-flaky-network-timeout-for-the-tail-20261112-115418-api-0-1 | loadlore/synthetic:dev |
| loadlore-flaky-network-timeout-for-the-tail-20261112-115418-api-1-1 | loadlore/synthetic:dev |
| loadlore-flaky-network-timeout-for-the-tail-20261112-115418-inventory-0-1 | loadlore/synthetic:dev |
| loadlore-flaky-network-timeout-for-the-tail-20261112-115418-lb-1 | haproxy:3.2.26 |
| loadlore-flaky-network-timeout-for-the-tail-20261112-115418-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-flaky-network-timeout-for-the-tail-20261112-115418-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-flaky-network-timeout-for-the-tail-20261112-115418-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-flaky-network-timeout-for-the-tail-20261112-115418-lore-toxiproxy-1 | ghcr.io/shopify/toxiproxy:2.13.0 |

</details>

## Per-window data

<details><summary>24 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | inventory CPU | lb CPU | lb up | api→inventory pool | wait | k6 CPU | Toxiproxy CPU | k6 / Toxiproxy stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 92 | 91 | 0 | 0% | 3.1 ms | 4.5 ms | 14% | 17% | 4.9% | 2 | 1/100 | 0.0 ms | 2.2% | 10% | 0% / 0% |
| 0:10 | 175 | 174 | 0 | 0% | 2.7 ms | 3.6 ms | 18% | 29% | 3.8% | 2 | 2/100 | 0.0 ms | 2.4% | 14% | 0% / 0% |
| 0:20 | 258 | 258 | 0 | 0% | 2.7 ms | 3.6 ms | 26% | 42% | 5.0% | 2 | 1/100 | 0.0 ms | 3.0% | 20% | 0% / 0% |
| 0:30 | 300 | 300 | 0 | 0% | 2.7 ms | 4.0 ms | 30% | 48% | 5.4% | 2 | 2/100 | 0.0 ms | 3.3% | 22% | 0% / 0% |
| 0:40 | 300 | 300 | 0 | 0% | 2.7 ms | 4.1 ms | 30% | 48% | 5.3% | 2 | 1/100 | 0.0 ms | 3.3% | 22% | 0% / 0% |
| 0:50 | 300 | 300 | 0 | 0% | 2.7 ms | 3.9 ms | 30% | 48% | 5.4% | 2 | 1/100 | 0.0 ms | 3.3% | 22% | 0% / 0% |
| 1:00 | 300 | 299 | 0 | 0% | 2.7 ms | 226 ms | 30% | 48% | 4.7% | 2 | 9/100 | 0.0 ms | 3.4% | 22% | 0% / 0% |
| 1:10 | 300 | 300 | 0 | 0% | 2.7 ms | 225 ms | 30% | 48% | 4.5% | 2 | 9/100 | 0.0 ms | 3.3% | 22% | 0% / 0% |
| 1:20 | 300 | 300 | 0 | 0% | 2.7 ms | 225 ms | 30% | 48% | 4.4% | 2 | 10/100 | 0.0 ms | 3.4% | 22% | 0% / 0% |
| 1:30 | 300 | 300 | 0 | 0% | 2.7 ms | 225 ms | 30% | 48% | 4.5% | 2 | 10/100 | 0.0 ms | 3.4% | 22% | 0% / 0% |
| 1:40 | 300 | 300 | 0 | 0% | 2.7 ms | 224 ms | 30% | 48% | 4.5% | 2 | 12/100 | 0.0 ms | 3.4% | 22% | 0% / 0% |
| 1:50 | 300 | 300 | 0 | 0% | 2.7 ms | 226 ms | 30% | 48% | 4.6% | 2 | 12/100 | 0.0 ms | 3.3% | 22% | 0% / 0% |
| 2:00 | 300 | 300 | 0 | 0% | 2.7 ms | 225 ms | 30% | 48% | 4.5% | 2 | 12/100 +1 | 0.0 ms | 3.4% | 22% | 0% / 0% |
| 2:10 | 300 | 300 | 0 | 0% | 2.7 ms | 226 ms | 30% | 48% | 4.5% | 2 | 12/100 | 0.0 ms | 3.5% | 22% | 0% / 0% |
| 2:20 | 300 | 300 | 0 | 0% | 2.7 ms | 226 ms | 30% | 48% | 4.4% | 2 | 13/100 | 0.0 ms | 3.4% | 22% | 0% / 0% |
| 2:30 | 300 | 299 | 0 | 0% | 2.7 ms | 224 ms | 30% | 48% | 4.6% | 2 | 8/100 | 0.0 ms | 3.4% | 22% | 0% / 0% |
| 2:40 | 300 | 300 | 0 | 0% | 2.7 ms | 225 ms | 30% | 48% | 4.6% | 2 | 9/100 | 0.0 ms | 3.3% | 22% | 0% / 0% |
| 2:50 | 300 | 301 | 0 | 0% | 2.7 ms | 225 ms | 30% | 48% | 4.5% | 2 | 15/100 | 0.0 ms | 3.4% | 22% | 0% / 0% |
| 3:00 | 300 | 301 | 0 | 0% | 2.7 ms | 207 ms | 30% | 48% | 4.0% | 2 | 6/100 | 0.0 ms | 3.3% | 23% | 0% / 0% |
| 3:10 | 300 | 300 | 0 | 0% | 2.7 ms | 3.6 ms | 30% | 48% | 4.9% | 2 | 1/100 | 0.0 ms | 3.3% | 22% | 0% / 0% |
| 3:20 | 300 | 300 | 0 | 0% | 2.7 ms | 3.7 ms | 30% | 48% | 5.2% | 2 | 0/100 | 0.0 ms | 3.2% | 22% | 0% / 0% |
| 3:30 | 300 | 300 | 0 | 0% | 2.7 ms | 3.7 ms | 30% | 48% | 5.2% | 2 | 1/100 | 0.0 ms | 3.2% | 22% | 0% / 0% |
| 3:40 | 300 | 300 | 0 | 0% | 2.7 ms | 3.7 ms | 30% | 48% | 5.1% | 2 | 1/100 | 0.0 ms | 3.3% | 22% | 0% / 0% |
| 3:50 | 300 | 300 | 0 | 0% | 2.7 ms | 3.7 ms | 29% | 46% | 5.1% | 2 | 1/100 | 0.0 ms | 2.6% | 22% | 0% / 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 and Toxiproxy CPU are against their budgets.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: flaky-network-timeout-for-the-tail

# Solution: a timeout sized to the tail, 1 s per call. A call whose reply
# lost a packet waits for TCP to resend it, about 200 ms, and still
# succeeds: the loss shows as a slower page, not a failed one.

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
        timeout_ms: 1000
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
