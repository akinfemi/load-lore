# fixture: run report

_run `fixture-run` · completed · 10 Nov 2026 12:00 UTC · scale 1 · seed 1_

## Verdict

**Broke.** Broke 0:40 into the load, at 450 rps offered: p99 reached 120 ms (5.0 ms at low load). Most saturated then: api → db pool 10 of 10 connections in use. Around then: latency +20 ms on connections into db for 10s at 0:30.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); saturation: loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections. Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 0:40

Measured in the 10-second window from 0:40 to 0:50 (the break). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 1,000 rps<br/>offered 450 rps"])
    api["api<br/>1 CPU · 512m<br/>CPU 18%"]
    db[("db<br/>postgres 17<br/>1 CPU · 512m<br/>CPU 23%")]
    load -->|"90% read · 10% write<br/>450 rps · p99 120 ms"| api
    api -->|"read<br/>pool 10<br/>in use 10/10, 27 waiting"| db
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class api hot
    class db ok
```

<sub>Sources: offered from the load profile in plan.json (offered arrival rate); load edge from k6_http_reqs_total (k6: all responses) and k6_http_req_duration_seconds (k6 histogram, end to end); CPU from container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit; calls/s from loadlore_calls_total (the caller's outbound call attempts) and their p99 from loadlore_call_duration_seconds (the caller's histogram, pool wait included); pools from loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections and loadlore_pool_pending (callers waiting for a connection); conns from pg_stat_activity_count and pg_settings_max_connections (postgres_exporter).</sub>

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
    x-axis "seconds" 0 --> 90
    y-axis "requests/s" 0 --> 1000
    line [0, 0, 0, 1000, 1000, 0, 0, 0, 0, 0]
    line [50, 150, 250, 350, 450, 550, 650, 750, 850, 950]
    line [50, 150, 250, 350, 450, 550, 650, 750, 850, 950]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 0:30 latency +20 ms on connections into db for 10s; 0:40 break.</sub>

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
    x-axis "seconds" 0 --> 90
    y-axis "ms" 0 --> 1200
    line [0, 0, 0, 1200, 1200, 0, 0, 0, 0, 0]
    line [2, 2, 2, 2, 2, 2, 2, 2, 2, 2]
    line [2.009, 2.009, 2.009, 2.009, 2.009, 2.009, 2.009, 2.009, 2.009, 2.009]
    line [5, 5, 5, 5, 119.73, 319.121, 518.392, 717.592, 916.745, 1115.866]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 0:30 latency +20 ms on connections into db for 10s; 0:40 break.</sub>

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
    x-axis "seconds" 0 --> 90
    y-axis "% of requests" 0 --> 1
    line [0, 0, 0, 1, 1, 0, 0, 0, 0, 0]
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 0:30 latency +20 ms on connections into db for 10s; 0:40 break.</sub>

```mermaid
---
config:
  themeVariables:
    xyChart:
      backgroundColor: "transparent"
      plotColorPalette: "#f4cccc, #1f77b4, #ff7f0e"
---
xychart-beta
    title "CPU against each limit"
    x-axis "seconds" 0 --> 90
    y-axis "% of limit" 0 --> 100
    line [0, 0, 0, 100, 100, 0, 0, 0, 0, 0]
    line [2.2, 6.2, 10.2, 14.2, 18.2, 22.2, 26.2, 30.2, 34.2, 38.2]
    line [2.75, 7.75, 12.75, 17.75, 22.75, 27.75, 32.75, 37.75, 42.75, 47.75]
```

<sub>Blue: api. Orange: db (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 0:30 latency +20 ms on connections into db for 10s; 0:40 break.</sub>

## Diagnosis

### 1. Connection pool exhausted (api → db), from 0:40

From 0:40, every connection in api's pool to db was in use and requests waited for one, while db still had CPU to spare. Requests were queueing for a connection, not for the database.

Evidence:

- 10 of 10 connections in use at 0:40 (0:40 to 1:40) <sub>(loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections)</sub>
- up to 177 requests waiting for a connection at 1:30 <sub>(loadlore_pool_pending (callers waiting for a connection))</sub>
- mean wait for a connection 278 ms at 1:30, up from 0.1 ms at 0:30 <sub>(loadlore_pool_wait_seconds (time to get a connection))</sub>
- db CPU at 48% of its limit at 1:30: not saturated <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- api CPU at 38% of its limit at 1:30 <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>

**Try next:** Add PgBouncer, or resize the pools across replicas: 1 replica × 10 connections = 10.

### 2. Queueing knee (api pool to db), from 0:40

From 0:40, p99 stayed above 15 ms (5.0 ms at low load) with api pool to db at 100%. Near saturation requests wait behind each other, so latency grows non-linearly, not in proportion to load.

Evidence:

- median end-to-end p99 by api pool to db: below 50%: 5.0 ms (1 window); 50–70%: 5.0 ms (1 window); 70–80%: 5.0 ms (1 window); 90% and up: 518 ms (7 windows) <sub>(k6_http_req_duration_seconds (k6 histogram, end to end); loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections)</sub>
- 0:40 to 1:10: p99 120 ms, 319 ms, 518 ms with api pool to db 100%, 100%, 100% <sub>(k6_http_req_duration_seconds (k6 histogram, end to end); loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections)</sub>

**Try next:** Add capacity or cut per-request work on api, then re-run the ramp.

<sub>Rules version 1. Rules are deterministic and offline; their thresholds are listed below.</sub>

## Predictions

3 of 4 scored predictions held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 50 until load_rps >= 500` | **broken** at 0:40 | 120 ms | 0:00 to 0:50, when load_rps >= 500 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |
| `error_rate < 1%` | **held** | 0% | the whole run | source: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6) |
| `db.cpu < 90% after 50s` | **held** | 48% | from 0:50 to the end | source: container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit |
| `rps >= 400 after 60s` | **held** | 650 rps | from 1:00 to the end | source: k6_http_reqs_total{expected_response="true"} (k6: successful responses) |

