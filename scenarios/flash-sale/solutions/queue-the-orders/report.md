# flash-sale-queue-the-orders: run report

_run `flash-sale-queue-the-orders-20261116-155727` · completed · 16 Nov 2026 15:57 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 1,500 rps offered, 1,500 rps served at best, worst 10-second p99 1.8 ms, at most 0% errors. But jobs backed up in orders: the oldest waited 62.8 s (see consumer lag).

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 2:30

Measured in the 10-second window from 2:30 to 2:40 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 1,500 rps<br/>offered 1,500 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 19%<br/>2/2 replicas up"}}
    api["api ×2<br/>1 CPU · 512m<br/>CPU 33%<br/>in flight 1"]
    db[("db<br/>postgres 17<br/>1 CPU · 1g<br/>CPU 12%<br/>conns 86/100")]
    orders[("orders<br/>queue · Redis Streams<br/>0.5 CPU · 256m<br/>CPU 10%<br/>21,828 jobs, oldest 24.68 s")]
    fulfil["fulfil<br/>worker<br/>0.5 CPU · 256m<br/>CPU 25%<br/>499 jobs/s, 5/5 busy"]
    load -->|"40% read · 60% write<br/>1,500 rps · p99 1.5 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>1,500 rps · p99 1.0 ms"| api
    api -->|"read on reads<br/>pool 40, min 40<br/>598 calls/s · p99 0.5 ms<br/>in use 1/80"| db
    api -->|"publish on writes<br/>pool 40<br/>902 calls/s · p99 0.5 ms<br/>in use 1/80"| orders
    orders -->|"consume, 5 at a time<br/>499 jobs/s"| fulfil
    fulfil -->|"write<br/>pool 5, min 5<br/>499 calls/s · p99 28 ms<br/>in use 5/5"| db
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb,api,db,orders ok
    class fulfil hot
