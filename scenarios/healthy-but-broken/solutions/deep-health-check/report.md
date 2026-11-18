# healthy-but-broken-deep-health-check: run report

_run `healthy-but-broken-deep-health-check-20261118-094135` · completed · 18 Nov 2026 09:41 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 300 rps offered, 300 rps served at best, worst 10-second p99 1.08 s, at most 12% errors.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 1:40

Measured in the 10-second window from 1:40 to 1:50 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 300 rps<br/>offered 300 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 4.4%<br/>2/3 replicas up"}}
    api["api ×3<br/>0.5 CPU · 256m<br/>CPU 9.7%<br/>in flight 1"]
    db[("db<br/>postgres 17<br/>1 CPU · 1g<br/>disk gp3 · 3,000 IOPS<br/>CPU 3.6%<br/>conns 91/100")]
    load -->|"100% read · 0% write<br/>300 rps · p99 1.6 ms"| lb
    lb -->|"round-robin<br/>checks every 1s, out after 2<br/>300 rps · p99 1.0 ms"| api
    api -->|"read<br/>pool 30, min 30<br/>300 calls/s · p99 0.5 ms<br/>in use 1/90"| db
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
    x-axis "seconds" 0 --> 290
    y-axis "requests/s" 0 --> 400
    line [0, 0, 0, 0, 0, 0, 400, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 400, 0, 0]
    line [91.667, 175, 258.333, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300]
    line [90.087, 173.979, 257.378, 299.934, 299.999, 300.003, 264.4, 299.973, 300.027, 299.97, 300.057, 299.973, 299.999, 300.001, 300, 300, 300, 299.97, 300, 300.03, 300, 300.027, 299.973, 300, 300, 300, 299.997, 300.003, 299.973, 300]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 1:00 stall; 1:01 partition between api[2] and db for 2m0s; 4:33 api-2 health_status: unhealthy.</sub>

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
    x-axis "seconds" 0 --> 290
    y-axis "ms" 0 --> 1200
    line [0, 0, 0, 0, 0, 0, 1200, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1200, 0, 0]
    line [2.285, 1.762, 1.403, 1.363, 1.376, 1.378, 1.389, 1.368, 1.364, 1.366, 1.372, 1.365, 1.377, 1.368, 1.368, 1.372, 1.362, 1.37, 1.368, 1.389, 1.375, 1.374, 1.386, 1.382, 1.38, 1.38, 1.373, 1.38, 1.386, 1.379]
    line [3.675, 2.692, 2.475, 1.5, 1.499, 1.503, 1052.563, 1.496, 1.496, 1.494, 1.496, 1.495, 1.499, 1.499, 1.495, 1.496, 1.494, 1.497, 1.497, 1.5, 1.497, 1.498, 1.502, 1.501, 1.5, 1.5, 1.499, 1.5, 1.501, 1.5]
    line [4.188, 2.995, 2.957, 2.246, 1.636, 1.749, 1082.919, 1.572, 1.598, 1.532, 1.572, 1.567, 1.616, 1.953, 1.53, 1.574, 1.538, 1.585, 1.613, 1.605, 1.583, 1.644, 1.779, 1.756, 1.718, 1.756, 1.759, 1.746, 1.705, 1.791]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 1:00 stall; 1:01 partition between api[2] and db for 2m0s; 4:33 api-2 health_status: unhealthy.</sub>

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
    x-axis "seconds" 0 --> 290
    y-axis "% of requests" 0 --> 15
    line [0, 0, 0, 0, 0, 0, 15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 15, 0, 0]
    line [0, 0, 0, 0, 0, 0, 11.867, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 1:00 stall; 1:01 partition between api[2] and db for 2m0s; 4:33 api-2 health_status: unhealthy.</sub>

| Service | Failed requests | Share of its failures | Cause | From calls to | First seen | What it means |
| --- | --- | --- | --- | --- | --- | --- |
| api | 355 | 100% | `pool_timeout` | db | 1:00 | they timed out waiting for a pool connection to db |
| api | 1 | 0.3% | `timeout` | db | 1:00 | calls to db ran past their timeout |

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
    x-axis "seconds" 0 --> 290
    y-axis "% of limit" 0 --> 100
    line [0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, 0, 0]
    line [5.015, 6.914, 8.828, 9.871, 9.957, 10.128, 9.812, 9.798, 9.745, 9.689, 9.691, 9.65, 9.846, 9.86, 9.655, 9.672, 9.585, 9.725, 9.777, 9.881, 10.181, 10.23, 10.457, 10.311, 10.168, 10.31, 10.178, 10.25, 10.282, 10.146]
    line [3.985, 3.71, 3.69, 4.038, 3.445, 3.881, 3.541, 3.406, 3.391, 3.475, 3.615, 3.229, 3.494, 3.61, 3.453, 3.542, 3.608, 3.455, 3.642, 3.677, 3.512, 3.292, 3.598, 4.03, 3.555, 3.679, 3.527, 3.661, 3.526, 4.265]
    line [3.194, 3.754, 4.19, 4.367, 4.625, 4.629, 4.445, 4.346, 4.238, 4.256, 4.436, 4.213, 4.432, 4.43, 4.239, 4.208, 4.179, 4.224, 4.363, 4.396, 4.379, 4.456, 4.703, 4.465, 4.69, 4.604, 4.78, 4.48, 4.799, 4.626]
```

<sub>Blue: api. Orange: db. Green: lb (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 1:00 stall; 1:01 partition between api[2] and db for 2m0s; 4:33 api-2 health_status: unhealthy.</sub>

## Diagnosis

No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version 23; thresholds below).

## Predictions

The design has no `expect` lines. Writing a prediction before a run turns it into a test of your mental model.

## Timeline

| Time | Event |
| --- | --- |
| 1:00 | stall: p99 reached 1.08 s (3.0 ms at low load), 12% of requests failed, only 88% of the offered rate was served for 10 s, then recovered; too short to count as a break; no diagnosis covers it |
| 1:01 | fault started: partition between api[2] and db for 2m0s |
| 3:01 | fault ended: partition between api[2] and db for 2m0s |
| 4:33 | container: api-2 health_status: unhealthy |

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
| services.api.pools.db.min | 30 | set in the design: each replica opens 30 connections before the load starts and keeps them open |
| services.api.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- load generator: 152 VUs started before the load, the scenario's measured prealloc_vus 152 at --scale 1, more than 300 rps × the SLO's 500 ms
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
- **Host disk stalls:** for 2 seconds or more, a Postgres read no blocks and committed at most 10% of its median over the 10 s before (50 a second or more), while 5 or more queries waited on disk reads or WAL flushes and its exporter answered; on a modelled disk it had also been reading 20 blocks a second or more (median), which its caps can't stop for seconds; not under a disk_slow fault on it. Near the break it makes the run inconclusive; anywhere else it's a caveat, and its windows are flagged and left out of exports.

## Environment

- loadl dev, profile 5m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 1015 (hash 1013, memory 1007, syscalls 1024): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (1015) was within 5.0% of the run it's compared with (973)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 86,250 requests, 0 dropped arrivals, 0.4% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-healthy-but-broken-deep-health-check-20261118-094135-api-0-1 | loadlore/synthetic:dev |
| loadlore-healthy-but-broken-deep-health-check-20261118-094135-api-1-1 | loadlore/synthetic:dev |
| loadlore-healthy-but-broken-deep-health-check-20261118-094135-api-2-1 | loadlore/synthetic:dev |
| loadlore-healthy-but-broken-deep-health-check-20261118-094135-db-1 | postgres:17.11 |
| loadlore-healthy-but-broken-deep-health-check-20261118-094135-lb-1 | haproxy:3.2.26 |
| loadlore-healthy-but-broken-deep-health-check-20261118-094135-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-healthy-but-broken-deep-health-check-20261118-094135-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-healthy-but-broken-deep-health-check-20261118-094135-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-healthy-but-broken-deep-health-check-20261118-094135-lore-prometheus-1 | prom/prometheus:v3.16.0 |

</details>

## Per-window data

<details><summary>30 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | lb CPU | lb up | api→db pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 92 | 90 | 0 | 0% | 2.3 ms | 4.2 ms | 5.0% | 4.0% | 91 | 3.2% | 3 | 1/90 | 0.0 ms | 2.5% | 0% |
| 0:10 | 175 | 174 | 0 | 0% | 1.8 ms | 3.0 ms | 6.9% | 3.7% | 91 | 3.8% | 3 | 1/90 | 0.0 ms | 2.5% | 0% |
| 0:20 | 258 | 257 | 0 | 0% | 1.4 ms | 3.0 ms | 8.8% | 3.7% | 91 | 4.2% | 3 | 0/90 | 0.0 ms | 3.2% | 0% |
| 0:30 | 300 | 300 | 0 | 0% | 1.4 ms | 2.2 ms | 9.9% | 4.0% | 91 | 4.4% | 3 | 1/90 | 0.0 ms | 3.5% | 0% |
| 0:40 | 300 | 300 | 0 | 0% | 1.4 ms | 1.6 ms | 10.0% | 3.4% | 91 | 4.6% | 3 | 1/90 | 0.0 ms | 3.6% | 0% |
| 0:50 | 300 | 300 | 0 | 0% | 1.4 ms | 1.7 ms | 10% | 3.9% | 91 | 4.6% | 3 | 1/90 | 0.0 ms | 3.6% | 0% |
| 1:00 | 300 | 264 | 0 | 12% | 1.4 ms | 1.08 s | 9.8% | 3.5% | 91 | 4.4% | 2 | 31/90 +100 | 118 ms | 3.6% | 0% |
| 1:10 | 300 | 300 | 0 | 0% | 1.4 ms | 1.6 ms | 9.8% | 3.4% | 91 | 4.3% | 2 | 30/90 | 0.0 ms | 3.5% | 0% |
| 1:20 | 300 | 300 | 0 | 0% | 1.4 ms | 1.6 ms | 9.7% | 3.4% | 91 | 4.2% | 2 | 1/90 | 0.0 ms | 3.5% | 0% |
| 1:30 | 300 | 300 | 0 | 0% | 1.4 ms | 1.5 ms | 9.7% | 3.5% | 91 | 4.3% | 2 | 0/90 | 0.0 ms | 3.6% | 0% |
| 1:40 | 300 | 300 | 0 | 0% | 1.4 ms | 1.6 ms | 9.7% | 3.6% | 91 | 4.4% | 2 | 1/90 | 0.0 ms | 3.5% | 0% |
| 1:50 | 300 | 300 | 0 | 0% | 1.4 ms | 1.6 ms | 9.7% | 3.2% | 91 | 4.2% | 2 | 1/90 | 0.0 ms | 3.5% | 0% |
| 2:00 | 300 | 300 | 0 | 0% | 1.4 ms | 1.6 ms | 9.8% | 3.5% | 91 | 4.4% | 2 | 0/90 | 0.0 ms | 3.6% | 0% |
| 2:10 | 300 | 300 | 0 | 0% | 1.4 ms | 2.0 ms | 9.9% | 3.6% | 91 | 4.4% | 2 | 1/90 | 0.0 ms | 3.6% | 0% |
| 2:20 | 300 | 300 | 0 | 0% | 1.4 ms | 1.5 ms | 9.7% | 3.5% | 91 | 4.2% | 2 | 1/90 | 0.0 ms | 3.5% | 0% |
| 2:30 | 300 | 300 | 0 | 0% | 1.4 ms | 1.6 ms | 9.7% | 3.5% | 91 | 4.2% | 2 | 0/90 | 0.0 ms | 3.5% | 0% |
| 2:40 | 300 | 300 | 0 | 0% | 1.4 ms | 1.5 ms | 9.6% | 3.6% | 91 | 4.2% | 2 | 1/90 | 0.0 ms | 3.5% | 0% |
| 2:50 | 300 | 300 | 0 | 0% | 1.4 ms | 1.6 ms | 9.7% | 3.5% | 91 | 4.2% | 2 | 1/90 | 0.0 ms | 3.6% | 0% |
| 3:00 | 300 | 300 | 0 | 0% | 1.4 ms | 1.6 ms | 9.8% | 3.6% | 91 | 4.4% | 2 | 0/90 | 0.0 ms | 3.5% | 0% |
| 3:10 | 300 | 300 | 0 | 0% | 1.4 ms | 1.6 ms | 9.9% | 3.7% | 92 | 4.4% | 2 | 1/90 | 0.0 ms | 3.8% | 0% |
| 3:20 | 300 | 300 | 0 | 0% | 1.4 ms | 1.6 ms | 10% | 3.5% | 92 | 4.4% | 2 | 1/90 | 0.0 ms | 3.5% | 0% |
| 3:30 | 300 | 300 | 0 | 0% | 1.4 ms | 1.6 ms | 10% | 3.3% | 92 | 4.5% | 2 | 0/90 | 0.0 ms | 3.6% | 0% |
| 3:40 | 300 | 300 | 0 | 0% | 1.4 ms | 1.8 ms | 10% | 3.6% | 92 | 4.7% | 2 | 1/90 | 0.0 ms | 3.7% | 0% |
| 3:50 | 300 | 300 | 0 | 0% | 1.4 ms | 1.8 ms | 10% | 4.0% | 93 | 4.5% | 2 | 1/90 | 0.0 ms | 3.6% | 0% |
| 4:00 | 300 | 300 | 0 | 0% | 1.4 ms | 1.7 ms | 10% | 3.6% | 93 | 4.7% | 2 | 1/90 | 0.0 ms | 3.7% | 0% |
| 4:10 | 300 | 300 | 0 | 0% | 1.4 ms | 1.8 ms | 10% | 3.7% | 93 | 4.6% | 2 | 1/90 | 0.0 ms | 3.7% | 0% |
| 4:20 | 300 | 300 | 0 | 0% | 1.4 ms | 1.8 ms | 10% | 3.5% | 93 | 4.8% | 2 | 1/90 | 0.0 ms | 3.6% | 0% |
| 4:30 | 300 | 300 | 0 | 0% | 1.4 ms | 1.7 ms | 10% | 3.7% | 93 | 4.5% | 2 | 1/90 | 0.0 ms | 3.6% | 0% |
| 4:40 | 300 | 300 | 0 | 0% | 1.4 ms | 1.7 ms | 10% | 3.5% | 93 | 4.8% | 2 | 1/90 | 0.0 ms | 3.7% | 0% |
| 4:50 | 300 | 300 | 0 | 0% | 1.4 ms | 1.8 ms | 10% | 4.3% | 92 | 4.6% | 2 | 1/90 | 0.0 ms | 3.2% | 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 CPU is against its budget.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: healthy-but-broken-deep-health-check

# Solution: a deep health check. The load balancer asks each replica's
# /readyz, which answers only when the replica can reach its dependencies,
# every second, and takes it out after 2 failed checks.

services:
  lb:
    kind: lb
    target: api
    health_check: { path: /readyz, interval_ms: 1000, fall: 2, rise: 2 }

  api:
    image: loadlore/synthetic
    replicas: 3
    resources: { cpu: 0.5, memory: 256m }
    behavior:
      cpu_ms: 0.3
      calls:
        - { target: db, op: read }
      resilience:
        timeout_ms: 1000
    pools:
      db: { size: 30, min: 30 }   # 3 replicas x 30 = 90 connections, all opened at start

  db:
    kind: postgres
    version: "17"
    resources: { cpu: 1, memory: 1g }
    dataset: { rows: 100000, row_bytes: 256 }
    disk: { profile: gp3 }

workload:
  entry: lb
  profile:
    - ramp: { from: 50, to: 300, over: 30s }
    - hold: { rps: 300, for: 4m30s }
  mix: { read: 1.0 }

# From 1:01, the third replica can't reach the database, for two minutes:
# a security group change, a broken route on its node, a full connection
# table on its host. Its process is fine.
scenario:
  - { at: 1m1s, inject: partition, between: ["api[2]", db], for: 2m }
```
