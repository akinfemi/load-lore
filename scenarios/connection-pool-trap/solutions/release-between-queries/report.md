# connection-pool-trap-release-between-queries: run report

_run `connection-pool-trap-release-between-queries-20261111-122145` · completed · 11 Nov 2026 12:21 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 1,400 rps offered, 1,400 rps served at best, worst 10-second p99 108 ms, at most 0% errors.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 5:40

Measured in the 10-second window from 5:40 to 5:50 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 1,400 rps<br/>offered 1,400 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 22%<br/>4/4 replicas up"}}
    api["api ×4<br/>0.6 CPU · 512m<br/>CPU 31%<br/>in flight 115"]
    db[("db<br/>postgres 17<br/>1 CPU · 1g<br/>CPU 7.9%<br/>conns 41/100")]
    payments["payments<br/>0.5 CPU · 256m<br/>CPU 30%<br/>in flight 0"]
    load -->|"90% read · 10% write<br/>1,400 rps · p99 88 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>1,400 rps · p99 100 ms"| api
    api -->|"read, write on writes<br/>pool 10<br/>1,539 calls/s · p99 0.7 ms<br/>in use 1/40"| db
    api -->|"http<br/>pool 50<br/>1,400 calls/s · p99 100 ms<br/>in use 115/200"| payments
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb,api,db,payments ok
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
    y-axis "requests/s" 0 --> 1500
    line [1500, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [121.667, 165, 208.333, 251.667, 295, 338.333, 381.667, 425, 468.333, 511.667, 555, 598.333, 641.667, 685, 728.333, 771.667, 815, 858.333, 901.667, 945, 988.333, 1031.667, 1075, 1118.333, 1161.667, 1205, 1248.333, 1291.667, 1335, 1378.333, 1400, 1400, 1400, 1400, 1400, 1400, 1400, 1400, 1400, 1400, 1400, 1400]
    line [120.147, 164.424, 208.02, 251.097, 294.298, 337.866, 381.159, 424.542, 467.82, 511.171, 554.738, 597.712, 641.075, 684.482, 727.951, 771.013, 814.429, 857.942, 901.32, 944.347, 987.795, 1031.177, 1074.572, 1117.858, 1161.203, 1204.357, 1247.931, 1290.952, 1334.636, 1377.679, 1400.182, 1400.016, 1400.031, 1399.673, 1400.289, 1399.986, 1400.181, 1399.652, 1400.164, 1400.002, 1400.171, 1399.813]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 0:00 latency +80 ms on connections into payments.</sub>

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
    y-axis "ms" 0 --> 120
    line [120, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [84.683, 84.511, 84.272, 84.056, 83.781, 83.591, 82.91, 82.737, 82.367, 82.093, 81.244, 81.4, 81.406, 81.654, 80.913, 80.826, 80.654, 80.638, 80.569, 80.437, 80.505, 80.56, 80.528, 80.632, 80.701, 80.761, 80.744, 80.785, 80.865, 81.429, 81.464, 80.932, 81.007, 80.903, 80.992, 80.96, 81.027, 81.115, 81.136, 81.03, 80.913, 81.141]
    line [88.04, 88.001, 87.977, 87.955, 87.928, 87.909, 87.841, 87.823, 87.786, 87.759, 87.674, 87.69, 87.692, 87.715, 87.639, 87.628, 87.605, 87.603, 87.593, 87.573, 87.583, 87.592, 87.587, 87.602, 87.612, 87.62, 87.618, 87.623, 87.633, 88.022, 87.753, 87.641, 87.65, 87.638, 87.648, 87.644, 87.652, 87.661, 87.663, 87.652, 87.639, 87.664]
    line [88.339, 88.311, 88.306, 88.302, 88.296, 88.292, 88.279, 88.275, 88.268, 88.262, 88.245, 88.249, 88.251, 88.254, 88.238, 88.236, 88.232, 88.231, 88.229, 88.225, 88.227, 88.229, 88.228, 88.231, 88.233, 88.235, 88.234, 88.235, 88.237, 107.849, 88.312, 88.239, 88.241, 88.238, 88.24, 88.24, 88.241, 88.243, 88.243, 88.241, 88.238, 88.243]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 0:00 latency +80 ms on connections into payments.</sub>

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
    line [1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 0:00 latency +80 ms on connections into payments.</sub>

```mermaid
---
config:
  themeVariables:
    xyChart:
      backgroundColor: "transparent"
      plotColorPalette: "#f4cccc, #1f77b4, #ff7f0e, #2ca02c, #9467bd"
---
xychart-beta
    title "CPU against each limit"
    x-axis "seconds" 0 --> 410
    y-axis "% of limit" 0 --> 100
    line [100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [5.289, 5.698, 6.585, 7.183, 7.975, 8.837, 9.443, 10.385, 11.2, 12.005, 12.855, 13.819, 14.644, 15.496, 16.33, 17.362, 18.216, 19.164, 20.054, 20.879, 21.829, 22.806, 23.708, 24.704, 25.673, 26.57, 27.317, 28.254, 29.139, 31.299, 31.721, 30.711, 30.748, 30.648, 30.776, 30.689, 30.775, 30.732, 30.721, 30.768, 30.704, 30.006]
    line [5.308, 4.071, 4.223, 4.246, 4.165, 4.298, 4.112, 4.371, 4.305, 4.463, 4.602, 4.813, 4.945, 5.056, 5.123, 5.493, 5.548, 5.674, 5.79, 6.014, 6.234, 6.436, 6.441, 6.842, 6.918, 7.184, 7.186, 7.509, 7.572, 9.239, 8.868, 8.01, 8.137, 7.984, 7.919, 8.039, 8.14, 8.051, 7.952, 8.069, 7.975, 7.883]
    line [5.191, 4.98, 5.589, 5.906, 6.183, 6.624, 6.981, 7.623, 8.094, 8.445, 9.134, 9.94, 10.381, 10.989, 11.763, 12.336, 12.88, 13.591, 13.975, 15.02, 15.606, 15.817, 16.506, 17.555, 18.104, 19.102, 19.296, 20.033, 20.398, 22.319, 18.574, 19.442, 21.295, 21.658, 21.807, 21.893, 21.808, 21.594, 21.696, 21.759, 21.423, 20.83]
    line [5.304, 6.557, 7.634, 8.021, 8.734, 9.484, 10.168, 11.301, 11.933, 12.934, 13.841, 14.734, 15.668, 16.694, 17.537, 18.446, 19.297, 20.144, 20.952, 21.552, 22.474, 23.272, 24.046, 24.975, 25.792, 26.736, 27.479, 28.297, 29.152, 31.318, 31.369, 30.402, 30.264, 30.319, 30.415, 30.336, 30.419, 30.271, 30.371, 30.425, 30.396, 29.295]
```

<sub>Blue: api. Orange: db. Green: lb. Purple: payments (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 0:00 latency +80 ms on connections into payments.</sub>

## Diagnosis

No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version 5; thresholds below).

## Predictions

2 of 2 scored predictions held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 300 until load_rps >= 1000` | **held** | 88 ms | 0:00 to 3:30, when load_rps >= 1,000 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |
| `error_rate < 1% throughout` | **held** | 0% | the whole run | source: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6) |

## Timeline

| Time | Event |
| --- | --- |
| 0:00 | fault started: latency +80 ms on connections into payments |
| 7:00 | fault ended: latency +80 ms on connections into payments |

<sub>From events.jsonl: the fault timeline and Docker container events.</sub>

## Assumptions

### From the design

The design lists no assumptions.

### Chosen by Loadlore

| Field | Value | Why |
| --- | --- | --- |
| workload.timeout_ms | 10s | the default; k6 gives up on a request after this long and counts it as failed |
| workload.slo.p99_ms | 500 ms | the default; a break on p99 alone needs p99 at or above this as well as 10x the low-load p99, so latency users would still accept isn't called a break |
| resources.cpu_period_ms | 100 ms for api, db, lb, payments | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |
| services.api.pools.db.min | 0 | the default, as pgxpool's: each replica opens connections only when callers need them, so a pool fills as load rises, and a burst opens many at once |
| services.api.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- Loadlore's own containers get fixed budgets: load generator 1.5 CPU (peak 1400 rps), Toxiproxy 1 CPU (about 4340 proxied calls/s at peak), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each

## Thresholds

- **Broke:** 2 windows in a row with at least 1.0% of requests failing or successful responses below 90% of the offered rate, or 3 windows in a row with any of those or p99 at least 10x the low-load p99 (the median p99 of windows offered at most 25% of the peak, before errors or a shortfall in 2 windows in a row) and at least the SLO, 500 ms (workload.slo.p99_ms): 883 ms in this run. A shorter episode is shown as a stall in the timeline, not a break.
- **Doubtful:** a break on p99 alone, when the load generator was stalled at its CPU limit in 1.0% or more of its scheduling periods in one of the break's windows and the entry service's own p99 (loadlore_request_duration_seconds (the service's own histogram)) was below the limit in one of them. The verdict stays what it was.
- **Inconclusive:** before the break (or anywhere, if nothing broke), the load generator dropped more than 1.0% of a window's arrivals, or the load generator or Toxiproxy used 90% or more of its CPU budget, or was stalled at its CPU limit in 5.0% or more of its scheduling periods (from 1.0% it's noted as a caveat). The first 10 s of load are a warm-up and don't count: the load generator starts its VUs and connections open then. A cause is named only with evidence: on a Linux host, the hypervisor giving 2.0% or more of CPU time to other virtual machines (steal), or other programs using 10% or more of the host's CPUs (at least 0.5); inside a VM on this machine, only as a possibility.
- **Connection pool exhausted:** for two windows in a row, a pool at 95% or more of its size with callers waiting, while the target wasn't busy (busy: its CPU at 80% or more, or its time per call with a connection at least 3x the low-load value and 1 ms above it; CPU throttling alone doesn't count) and the caller's CPU stayed below 90%.
- **Queueing knee:** for 3 windows in a row, p99 at least 3x the low-load p99 and 10 ms above it, with a CPU or pool at 70% or more. When that is a pool whose target was busy in any of those windows, or a service's CPU whose requests spent 50% or more of their time in calls to a busy target, the knee is the target's.
- **Connection storm:** up to the break, a service's pools to a Postgres opening at least 50% of their combined size, and 10 or more connections, in one window, while p99 in that window or the next was at least 10x the low-load p99 and 10 ms above it. New connections are counted by loadlore_pool_connect_seconds (new connections the pools opened, and how long each took), or for runs from before it, from the rise in open connections.

## Environment

- loadl dev, profile 7m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 952 (hash 937, memory 940, syscalls 979): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (952) was within 5.0% of the run it's compared with (991)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 392,998 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-connection-pool-trap-release-between-queries-20261111-122145-api-0-1 | loadlore/synthetic:dev |
| loadlore-connection-pool-trap-release-between-queries-20261111-122145-api-1-1 | loadlore/synthetic:dev |
| loadlore-connection-pool-trap-release-between-queries-20261111-122145-api-2-1 | loadlore/synthetic:dev |
| loadlore-connection-pool-trap-release-between-queries-20261111-122145-api-3-1 | loadlore/synthetic:dev |
| loadlore-connection-pool-trap-release-between-queries-20261111-122145-db-1 | postgres:17.11 |
| loadlore-connection-pool-trap-release-between-queries-20261111-122145-lb-1 | haproxy:3.2.26 |
| loadlore-connection-pool-trap-release-between-queries-20261111-122145-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-connection-pool-trap-release-between-queries-20261111-122145-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-connection-pool-trap-release-between-queries-20261111-122145-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-connection-pool-trap-release-between-queries-20261111-122145-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-connection-pool-trap-release-between-queries-20261111-122145-lore-toxiproxy-1 | ghcr.io/shopify/toxiproxy:2.13.0 |
| loadlore-connection-pool-trap-release-between-queries-20261111-122145-payments-0-1 | loadlore/synthetic:dev |

</details>

## Per-window data

<details><summary>42 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | lb CPU | lb up | payments CPU | api→db pool | wait | api→payments pool | wait | k6 CPU | Toxiproxy CPU | k6 / Toxiproxy stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 122 | 120 | 0 | 0% | 85 ms | 88 ms | 5.3% | 5.3% | 7 | 5.2% | 4 | 5.3% | 0/40 | 0.0 ms | 12/200 | 0.0 ms | 4.4% | 7.9% | 0% / 0% |
| 0:10 | 165 | 164 | 0 | 0% | 85 ms | 88 ms | 5.7% | 4.1% | 9 | 5.0% | 4 | 6.6% | 0/40 | 0.0 ms | 16/200 | 0.0 ms | 3.3% | 6.9% | 0% / 0% |
| 0:20 | 208 | 208 | 0 | 0% | 84 ms | 88 ms | 6.6% | 4.2% | 10 | 5.6% | 4 | 7.6% | 1/40 | 0.0 ms | 19/200 | 0.0 ms | 3.8% | 8.1% | 0% / 0% |
| 0:30 | 252 | 251 | 0 | 0% | 84 ms | 88 ms | 7.2% | 4.2% | 10 | 5.9% | 4 | 8.0% | 0/40 | 0.0 ms | 22/200 | 0.0 ms | 3.9% | 8.7% | 0% / 0% |
| 0:40 | 295 | 294 | 0 | 0% | 84 ms | 88 ms | 8.0% | 4.2% | 10 | 6.2% | 4 | 8.7% | 1/40 | 0.0 ms | 26/200 | 0.0 ms | 4.2% | 9.3% | 0% / 0% |
| 0:50 | 338 | 338 | 0 | 0% | 84 ms | 88 ms | 8.8% | 4.3% | 10 | 6.6% | 4 | 9.5% | 1/40 | 0.0 ms | 29/200 | 0.0 ms | 4.6% | 10% | 0% / 0% |
| 1:00 | 382 | 381 | 0 | 0% | 83 ms | 88 ms | 9.4% | 4.1% | 10 | 7.0% | 4 | 10% | 1/40 | 0.0 ms | 32/200 | 0.0 ms | 4.5% | 11% | 0% / 0% |
| 1:10 | 425 | 425 | 0 | 0% | 83 ms | 88 ms | 10% | 4.4% | 10 | 7.6% | 4 | 11% | 0/40 | 0.0 ms | 36/200 | 0.0 ms | 5.1% | 12% | 0% / 0% |
| 1:20 | 468 | 468 | 0 | 0% | 82 ms | 88 ms | 11% | 4.3% | 10 | 8.1% | 4 | 12% | 1/40 | 0.0 ms | 39/200 | 0.0 ms | 5.3% | 13% | 0% / 0% |
| 1:30 | 512 | 511 | 0 | 0% | 82 ms | 88 ms | 12% | 4.5% | 10 | 8.4% | 4 | 13% | 1/40 | 0.0 ms | 44/200 | 0.0 ms | 5.8% | 13% | 0% / 0% |
| 1:40 | 555 | 555 | 0 | 0% | 81 ms | 88 ms | 13% | 4.6% | 10 | 9.1% | 4 | 14% | 0/40 | 0.0 ms | 47/200 | 0.0 ms | 6.1% | 14% | 0% / 0% |
| 1:50 | 598 | 598 | 0 | 0% | 81 ms | 88 ms | 14% | 4.8% | 11 | 9.9% | 4 | 15% | 1/40 | 0.0 ms | 50/200 | 0.0 ms | 6.5% | 15% | 0% / 0% |
| 2:00 | 642 | 641 | 0 | 0% | 81 ms | 88 ms | 15% | 4.9% | 11 | 10% | 4 | 16% | 1/40 | 0.0 ms | 53/200 | 0.0 ms | 7.1% | 16% | 0% / 0% |
| 2:10 | 685 | 684 | 0 | 0% | 82 ms | 88 ms | 15% | 5.1% | 11 | 11% | 4 | 17% | 2/40 | 0.0 ms | 56/200 | 0.0 ms | 7.5% | 17% | 0% / 0% |
| 2:20 | 728 | 728 | 0 | 0% | 81 ms | 88 ms | 16% | 5.1% | 11 | 12% | 4 | 18% | 0/40 | 0.0 ms | 60/200 | 0.0 ms | 7.7% | 17% | 0% / 0% |
| 2:30 | 772 | 771 | 0 | 0% | 81 ms | 88 ms | 17% | 5.5% | 11 | 12% | 4 | 18% | 1/40 | 0.0 ms | 64/200 | 0.0 ms | 8.1% | 18% | 0% / 0% |
| 2:40 | 815 | 814 | 0 | 0% | 81 ms | 88 ms | 18% | 5.5% | 11 | 13% | 4 | 19% | 2/40 | 0.0 ms | 67/200 | 0.0 ms | 8.6% | 19% | 0% / 0% |
| 2:50 | 858 | 858 | 0 | 0% | 81 ms | 88 ms | 19% | 5.7% | 11 | 14% | 4 | 20% | 1/40 | 0.0 ms | 70/200 | 0.0 ms | 8.8% | 19% | 0% / 0% |
| 3:00 | 902 | 901 | 0 | 0% | 81 ms | 88 ms | 20% | 5.8% | 11 | 14% | 4 | 21% | 1/40 | 0.0 ms | 74/200 | 0.0 ms | 9.0% | 20% | 0% / 0% |
| 3:10 | 945 | 944 | 0 | 0% | 80 ms | 88 ms | 21% | 6.0% | 11 | 15% | 4 | 22% | 1/40 | 0.0 ms | 77/200 | 0.0 ms | 9.3% | 21% | 0% / 0% |
| 3:20 | 988 | 988 | 0 | 0% | 81 ms | 88 ms | 22% | 6.2% | 11 | 16% | 4 | 22% | 1/40 | 0.0 ms | 81/200 | 0.0 ms | 9.7% | 21% | 0% / 0% |
| 3:30 | 1,032 | 1,031 | 0 | 0% | 81 ms | 88 ms | 23% | 6.4% | 11 | 16% | 4 | 23% | 1/40 | 0.0 ms | 85/200 | 0.0 ms | 9.8% | 22% | 0% / 0% |
| 3:40 | 1,075 | 1,075 | 0 | 0% | 81 ms | 88 ms | 24% | 6.4% | 11 | 17% | 4 | 24% | 1/40 | 0.0 ms | 88/200 | 0.0 ms | 10% | 23% | 0% / 0% |
| 3:50 | 1,118 | 1,118 | 0 | 0% | 81 ms | 88 ms | 25% | 6.8% | 12 | 18% | 4 | 25% | 2/40 | 0.0 ms | 92/200 | 0.0 ms | 11% | 23% | 0% / 0% |
| 4:00 | 1,162 | 1,161 | 0 | 0% | 81 ms | 88 ms | 26% | 6.9% | 13 | 18% | 4 | 26% | 2/40 | 0.0 ms | 96/200 | 0.0 ms | 11% | 24% | 0% / 0% |
| 4:10 | 1,205 | 1,204 | 0 | 0% | 81 ms | 88 ms | 27% | 7.2% | 13 | 19% | 4 | 27% | 2/40 | 0.0 ms | 99/200 | 0.0 ms | 11% | 25% | 0% / 0% |
| 4:20 | 1,248 | 1,248 | 0 | 0% | 81 ms | 88 ms | 27% | 7.2% | 13 | 19% | 4 | 27% | 2/40 | 0.0 ms | 102/200 | 0.0 ms | 11% | 25% | 0% / 0% |
| 4:30 | 1,292 | 1,291 | 0 | 0% | 81 ms | 88 ms | 28% | 7.5% | 13 | 20% | 4 | 28% | 1/40 | 0.0 ms | 106/200 | 0.0 ms | 12% | 26% | 0% / 0% |
| 4:40 | 1,335 | 1,335 | 0 | 0% | 81 ms | 88 ms | 29% | 7.6% | 13 | 20% | 4 | 29% | 2/40 | 0.0 ms | 109/200 | 0.0 ms | 12% | 27% | 0% / 0% |
| 4:50 | 1,378 | 1,378 | 0 | 0% | 81 ms | 108 ms | 31% | 9.2% | 41 | 22% | 4 | 31% | 3/40 +4 | 0.1 ms | 114/200 | 0.0 ms | 13% | 29% | 0% / 0% |
| 5:00 | 1,400 | 1,400 | 0 | 0% | 81 ms | 88 ms | 32% | 8.9% | 41 | 19% | 4 | 31% | 2/40 | 0.0 ms | 114/200 | 0.0 ms | 13% | 29% | 0% / 0% |
| 5:10 | 1,400 | 1,400 | 0 | 0% | 81 ms | 88 ms | 31% | 8.0% | 41 | 19% | 4 | 30% | 1/40 | 0.0 ms | 113/200 | 0.0 ms | 13% | 28% | 0% / 0% |
| 5:20 | 1,400 | 1,400 | 0 | 0% | 81 ms | 88 ms | 31% | 8.1% | 41 | 21% | 4 | 30% | 1/40 | 0.0 ms | 113/200 | 0.0 ms | 12% | 28% | 0% / 0% |
| 5:30 | 1,400 | 1,400 | 0 | 0% | 81 ms | 88 ms | 31% | 8.0% | 41 | 22% | 4 | 30% | 1/40 | 0.0 ms | 114/200 | 0.0 ms | 13% | 28% | 0% / 0% |
| 5:40 | 1,400 | 1,400 | 0 | 0% | 81 ms | 88 ms | 31% | 7.9% | 41 | 22% | 4 | 30% | 1/40 | 0.0 ms | 115/200 | 0.0 ms | 12% | 28% | 0% / 0% |
| 5:50 | 1,400 | 1,400 | 0 | 0% | 81 ms | 88 ms | 31% | 8.0% | 41 | 22% | 4 | 30% | 1/40 | 0.0 ms | 114/200 | 0.0 ms | 13% | 28% | 0% / 0% |
| 6:00 | 1,400 | 1,400 | 0 | 0% | 81 ms | 88 ms | 31% | 8.1% | 41 | 22% | 4 | 30% | 1/40 | 0.0 ms | 114/200 | 0.0 ms | 13% | 28% | 0% / 0% |
| 6:10 | 1,400 | 1,400 | 0 | 0% | 81 ms | 88 ms | 31% | 8.1% | 41 | 22% | 4 | 30% | 1/40 | 0.0 ms | 114/200 | 0.0 ms | 12% | 28% | 0% / 0% |
| 6:20 | 1,400 | 1,400 | 0 | 0% | 81 ms | 88 ms | 31% | 8.0% | 41 | 22% | 4 | 30% | 1/40 | 0.0 ms | 114/200 | 0.0 ms | 13% | 28% | 0% / 0% |
| 6:30 | 1,400 | 1,400 | 0 | 0% | 81 ms | 88 ms | 31% | 8.1% | 41 | 22% | 4 | 30% | 1/40 | 0.0 ms | 114/200 | 0.0 ms | 13% | 28% | 0% / 0% |
| 6:40 | 1,400 | 1,400 | 0 | 0% | 81 ms | 88 ms | 31% | 8.0% | 41 | 21% | 4 | 30% | 2/40 | 0.0 ms | 114/200 | 0.0 ms | 13% | 28% | 0% / 0% |
| 6:50 | 1,400 | 1,400 | 0 | 0% | 81 ms | 88 ms | 30% | 7.9% | 41 | 21% | 4 | 29% | 1/40 | 0.0 ms | 114/200 | 0.0 ms | 12% | 27% | 0% / 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 and Toxiproxy CPU are against their budgets.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: connection-pool-trap-release-between-queries

# The starting design with connections released between queries.
#
# A checkout service: every request reads the cart from Postgres, then asks
# the payment provider to authorize the card, and one in ten also writes
# the order. Like Rails and Django, the api's framework checks out a
# database connection when a request first needs one and keeps it until the
# response.

services:
  lb:
    kind: lb
    target: api
    health_check: { interval_ms: 2000, fall: 3, rise: 2 }

  api:
    image: loadlore/synthetic
    replicas: 4
    resources: { cpu: 0.6, memory: 512m }
    behavior:
      cpu_ms: 0.3
      calls:
        - { target: db, op: read }                          # the cart
        - { target: payments, op: http }                    # authorize the card
        - { target: db, op: write, when: write }            # the order
    pools:
      db: { size: 10, hold: call }      # take a connection for each query, then hand it back
      payments: 50

  payments:
    image: loadlore/synthetic
    resources: { cpu: 0.5, memory: 256m }
    behavior:
      cpu_ms: 0.05

  db:
    kind: postgres
    version: "17"
    resources: { cpu: 1, memory: 1g }
    config: { max_connections: 100, shared_buffers: 256MB }
    dataset: { rows: 1000000, row_bytes: 512 }

workload:
  entry: lb
  profile:
    - ramp: { from: 100, to: 1400, over: 5m }
    - hold: { rps: 1400, for: 2m }
  mix: { read: 0.9, write: 0.1 }
  keys: { distribution: zipf, skew: 1.1, count: 1000000 }

# The payment provider is outside your system and answers in about 80 ms,
# modeled as network latency on the way to it for the whole run.
scenario:
  - { at: 0s, inject: latency, target: payments, add_ms: 80 }

# Your predictions, scored in the report. Replace these with your own
# before you run it.
expect:
  - p99_ms < 300 until load_rps >= 1000
  - error_rate < 1% throughout
```