```

<sub>Sources: offered from the load profile in plan.json (offered arrival rate); load edge from k6_http_reqs_total (k6: all responses) and k6_http_req_duration_seconds (k6 histogram, end to end); CPU from container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit; calls/s from loadlore_calls_total (the caller's outbound call attempts) and their p99 from loadlore_call_duration_seconds (the caller's histogram, pool wait included); pools from loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections and loadlore_pool_pending (callers waiting for a connection); lb traffic from haproxy_backend_http_requests_total (HAProxy), replicas up from haproxy_backend_active_servers (HAProxy), and the lb edge's p99 from loadlore_request_duration_seconds (the service's own histogram); conns from pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); a queue's jobs from redis_stream_length (redis_exporter): jobs published and not yet finished, waiting or running and its oldest from redis_stream_first_entry_id (redis_exporter): the oldest unfinished job's publish time; a worker's jobs/s from loadlore_jobs_total (the workers' finished jobs, by result) and busy runners from loadlore_jobs_running (summed over the worker's replicas).</sub>

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
    line [197.06, 199.901, 200.022, 200.071, 199.892, 199.999, 305.045, 521.7, 738.416, 954.857, 1171.717, 1388.459, 1499.849, 1499.96, 1499.948, 1500.093, 1499.951, 1499.927, 869.921, 200.277, 199.976, 199.94, 199.982, 200.011, 199.966, 200.237, 199.796, 199.97, 199.997, 200.493, 199.649]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 0:01 jolly_heyrovsky die (exit code 0); 0:01 disk_slow +1 ms on every write and fsync of db's disk.</sub>

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
    y-axis "ms" 0 --> 2
    line [2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [0.67, 0.613, 0.616, 0.612, 0.609, 0.606, 0.579, 0.573, 0.581, 0.588, 0.616, 0.601, 0.591, 0.596, 0.594, 0.595, 0.602, 0.603, 0.6, 0.601, 0.604, 0.602, 0.597, 0.603, 0.598, 0.599, 0.601, 0.611, 0.614, 0.61, 0.606]
    line [1.02, 0.807, 0.805, 0.799, 0.781, 0.804, 0.751, 0.768, 0.808, 0.83, 0.863, 0.864, 0.88, 0.88, 0.881, 0.885, 0.931, 0.913, 0.84, 0.84, 0.84, 0.843, 0.832, 0.869, 0.832, 0.854, 0.824, 0.777, 0.818, 0.79, 0.781]
    line [1.471, 1.088, 1.094, 1.196, 1.239, 1.172, 1.156, 1.281, 1.237, 1.46, 1.503, 1.4, 1.522, 1.494, 1.828, 1.474, 1.743, 1.463, 1.287, 1.266, 1.22, 1.312, 1.176, 1.338, 1.24, 1.335, 1.358, 1.058, 1.091, 1.029, 1.08]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 0:01 jolly_heyrovsky die (exit code 0); 0:01 disk_slow +1 ms on every write and fsync of db's disk.</sub>

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

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 0:01 jolly_heyrovsky die (exit code 0); 0:01 disk_slow +1 ms on every write and fsync of db's disk.</sub>

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
    line [5.737, 5.094, 5.199, 5.128, 5.174, 5.075, 7.153, 11.723, 16.465, 21.246, 26.245, 30.486, 32.676, 32.802, 32.987, 32.873, 33.395, 33.294, 19.499, 5.195, 5.088, 5.096, 5.085, 5.093, 5.055, 5.102, 5.125, 5.145, 5.111, 5.121, 5.003]
    line [6.975, 3.916, 3.767, 3.697, 3.709, 3.858, 4.146, 5.331, 7.764, 10.605, 11.03, 11.365, 11.869, 11.793, 12.322, 12.1, 12.482, 12.461, 10.584, 9.028, 8.97, 8.946, 8.938, 8.964, 8.979, 8.914, 8.855, 4.089, 3.653, 3.812, 3.698]
    line [8.24, 7.268, 7.229, 7.206, 7.276, 7.027, 9.542, 14.812, 21.108, 23.983, 24.287, 24.485, 25.046, 25.159, 25.557, 25.322, 26.228, 26.047, 23.957, 22.281, 22.174, 22.145, 22.191, 22.157, 22.096, 22.243, 22.123, 8.02, 7.243, 7.248, 6.982]
    line [6.562, 4.562, 4.528, 4.496, 4.492, 4.378, 5.746, 8.543, 10.514, 12.245, 17.428, 20.999, 21.725, 22.803, 20.544, 18.876, 18.549, 17.559, 11.968, 4.336, 4.45, 4.376, 4.236, 4.359, 4.199, 4.427, 4.297, 4.387, 4.38, 4.236, 4.347]
    line [5.348, 3.268, 3.181, 3.302, 3.298, 3.175, 3.716, 5.169, 7.006, 8.094, 8.816, 9.317, 9.698, 9.916, 10.034, 10.114, 10.779, 10.366, 8.081, 6.01, 6.028, 5.99, 6.01, 6.029, 5.997, 6.064, 5.897, 3.362, 3.183, 3.205, 3.107]
```

