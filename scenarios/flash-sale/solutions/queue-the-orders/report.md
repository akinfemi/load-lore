# flash-sale-queue-the-orders: run report

_run `flash-sale-queue-the-orders-20261118-080449` · completed · 18 Nov 2026 08:05 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 1,500 rps offered, 1,500 rps served at best, worst 10-second p99 3.0 ms, at most 0% errors. But jobs backed up in orders: the oldest waited 62.7 s (see consumer lag).

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 2:30

Measured in the 10-second window from 2:30 to 2:40 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 1,500 rps<br/>offered 1,500 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 16%<br/>2/2 replicas up"}}
    api["api ×2<br/>1 CPU · 512m<br/>CPU 32%<br/>in flight 2"]
    db[("db<br/>postgres 17<br/>1 CPU · 1g<br/>disk gp3 · 3,000 IOPS<br/>CPU 11%<br/>conns 86/100")]
    orders[("orders<br/>queue · Redis Streams<br/>0.5 CPU · 256m<br/>CPU 7.5%<br/>21,640 jobs, oldest 24.48 s")]
    fulfil["fulfil<br/>worker<br/>0.5 CPU · 256m<br/>CPU 24%<br/>499 jobs/s, 5/5 busy"]
    load -->|"40% read · 60% write<br/>1,500 rps · p99 2.1 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>1,500 rps · p99 1.1 ms"| api
    api -->|"read on reads<br/>pool 40, min 40<br/>598 calls/s · p99 0.7 ms<br/>in use 1/80"| db
    api -->|"publish on writes<br/>pool 40<br/>902 calls/s · p99 0.5 ms<br/>in use 2/80"| orders
    orders -->|"consume, 5 at a time<br/>499 jobs/s"| fulfil
    fulfil -->|"write<br/>pool 5, min 5<br/>499 calls/s · p99 28 ms<br/>in use 5/5"| db
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb,api,db,orders ok
    class fulfil hot
```

<sub>Sources: offered from the load profile in plan.json (offered arrival rate); load edge from k6_http_reqs_total (k6: all responses) and k6_http_req_duration_seconds (k6 histogram, end to end); CPU from container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit; calls/s from loadlore_calls_total (the caller's outbound call attempts) and their p99 from loadlore_call_duration_seconds (the caller's histogram, pool wait included); pools from loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections and loadlore_pool_pending (callers waiting for a connection); lb traffic from haproxy_backend_http_requests_total (HAProxy), replicas up from haproxy_backend_active_servers (HAProxy), and the lb edge's p99 from loadlore_request_duration_seconds (the service's own histogram); conns from pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); a queue's jobs from redis_stream_length (redis_exporter): jobs published and not yet finished, waiting or running and its oldest from redis_stream_first_entry_id (redis_exporter): the oldest unfinished job's publish time; a worker's jobs/s from loadlore_jobs_total (the workers' finished jobs, by result) and busy runners from loadlore_job_runner_busy_seconds_total, or loadlore_jobs_running in runs from before it (summed over the worker's replicas).</sub>

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
    x-axis "seconds" 0 --> 300
    y-axis "requests/s" 0 --> 2000
    line [200, 200, 200, 200, 200, 200, 308.333, 525, 741.667, 958.333, 1175, 1391.667, 1500, 1500, 1500, 1500, 1500, 1500, 850, 200, 200, 200, 200, 200, 200, 200, 200, 200, 200, 200, 200]
    line [198.14, 200.019, 200.042, 199.825, 200.119, 200.041, 306.258, 522.909, 739.633, 956.425, 1172.909, 1389.543, 1499.936, 1500.035, 1499.848, 1500.065, 1500.017, 1499.744, 862.726, 199.889, 199.993, 200.003, 200.001, 200.032, 200.002, 199.987, 200.058, 200.021, 199.92, 200.147, 199.841]
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
    x-axis "seconds" 0 --> 300
    y-axis "ms" 0 --> 4
    line [1.378, 1.329, 1.328, 1.328, 1.328, 1.329, 1.319, 1.292, 1.257, 1.239, 1.229, 1.226, 1.224, 1.225, 1.227, 1.229, 1.228, 1.238, 1.239, 1.296, 1.296, 1.299, 1.322, 1.331, 1.308, 1.306, 1.308, 1.328, 1.328, 1.327, 1.327]
    line [1.771, 1.523, 1.481, 1.493, 1.489, 1.492, 1.471, 1.403, 1.379, 1.379, 1.375, 1.377, 1.379, 1.38, 1.38, 1.388, 1.38, 1.464, 1.404, 1.443, 1.417, 1.475, 1.491, 1.517, 1.454, 1.45, 1.457, 1.494, 1.482, 1.485, 1.476]
    line [2.954, 2.012, 1.769, 1.976, 1.86, 1.899, 1.914, 1.759, 1.682, 1.893, 1.77, 1.904, 1.99, 2.069, 1.986, 2.112, 2.043, 2.065, 1.919, 1.781, 1.674, 2.018, 1.899, 1.951, 1.628, 1.953, 1.667, 1.88, 1.878, 1.887, 1.734]
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
    x-axis "seconds" 0 --> 300
    y-axis "% of requests" 0 --> 1
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included.</sub>

```mermaid
---
config:
  themeVariables:
    xyChart:
      backgroundColor: "transparent"
      plotColorPalette: "#1f77b4, #ff7f0e, #2ca02c, #9467bd, #8c564b"