## Timeline

| Time | Event |
| --- | --- |
| 0:30 | fault started: latency +20 ms on connections into db for 10s |
| 0:40 | fault ended: latency +20 ms on connections into db for 10s |

<sub>From events.jsonl: the fault timeline and Docker container events.</sub>

## Assumptions

### From the design

The design lists no assumptions.

### Chosen by Loadlore

| Field | Value | Why |
| --- | --- | --- |
| workload.timeout_ms | 10s | the default; k6 gives up on a request after this long and counts it as failed |
| resources.cpu_period_ms | 100 ms for api, db | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |

### How the run was planned

- Loadlore's own containers get fixed budgets: load generator 1 CPU (peak 1000 rps), Toxiproxy 0.5 CPU (about 1000 proxied calls/s at peak), Prometheus 0.25, cAdvisor 0.25, exporters 0.1 each

## Thresholds

- **Broke:** 2 windows in a row with at least 1.0% of requests failing or successful responses below 90% of the offered rate, or 3 windows in a row with any of those or p99 at least 10x the low-load p99 (the median p99 of windows offered at most 25% of the peak, before errors or a shortfall in 2 windows in a row) and at least 100 ms. A shorter episode is shown as a stall in the timeline, not a break.
- **Inconclusive:** before the break (or anywhere, if nothing broke), the load generator dropped more than 1.0% of a window's arrivals, or the load generator or Toxiproxy used 90% or more of its CPU budget, or was stalled at its CPU limit in 5.0% or more of its scheduling periods (from 1.0% it's noted as a caveat). The first 10 s of load are a warm-up and don't count: the load generator starts its VUs and connections open then. A cause is named only with evidence: on a Linux host, the hypervisor giving 2.0% or more of CPU time to other virtual machines (steal), or other programs using 10% or more of the host's CPUs (at least 0.5); inside a VM on this machine, only as a possibility.
- **Connection pool exhausted:** for two windows in a row, a pool at 95% or more of its size with callers waiting, while the target's CPU stayed below 80% and the caller's below 90%.
- **Queueing knee:** for 3 windows in a row, p99 at least 3x the low-load p99 and 10 ms above it, with a CPU or pool at 70% or more.

## Environment

- loadl test, profile 1m40s, scale 1, seed 1
- Host: 8 CPUs, 0.0 GiB memory,  (, kernel ), cgroup v
- Docker , Compose 
- Host speed index: not measured
- Data: a fresh copy of the cached seeded dataset

## Per-window data

<details><summary>10 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | api→db pool | wait | k6 CPU | Toxiproxy CPU | k6 / Toxiproxy stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 50 | 50 | 0 | 0% | 2.0 ms | 5.0 ms | 2.2% | 2.8% | – | 4/10 | 0.1 ms | 20% | – | – / – |
| 0:10 | 150 | 150 | 0 | 0% | 2.0 ms | 5.0 ms | 6.2% | 7.7% | – | 6/10 | 0.1 ms | 20% | – | – / – |
| 0:20 | 250 | 250 | 0 | 0% | 2.0 ms | 5.0 ms | 10% | 13% | – | 8/10 | 0.1 ms | 20% | – | – / – |
| 0:30 | 350 | 350 | 0 | 0% | 2.0 ms | 5.0 ms | 14% | 18% | – | 10/10 | 0.1 ms | 20% | – | – / – |
| 0:40 | 450 | 450 | 0 | 0% | 2.0 ms | 120 ms | 18% | 23% | – | 10/10 +27 | 28 ms | 20% | – | – / – |
| 0:50 | 550 | 550 | 0 | 0% | 2.0 ms | 319 ms | 22% | 28% | – | 10/10 +57 | 78 ms | 20% | – | – / – |
| 1:00 | 650 | 650 | 0 | 0% | 2.0 ms | 518 ms | 26% | 33% | – | 10/10 +87 | 128 ms | 20% | – | – / – |
| 1:10 | 750 | 750 | 0 | 0% | 2.0 ms | 718 ms | 30% | 38% | – | 10/10 +117 | 178 ms | 20% | – | – / – |
| 1:20 | 850 | 850 | 0 | 0% | 2.0 ms | 917 ms | 34% | 43% | – | 10/10 +147 | 228 ms | 20% | – | – / – |
| 1:30 | 950 | 950 | 0 | 0% | 2.0 ms | 1.12 s | 38% | 48% | – | 10/10 +177 | 278 ms | 20% | – | – / – |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 and Toxiproxy CPU are against their budgets.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: fixture
services:
  api:
    image: loadlore/synthetic
    resources: { cpu: 1 }
    behavior: { calls: [ { target: db, op: read } ] }
    pools: { db: 10 }
  db:
    kind: postgres
    resources: { cpu: 1 }
    dataset: { rows: 1000 }
workload:
  entry: api
  profile: [ { ramp: { from: 0, to: 1000, over: 100s } } ]
expect:
  - p99_ms < 50 until load_rps >= 500
  - error_rate < 1%
  - db.cpu < 90% after 50s
  - rps >= 400 after 60s
```