<sub>Blue: api. Orange: db. Green: fulfil. Purple: lb. Brown: orders (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 0:01 jolly_heyrovsky die (exit code 0); 0:01 disk_slow +1 ms on every write and fsync of db's disk.</sub>

## Diagnosis

### 1. Consumer lag (orders), from 1:30

From 1:30, jobs went into orders faster than fulfil finished them: at 2:10, 904 a second published against 497 finished. The queue grew to 31,355 jobs, and the oldest had waited 62.8 s. fulfil's writes to db took 9.7 ms against 2.0 ms before the lag, with 3.4 backends of db waiting on locks: its jobs update the same rows, and a row takes one write at a time, each keeping its lock until it commits. More job runners would only wait in line for the same rows.

Evidence:

- 904 jobs a second published to orders at 2:10 <sub>(redis_stream_entries_added_total (redis_exporter))</sub>
- 497 jobs a second finished by fulfil at 2:10 <sub>(loadlore_jobs_total (the workers' finished jobs, by result))</sub>
- 31,355 jobs in orders at 3:00, against 5 job runners across fulfil <sub>(redis_stream_length (redis_exporter): jobs published and not yet finished, waiting or running)</sub>
- the oldest unfinished job had waited 62.8 s <sub>(redis_stream_first_entry_id (redis_exporter): the oldest unfinished job's publish time)</sub>
- p99 wait of the jobs taken at 3:00: 39.93 s <sub>(loadlore_job_wait_seconds (the workers' histogram: publish to start))</sub>
- fulfil CPU at 24% of its limit at 3:00, over 1 replica <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- 9.8 ms per job on fulfil at 3:00, against 2.1 ms before the lag <sub>(loadlore_job_duration_seconds (the workers' histogram: time to run a job))</sub>
- 5 of fulfil's 5 job runners busy on average at 3:00 <sub>(loadlore_jobs_running (summed over the worker's replicas))</sub>
- fulfil's writes to db took 9.7 ms at 3:00, against 2.0 ms before the lag <sub>(loadlore_call_duration_seconds by op (the caller's own timings, pool wait included))</sub>
- 3.4 backends of db waiting on locks at 3:00, on average <sub>(pg_lock_waits_waiting: pg_stat_activity backends with wait_event_type Lock (postgres_exporter, Loadlore's query))</sub>

**Try next:** Don't add job runners or replicas to fulfil: the hot rows set its pace. Make the writes to them fewer or cheaper: apply many jobs in one write (a batch), or spread them over more rows. Its workers finished at most about 500 jobs a second, against 904 published. If that pace catches up after the peak, the queue is doing its job: check that the oldest job's wait is one your users accept.

<sub>Rules version 14. Rules are deterministic and offline; their thresholds are listed below.</sub>

## Predictions

1 of 1 scored prediction held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `orders.oldest_job_ms < 120s` | **held** | 62.83 s | the whole run | source: redis_stream_first_entry_id (redis_exporter): the oldest unfinished job's publish time |

## Timeline

| Time | Event |
| --- | --- |
| 0:01 | container: jolly_heyrovsky die (exit code 0) |
| 0:01 | fault started: disk_slow +1 ms on every write and fsync of db's disk |
| 5:10 | container: amazing_ride start |
| 5:11 | container: amazing_ride die (exit code 0) |
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
| resources.cpu_period_ms | 100 ms for api, db, fulfil, lb, orders | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.db.disk | the Docker host's | no disk block: a read that misses db's memory goes to the Docker host's disk, which on a laptop mostly answers from the host's own memory, fast and uncapped. Add disk: {} for a cloud volume's wait and IOPS cap |
| services.fulfil.consume.concurrency | 5 | Sidekiq's default: each replica runs up to 5 jobs from orders at once, and the rest wait in the queue |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |
| services.api.pools.db.min | 40 | set in the design: each replica opens 40 connections before the load starts and keeps them open |
| services.api.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |
| services.fulfil.pools.db.min | 5 | set in the design: each replica opens 5 connections before the load starts and keeps them open |
| services.fulfil.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- Loadlore's own containers get fixed budgets: load generator 1.5 CPU (peak 1500 rps), Toxiproxy 1 CPU (about 3000 proxied calls/s at peak), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each

## Thresholds

- **Broke:** 2 windows in a row with at least 1.0% of requests failing or successful responses below 90% of the offered rate, or 3 windows in a row with any of those or p99 at least 10x the low-load p99 (the median p99 of windows offered at most 25% of the peak, before errors or a shortfall in 2 windows in a row) and at least the SLO, 500 ms (workload.slo.p99_ms): 500 ms in this run. A shorter episode is shown as a stall in the timeline, not a break.
- **Doubtful:** a break on p99 alone, when the load generator was stalled at its CPU limit in 1.0% or more of its scheduling periods in one of the break's windows and the entry service's own p99 (loadlore_request_duration_seconds (the service's own histogram)) was below the limit in one of them. The verdict stays what it was.
- **Inconclusive:** before the break (or anywhere, if nothing broke), the load generator dropped more than 1.0% of a window's arrivals (more than 5.0% in a window where the system itself failed 1.0% or more of requests; up to that it's a caveat that gives the share), or the load generator or Toxiproxy used 90% or more of its CPU budget, or was stalled at its CPU limit in 5.0% or more of its scheduling periods (from 1.0% it's noted as a caveat). The first 10 s of load are a warm-up and don't count: the load generator starts its VUs and connections open then. A cause is named only with evidence: on a Linux host, the hypervisor giving 2.0% or more of CPU time to other virtual machines (steal), or other programs using 10% or more of the host's CPUs (at least 0.5); inside a VM on this machine, only as a possibility.
- **Toxiproxy stalls:** no new connection accepted on one of Toxiproxy's edges for 1 s or more, where it had accepted 10 a second or more over the 5 s before, then 10 or more accepted within 100 ms with a median spacing under 0.5 ms: connections queued behind one upstream dial, which a real network doesn't do. Up to the end of the break's first window it makes the run inconclusive; after a break the system didn't recover from, it's a doubt on the recovery, and `loadl scenario check` doesn't judge claims that rest on not recovering; in a run that held, or recovered, it's a caveat: a stall only adds failure.
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

## Environment

- loadl dev, profile 5m10s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 990 (hash 1006, memory 975, syscalls 990): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (990) was within 5.0% of the run it's compared with (1002)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 185,499 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-flash-sale-queue-the-orders-20261116-155727-api-0-1 | loadlore/synthetic:dev |
| loadlore-flash-sale-queue-the-orders-20261116-155727-api-1-1 | loadlore/synthetic:dev |
| loadlore-flash-sale-queue-the-orders-20261116-155727-db-1 | postgres:17.11 |
| loadlore-flash-sale-queue-the-orders-20261116-155727-fulfil-0-1 | loadlore/synthetic:dev |
| loadlore-flash-sale-queue-the-orders-20261116-155727-lb-1 | haproxy:3.2.26 |
| loadlore-flash-sale-queue-the-orders-20261116-155727-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-flash-sale-queue-the-orders-20261116-155727-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-flash-sale-queue-the-orders-20261116-155727-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-flash-sale-queue-the-orders-20261116-155727-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-flash-sale-queue-the-orders-20261116-155727-lore-redisexp-orders-1 | oliver006/redis_exporter:v1.97.0 |
| loadlore-flash-sale-queue-the-orders-20261116-155727-lore-toxiproxy-1 | ghcr.io/shopify/toxiproxy:2.13.0 |
| loadlore-flash-sale-queue-the-orders-20261116-155727-orders-1 | redis:8.2 |

</details>

## Per-window data

<details><summary>31 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | fulfil CPU | lb CPU | lb up | orders CPU | orders jobs | oldest | published/s | finished/s | api→db pool | wait | api→orders pool | wait | fulfil→db pool | wait | k6 CPU | Toxiproxy CPU | k6 / Toxiproxy stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 200 | 197 | 0 | 0% | 0.7 ms | 1.5 ms | 5.7% | 7.0% | 86 | 8.2% | 6.6% | 2 | 5.3% | 0 | 0.0 ms | 117 | 117 | 0/80 | 0.0 ms | 1/80 | – | 0/5 | 0.0 ms | 5.2% | 6.6% | 0% / 0% |
| 0:10 | 200 | 200 | 0 | 0% | 0.6 ms | 1.1 ms | 5.1% | 3.9% | 86 | 7.3% | 4.6% | 2 | 3.3% | 0 | 0.0 ms | 122 | 122 | 1/80 | 0.0 ms | 1/80 | – | 0/5 | 0.0 ms | 2.7% | 5.2% | 0% / 0% |
| 0:20 | 200 | 200 | 0 | 0% | 0.6 ms | 1.1 ms | 5.2% | 3.8% | 86 | 7.2% | 4.5% | 2 | 3.2% | 1 | 0.0 ms | 118 | 119 | 0/80 | 0.0 ms | 1/80 | – | 0/5 | 0.0 ms | 3.1% | 5.2% | 0% / 0% |
| 0:30 | 200 | 200 | 0 | 0% | 0.6 ms | 1.2 ms | 5.1% | 3.7% | 86 | 7.2% | 4.5% | 2 | 3.3% | 0 | 0.0 ms | 120 | 119 | 0/80 | 0.0 ms | 1/80 | – | 1/5 | 0.0 ms | 3.0% | 5.1% | 0% / 0% |
| 0:40 | 200 | 200 | 0 | 0% | 0.6 ms | 1.2 ms | 5.2% | 3.7% | 86 | 7.3% | 4.5% | 2 | 3.3% | 0 | 0.0 ms | 120 | 121 | 0/80 | 0.0 ms | 0/80 | – | 0/5 | 0.0 ms | 2.7% | 5.1% | 0% / 0% |
| 0:50 | 200 | 200 | 0 | 0% | 0.6 ms | 1.2 ms | 5.1% | 3.9% | 86 | 7.0% | 4.4% | 2 | 3.2% | 1 | 0.0 ms | 118 | 117 | 1/80 | 0.0 ms | 0/80 | – | 0/5 | 0.0 ms | 3.0% | 5.0% | 0% / 0% |
| 1:00 | 308 | 305 | 0 | 0% | 0.6 ms | 1.2 ms | 7.2% | 4.1% | 86 | 9.5% | 5.7% | 2 | 3.7% | 1 | 0.0 ms | 181 | 181 | 0/80 | 0.0 ms | 1/80 | – | 2/5 | 0.0 ms | 3.8% | 7.0% | 0% / 0% |
| 1:10 | 525 | 522 | 0 | 0% | 0.6 ms | 1.3 ms | 12% | 5.3% | 86 | 15% | 8.5% | 2 | 5.2% | 2 | 3.0 ms | 305 | 306 | 0/80 | 0.0 ms | 0/80 | – | 3/5 | 0.0 ms | 5.7% | 12% | 0% / 0% |
| 1:20 | 742 | 738 | 0 | 0% | 0.6 ms | 1.2 ms | 16% | 7.8% | 86 | 21% | 11% | 2 | 7.0% | 13 | 30 ms | 443 | 440 | 0/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 7.7% | 17% | 0% / 0% |
| 1:30 | 958 | 955 | 0 | 0% | 0.6 ms | 1.5 ms | 21% | 11% | 86 | 24% | 12% | 2 | 8.1% | 604 | 1.16 s | 569 | 498 | 2/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 9.1% | 21% | 0% / 0% |
| 1:40 | 1,175 | 1,172 | 0 | 0% | 0.6 ms | 1.5 ms | 26% | 11% | 86 | 24% | 17% | 2 | 8.8% | 2,532 | 3.74 s | 706 | 497 | 2/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 10% | 23% | 0% / 0% |
| 1:50 | 1,392 | 1,388 | 0 | 0% | 0.6 ms | 1.4 ms | 30% | 11% | 86 | 24% | 21% | 2 | 9.3% | 5,703 | 7.19 s | 834 | 498 | 1/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 12% | 26% | 0% / 0% |
| 2:00 | 1,500 | 1,500 | 0 | 0% | 0.6 ms | 1.5 ms | 33% | 12% | 86 | 25% | 22% | 2 | 9.7% | 9,806 | 11.26 s | 900 | 497 | 1/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 13% | 28% | 0% / 0% |
| 2:10 | 1,500 | 1,500 | 0 | 0% | 0.6 ms | 1.5 ms | 33% | 12% | 86 | 25% | 23% | 2 | 9.9% | 13,800 | 15.77 s | 904 | 497 | 1/80 | 0.0 ms | 2/80 | – | 5/5 | 0.0 ms | 13% | 28% | 0% / 0% |
| 2:20 | 1,500 | 1,500 | 0 | 0% | 0.6 ms | 1.8 ms | 33% | 12% | 86 | 26% | 21% | 2 | 10% | 17,769 | 20.26 s | 892 | 496 | 2/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 13% | 28% | 0% / 0% |
| 2:30 | 1,500 | 1,500 | 0 | 0% | 0.6 ms | 1.5 ms | 33% | 12% | 86 | 25% | 19% | 2 | 10% | 21,828 | 24.68 s | 903 | 499 | 1/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 13% | 29% | 0% / 0% |
| 2:40 | 1,500 | 1,500 | 0 | 0% | 0.6 ms | 1.7 ms | 33% | 12% | 86 | 26% | 19% | 2 | 11% | 25,858 | 29.23 s | 902 | 497 | 1/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 14% | 30% | 0% / 0% |
| 2:50 | 1,500 | 1,500 | 0 | 0% | 0.6 ms | 1.5 ms | 33% | 12% | 86 | 26% | 18% | 2 | 10% | 29,811 | 33.63 s | 891 | 497 | 1/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 13% | 30% | 0% / 0% |
| 3:00 | 850 | 870 | 0 | 0% | 0.6 ms | 1.3 ms | 19% | 11% | 86 | 24% | 12% | 2 | 8.1% | 30,875 | 38.11 s | 523 | 498 | 0/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 8.2% | 20% | 0% / 0% |
| 3:10 | 200 | 200 | 0 | 0% | 0.6 ms | 1.3 ms | 5.2% | 9.0% | 86 | 22% | 4.3% | 2 | 6.0% | 27,116 | 42.58 s | 117 | 499 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.9% | 11% | 0% / 0% |
| 3:20 | 200 | 200 | 0 | 0% | 0.6 ms | 1.2 ms | 5.1% | 9.0% | 86 | 22% | 4.4% | 2 | 6.0% | 23,345 | 47.02 s | 122 | 498 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.4% | 11% | 0% / 0% |
| 3:30 | 200 | 200 | 0 | 0% | 0.6 ms | 1.3 ms | 5.1% | 8.9% | 86 | 22% | 4.4% | 2 | 6.0% | 19,594 | 51.50 s | 122 | 499 | 1/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.8% | 11% | 0% / 0% |
| 3:40 | 200 | 200 | 0 | 0% | 0.6 ms | 1.2 ms | 5.1% | 8.9% | 86 | 22% | 4.2% | 2 | 6.0% | 15,783 | 55.90 s | 120 | 500 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.4% | 11% | 0% / 0% |
| 3:50 | 200 | 200 | 0 | 0% | 0.6 ms | 1.3 ms | 5.1% | 9.0% | 86 | 22% | 4.4% | 2 | 6.0% | 12,021 | 60.30 s | 123 | 499 | 0/80 | 0.0 ms | 1/80 | – | 5/5 | 0.0 ms | 2.8% | 11% | 0% / 0% |
| 4:00 | 200 | 200 | 0 | 0% | 0.6 ms | 1.2 ms | 5.1% | 9.0% | 86 | 22% | 4.2% | 2 | 6.0% | 8,261 | 62.83 s | 124 | 500 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.4% | 11% | 0% / 0% |
| 4:10 | 200 | 200 | 0 | 0% | 0.6 ms | 1.3 ms | 5.1% | 8.9% | 86 | 22% | 4.4% | 2 | 6.1% | 4,494 | 61.67 s | 123 | 499 | 1/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.8% | 11% | 0% / 0% |
| 4:20 | 200 | 200 | 0 | 0% | 0.6 ms | 1.4 ms | 5.1% | 8.9% | 86 | 22% | 4.3% | 2 | 5.9% | 675 | 30.37 s | 116 | 500 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.4% | 11% | 0% / 0% |
| 4:30 | 200 | 200 | 0 | 0% | 0.6 ms | 1.1 ms | 5.1% | 4.1% | 86 | 8.0% | 4.4% | 2 | 3.4% | 1 | 1.0 ms | 119 | 143 | 0/80 | 0.0 ms | 0/80 | – | 5/5 | 0.0 ms | 2.6% | 5.5% | 0% / 0% |
| 4:40 | 200 | 200 | 0 | 0% | 0.6 ms | 1.1 ms | 5.1% | 3.7% | 86 | 7.2% | 4.4% | 2 | 3.2% | 0 | 0.0 ms | 120 | 120 | 0/80 | 0.0 ms | 1/80 | – | 1/5 | 0.0 ms | 3.1% | 5.2% | 0% / 0% |
| 4:50 | 200 | 200 | 0 | 0% | 0.6 ms | 1.0 ms | 5.1% | 3.8% | 86 | 7.2% | 4.2% | 2 | 3.2% | 0 | 0.0 ms | 123 | 123 | 0/80 | 0.0 ms | 1/80 | – | 0/5 | 0.0 ms | 2.6% | 5.2% | 0% / 0% |
| 5:00 | 200 | 200 | 0 | 0% | 0.6 ms | 1.1 ms | 5.0% | 3.7% | 86 | 7.0% | 4.3% | 2 | 3.1% | 0 | 0.0 ms | 119 | 118 | 0/80 | 0.0 ms | 1/80 | – | 0/5 | 0.0 ms | 2.8% | 5.0% | 0% / 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 and Toxiproxy CPU are against their budgets. A queue's jobs are redis_stream_length (redis_exporter): jobs published and not yet finished, waiting or running, at the window's end; oldest is redis_stream_first_entry_id (redis_exporter): the oldest unfinished job's publish time, at most over the window; published is redis_stream_entries_added_total (redis_exporter) and finished loadlore_jobs_total (the workers' finished jobs, by result).

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
