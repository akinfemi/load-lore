# slow-disk-more-connections: run report

_run `slow-disk-more-connections-20261118-000514` · completed · 18 Nov 2026 00:05 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 400 rps offered, 401 rps served at best, worst 10-second p99 270 ms, at most 0% errors.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 3:00

Measured in the 10-second window from 3:00 to 3:10 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 400 rps<br/>offered 400 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 3.5%<br/>2/2 replicas up"}}
    api["api ×2<br/>0.5 CPU · 256m<br/>CPU 18%<br/>in flight 43"]
    db[("db<br/>postgres 17<br/>1 CPU · 1g<br/>CPU 4.5%<br/>conns 81/100")]
    load -->|"50% read · 50% write<br/>401 rps · p99 270 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>402 rps · p99 136 ms"| api
    api -->|"read, write on writes<br/>pool 40, min 40<br/>601 calls/s · p99 127 ms<br/>in use 43/80"| db
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
    line [107.759, 224.691, 341.132, 399.999, 400.28, 399.72, 398.976, 399.787, 400.051, 400.063, 400.109, 399.366, 400.738, 399.492, 400.033, 400.488, 400.286, 399.294, 401.345, 400.229, 399.745, 400.017, 399.999, 399.98]
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
    y-axis "ms" 0 --> 300
    line [0, 0, 0, 0, 0, 0, 300, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [1.896, 1.954, 1.418, 1.49, 1.809, 1.381, 1.73, 62.737, 65.345, 1.626, 1.53, 1.596, 1.466, 67.488, 1.496, 1.494, 62.529, 1.384, 1.791, 1.379, 1.806, 1.486, 1.429, 1.693]
    line [3.376, 2.486, 2.222, 2.202, 2.19, 2.147, 115.534, 117.693, 118.32, 117.3, 117.303, 117.067, 117.212, 118.405, 117.32, 117.408, 117.856, 117.027, 107.032, 2.169, 2.204, 2.169, 2.135, 2.155]
    line [4.227, 2.921, 2.494, 2.471, 2.439, 2.47, 124.096, 123.539, 123.664, 123.491, 123.492, 123.413, 123.442, 123.737, 123.464, 123.482, 123.571, 123.44, 270.327, 2.378, 2.486, 2.437, 2.424, 2.428]
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
    line [7.644, 11.461, 15.604, 18.083, 18.138, 18.128, 18.394, 18.186, 18.256, 18.253, 18.262, 18.303, 18.31, 18.308, 18.295, 18.277, 18.335, 18.347, 18.078, 18.07, 18.096, 18.077, 18.086, 17.807]
    line [4.497, 4.515, 4.138, 4.658, 4.327, 4.257, 5.171, 4.872, 4.734, 5.373, 4.812, 4.896, 4.871, 4.794, 4.807, 5.215, 4.741, 4.932, 4.475, 4.27, 4.345, 4.558, 4.232, 4.231]
    line [4.728, 3.823, 4.505, 4.978, 4.883, 4.931, 4.488, 4.369, 4.337, 4.484, 4.436, 4.377, 4.512, 4.46, 4.377, 4.49, 4.337, 4.268, 3.519, 3.391, 4.196, 4.891, 4.793, 4.639]
```

<sub>Blue: api. Orange: db. Green: lb (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 1:02 disk_slow +30 ms on every write and fsync of db's disk for 2m0s.</sub>

## Diagnosis

No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version 21; thresholds below).

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
| services.db.disk | laptop: the Docker host's | no disk block: a read that misses db's memory goes to the Docker host's disk, which on a laptop mostly answers from the host's own memory, fast and uncapped, and a commit's fsync returns in a fraction of a millisecond. Add disk: { profile: gp3 } for a cloud volume's waits and caps |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |
| services.api.pools.db.min | 40 | set in the design: each replica opens 40 connections before the load starts and keeps them open |
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

## Environment

- loadl dev, profile 4m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 1001 (hash 1007, memory 1012, syscalls 984): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (1001) was within 5.0% of the run it's compared with (977)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 90,749 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-slow-disk-more-connections-20261118-000514-api-0-1 | loadlore/synthetic:dev |
| loadlore-slow-disk-more-connections-20261118-000514-api-1-1 | loadlore/synthetic:dev |
| loadlore-slow-disk-more-connections-20261118-000514-db-1 | postgres:17.11 |
| loadlore-slow-disk-more-connections-20261118-000514-lb-1 | haproxy:3.2.26 |
| loadlore-slow-disk-more-connections-20261118-000514-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-slow-disk-more-connections-20261118-000514-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-slow-disk-more-connections-20261118-000514-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-slow-disk-more-connections-20261118-000514-lore-prometheus-1 | prom/prometheus:v3.16.0 |

</details>

## Per-window data

<details><summary>24 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | lb CPU | lb up | api→db pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 108 | 108 | 0 | 0% | 1.9 ms | 4.2 ms | 7.6% | 4.5% | 81 | 4.7% | 2 | 1/80 | 0.0 ms | 2.8% | 0% |
| 0:10 | 225 | 225 | 0 | 0% | 2.0 ms | 2.9 ms | 11% | 4.5% | 81 | 3.8% | 2 | 0/80 | 0.0 ms | 3.0% | 0% |
| 0:20 | 342 | 341 | 0 | 0% | 1.4 ms | 2.5 ms | 16% | 4.1% | 81 | 4.5% | 2 | 1/80 | 0.0 ms | 3.4% | 0% |
| 0:30 | 400 | 400 | 0 | 0% | 1.5 ms | 2.5 ms | 18% | 4.7% | 82 | 5.0% | 2 | 1/80 | 0.0 ms | 3.6% | 0% |
| 0:40 | 400 | 400 | 0 | 0% | 1.8 ms | 2.4 ms | 18% | 4.3% | 81 | 4.9% | 2 | 1/80 | 0.0 ms | 3.6% | 0% |
| 0:50 | 400 | 400 | 0 | 0% | 1.4 ms | 2.5 ms | 18% | 4.3% | 81 | 4.9% | 2 | 1/80 | 0.0 ms | 3.6% | 0% |
| 1:00 | 400 | 399 | 0 | 0% | 1.7 ms | 124 ms | 18% | 5.2% | 81 | 4.5% | 2 | 25/80 | 0.0 ms | 3.6% | 0% |
| 1:10 | 400 | 400 | 0 | 0% | 63 ms | 124 ms | 18% | 4.9% | 81 | 4.4% | 2 | 24/80 | 0.0 ms | 3.6% | 0% |
| 1:20 | 400 | 400 | 0 | 0% | 65 ms | 124 ms | 18% | 4.7% | 81 | 4.3% | 2 | 22/80 | 0.0 ms | 3.6% | 0% |
| 1:30 | 400 | 400 | 0 | 0% | 1.6 ms | 123 ms | 18% | 5.4% | 82 | 4.5% | 2 | 21/80 | 0.0 ms | 3.6% | 0% |
| 1:40 | 400 | 400 | 0 | 0% | 1.5 ms | 123 ms | 18% | 4.8% | 81 | 4.4% | 2 | 27/80 | 0.0 ms | 3.5% | 0% |
| 1:50 | 400 | 399 | 0 | 0% | 1.6 ms | 123 ms | 18% | 4.9% | 81 | 4.4% | 2 | 23/80 | 0.0 ms | 3.6% | 0% |
| 2:00 | 400 | 401 | 0 | 0% | 1.5 ms | 123 ms | 18% | 4.9% | 81 | 4.5% | 2 | 26/80 | 0.0 ms | 3.6% | 0% |
| 2:10 | 400 | 399 | 0 | 0% | 67 ms | 124 ms | 18% | 4.8% | 81 | 4.5% | 2 | 26/80 | 0.0 ms | 3.6% | 0% |
| 2:20 | 400 | 400 | 0 | 0% | 1.5 ms | 123 ms | 18% | 4.8% | 81 | 4.4% | 2 | 24/80 | 0.0 ms | 3.6% | 0% |
| 2:30 | 400 | 400 | 0 | 0% | 1.5 ms | 123 ms | 18% | 5.2% | 82 | 4.5% | 2 | 24/80 | 0.0 ms | 3.5% | 0% |
| 2:40 | 400 | 400 | 0 | 0% | 63 ms | 124 ms | 18% | 4.7% | 81 | 4.3% | 2 | 29/80 | 0.0 ms | 3.6% | 0% |
| 2:50 | 400 | 399 | 0 | 0% | 1.4 ms | 123 ms | 18% | 4.9% | 81 | 4.3% | 2 | 43/80 | 0.0 ms | 3.6% | 0% |
| 3:00 | 400 | 401 | 0 | 0% | 1.8 ms | 270 ms | 18% | 4.5% | 81 | 3.5% | 2 | 43/80 | 0.0 ms | 3.7% | 0% |
| 3:10 | 400 | 400 | 0 | 0% | 1.4 ms | 2.4 ms | 18% | 4.3% | 81 | 3.4% | 2 | 0/80 | 0.0 ms | 3.8% | 0% |
| 3:20 | 400 | 400 | 0 | 0% | 1.8 ms | 2.5 ms | 18% | 4.3% | 81 | 4.2% | 2 | 0/80 | 0.0 ms | 3.7% | 0% |
| 3:30 | 400 | 400 | 0 | 0% | 1.5 ms | 2.4 ms | 18% | 4.6% | 81 | 4.9% | 2 | 1/80 | 0.0 ms | 3.6% | 0% |
| 3:40 | 400 | 400 | 0 | 0% | 1.4 ms | 2.4 ms | 18% | 4.2% | 81 | 4.8% | 2 | 1/80 | 0.0 ms | 3.6% | 0% |
| 3:50 | 400 | 400 | 0 | 0% | 1.7 ms | 2.4 ms | 18% | 4.2% | 81 | 4.6% | 2 | 2/80 | 0.0 ms | 3.1% | 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 CPU is against its budget.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: slow-disk-more-connections

# Solution: more connections, 40 per replica. The database's CPU is idle;
# its commits wait on the disk. Commits that wait at the same time share
# one flush of the write-ahead log (group commit), so more of them at once
# get more done per fsync.

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
      db: { size: 40, min: 40 }   # 2 replicas x 40 = 80 connections

  db:
    kind: postgres
    version: "17"
    resources: { cpu: 1, memory: 1g }
    dataset: { rows: 100000, row_bytes: 256 }

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
