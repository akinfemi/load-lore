# statement-day-provisioned-iops: run report

_run `statement-day-provisioned-iops-20261118-002131` · completed · 18 Nov 2026 00:21 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 3,500 rps offered, 3,401 rps served at best, worst 10-second p99 6.7 ms, at most 0% errors.

Caveat: at 2:30 the load generator hit its CPU limit and was stalled in 2.3% of its scheduling periods (container_cpu_cfs_throttled_periods_total ÷ container_cpu_cfs_periods_total (cAdvisor)); work caught in a stall waits for the next 100 ms period, which can show up in p99.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 2:50

Measured in the 10-second window from 2:50 to 3:00 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 3,500 rps<br/>offered 3,408 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 31%<br/>2/2 replicas up"}}
    api["api ×2<br/>1 CPU · 512m<br/>CPU 68%<br/>in flight 8"]
    db[("db<br/>postgres 17<br/>1 CPU · 512m<br/>disk 1 ms · 10,000 IOPS<br/>CPU 27%<br/>conns 81/100")]
    load -->|"100% read · 0% write<br/>3,401 rps · p99 5.0 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>3,401 rps · p99 4.3 ms"| api
    api -->|"read<br/>pool 40, min 40<br/>3,376 calls/s · p99 3.5 ms<br/>in use 8/80"| db
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
      plotColorPalette: "#9e9e9e, #1f77b4"
---
xychart-beta
    title "Throughput"
    x-axis "seconds" 0 --> 170
    y-axis "requests/s" 0 --> 4000
    line [291.667, 475, 658.333, 841.667, 1025, 1208.333, 1391.667, 1575, 1758.333, 1941.667, 2125, 2308.333, 2491.667, 2675, 2858.333, 3041.667, 3225, 3408.333]
    line [276.943, 467.921, 651.165, 834.653, 1017.766, 1201.322, 1384.444, 1567.694, 1751.487, 1934.552, 2117.851, 2300.956, 2484.824, 2667.68, 2851.486, 3034.538, 3217.83, 3401.084]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)).</sub>

```mermaid
---
config:
  themeVariables:
    xyChart:
      backgroundColor: "transparent"
      plotColorPalette: "#2ca02c, #ff7f0e, #d62728"
---
xychart-beta
    title "Latency, end to end"
    x-axis "seconds" 0 --> 170
    y-axis "ms" 0 --> 8
    line [3.263, 2.792, 2.272, 1.983, 2.007, 2.015, 2.015, 2.02, 2.028, 2.032, 2.063, 2.038, 2.037, 2.046, 2.048, 2.056, 2.064, 2.081]
    line [3.914, 3.544, 3.371, 3.101, 3.098, 3.113, 3.101, 3.131, 3.136, 3.161, 4.516, 3.185, 3.181, 3.19, 3.205, 3.244, 3.243, 3.454]
    line [5.023, 3.838, 3.586, 3.541, 3.458, 3.479, 3.408, 3.525, 3.414, 3.538, 6.664, 3.665, 3.554, 3.619, 3.864, 4.375, 4.015, 4.998]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)).</sub>

```mermaid
---
config:
  themeVariables:
    xyChart:
      backgroundColor: "transparent"
      plotColorPalette: "#d62728"
---
xychart-beta
    title "Errors"
    x-axis "seconds" 0 --> 170
    y-axis "% of requests" 0 --> 1
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included.</sub>

```mermaid
---
config:
  themeVariables:
    xyChart:
      backgroundColor: "transparent"
      plotColorPalette: "#1f77b4, #ff7f0e, #2ca02c"
---
xychart-beta
    title "CPU against each limit"
    x-axis "seconds" 0 --> 170
    y-axis "% of limit" 0 --> 100
    line [6.648, 10.118, 13.742, 17.368, 20.936, 24.444, 28.189, 31.938, 35.463, 39.144, 42.907, 46.173, 49.978, 53.413, 56.798, 60.827, 64.746, 68.357]
    line [4.015, 5.078, 5.469, 6.312, 6.787, 7.858, 8.72, 9.622, 10.283, 11.409, 13.389, 14, 15.962, 17.2, 19.173, 22.636, 24.883, 26.584]
    line [5.393, 5.682, 7.485, 9.013, 10.663, 12.077, 13.199, 14.532, 15.489, 16.545, 15.08, 18.524, 20.752, 22.468, 22.628, 22.182, 25.136, 30.646]
```

<sub>Blue: api. Orange: db. Green: lb (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window).</sub>

## Diagnosis

No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version 22; thresholds below).

## Predictions