---
xychart-beta
    title "CPU against each limit"
    x-axis "seconds" 0 --> 300
    y-axis "% of limit" 0 --> 100
    line [4.858, 4.75, 4.755, 4.762, 4.762, 4.814, 6.893, 11.148, 15.531, 20.037, 24.64, 29.246, 31.7, 31.794, 31.859, 32.001, 32.005, 32.374, 18.425, 4.851, 4.826, 4.943, 5.11, 5.267, 4.909, 4.91, 4.924, 4.75, 4.762, 4.752, 4.657]
    line [6.789, 3.412, 3.237, 3.428, 3.302, 3.401, 3.821, 4.925, 6.693, 9.342, 9.499, 9.846, 10.24, 10.276, 10.421, 10.522, 10.397, 11.194, 9.188, 7.799, 7.951, 8.045, 9.153, 9.556, 8.361, 8.234, 8.384, 3.634, 3.321, 3.44, 3.328]
    line [6.527, 6.524, 6.263, 6.417, 6.456, 6.354, 8.894, 13.644, 19.327, 22.249, 22.632, 22.873, 23.283, 23.534, 23.64, 23.576, 23.81, 24.678, 22.28, 20.989, 20.938, 21.195, 22.829, 23.541, 21.567, 21.501, 21.316, 7.29, 6.362, 6.607, 6.087]
    line [3.886, 3.178, 3.18, 3.186, 3.203, 3.257, 4.193, 6.329, 9.271, 11.627, 13.4, 15.39, 16.313, 15.1, 15.484, 16.316, 15.033, 16.539, 9.075, 3.272, 3.421, 3.395, 3.832, 4.093, 3.353, 3.479, 3.466, 3.153, 3.022, 3.111, 3.2]
    line [2.563, 2.552, 2.498, 2.539, 2.536, 2.601, 3.041, 4.159, 5.5, 5.947, 6.278, 6.828, 7.333, 7.439, 7.342, 7.467, 7.367, 7.912, 6.085, 4.577, 4.566, 4.708, 5.283, 5.758, 4.74, 4.76, 4.774, 2.785, 2.536, 2.584, 2.599]
