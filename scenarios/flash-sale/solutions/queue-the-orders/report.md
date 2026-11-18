# flash-sale-queue-the-orders: run report

_run `flash-sale-queue-the-orders-20261118-005055` · completed · 18 Nov 2026 00:51 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 1,500 rps offered, 1,500 rps served at best, worst 10-second p99 23 ms, at most 0% errors. But jobs backed up in orders: the oldest waited 69.1 s (see consumer lag).

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 2:20

Measured in the 10-second window from 2:20 to 2:30 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 1,500 rps<br/>offered 1,500 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 16%<br/>2/2 replicas up"}}
    api["api ×2<br/>1 CPU · 512m<br/>CPU 32%<br/>in flight 2"]
    db[("db<br/>postgres 17<br/>1 CPU · 1g<br/>CPU 9.4%<br/>conns 86/100")]
    orders[("orders<br/>queue · Redis Streams<br/>0.5 CPU · 256m<br/>CPU 6.8%<br/>18,164 jobs, oldest 20.73 s")]
    fulfil["fulfil<br/>worker<br/>0.5 CPU · 256m<br/>CPU 21%<br/>444 jobs/s, 5/5 busy"]
    load -->|"40% read · 60% write<br/>1,500 rps · p99 1.9 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>1,500 rps · p99 1.0 ms"| api
    api -->|"read on reads<br/>pool 40, min 40<br/>608 calls/s · p99 0.5 ms<br/>in use 1/80"| db
    api -->|"publish on writes<br/>pool 40<br/>893 calls/s · p99 0.5 ms<br/>in use 1/80"| orders
    orders -->|"consume, 5 at a time<br/>444 jobs/s"| fulfil
    fulfil -->|"write<br/>pool 5, min 5<br/>444 calls/s · p99 37 ms<br/>in use 5/5"| db
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
      plotColorPalette: "#f4cccc, #9e9e9e, #1f77b4"