1 of 1 scored prediction held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 100 until load_rps >= 1000` | **held** | 5.0 ms | 0:00 to 0:40, when load_rps >= 1,000 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |

Loads and request rates depend on this machine: slower cores break the same design at a lower load, so a load guess can miss even when the reasoning behind it was right.

## Assumptions

### From the design

| Field | Value | Basis | Why |
| --- | --- | --- | --- |
| `services.api.behavior.cpu_ms` |  | guessed | a small JSON handler |
| `workload.keys` |  | guessed | every customer got the email, so any one is as likely to click as another |
| `services.db.disk` |  | guessed | a general-purpose cloud volume at its baseline |

3 of these are guessed, not measured: the result depends on them.

### Chosen by Loadlore

| Field | Value | Why |
| --- | --- | --- |
| workload.timeout_ms | 10s | the default; k6 gives up on a request after this long and counts it as failed |
| workload.slo.p99_ms | 500 ms | the default; a break on p99 alone needs p99 at or above this as well as 10x the low-load p99, so latency users would still accept isn't called a break |
| network.profile | same-zone: 0.3 ms round trip | the default: every service in one availability zone. AWS says a round trip between two instances in the same zone is generally sub-millisecond with enhanced networking, and publishes no exact figure, so the 0.3 ms round trip Loadlore adds between any two containers is its own estimate, inside that bound. Applied between any two of the stack's containers, the load generator and the entry point included, as half the round trip on each side's outgoing packets; Loadlore's own tooling isn't slowed. Not scaled with --scale. Source: AWS Architecture Blog, Improving performance and reducing cost using Availability Zone Affinity: https://aws.amazon.com/blogs/architecture/improving-performance-and-reducing-cost-using-availability-zone-affinity/ |
| resources.cpu_period_ms | 100 ms for api, db, lb | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.db.disk | no profile: 1 ms per read, 10000 IOPS | set in the design: every read that misses db's memory waits this long, and reads and writes are each capped at this rate, as on a cloud volume (unset values default to about a gp3 volume's: 1 ms, 3000 IOPS) |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |
| services.api.pools.db.min | 40 | set in the design: each replica opens 40 connections before the load starts and keeps them open |
| services.api.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- Loadlore's own containers get fixed budgets: load generator 1.5 CPU (peak 3500 rps), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each. No fault acts on a connection, so every connection is direct and there is no Toxiproxy

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
- **Host disk stalls:** for 2 seconds or more, a Postgres on the Docker host's own disk read no blocks and committed at most 10% of its median over the 10 s before (50 a second or more), while 5 or more queries waited on disk reads or WAL flushes and its exporter answered. Near the break it makes the run inconclusive; anywhere else it's a caveat, and its windows are flagged and left out of exports.

## Environment

- loadl dev, profile 3m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 966 (hash 908, memory 1014, syscalls 977): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (966) was within 5.0% of the run it's compared with (1000)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 332,969 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-statement-day-provisioned-iops-20261118-002131-api-0-1 | loadlore/synthetic:dev |
| loadlore-statement-day-provisioned-iops-20261118-002131-api-1-1 | loadlore/synthetic:dev |
| loadlore-statement-day-provisioned-iops-20261118-002131-db-1 | postgres:17.11 |
| loadlore-statement-day-provisioned-iops-20261118-002131-lb-1 | haproxy:3.2.26 |
| loadlore-statement-day-provisioned-iops-20261118-002131-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-statement-day-provisioned-iops-20261118-002131-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-statement-day-provisioned-iops-20261118-002131-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-statement-day-provisioned-iops-20261118-002131-lore-prometheus-1 | prom/prometheus:v3.16.0 |

</details>

## Per-window data

<details><summary>18 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | lb CPU | lb up | api→db pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 292 | 277 | 0 | 0% | 3.3 ms | 5.0 ms | 6.6% | 4.0% | 81 | 5.4% | 2 | 1/80 | 0.0 ms | 6.8% | 3.0% |
| 0:10 | 475 | 468 | 0 | 0% | 2.8 ms | 3.8 ms | 10% | 5.1% | 81 | 5.7% | 2 | 2/80 | 0.0 ms | 4.6% | 0% |
| 0:20 | 658 | 651 | 0 | 0% | 2.3 ms | 3.6 ms | 14% | 5.5% | 81 | 7.5% | 2 | 2/80 | 0.0 ms | 5.5% | 0% |
| 0:30 | 842 | 835 | 0 | 0% | 2.0 ms | 3.5 ms | 17% | 6.3% | 81 | 9.0% | 2 | 2/80 | 0.0 ms | 6.5% | 0% |
| 0:40 | 1,025 | 1,018 | 0 | 0% | 2.0 ms | 3.5 ms | 21% | 6.8% | 81 | 11% | 2 | 3/80 | 0.0 ms | 6.5% | 0% |
| 0:50 | 1,208 | 1,201 | 0 | 0% | 2.0 ms | 3.5 ms | 24% | 7.9% | 81 | 12% | 2 | 3/80 | 0.0 ms | 6.7% | 0% |
| 1:00 | 1,392 | 1,384 | 0 | 0% | 2.0 ms | 3.4 ms | 28% | 8.7% | 81 | 13% | 2 | 3/80 | 0.0 ms | 7.2% | 0% |
| 1:10 | 1,575 | 1,568 | 0 | 0% | 2.0 ms | 3.5 ms | 32% | 9.6% | 81 | 15% | 2 | 3/80 | 0.0 ms | 8.5% | 0% |
| 1:20 | 1,758 | 1,751 | 0 | 0% | 2.0 ms | 3.4 ms | 35% | 10% | 81 | 15% | 2 | 3/80 | 0.0 ms | 8.2% | 0% |
| 1:30 | 1,942 | 1,935 | 0 | 0% | 2.0 ms | 3.5 ms | 39% | 11% | 81 | 17% | 2 | 10/80 | 0.0 ms | 9.8% | 1.0% |
| 1:40 | 2,125 | 2,118 | 0 | 0% | 2.1 ms | 6.7 ms | 43% | 13% | 81 | 15% | 2 | 6/80 | 0.0 ms | 11% | 0% |
| 1:50 | 2,308 | 2,301 | 0 | 0% | 2.0 ms | 3.7 ms | 46% | 14% | 81 | 19% | 2 | 6/80 | 0.0 ms | 11% | 0% |
| 2:00 | 2,492 | 2,485 | 0 | 0% | 2.0 ms | 3.6 ms | 50% | 16% | 81 | 21% | 2 | 6/80 | 0.0 ms | 13% | 0% |
| 2:10 | 2,675 | 2,668 | 0 | 0% | 2.0 ms | 3.6 ms | 53% | 17% | 81 | 22% | 2 | 6/80 | 0.0 ms | 13% | 0% |
| 2:20 | 2,858 | 2,851 | 0 | 0% | 2.0 ms | 3.9 ms | 57% | 19% | 81 | 23% | 2 | 7/80 | 0.0 ms | 14% | 1.0% |
| 2:30 | 3,042 | 3,035 | 0 | 0% | 2.1 ms | 4.4 ms | 61% | 23% | 81 | 22% | 2 | 19/80 +1 | 0.0 ms | 16% | 2.3% |
| 2:40 | 3,225 | 3,218 | 0 | 0% | 2.1 ms | 4.0 ms | 65% | 25% | 81 | 25% | 2 | 6/80 | 0.0 ms | 18% | 1.7% |
| 2:50 | 3,408 | 3,401 | 0 | 0% | 2.1 ms | 5.0 ms | 68% | 27% | 81 | 31% | 2 | 8/80 | 0.0 ms | 18% | 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 CPU is against its budget.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: statement-day-provisioned-iops

# A bank's statements api. On the first of the month the bank emails every
# one of its 5 million customers that their statement is ready, and each
# click opens the statement page, which reads that customer's row. On a
# normal day a few hundred thousand busy customers make most of the
# traffic; today every customer is as likely as any other.

services:
  lb:
    kind: lb
    target: api
    health_check: { interval_ms: 2000, fall: 3, rise: 2 }

  api:
    image: loadlore/synthetic
    replicas: 2
    resources: { cpu: 1, memory: 512m }
    behavior:
      cpu_ms: 0.3
      calls:
        - { target: db, op: read }   # the customer's statement row
      resilience:
        timeout_ms: 2000
    pools:
      db: { size: 40, min: 40 }   # opened at start; 2 replicas x 40 = 80 connections, under max_connections

  db:
    kind: postgres
    version: "17"
    resources: { cpu: 1, memory: 512m }
    config: { max_connections: 100, shared_buffers: 128MB }
    dataset: { rows: 5000000, row_bytes: 512 }   # about 2.5 GB with its index
    disk: { iops: 10000 }   # was {}: 3,000 IOPS, gp3's baseline

workload:
  entry: lb
  profile:
    - ramp: { from: 200, to: 3500, over: 3m }
  mix: { read: 1.0 }
  keys: { distribution: uniform }   # the email went to everyone

assumptions:
  - { field: services.api.behavior.cpu_ms, basis: guessed, why: "a small JSON handler" }
  - { field: workload.keys, basis: guessed, why: "every customer got the email, so any one is as likely to click as another" }
  - { field: services.db.disk, basis: guessed, why: "a general-purpose cloud volume at its baseline" }

# Your predictions, scored in the report. The line here is an example;
# loadl predict design.yaml replaces it with your answers.
expect:
  - p99_ms < 100 until load_rps >= 1000
```