```

<sub>Blue: api. Orange: db. Green: fulfil. Purple: lb. Brown: orders (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window).</sub>

## Diagnosis

### 1. Consumer lag (orders), from 1:30

From 1:30, jobs went into orders faster than fulfil finished them: at 2:00, 901 a second published against 494 finished. The queue grew to 31,299 jobs, and the oldest had waited 62.7 s. fulfil's writes to db took 9.3 ms against 2.1 ms before the lag, with 3.2 backends of db waiting on locks: its jobs update the same rows, and a row takes one write at a time, each keeping its lock until it commits. More job runners would only wait in line for the same rows.

Evidence:

- 901 jobs a second published to orders at 2:00 <sub>(redis_stream_entries_added_total (redis_exporter), less loadlore_jobs_requeued_total (retries put back))</sub>
- 494 jobs a second finished by fulfil at 2:00 <sub>(loadlore_jobs_total (the workers' finished jobs, by result))</sub>
- 31,299 jobs in orders at 3:00, against 5 job runners across fulfil <sub>(redis_stream_length (redis_exporter): jobs published and not yet finished, waiting or running)</sub>
- the oldest unfinished job had waited 62.7 s <sub>(redis_stream_first_entry_id (redis_exporter): the oldest unfinished job's publish time)</sub>
- p99 wait of the jobs taken at 3:00: 39.93 s <sub>(loadlore_job_wait_seconds (the workers' histogram: publish to start))</sub>
- fulfil CPU at 22% of its limit at 3:00, over 1 replica <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- 9.4 ms per job on fulfil at 3:00, against 2.2 ms before the lag <sub>(loadlore_job_duration_seconds (the workers' histogram: time to run a job))</sub>
- 5 of fulfil's 5 job runners busy on average at 3:00 <sub>(loadlore_job_runner_busy_seconds_total, or loadlore_jobs_running in runs from before it (summed over the worker's replicas))</sub>
- fulfil's writes to db took 9.3 ms at 3:00, against 2.1 ms before the lag <sub>(loadlore_call_duration_seconds by op (the caller's own timings, pool wait included))</sub>
- 3.2 backends of db waiting on locks at 3:00, on average <sub>(pg_lock_waits_waiting: pg_stat_activity backends with wait_event_type Lock (postgres_exporter, Loadlore's query))</sub>

**Try next:** Don't add job runners or replicas to fulfil: the hot rows set its pace. Make the writes to them fewer or cheaper: apply many jobs in one write (a batch), or spread them over more rows. Its workers finished at most about 500 jobs a second, against 903 published. If that pace catches up after the peak, the queue is doing its job: check that the oldest job's wait is one your users accept.

<sub>Rules version 23. Rules are deterministic and offline; their thresholds are listed below.</sub>

## Predictions

1 of 1 scored prediction held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `orders.oldest_job_ms < 120s` | **held** | 62.70 s | the whole run | source: redis_stream_first_entry_id (redis_exporter): the oldest unfinished job's publish time |

## Assumptions

### From the design

| Field | Value | Basis | Why |
| --- | --- | --- | --- |
| `services.orders.resources.memory` |  | guessed | a small managed Redis for background jobs |
| `services.api.behavior.cpu_ms` |  | guessed | a small JSON handler |
| `workload.mix` |  | guessed | during the drop, most requests are purchases |
| `workload.keys` |  | guessed | one shoe in a catalog of a thousand takes nineteen requests in twenty |
| `workload.profile` |  | guessed | buyers arrive over the drop's first minute and leave once it sells out |

5 of these are guessed, not measured: the result depends on them.

### Chosen by Loadlore

| Field | Value | Why |
| --- | --- | --- |
| services.orders.config.maxmemory | 192m | 75% of the container's 256m memory limit, as managed Redis usually does; the rest is for fragmentation and client buffers. Set maxmemory: 0 for stock Redis (no cap) |
| services.orders.config.maxmemory-policy | noeviction | a queue keeps every job: once it is full, publishing fails with an error until workers make room, as Sidekiq and BullMQ ask of their Redis |
| workload.timeout_ms | 10s | the default; k6 gives up on a request after this long and counts it as failed |
| workload.slo.p99_ms | 500 ms | the default; a break on p99 alone needs p99 at or above this as well as 10x the low-load p99, so latency users would still accept isn't called a break |
| network.profile | same-zone: 0.3 ms round trip | the default: every service in one availability zone. AWS says a round trip between two instances in the same zone is generally sub-millisecond with enhanced networking, and publishes no exact figure, so the 0.3 ms round trip Loadlore adds between any two containers is its own estimate, inside that bound. Applied between any two of the stack's containers, the load generator and the entry point included, as half the round trip on each side's outgoing packets; Loadlore's own tooling isn't slowed. Not scaled with --scale. Source: AWS Architecture Blog, Improving performance and reducing cost using Availability Zone Affinity: https://aws.amazon.com/blogs/architecture/improving-performance-and-reducing-cost-using-availability-zone-affinity/ |
| resources.cpu_period_ms | 100 ms for api, db, fulfil, lb, orders | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.db.disk | gp3: 1 ms per read, 1 ms per write, 3000 IOPS, 125 MiB/s | a gp3 volume at its baseline: 3,000 IOPS and 125 MiB/s, included in its price, and up to 80,000 IOPS provisioned. AWS publishes no exact latency, only single-digit milliseconds, so the 1 ms Loadlore adds to every read, write and fsync is its own estimate: the low end of that range, so the disk is flattered rather than handicapped. Reads and writes are capped separately, so a mixed load can reach up to twice the IOPS. Provisioned throughput isn't modelled. Source: AWS, Amazon EBS General Purpose SSD volumes: https://docs.aws.amazon.com/ebs/latest/userguide/general-purpose.html |
| services.fulfil.consume.concurrency | 5 | Sidekiq's default: each replica runs up to 5 jobs from orders at once, and the rest wait in the queue |
| services.fulfil.consume.retries | none | not set in the design: a job whose calls fail is dropped, not retried. Real job systems retry (Sidekiq up to 25 times), so add retries to see the backlog retries make |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |
| services.api.pools.db.min | 40 | set in the design: each replica opens 40 connections before the load starts and keeps them open |
| services.api.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |
| services.fulfil.pools.db.min | 5 | set in the design: each replica opens 5 connections before the load starts and keeps them open |
| services.fulfil.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- Loadlore's own containers get fixed budgets: load generator 1.5 CPU (peak 1500 rps), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each. No fault acts on a connection, so every connection is direct and there is no Toxiproxy

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

- loadl dev, profile 5m10s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 1008 (hash 971, memory 1051, syscalls 1002): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (1008) was within 5.0% of the run it's compared with (965)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 185,499 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-flash-sale-queue-the-orders-20261118-080449-api-0-1 | loadlore/synthetic:dev |
| loadlore-flash-sale-queue-the-orders-20261118-080449-api-1-1 | loadlore/synthetic:dev |
| loadlore-flash-sale-queue-the-orders-20261118-080449-db-1 | postgres:17.11 |
| loadlore-flash-sale-queue-the-orders-20261118-080449-fulfil-0-1 | loadlore/synthetic:dev |
| loadlore-flash-sale-queue-the-orders-20261118-080449-lb-1 | haproxy:3.2.26 |
| loadlore-flash-sale-queue-the-orders-20261118-080449-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-flash-sale-queue-the-orders-20261118-080449-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-flash-sale-queue-the-orders-20261118-080449-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-flash-sale-queue-the-orders-20261118-080449-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-flash-sale-queue-the-orders-20261118-080449-lore-redisexp-orders-1 | oliver006/redis_exporter:v1.97.0 |
| loadlore-flash-sale-queue-the-orders-20261118-080449-orders-1 | redis:8.2 |

</details>

## Per-window data

<details><summary>31 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | fulfil CPU | lb CPU | lb up | orders CPU | orders jobs | oldest | published/s | finished/s | api→db pool | wait | api→orders pool | wait | fulfil→db pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 200 | 198 | 0 | 0% | 1.4 ms | 3.0 ms | 4.9% | 6.8% | 86 | 6.5% | 3.9% | 2 | 2.6% | 0 | 0.0 ms | 118 | 118 | 0/80 | 0.0 ms | 0/80 | – | 1/5 | 0.0 ms | 4.3% | 0% |
| 0:10 | 200 | 200 | 0 | 0% | 1.3 ms | 2.0 ms | 4.7% | 3.4% | 86 | 6.5% | 3.2% | 2 | 2.6% | 0 | 0.0 ms | 122 | 122 | 0/80 | 0.0 ms | 0/80 | – | 1/5 | 0.0 ms | 2.3% | 0% |
| 0:20 | 200 | 200 | 0 | 0% | 1.3 ms | 1.8 ms | 4.8% | 3.2% | 86 | 6.3% | 3.2% | 2 | 2.5% | 0 | 0.0 ms | 118 | 118 | 0/80 | 0.0 ms | 0/80 | – | 1/5 | 0.0 ms | 2.6% | 0% |
| 0:30 | 200 | 200 | 0 | 0% | 1.3 ms | 2.0 ms | 4.8% | 3.4% | 86 | 6.4% | 3.2% | 2 | 2.5% | 1 | 0.0 ms | 119 | 119 | 0/80 | 0.0 ms | 0/80 | – | 1/5 | 0.0 ms | 2.5% | 0% |
| 0:40 | 200 | 200 | 0 | 0% | 1.3 ms | 1.9 ms | 4.8% | 3.3% | 86 | 6.5% | 3.2% | 2 | 2.5% | 0 | 0.0 ms | 121 | 120 | 0/80 | 0.0 ms | 0/80 | – | 1/5 | 0.0 ms | 2.3% | 0% |
| 0:50 | 200 | 200 | 0 | 0% | 1.3 ms | 1.9 ms | 4.8% | 3.4% | 86 | 6.4% | 3.3% | 2 | 2.6% | 0 | 0.0 ms | 117 | 118 | 0/80 | 0.0 ms | 0/80 | – | 1/5 | 0.0 ms | 2.6% | 0% |
| 1:00 | 308 | 306 | 0 | 0% | 1.3 ms | 1.9 ms | 6.9% | 3.8% | 86 | 8.9% | 4.2% | 2 | 3.0% | 1 | 0.0 ms | 182 | 182 | 0/80 | 0.0 ms | 1/80 | – | 1/5 | 0.0 ms | 3.3% | 0% |
| 1:10 | 525 | 523 | 0 | 0% | 1.3 ms | 1.8 ms | 11% | 4.9% | 86 | 14% | 6.3% | 2 | 4.2% | 3 | 0.0 ms | 307 | 306 | 0/80 | 0.0 ms | 1/80 | – | 2/5 | 0.0 ms | 4.8% | 0% |
| 1:20 | 742 | 740 | 0 | 0% | 1.3 ms | 1.7 ms | 16% | 6.7% | 86 | 19% | 9.3% | 2 | 5.5% | 12 | 16 ms | 443 | 441 | 0/80 | 0.0 ms | 1/80 | – | 4/5 | 0.0 ms | 6.0% | 0% |
| 1:30 | 958 | 956 | 0 | 0% | 1.2 ms | 1.9 ms | 20% | 9.3% | 86 | 22% | 12% | 2 | 5.9% | 547 | 1.08 s | 572 | 499 | 1/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 8.2% | 0% |
| 1:40 | 1,175 | 1,173 | 0 | 0% | 1.2 ms | 1.8 ms | 25% | 9.5% | 86 | 23% | 13% | 2 | 6.3% | 2,423 | 3.60 s | 705 | 498 | 1/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 8.3% | 0% |
| 1:50 | 1,392 | 1,390 | 0 | 0% | 1.2 ms | 1.9 ms | 29% | 9.8% | 86 | 23% | 15% | 2 | 6.8% | 5,540 | 7.03 s | 835 | 499 | 0/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 9.1% | 0% |
| 2:00 | 1,500 | 1,500 | 0 | 0% | 1.2 ms | 2.0 ms | 32% | 10% | 86 | 23% | 16% | 2 | 7.3% | 9,634 | 11.13 s | 901 | 494 | 0/80 | 0.0 ms | 2/80 | – | 5/5 | 0.0 ms | 9.8% | 0% |
| 2:10 | 1,500 | 1,500 | 0 | 0% | 1.2 ms | 2.1 ms | 32% | 10% | 86 | 24% | 15% | 2 | 7.4% | 13,635 | 15.59 s | 903 | 498 | 1/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 9.8% | 0% |
| 2:20 | 1,500 | 1,500 | 0 | 0% | 1.2 ms | 2.0 ms | 32% | 10% | 86 | 24% | 15% | 2 | 7.3% | 17,604 | 20.06 s | 894 | 499 | 3/80 | 0.0 ms | 2/80 | – | 5/5 | 0.0 ms | 9.9% | 0% |
| 2:30 | 1,500 | 1,500 | 0 | 0% | 1.2 ms | 2.1 ms | 32% | 11% | 86 | 24% | 16% | 2 | 7.5% | 21,640 | 24.48 s | 901 | 499 | 1/80 | 0.0 ms | 2/80 | – | 5/5 | 0.0 ms | 9.8% | 0% |
| 2:40 | 1,500 | 1,500 | 0 | 0% | 1.2 ms | 2.0 ms | 32% | 10% | 86 | 24% | 15% | 2 | 7.4% | 25,664 | 29.01 s | 900 | 499 | 2/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 10% | 0% |
| 2:50 | 1,500 | 1,500 | 0 | 0% | 1.2 ms | 2.1 ms | 32% | 11% | 86 | 25% | 17% | 2 | 7.9% | 29,615 | 33.40 s | 893 | 498 | 1/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 11% | 0% |
| 3:00 | 850 | 863 | 0 | 0% | 1.2 ms | 1.9 ms | 18% | 9.2% | 86 | 22% | 9.1% | 2 | 6.1% | 30,876 | 37.86 s | 518 | 500 | 1/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 6.7% | 0% |
| 3:10 | 200 | 200 | 0 | 0% | 1.3 ms | 1.8 ms | 4.9% | 7.8% | 86 | 21% | 3.3% | 2 | 4.6% | 27,167 | 42.35 s | 118 | 498 | 1/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.5% | 0% |
| 3:20 | 200 | 200 | 0 | 0% | 1.3 ms | 1.7 ms | 4.8% | 8.0% | 86 | 21% | 3.4% | 2 | 4.6% | 23,387 | 46.79 s | 121 | 498 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.1% | 0% |
| 3:30 | 200 | 200 | 0 | 0% | 1.3 ms | 2.0 ms | 4.9% | 8.0% | 87 | 21% | 3.4% | 2 | 4.7% | 19,631 | 51.26 s | 123 | 499 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.6% | 0% |
| 3:40 | 200 | 200 | 0 | 0% | 1.3 ms | 1.9 ms | 5.1% | 9.2% | 86 | 23% | 3.8% | 2 | 5.3% | 15,822 | 55.66 s | 119 | 499 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.4% | 0% |
| 3:50 | 200 | 200 | 0 | 0% | 1.3 ms | 2.0 ms | 5.3% | 9.6% | 86 | 24% | 4.1% | 2 | 5.8% | 12,078 | 60.09 s | 123 | 498 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 3.0% | 0% |
| 4:00 | 200 | 200 | 0 | 0% | 1.3 ms | 1.6 ms | 4.9% | 8.4% | 86 | 22% | 3.4% | 2 | 4.7% | 8,328 | 62.70 s | 124 | 500 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.2% | 0% |
| 4:10 | 200 | 200 | 0 | 0% | 1.3 ms | 2.0 ms | 4.9% | 8.2% | 86 | 22% | 3.5% | 2 | 4.8% | 4,564 | 61.83 s | 122 | 499 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.6% | 0% |
| 4:20 | 200 | 200 | 0 | 0% | 1.3 ms | 1.7 ms | 4.9% | 8.4% | 86 | 21% | 3.5% | 2 | 4.8% | 742 | 30.98 s | 116 | 497 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.2% | 0% |
| 4:30 | 200 | 200 | 0 | 0% | 1.3 ms | 1.9 ms | 4.7% | 3.6% | 86 | 7.3% | 3.2% | 2 | 2.8% | 0 | 0.0 ms | 119 | 137 | 1/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.6% | 0% |
| 4:40 | 200 | 200 | 0 | 0% | 1.3 ms | 1.9 ms | 4.8% | 3.3% | 86 | 6.4% | 3.0% | 2 | 2.5% | 1 | 0.0 ms | 119 | 120 | 0/80 | 0.0 ms | 0/80 | – | 1/5 | 0.0 ms | 2.2% | 0% |
| 4:50 | 200 | 200 | 0 | 0% | 1.3 ms | 1.9 ms | 4.8% | 3.4% | 86 | 6.6% | 3.1% | 2 | 2.6% | 0 | 0.0 ms | 123 | 123 | 0/80 | 0.0 ms | 0/80 | – | 1/5 | 0.0 ms | 2.6% | 0% |
| 5:00 | 200 | 200 | 0 | 0% | 1.3 ms | 1.7 ms | 4.7% | 3.3% | 86 | 6.1% | 3.2% | 2 | 2.6% | 0 | 0.0 ms | 117 | 118 | 0/80 | 0.0 ms | 0/80 | – | 1/5 | 0.0 ms | 2.0% | 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 CPU is against its budget. A queue's jobs are redis_stream_length (redis_exporter): jobs published and not yet finished, waiting or running, at the window's end; oldest is redis_stream_first_entry_id (redis_exporter): the oldest unfinished job's publish time, at most over the window; published is redis_stream_entries_added_total (redis_exporter), less loadlore_jobs_requeued_total (retries put back) and finished loadlore_jobs_total (the workers' finished jobs, by result).

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: flash-sale-queue-the-orders

# The flash sale with its purchases queued: checkout puts each purchase on
# a queue and answers at once, and a worker takes them off and updates the
# stock, a few at a time.

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
        - { target: db, op: read, when: read }    # a product page reads the stock
        - { target: orders, op: publish, when: write }  # a purchase goes on the queue
      resilience:
        timeout_ms: 2000
    pools:
      db: { size: 40, min: 40 }   # opened at start; 2 replicas x 40 = 80 connections, under max_connections
      orders: 40

  orders:
    kind: queue
    resources: { cpu: 0.5, memory: 256m }

  fulfil:
    image: loadlore/synthetic
    role: worker
    resources: { cpu: 0.5, memory: 256m }
    consume: { from: orders }   # 5 jobs at a time, Sidekiq's default
    behavior:
      cpu_ms: 0.1
      calls:
        - { target: db, op: write }   # take one from the stock
    pools:
      db: { size: 5, min: 5 }

  db:
    kind: postgres
    version: "17"
    resources: { cpu: 1, memory: 1g }
    config: { max_connections: 100, shared_buffers: 256MB }
    dataset: { rows: 1000, row_bytes: 256 }
    disk: { profile: gp3 }   # a network-attached cloud volume: each commit waits about 1 ms longer for its WAL

workload:
  entry: lb
  mix: { read: 0.4, write: 0.6 }
  keys: { distribution: hotspot, share: 0.95, fraction: 0.001, count: 1000 }
  profile:
    - hold: { rps: 200, for: 1m }                # a normal morning
    - ramp: { from: 200, to: 1500, over: 1m }    # the drop opens
    - hold: { rps: 1500, for: 1m }
    - ramp: { from: 1500, to: 200, over: 10s }   # sold out
    - hold: { rps: 200, for: 2m }

assumptions:
  - { field: services.orders.resources.memory, basis: guessed, why: "a small managed Redis for background jobs" }
  - { field: services.api.behavior.cpu_ms, basis: guessed, why: "a small JSON handler" }
  - { field: workload.mix, basis: guessed, why: "during the drop, most requests are purchases" }
  - { field: workload.keys, basis: guessed, why: "one shoe in a catalog of a thousand takes nineteen requests in twenty" }
  - { field: workload.profile, basis: guessed, why: "buyers arrive over the drop's first minute and leave once it sells out" }

# Your predictions, scored in the report. The line here is an example;
# loadl predict design.yaml replaces it with your answers.
expect:
  - orders.oldest_job_ms < 120s
```
