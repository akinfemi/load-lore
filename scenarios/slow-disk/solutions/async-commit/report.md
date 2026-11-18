# slow-disk-async-commit: run report

_run `slow-disk-async-commit-20261118-082039` · completed · 18 Nov 2026 08:20 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 400 rps offered, 400 rps served at best, worst 10-second p99 4.4 ms, at most 0% errors.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 0:40

Measured in the 10-second window from 0:40 to 0:50 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 400 rps<br/>offered 400 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 5.3%<br/>2/2 replicas up"}}
    api["api ×2<br/>0.5 CPU · 256m<br/>CPU 19%<br/>in flight 1"]
    db[("db<br/>postgres 17<br/>1 CPU · 1g<br/>disk gp3 · 3,000 IOPS<br/>CPU 3.9%<br/>conns 17/100")]
    load -->|"50% read · 50% write<br/>400 rps · p99 1.9 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>400 rps · p99 2.0 ms"| api
    api -->|"read, write on writes<br/>pool 8, min 8<br/>601 calls/s · p99 0.5 ms<br/>in use 0/16"| db
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
    x-axis "seconds" 0 --> 230
    y-axis "requests/s" 0 --> 500
    line [0, 0, 0, 0, 0, 0, 500, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [108.333, 225, 341.667, 400, 400, 400, 400, 400, 400, 400, 400, 400, 400, 400, 400, 400, 400, 400, 400, 400, 400, 400, 400, 400]
    line [106.263, 223.529, 340.088, 399.949, 400.248, 399.724, 400.056, 400.012, 399.969, 400.098, 399.931, 400.097, 399.973, 399.881, 400.002, 400.036, 399.966, 400.114, 399.89, 400.201, 399.807, 399.983, 400.004, 399.997]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 1:02 disk_slow +30 ms on every write and fsync of db's disk for 2m0s.</sub>

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
    y-axis "ms" 0 --> 5
    line [0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [2.391, 1.78, 1.579, 1.506, 1.52, 1.507, 1.513, 1.518, 1.515, 1.505, 1.505, 1.498, 1.52, 1.53, 1.5, 1.491, 1.512, 1.494, 1.507, 1.497, 1.507, 1.499, 1.478, 1.512]
    line [3.938, 2.834, 2.393, 1.783, 1.78, 1.78, 1.782, 1.781, 1.782, 1.781, 1.779, 1.778, 1.778, 1.778, 1.777, 1.776, 1.776, 1.778, 1.778, 1.775, 1.775, 1.775, 1.776, 1.776]
    line [4.446, 3.172, 2.966, 1.945, 1.899, 1.91, 1.918, 1.91, 1.904, 1.915, 1.895, 1.89, 1.86, 1.863, 1.859, 1.838, 1.829, 1.887, 1.886, 1.828, 1.815, 1.801, 1.862, 1.844]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 1:02 disk_slow +30 ms on every write and fsync of db's disk for 2m0s.</sub>

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
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 1:02 disk_slow +30 ms on every write and fsync of db's disk for 2m0s.</sub>

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
    line [6.887, 11.046, 15.991, 18.506, 18.597, 18.539, 18.707, 18.608, 18.586, 18.589, 18.438, 18.549, 18.431, 18.606, 18.433, 18.428, 18.423, 18.505, 18.41, 18.327, 18.437, 18.304, 18.458, 18.167]
    line [3.721, 3.794, 3.868, 4.445, 3.906, 3.93, 4.137, 3.939, 3.921, 4.413, 3.686, 3.904, 3.737, 3.888, 3.854, 4.122, 3.887, 3.843, 3.787, 3.735, 3.794, 4.008, 3.754, 3.757]
    line [2.973, 3.592, 4.783, 5.164, 5.271, 5.165, 5.231, 5.252, 5.186, 5.163, 5.11, 5.148, 5.187, 5.164, 5.015, 5.085, 5.072, 5.083, 5.187, 5.049, 5.181, 5.011, 5.153, 5.038]
```

<sub>Blue: api. Orange: db. Green: lb (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 1:02 disk_slow +30 ms on every write and fsync of db's disk for 2m0s.</sub>

## Diagnosis

No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version 23; thresholds below).

## Predictions

The design has no `expect` lines. Writing a prediction before a run turns it into a test of your mental model.

## Timeline

| Time | Event |
| --- | --- |
| 1:02 | fault started: disk_slow +30 ms on every write and fsync of db's disk for 2m0s |
| 3:02 | fault ended: disk_slow +30 ms on every write and fsync of db's disk for 2m0s |

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
| resources.cpu_period_ms | 100 ms for api, db, lb | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.db.disk | gp3: 1 ms per read, 1 ms per write, 3000 IOPS, 125 MiB/s | a gp3 volume at its baseline: 3,000 IOPS and 125 MiB/s, included in its price, and up to 80,000 IOPS provisioned. AWS publishes no exact latency, only single-digit milliseconds, so the 1 ms Loadlore adds to every read, write and fsync is its own estimate: the low end of that range, so the disk is flattered rather than handicapped. Reads and writes are capped separately, so a mixed load can reach up to twice the IOPS. Provisioned throughput isn't modelled. Source: AWS, Amazon EBS General Purpose SSD volumes: https://docs.aws.amazon.com/ebs/latest/userguide/general-purpose.html |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |
| services.api.pools.db.min | 8 | set in the design: each replica opens 8 connections before the load starts and keeps them open |
| services.api.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- Loadlore's own containers get fixed budgets: load generator 1.5 CPU (peak 400 rps), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each. No fault acts on a connection, so every connection is direct and there is no Toxiproxy

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

- loadl dev, profile 4m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 978 (hash 939, memory 1006, syscalls 989): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (978) was within 5.0% of the run it's compared with (997)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 90,749 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-slow-disk-async-commit-20261118-082039-api-0-1 | loadlore/synthetic:dev |
| loadlore-slow-disk-async-commit-20261118-082039-api-1-1 | loadlore/synthetic:dev |
| loadlore-slow-disk-async-commit-20261118-082039-db-1 | postgres:17.11 |
| loadlore-slow-disk-async-commit-20261118-082039-lb-1 | haproxy:3.2.26 |
| loadlore-slow-disk-async-commit-20261118-082039-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-slow-disk-async-commit-20261118-082039-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-slow-disk-async-commit-20261118-082039-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-slow-disk-async-commit-20261118-082039-lore-prometheus-1 | prom/prometheus:v3.16.0 |

</details>

## Per-window data

<details><summary>24 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | lb CPU | lb up | api→db pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 108 | 106 | 0 | 0% | 2.4 ms | 4.4 ms | 6.9% | 3.7% | 17 | 3.0% | 2 | 0/16 | 0.0 ms | 2.5% | 0% |
| 0:10 | 225 | 224 | 0 | 0% | 1.8 ms | 3.2 ms | 11% | 3.8% | 17 | 3.6% | 2 | 1/16 | 0.0 ms | 2.8% | 0% |
| 0:20 | 342 | 340 | 0 | 0% | 1.6 ms | 3.0 ms | 16% | 3.9% | 17 | 4.8% | 2 | 1/16 | 0.0 ms | 3.7% | 0% |
| 0:30 | 400 | 400 | 0 | 0% | 1.5 ms | 1.9 ms | 19% | 4.4% | 17 | 5.2% | 2 | 1/16 | 0.0 ms | 4.1% | 0% |
| 0:40 | 400 | 400 | 0 | 0% | 1.5 ms | 1.9 ms | 19% | 3.9% | 17 | 5.3% | 2 | 0/16 | 0.0 ms | 4.0% | 0% |
| 0:50 | 400 | 400 | 0 | 0% | 1.5 ms | 1.9 ms | 19% | 3.9% | 17 | 5.2% | 2 | 1/16 | 0.0 ms | 4.0% | 0% |
| 1:00 | 400 | 400 | 0 | 0% | 1.5 ms | 1.9 ms | 19% | 4.1% | 17 | 5.2% | 2 | 0/16 | 0.0 ms | 4.1% | 0% |
| 1:10 | 400 | 400 | 0 | 0% | 1.5 ms | 1.9 ms | 19% | 3.9% | 17 | 5.3% | 2 | 0/16 | 0.0 ms | 4.0% | 0% |
| 1:20 | 400 | 400 | 0 | 0% | 1.5 ms | 1.9 ms | 19% | 3.9% | 17 | 5.2% | 2 | 1/16 | 0.0 ms | 4.0% | 0% |
| 1:30 | 400 | 400 | 0 | 0% | 1.5 ms | 1.9 ms | 19% | 4.4% | 17 | 5.2% | 2 | 0/16 | 0.0 ms | 4.1% | 0% |
| 1:40 | 400 | 400 | 0 | 0% | 1.5 ms | 1.9 ms | 18% | 3.7% | 17 | 5.1% | 2 | 0/16 | 0.0 ms | 3.9% | 0% |
| 1:50 | 400 | 400 | 0 | 0% | 1.5 ms | 1.9 ms | 19% | 3.9% | 17 | 5.1% | 2 | 1/16 | 0.0 ms | 3.8% | 0% |
| 2:00 | 400 | 400 | 0 | 0% | 1.5 ms | 1.9 ms | 18% | 3.7% | 17 | 5.2% | 2 | 0/16 | 0.0 ms | 3.9% | 0% |
| 2:10 | 400 | 400 | 0 | 0% | 1.5 ms | 1.9 ms | 19% | 3.9% | 17 | 5.2% | 2 | 1/16 | 0.0 ms | 3.9% | 0% |
| 2:20 | 400 | 400 | 0 | 0% | 1.5 ms | 1.9 ms | 18% | 3.9% | 17 | 5.0% | 2 | 0/16 | 0.0 ms | 3.9% | 0% |
| 2:30 | 400 | 400 | 0 | 0% | 1.5 ms | 1.8 ms | 18% | 4.1% | 17 | 5.1% | 2 | 1/16 | 0.0 ms | 3.8% | 0% |
| 2:40 | 400 | 400 | 0 | 0% | 1.5 ms | 1.8 ms | 18% | 3.9% | 17 | 5.1% | 2 | 1/16 | 0.0 ms | 3.8% | 0% |
| 2:50 | 400 | 400 | 0 | 0% | 1.5 ms | 1.9 ms | 19% | 3.8% | 17 | 5.1% | 2 | 1/16 | 0.0 ms | 3.9% | 0% |
| 3:00 | 400 | 400 | 0 | 0% | 1.5 ms | 1.9 ms | 18% | 3.8% | 17 | 5.2% | 2 | 1/16 | 0.0 ms | 4.0% | 0% |
| 3:10 | 400 | 400 | 0 | 0% | 1.5 ms | 1.8 ms | 18% | 3.7% | 17 | 5.0% | 2 | 0/16 | 0.0 ms | 3.9% | 0% |
| 3:20 | 400 | 400 | 0 | 0% | 1.5 ms | 1.8 ms | 18% | 3.8% | 17 | 5.2% | 2 | 0/16 | 0.0 ms | 3.9% | 0% |
| 3:30 | 400 | 400 | 0 | 0% | 1.5 ms | 1.8 ms | 18% | 4.0% | 17 | 5.0% | 2 | 1/16 | 0.0 ms | 3.8% | 0% |
| 3:40 | 400 | 400 | 0 | 0% | 1.5 ms | 1.9 ms | 18% | 3.8% | 17 | 5.2% | 2 | 0/16 | 0.0 ms | 3.9% | 0% |
| 3:50 | 400 | 400 | 0 | 0% | 1.5 ms | 1.8 ms | 18% | 3.8% | 17 | 5.0% | 2 | 0/16 | 0.0 ms | 3.2% | 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 CPU is against its budget.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: slow-disk-async-commit

# Solution: synchronous_commit off. A commit returns as soon as its
# changes are in the write-ahead log's memory buffer; the WAL writer flushes
# them to disk shortly after. Commits stop waiting on the disk, at the cost
# of losing the last few hundred milliseconds of commits if the server
# crashes.

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
        - { target: db, op: read }                   # the recent orders
        - { target: db, op: write, when: write }     # the new order
      resilience:
        timeout_ms: 1000
    pools:
      db: { size: 8, min: 8 }   # 2 replicas x 8 = 16 connections

  db:
    kind: postgres
    version: "17"
    resources: { cpu: 1, memory: 1g }
    config: { synchronous_commit: "off" }
    dataset: { rows: 100000, row_bytes: 256 }
    disk: { profile: gp3 }

workload:
  entry: lb
  profile:
    - ramp: { from: 50, to: 400, over: 30s }
    - hold: { rps: 400, for: 3m30s }
  mix: { read: 0.5, write: 0.5 }

# From 1:01, the database's disk is slow for two minutes: every write and
# every fsync takes 30 ms longer. A noisy neighbour on shared storage, a
# cloud volume out of burst credits, a failing drive.
scenario:
  - { at: 1m1s, inject: disk_slow, target: db, add_ms: 30, for: 2m }
```