---
xychart-beta
    title "Throughput"
    x-axis "seconds" 0 --> 300
    y-axis "requests/s" 0 --> 2000
    line [2000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [200, 200, 200, 200, 200, 200, 308.333, 525, 741.667, 958.333, 1175, 1391.667, 1500, 1500, 1500, 1500, 1500, 1500, 850, 200, 200, 200, 200, 200, 200, 200, 200, 200, 200, 200, 200]
    line [196.837, 200.003, 199.922, 200.189, 199.845, 199.986, 304.849, 521.481, 738.159, 954.774, 1171.444, 1388.226, 1499.838, 1499.991, 1500.144, 1499.852, 1499.971, 1499.992, 871.149, 200.115, 200.081, 199.994, 199.967, 199.963, 200.023, 200.009, 199.978, 199.99, 200.004, 200.408, 199.579]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 0:01 disk_slow +1 ms on every write and fsync of db's disk.</sub>

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
    x-axis "seconds" 0 --> 300
    y-axis "ms" 0 --> 25
    line [25, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [1.406, 1.331, 1.337, 1.333, 1.333, 1.334, 1.326, 1.303, 1.262, 1.244, 1.236, 1.23, 1.225, 1.225, 1.229, 1.229, 1.228, 1.229, 1.242, 1.306, 1.303, 1.303, 1.304, 1.305, 1.304, 1.305, 1.303, 1.306, 1.334, 1.332, 1.334]
    line [1.771, 1.491, 1.505, 1.496, 1.495, 1.502, 1.503, 1.504, 1.381, 1.38, 1.379, 1.38, 1.377, 1.378, 1.378, 1.376, 1.378, 1.38, 1.378, 1.455, 1.446, 1.454, 1.438, 1.464, 1.451, 1.444, 1.433, 1.446, 1.503, 1.502, 1.572]
    line [2.41, 1.884, 2.029, 1.917, 2.008, 2.029, 1.975, 22.865, 1.772, 1.845, 1.86, 1.87, 1.843, 1.917, 1.859, 1.785, 1.819, 1.89, 1.799, 1.731, 1.845, 1.997, 1.953, 2.059, 1.953, 1.899, 1.856, 1.776, 1.928, 2.078, 2.188]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 0:01 disk_slow +1 ms on every write and fsync of db's disk.</sub>

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
    x-axis "seconds" 0 --> 300
    y-axis "% of requests" 0 --> 1
    line [1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 0:01 disk_slow +1 ms on every write and fsync of db's disk.</sub>

```mermaid
---
config:
  themeVariables:
    xyChart:
      backgroundColor: "transparent"
      plotColorPalette: "#f4cccc, #1f77b4, #ff7f0e, #2ca02c, #9467bd, #8c564b"
---
xychart-beta
    title "CPU against each limit"
    x-axis "seconds" 0 --> 300
    y-axis "% of limit" 0 --> 100
    line [100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [4.827, 4.778, 4.789, 4.756, 4.734, 4.795, 6.853, 11.39, 15.558, 20.051, 24.645, 29.323, 31.601, 31.611, 31.587, 31.416, 31.438, 31.602, 18.519, 4.82, 4.9, 4.925, 4.993, 4.978, 4.948, 4.95, 4.956, 4.855, 4.814, 4.98, 4.763]
    line [3.414, 3.428, 3.476, 3.27, 3.351, 3.488, 3.687, 5.215, 6.918, 9.149, 9.42, 9.689, 9.77, 9.974, 9.416, 8.9, 9.211, 9.286, 8.996, 7.937, 7.875, 7.872, 7.923, 7.794, 7.929, 7.971, 7.885, 7.411, 3.269, 3.338, 3.329]
    line [6.44, 6.447, 6.446, 6.388, 6.377, 6.417, 8.708, 13.787, 19.208, 21.926, 22.195, 22.711, 22.892, 22.907, 20.925, 19.29, 20.153, 20.612, 22.147, 20.864, 21.089, 21.062, 20.832, 21.041, 20.84, 20.734, 21.168, 19.295, 6.456, 6.815, 6.837]
    line [3.931, 3.107, 3.308, 3.141, 3.234, 3.207, 4.276, 7.188, 5.787, 9.777, 13.442, 15.127, 15.595, 15.098, 15.88, 15.533, 15.784, 15.735, 10.096, 3.243, 3.459, 3.473, 3.594, 3.529, 3.495, 3.433, 3.503, 3.321, 3.114, 3.333, 3.205]
    line [2.442, 2.603, 2.499, 2.598, 2.482, 2.577, 3.107, 4.415, 5.419, 6.039, 6.435, 6.89, 6.941, 7.028, 6.755, 6.509, 6.736, 6.805, 5.913, 4.706, 4.997, 4.843, 4.744, 4.888, 4.986, 4.749, 4.747, 4.51, 2.694, 2.791, 2.869]
```

<sub>Blue: api. Orange: db. Green: fulfil. Purple: lb. Brown: orders (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 0:01 disk_slow +1 ms on every write and fsync of db's disk.</sub>

## Diagnosis

### 1. Consumer lag (orders), from 1:30

From 1:30, jobs went into orders faster than fulfil finished them: at 2:30, 902 a second published against 415 finished. The queue grew to 34,276 jobs, and the oldest had waited 69.1 s. fulfil's writes to db took 9.3 ms against 2.3 ms before the lag, with 3.3 backends of db waiting on locks: its jobs update the same rows, and a row takes one write at a time, each keeping its lock until it commits. More job runners would only wait in line for the same rows.

Evidence:

- 902 jobs a second published to orders at 2:30 <sub>(redis_stream_entries_added_total (redis_exporter), less loadlore_jobs_requeued_total (retries put back))</sub>
- 415 jobs a second finished by fulfil at 2:30 <sub>(loadlore_jobs_total (the workers' finished jobs, by result))</sub>
- 34,276 jobs in orders at 3:00, against 5 job runners across fulfil <sub>(redis_stream_length (redis_exporter): jobs published and not yet finished, waiting or running)</sub>
- the oldest unfinished job had waited 69.1 s <sub>(redis_stream_first_entry_id (redis_exporter): the oldest unfinished job's publish time)</sub>
- p99 wait of the jobs taken at 3:00: 49.67 s <sub>(loadlore_job_wait_seconds (the workers' histogram: publish to start))</sub>
- fulfil CPU at 22% of its limit at 3:00, over 1 replica <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- 9.4 ms per job on fulfil at 3:00, against 2.4 ms before the lag <sub>(loadlore_job_duration_seconds (the workers' histogram: time to run a job))</sub>
- 5 of fulfil's 5 job runners busy on average at 3:00 <sub>(loadlore_job_runner_busy_seconds_total, or loadlore_jobs_running in runs from before it (summed over the worker's replicas))</sub>
- fulfil's writes to db took 9.3 ms at 3:00, against 2.3 ms before the lag <sub>(loadlore_call_duration_seconds by op (the caller's own timings, pool wait included))</sub>
- 3.3 backends of db waiting on locks at 3:00, on average <sub>(pg_lock_waits_waiting: pg_stat_activity backends with wait_event_type Lock (postgres_exporter, Loadlore's query))</sub>

**Try next:** Don't add job runners or replicas to fulfil: the hot rows set its pace. Make the writes to them fewer or cheaper: apply many jobs in one write (a batch), or spread them over more rows. Its workers finished at most about 498 jobs a second, against 903 published. If that pace catches up after the peak, the queue is doing its job: check that the oldest job's wait is one your users accept.

<sub>Rules version 22. Rules are deterministic and offline; their thresholds are listed below.</sub>

## Predictions

1 of 1 scored prediction held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `orders.oldest_job_ms < 120s` | **held** | 69.08 s | the whole run | source: redis_stream_first_entry_id (redis_exporter): the oldest unfinished job's publish time |

## Timeline

| Time | Event |
| --- | --- |
| 0:01 | fault started: disk_slow +1 ms on every write and fsync of db's disk |
| 5:11 | fault ended: disk_slow +1 ms on every write and fsync of db's disk |

<sub>From events.jsonl: the fault timeline and Docker container events.</sub>

## Assumptions

### From the design

| Field | Value | Basis | Why |
| --- | --- | --- | --- |
| `scenario` |  | guessed | a commit on a network-attached volume such as gp3 waits about a millisecond more for the disk than on a laptop |
| `services.orders.resources.memory` |  | guessed | a small managed Redis for background jobs |
| `services.api.behavior.cpu_ms` |  | guessed | a small JSON handler |
| `workload.mix` |  | guessed | during the drop, most requests are purchases |
| `workload.keys` |  | guessed | one shoe in a catalog of a thousand takes nineteen requests in twenty |
| `workload.profile` |  | guessed | buyers arrive over the drop's first minute and leave once it sells out |

6 of these are guessed, not measured: the result depends on them.

### Chosen by Loadlore

| Field | Value | Why |
| --- | --- | --- |
| services.orders.config.maxmemory | 192m | 75% of the container's 256m memory limit, as managed Redis usually does; the rest is for fragmentation and client buffers. Set maxmemory: 0 for stock Redis (no cap) |
| services.orders.config.maxmemory-policy | noeviction | a queue keeps every job: once it is full, publishing fails with an error until workers make room, as Sidekiq and BullMQ ask of their Redis |
| workload.timeout_ms | 10s | the default; k6 gives up on a request after this long and counts it as failed |
| workload.slo.p99_ms | 500 ms | the default; a break on p99 alone needs p99 at or above this as well as 10x the low-load p99, so latency users would still accept isn't called a break |
| network.profile | same-zone: 0.3 ms round trip | the default: every service in one availability zone. AWS says a round trip between two instances in the same zone is generally sub-millisecond with enhanced networking, and publishes no exact figure, so the 0.3 ms round trip Loadlore adds between any two containers is its own estimate, inside that bound. Applied between any two of the stack's containers, the load generator and the entry point included, as half the round trip on each side's outgoing packets; Loadlore's own tooling isn't slowed. Not scaled with --scale. Source: AWS Architecture Blog, Improving performance and reducing cost using Availability Zone Affinity: https://aws.amazon.com/blogs/architecture/improving-performance-and-reducing-cost-using-availability-zone-affinity/ |
| resources.cpu_period_ms | 100 ms for api, db, fulfil, lb, orders | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.db.disk | laptop: the Docker host's | no disk block: a read that misses db's memory goes to the Docker host's disk, which on a laptop mostly answers from the host's own memory, fast and uncapped, and a commit's fsync returns in a fraction of a millisecond. Add disk: { profile: gp3 } for a cloud volume's waits and caps |
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
- **Host disk stalls:** for 2 seconds or more, a Postgres on the Docker host's own disk read no blocks and committed at most 10% of its median over the 10 s before (50 a second or more), while 5 or more queries waited on disk reads or WAL flushes and its exporter answered. Near the break it makes the run inconclusive; anywhere else it's a caveat, and its windows are flagged and left out of exports.

## Environment

- loadl dev, profile 5m10s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 1007 (hash 989, memory 1042, syscalls 991): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (1007) was within 5.0% of the run it's compared with (988)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 185,495 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-flash-sale-queue-the-orders-20261118-005055-api-0-1 | loadlore/synthetic:dev |
| loadlore-flash-sale-queue-the-orders-20261118-005055-api-1-1 | loadlore/synthetic:dev |
| loadlore-flash-sale-queue-the-orders-20261118-005055-db-1 | postgres:17.11 |
| loadlore-flash-sale-queue-the-orders-20261118-005055-fulfil-0-1 | loadlore/synthetic:dev |
| loadlore-flash-sale-queue-the-orders-20261118-005055-lb-1 | haproxy:3.2.26 |
| loadlore-flash-sale-queue-the-orders-20261118-005055-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-flash-sale-queue-the-orders-20261118-005055-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-flash-sale-queue-the-orders-20261118-005055-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-flash-sale-queue-the-orders-20261118-005055-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-flash-sale-queue-the-orders-20261118-005055-lore-redisexp-orders-1 | oliver006/redis_exporter:v1.97.0 |
| loadlore-flash-sale-queue-the-orders-20261118-005055-orders-1 | redis:8.2 |

</details>

## Per-window data

<details><summary>31 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | fulfil CPU | lb CPU | lb up | orders CPU | orders jobs | oldest | published/s | finished/s | api→db pool | wait | api→orders pool | wait | fulfil→db pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 200 | 197 | 0 | 0% | 1.4 ms | 2.4 ms | 4.8% | 3.4% | 86 | 6.4% | 3.9% | 2 | 2.4% | 0 | 0.0 ms | 117 | 117 | 0/80 | 0.0 ms | 0/80 | – | 1/5 | 0.0 ms | 4.3% | 1.0% |
| 0:10 | 200 | 200 | 0 | 0% | 1.3 ms | 1.9 ms | 4.8% | 3.4% | 86 | 6.4% | 3.1% | 2 | 2.6% | 0 | 0.0 ms | 122 | 122 | 0/80 | 0.0 ms | 0/80 | – | 1/5 | 0.0 ms | 2.4% | 0% |
| 0:20 | 200 | 200 | 0 | 0% | 1.3 ms | 2.0 ms | 4.8% | 3.5% | 86 | 6.4% | 3.3% | 2 | 2.5% | 0 | 0.0 ms | 119 | 118 | 0/80 | 0.0 ms | 0/80 | – | 1/5 | 0.0 ms | 2.6% | 0% |
| 0:30 | 200 | 200 | 0 | 0% | 1.3 ms | 1.9 ms | 4.8% | 3.3% | 86 | 6.4% | 3.1% | 2 | 2.6% | 0 | 0.0 ms | 119 | 119 | 0/80 | 0.0 ms | 0/80 | – | 1/5 | 0.0 ms | 2.3% | 0% |
| 0:40 | 200 | 200 | 0 | 0% | 1.3 ms | 2.0 ms | 4.7% | 3.4% | 86 | 6.4% | 3.2% | 2 | 2.5% | 0 | 0.0 ms | 120 | 120 | 0/80 | 0.0 ms | 0/80 | – | 1/5 | 0.0 ms | 2.4% | 0% |
| 0:50 | 200 | 200 | 0 | 0% | 1.3 ms | 2.0 ms | 4.8% | 3.5% | 86 | 6.4% | 3.2% | 2 | 2.6% | 0 | 0.0 ms | 117 | 118 | 0/80 | 0.0 ms | 0/80 | – | 1/5 | 0.0 ms | 2.6% | 0% |
| 1:00 | 308 | 305 | 0 | 0% | 1.3 ms | 2.0 ms | 6.9% | 3.7% | 86 | 8.7% | 4.3% | 2 | 3.1% | 1 | 0.0 ms | 181 | 181 | 0/80 | 0.0 ms | 0/80 | – | 1/5 | 0.0 ms | 3.3% | 0% |
| 1:10 | 525 | 521 | 0 | 0% | 1.3 ms | 23 ms | 11% | 5.2% | 86 | 14% | 7.2% | 2 | 4.4% | 0 | 0.0 ms | 306 | 305 | 1/80 | 0.0 ms | 1/80 | – | 2/5 | 0.0 ms | 5.0% | 0% |
| 1:20 | 742 | 738 | 0 | 0% | 1.3 ms | 1.8 ms | 16% | 6.9% | 86 | 19% | 5.8% | 2 | 5.4% | 4 | 33 ms | 442 | 440 | 0/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 6.1% | 0% |
| 1:30 | 958 | 955 | 0 | 0% | 1.2 ms | 1.8 ms | 20% | 9.1% | 86 | 22% | 9.8% | 2 | 6.0% | 593 | 1.16 s | 571 | 492 | 0/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 7.6% | 0% |
| 1:40 | 1,175 | 1,171 | 0 | 0% | 1.2 ms | 1.9 ms | 25% | 9.4% | 86 | 22% | 13% | 2 | 6.4% | 2,490 | 3.70 s | 704 | 492 | 1/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 8.5% | 0% |
| 1:50 | 1,392 | 1,388 | 0 | 0% | 1.2 ms | 1.9 ms | 29% | 9.7% | 86 | 23% | 15% | 2 | 6.9% | 5,621 | 7.15 s | 833 | 494 | 2/80 | 0.0 ms | 2/80 | – | 5/5 | 0.0 ms | 9.2% | 0% |
| 2:00 | 1,500 | 1,500 | 0 | 0% | 1.2 ms | 1.8 ms | 32% | 9.8% | 86 | 23% | 16% | 2 | 6.9% | 9,705 | 11.24 s | 900 | 491 | 2/80 | 0.0 ms | 2/80 | – | 5/5 | 0.0 ms | 9.6% | 0% |
| 2:10 | 1,500 | 1,500 | 0 | 0% | 1.2 ms | 1.9 ms | 32% | 10.0% | 86 | 23% | 15% | 2 | 7.0% | 13,761 | 15.75 s | 903 | 495 | 1/80 | 0.0 ms | 2/80 | – | 5/5 | 0.0 ms | 9.5% | 0% |
| 2:20 | 1,500 | 1,500 | 0 | 0% | 1.2 ms | 1.9 ms | 32% | 9.4% | 86 | 21% | 16% | 2 | 6.8% | 18,164 | 20.73 s | 893 | 444 | 1/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 9.8% | 0% |
| 2:30 | 1,500 | 1,500 | 0 | 0% | 1.2 ms | 1.8 ms | 31% | 8.9% | 86 | 19% | 16% | 2 | 6.5% | 23,051 | 26.15 s | 902 | 415 | 1/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 9.6% | 0% |
| 2:40 | 1,500 | 1,500 | 0 | 0% | 1.2 ms | 1.8 ms | 31% | 9.2% | 86 | 20% | 16% | 2 | 6.7% | 27,800 | 31.33 s | 900 | 431 | 1/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 9.7% | 0% |
| 2:50 | 1,500 | 1,500 | 0 | 0% | 1.2 ms | 1.9 ms | 32% | 9.3% | 86 | 21% | 16% | 2 | 6.8% | 32,478 | 36.62 s | 892 | 433 | 1/80 | 0.0 ms | 2/80 | – | 5/5 | 0.0 ms | 9.9% | 0% |
| 3:00 | 850 | 871 | 0 | 0% | 1.2 ms | 1.8 ms | 19% | 9.0% | 86 | 22% | 10% | 2 | 5.9% | 33,932 | 41.04 s | 523 | 497 | 2/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 6.9% | 0% |
| 3:10 | 200 | 200 | 0 | 0% | 1.3 ms | 1.7 ms | 4.8% | 7.9% | 86 | 21% | 3.2% | 2 | 4.7% | 30,257 | 45.55 s | 118 | 497 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.3% | 0% |
| 3:20 | 200 | 200 | 0 | 0% | 1.3 ms | 1.8 ms | 4.9% | 7.9% | 86 | 21% | 3.5% | 2 | 5.0% | 26,490 | 50.03 s | 121 | 497 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.1% | 0% |
| 3:30 | 200 | 200 | 0 | 0% | 1.3 ms | 2.0 ms | 4.9% | 7.9% | 86 | 21% | 3.5% | 2 | 4.8% | 22,769 | 54.46 s | 123 | 497 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.5% | 0% |
| 3:40 | 200 | 200 | 0 | 0% | 1.3 ms | 2.0 ms | 5.0% | 7.9% | 86 | 21% | 3.6% | 2 | 4.7% | 18,986 | 59.03 s | 119 | 493 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.1% | 0% |
| 3:50 | 200 | 200 | 0 | 0% | 1.3 ms | 2.1 ms | 5.0% | 7.8% | 86 | 21% | 3.5% | 2 | 4.9% | 15,292 | 63.49 s | 123 | 497 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.5% | 0% |
| 4:00 | 200 | 200 | 0 | 0% | 1.3 ms | 2.0 ms | 4.9% | 7.9% | 86 | 21% | 3.5% | 2 | 5.0% | 11,554 | 67.82 s | 124 | 497 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.1% | 0% |
| 4:10 | 200 | 200 | 0 | 0% | 1.3 ms | 1.9 ms | 4.9% | 8.0% | 86 | 21% | 3.4% | 2 | 4.7% | 7,841 | 69.08 s | 122 | 495 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.4% | 0% |
| 4:20 | 200 | 200 | 0 | 0% | 1.3 ms | 1.9 ms | 5.0% | 7.9% | 86 | 21% | 3.5% | 2 | 4.7% | 4,031 | 57.93 s | 116 | 498 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.1% | 0% |
| 4:30 | 200 | 200 | 0 | 0% | 1.3 ms | 1.8 ms | 4.9% | 7.4% | 86 | 19% | 3.3% | 2 | 4.5% | 236 | 27.18 s | 119 | 456 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.4% | 0% |
| 4:40 | 200 | 200 | 0 | 0% | 1.3 ms | 1.9 ms | 4.8% | 3.3% | 86 | 6.5% | 3.1% | 2 | 2.7% | 0 | 0.0 ms | 119 | 120 | 0/80 | 0.0 ms | 0/80 | – | 1/5 | 0.0 ms | 2.3% | 0% |
| 4:50 | 200 | 200 | 0 | 0% | 1.3 ms | 2.1 ms | 5.0% | 3.3% | 86 | 6.8% | 3.3% | 2 | 2.8% | 0 | 0.0 ms | 123 | 123 | 0/80 | 0.0 ms | 0/80 | – | 1/5 | 0.0 ms | 2.2% | 0% |
| 5:00 | 200 | 200 | 0 | 0% | 1.3 ms | 2.2 ms | 4.8% | 3.3% | 86 | 6.8% | 3.2% | 2 | 2.9% | 0 | 0.0 ms | 118 | 120 | 0/80 | 0.0 ms | 0/80 | – | 1/5 | 0.0 ms | 2.1% | 0% |

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

# The database's disk is a network-attached cloud volume: each commit waits
# about a millisecond longer for its WAL to be written, modeled as a slow
# disk for the whole run.
scenario:
  - { at: 0s, inject: disk_slow, target: db, add_ms: 1 }

assumptions:
  - { field: scenario, basis: guessed, why: "a commit on a network-attached volume such as gp3 waits about a millisecond more for the disk than on a laptop" }
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
