# backed-up-queue-more-runners: run report

_run `backed-up-queue-more-runners-20261116-132248` · completed · 16 Nov 2026 13:22 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 600 rps offered, 600 rps served at best, worst 10-second p99 2.8 ms, at most 0% errors.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 4:00

Measured in the 10-second window from 4:00 to 4:10 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 600 rps<br/>offered 600 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 9.6%<br/>2/2 replicas up"}}
    api["api ×2<br/>0.5 CPU · 256m<br/>CPU 40%<br/>in flight 1"]
    db[("db<br/>postgres 17<br/>1 CPU · 1g<br/>CPU 4.7%<br/>conns 5/100")]
    emails[("emails<br/>queue · Redis Streams<br/>0.5 CPU · 64m<br/>CPU 4.8%<br/>29 jobs")]
    sender["sender ×2<br/>worker<br/>0.5 CPU · 128m<br/>CPU 22%<br/>176 jobs/s, 41/50 busy"]
    provider["provider<br/>0.5 CPU · 256m<br/>CPU 5.7%<br/>in flight 0"]
    load -->|"70% read · 30% write<br/>600 rps · p99 1.9 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>600 rps · p99 1.8 ms"| api
    api -->|"read on reads, write on writes<br/>pool 10<br/>600 calls/s · p99 0.5 ms<br/>in use 0/20"| db
    api -->|"publish on writes<br/>pool 10<br/>176 calls/s · p99 0.5 ms<br/>in use 0/20"| emails
    emails -->|"consume, 25 at a time<br/>176 jobs/s"| sender
    sender -->|"http<br/>pool 25<br/>176 calls/s · p99 200 ms<br/>in use 41/50"| provider
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb,api,db,emails,provider ok
    class sender warm
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
    x-axis "seconds" 0 --> 290
    y-axis "requests/s" 0 --> 800
    line [800, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [150, 150, 150, 150, 150, 150, 168.75, 206.25, 243.75, 281.25, 318.75, 356.25, 393.75, 431.25, 468.75, 506.25, 543.75, 581.25, 600, 600, 600, 600, 600, 600, 600, 600, 600, 600, 600, 600]
    line [149.073, 150.032, 149.961, 150.05, 150.016, 149.975, 168.509, 206.037, 243.477, 281.09, 318.488, 355.938, 393.62, 430.872, 468.503, 506.08, 543.484, 581.029, 600.083, 599.912, 600.021, 599.969, 599.968, 600.04, 600.229, 599.719, 600.093, 600.084, 599.872, 600.018]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 0:00 latency +150 ms on connections into provider.</sub>

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
    y-axis "ms" 0 --> 3
    line [3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [1.08, 1.02, 1.029, 1.019, 1.035, 1.047, 1.012, 0.948, 0.886, 0.856, 0.832, 0.819, 0.815, 0.812, 0.811, 0.809, 0.811, 0.809, 0.807, 0.81, 0.806, 0.808, 0.807, 0.807, 0.807, 0.805, 0.806, 0.807, 0.807, 0.807]
    line [1.885, 1.734, 1.85, 1.796, 1.874, 1.965, 1.854, 1.624, 1.488, 1.404, 1.298, 1.267, 1.265, 1.242, 1.241, 1.236, 1.235, 1.241, 1.227, 1.218, 1.213, 1.202, 1.209, 1.225, 1.224, 1.221, 1.198, 1.205, 1.209, 1.249]
    line [2.833, 2.258, 2.466, 2.798, 2.505, 2.613, 2.596, 2.194, 1.921, 1.937, 1.731, 1.871, 1.752, 1.673, 1.686, 1.814, 1.828, 1.82, 1.768, 1.775, 1.836, 1.826, 1.845, 1.861, 1.945, 2.099, 1.88, 1.885, 1.994, 2.064]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 0:00 latency +150 ms on connections into provider.</sub>

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
    y-axis "% of requests" 0 --> 1
    line [1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 0:00 latency +150 ms on connections into provider.</sub>

```mermaid
---
config:
  themeVariables:
    xyChart:
      backgroundColor: "transparent"
      plotColorPalette: "#f4cccc, #1f77b4, #ff7f0e, #2ca02c, #9467bd, #8c564b, #e377c2"
---
xychart-beta
    title "CPU against each limit"
    x-axis "seconds" 0 --> 290
    y-axis "% of limit" 0 --> 100
    line [100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [13.415, 12.9, 12.898, 12.617, 12.918, 12.773, 14.033, 16.006, 18.095, 20.037, 22.112, 24.349, 26.585, 29.018, 31.373, 33.732, 36.18, 38.587, 39.748, 39.664, 39.753, 39.801, 39.879, 40.052, 39.949, 40.096, 39.951, 39.943, 39.968, 39.225]
    line [3.973, 4.468, 4.161, 5.274, 4.475, 4.45, 4.235, 4.079, 4.051, 3.966, 3.936, 3.9, 4.112, 4.246, 4.277, 5.023, 4.599, 4.743, 4.873, 4.901, 4.702, 5.476, 4.723, 4.845, 4.686, 4.805, 4.757, 5.285, 4.766, 5]
    line [5.437, 3.565, 3.783, 3.635, 3.68, 3.628, 3.573, 3.633, 3.431, 3.513, 3.541, 3.56, 3.852, 3.891, 4.052, 4.271, 4.592, 4.563, 4.627, 4.655, 4.651, 4.748, 4.661, 4.695, 4.817, 4.954, 4.999, 4.818, 4.718, 4.583]
    line [4.686, 4.853, 5.019, 5.158, 5.178, 5.2, 5.365, 5.346, 5.429, 5.768, 5.953, 6.329, 6.679, 7.312, 7.723, 8.319, 8.729, 9.439, 9.512, 9.534, 9.207, 9.35, 9.401, 9.729, 9.612, 9.781, 9.454, 9.697, 9.512, 9.288]
    line [3.207, 3.272, 3.173, 3.126, 3.262, 2.982, 3.478, 3.482, 3.457, 3.585, 3.707, 3.869, 4.243, 4.311, 4.563, 4.908, 5.283, 5.369, 5.51, 5.727, 5.547, 5.589, 5.607, 5.771, 5.746, 5.744, 5.865, 5.707, 5.96, 5.63]
    line [7.944, 7.351, 7.495, 7.524, 7.639, 7.273, 8.397, 9.667, 10.12, 11.416, 12.652, 13.695, 15.649, 16.105, 17.642, 19.025, 21.361, 22.03, 22.13, 23.19, 22.55, 22.89, 22.427, 22.954, 22.337, 22.707, 22.952, 22.013, 23.286, 21.624]
```

<sub>Blue: api. Orange: db. Green: emails. Purple: lb. Brown: provider. Pink: sender (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 0:00 latency +150 ms on connections into provider.</sub>

## Diagnosis

No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version 13; thresholds below).

## Predictions

The design has no `expect` lines. Writing a prediction before a run turns it into a test of your mental model.

## Timeline

| Time | Event |
| --- | --- |
| 0:00 | fault started: latency +150 ms on connections into provider |
| 5:00 | fault ended: latency +150 ms on connections into provider |

<sub>From events.jsonl: the fault timeline and Docker container events.</sub>

## Assumptions

### From the design

| Field | Value | Basis | Why |
| --- | --- | --- | --- |
| `scenario` |  | guessed | a transactional email API answers in about 150 ms |
| `workload.payload_kb` |  | guessed | an order with its items and address, which the email job carries |
| `services.emails.resources.memory` |  | guessed | a small managed Redis for background jobs |
| `workload.profile` |  | guessed | a sale that quadruples traffic over two minutes |

4 of these are guessed, not measured: the result depends on them.

### Chosen by Loadlore

| Field | Value | Why |
| --- | --- | --- |
| services.emails.config.maxmemory | 48m | 75% of the container's 64m memory limit, as managed Redis usually does; the rest is for fragmentation and client buffers. Set maxmemory: 0 for stock Redis (no cap) |
| services.emails.config.maxmemory-policy | noeviction | a queue keeps every job: once it is full, publishing fails with an error until workers make room, as Sidekiq and BullMQ ask of their Redis |
| workload.timeout_ms | 10s | the default; k6 gives up on a request after this long and counts it as failed |
| workload.slo.p99_ms | 500 ms | the default; a break on p99 alone needs p99 at or above this as well as 10x the low-load p99, so latency users would still accept isn't called a break |
| resources.cpu_period_ms | 100 ms for api, db, emails, lb, provider, sender | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.db.disk | the Docker host's | no disk block: a read that misses db's memory goes to the Docker host's disk, which on a laptop mostly answers from the host's own memory, fast and uncapped. Add disk: {} for a cloud volume's wait and IOPS cap |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |
| services.sender.consume.concurrency | 25 | set in the design: each replica runs up to 25 jobs from emails at once |
| services.api.pools.db.min | 0 | the default, as pgxpool's: each replica opens connections only when callers need them, so a pool fills as load rises, and a burst opens many at once |
| services.api.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- Loadlore's own containers get fixed budgets: load generator 1.5 CPU (peak 600 rps), Toxiproxy 1 CPU (about 1380 proxied calls/s at peak), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each

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

- loadl dev, profile 5m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 1011 (hash 998, memory 1020, syscalls 1016): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (1011) was within 5.0% of the run it's compared with (1028)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 125,999 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-backed-up-queue-more-runners-20261116-132248-api-0-1 | loadlore/synthetic:dev |
| loadlore-backed-up-queue-more-runners-20261116-132248-api-1-1 | loadlore/synthetic:dev |
| loadlore-backed-up-queue-more-runners-20261116-132248-db-1 | postgres:17.11 |
| loadlore-backed-up-queue-more-runners-20261116-132248-emails-1 | redis:8.2 |
| loadlore-backed-up-queue-more-runners-20261116-132248-lb-1 | haproxy:3.2.26 |
| loadlore-backed-up-queue-more-runners-20261116-132248-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-backed-up-queue-more-runners-20261116-132248-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-backed-up-queue-more-runners-20261116-132248-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-backed-up-queue-more-runners-20261116-132248-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-backed-up-queue-more-runners-20261116-132248-lore-redisexp-emails-1 | oliver006/redis_exporter:v1.97.0 |
| loadlore-backed-up-queue-more-runners-20261116-132248-lore-toxiproxy-1 | ghcr.io/shopify/toxiproxy:2.13.0 |
| loadlore-backed-up-queue-more-runners-20261116-132248-provider-0-1 | loadlore/synthetic:dev |
| loadlore-backed-up-queue-more-runners-20261116-132248-sender-0-1 | loadlore/synthetic:dev |
| loadlore-backed-up-queue-more-runners-20261116-132248-sender-1-1 | loadlore/synthetic:dev |

</details>

## Per-window data

<details><summary>30 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | emails CPU | emails jobs | oldest | published/s | finished/s | lb CPU | lb up | provider CPU | sender CPU | api→db pool | wait | api→emails pool | wait | sender→provider pool | wait | k6 CPU | Toxiproxy CPU | k6 / Toxiproxy stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 150 | 149 | 0 | 0% | 1.1 ms | 2.8 ms | 13% | 4.0% | 4 | 5.4% | 6 | 145 ms | 44 | 43 | 4.7% | 2 | 3.2% | 7.9% | 0/20 | 0.0 ms | 0/20 | – | 14/50 | 0.0 ms | 4.0% | 6.2% | 0% / 0% |
| 0:10 | 150 | 150 | 0 | 0% | 1.0 ms | 2.3 ms | 13% | 4.5% | 4 | 3.6% | 9 | 145 ms | 43 | 43 | 4.9% | 2 | 3.3% | 7.4% | 0/20 | 0.0 ms | 0/20 | – | 9/50 | 0.0 ms | 3.1% | 5.5% | 0% / 0% |
| 0:20 | 150 | 150 | 0 | 0% | 1.0 ms | 2.5 ms | 13% | 4.2% | 5 | 3.8% | 6 | 146 ms | 44 | 44 | 5.0% | 2 | 3.2% | 7.5% | 0/20 | 0.0 ms | 0/20 | – | 13/50 | 0.0 ms | 2.9% | 5.7% | 0% / 0% |
| 0:30 | 150 | 150 | 0 | 0% | 1.0 ms | 2.8 ms | 13% | 5.3% | 6 | 3.6% | 8 | 139 ms | 45 | 45 | 5.2% | 2 | 3.1% | 7.5% | 0/20 | 0.0 ms | 0/20 | – | 12/50 | 0.0 ms | 2.9% | 5.6% | 0% / 0% |
| 0:40 | 150 | 150 | 0 | 0% | 1.0 ms | 2.5 ms | 13% | 4.5% | 5 | 3.7% | 6 | 146 ms | 44 | 44 | 5.2% | 2 | 3.3% | 7.6% | 0/20 | 0.0 ms | 0/20 | – | 11/50 | 0.0 ms | 2.9% | 5.8% | 0% / 0% |
| 0:50 | 150 | 150 | 0 | 0% | 1.0 ms | 2.6 ms | 13% | 4.4% | 5 | 3.6% | 4 | 145 ms | 43 | 43 | 5.2% | 2 | 3.0% | 7.3% | 0/20 | 0.0 ms | 0/20 | – | 10/50 | 0.0 ms | 3.0% | 5.7% | 0% / 0% |
| 1:00 | 169 | 169 | 0 | 0% | 1.0 ms | 2.6 ms | 14% | 4.2% | 5 | 3.6% | 6 | 145 ms | 51 | 51 | 5.4% | 2 | 3.5% | 8.4% | 0/20 | 0.0 ms | 0/20 | – | 17/50 | 0.0 ms | 3.1% | 6.3% | 0% / 0% |
| 1:10 | 206 | 206 | 0 | 0% | 0.9 ms | 2.2 ms | 16% | 4.1% | 5 | 3.6% | 10 | 144 ms | 64 | 64 | 5.3% | 2 | 3.5% | 9.7% | 1/20 | 0.0 ms | 0/20 | – | 15/50 | 0.0 ms | 3.3% | 6.5% | 0% / 0% |
| 1:20 | 244 | 243 | 0 | 0% | 0.9 ms | 1.9 ms | 18% | 4.1% | 5 | 3.4% | 13 | 145 ms | 70 | 70 | 5.4% | 2 | 3.5% | 10% | 0/20 | 0.0 ms | 0/20 | – | 19/50 | 0.0 ms | 3.7% | 6.7% | 0% / 0% |
| 1:30 | 281 | 281 | 0 | 0% | 0.9 ms | 1.9 ms | 20% | 4.0% | 5 | 3.5% | 14 | 147 ms | 82 | 82 | 5.8% | 2 | 3.6% | 11% | 0/20 | 0.0 ms | 0/20 | – | 26/50 | 0.0 ms | 3.8% | 7.0% | 0% / 0% |
| 1:40 | 319 | 318 | 0 | 0% | 0.8 ms | 1.7 ms | 22% | 3.9% | 5 | 3.5% | 11 | 147 ms | 96 | 95 | 6.0% | 2 | 3.7% | 13% | 1/20 | 0.0 ms | 0/20 | – | 28/50 | 0.0 ms | 4.0% | 7.4% | 0% / 0% |
| 1:50 | 356 | 356 | 0 | 0% | 0.8 ms | 1.9 ms | 24% | 3.9% | 5 | 3.6% | 12 | 147 ms | 104 | 105 | 6.3% | 2 | 3.9% | 14% | 1/20 | 0.0 ms | 0/20 | – | 25/50 | 0.0 ms | 4.2% | 7.9% | 0% / 0% |
| 2:00 | 394 | 394 | 0 | 0% | 0.8 ms | 1.8 ms | 27% | 4.1% | 5 | 3.9% | 17 | 148 ms | 119 | 119 | 6.7% | 2 | 4.2% | 16% | 1/20 | 0.0 ms | 0/20 | – | 42/50 | 0.0 ms | 4.6% | 8.7% | 0% / 0% |
| 2:10 | 431 | 431 | 0 | 0% | 0.8 ms | 1.7 ms | 29% | 4.2% | 5 | 3.9% | 28 | 145 ms | 125 | 126 | 7.3% | 2 | 4.3% | 16% | 0/20 | 0.0 ms | 0/20 | – | 43/50 | 0.0 ms | 5.0% | 9.2% | 0% / 0% |
| 2:20 | 469 | 469 | 0 | 0% | 0.8 ms | 1.7 ms | 31% | 4.3% | 5 | 4.1% | 18 | 146 ms | 136 | 135 | 7.7% | 2 | 4.6% | 18% | 0/20 | 0.0 ms | 0/20 | – | 46/50 | 0.0 ms | 5.2% | 10.0% | 0% / 0% |
| 2:30 | 506 | 506 | 0 | 0% | 0.8 ms | 1.8 ms | 34% | 5.0% | 5 | 4.3% | 27 | 147 ms | 149 | 149 | 8.3% | 2 | 4.9% | 19% | 1/20 | 0.0 ms | 0/20 | – | 43/50 | 0.0 ms | 5.5% | 11% | 0% / 0% |
| 2:40 | 544 | 543 | 0 | 0% | 0.8 ms | 1.8 ms | 36% | 4.6% | 5 | 4.6% | 25 | 146 ms | 168 | 167 | 8.7% | 2 | 5.3% | 21% | 1/20 | 0.0 ms | 0/20 | – | 33/50 | 0.0 ms | 5.8% | 12% | 0% / 0% |
| 2:50 | 581 | 581 | 0 | 0% | 0.8 ms | 1.8 ms | 39% | 4.7% | 5 | 4.6% | 23 | 145 ms | 173 | 174 | 9.4% | 2 | 5.4% | 22% | 0/20 | 0.0 ms | 0/20 | – | 44/50 | 0.0 ms | 5.9% | 12% | 0% / 0% |
| 3:00 | 600 | 600 | 0 | 0% | 0.8 ms | 1.8 ms | 40% | 4.9% | 5 | 4.6% | 33 | 146 ms | 176 | 176 | 9.5% | 2 | 5.5% | 22% | 1/20 | 0.0 ms | 0/20 | – | 42/50 | 0.0 ms | 6.0% | 12% | 0% / 0% |
| 3:10 | 600 | 600 | 0 | 0% | 0.8 ms | 1.8 ms | 40% | 4.9% | 5 | 4.7% | 32 | 148 ms | 182 | 181 | 9.5% | 2 | 5.7% | 23% | 1/20 | 0.0 ms | 0/20 | – | 46/50 | 0.0 ms | 6.2% | 13% | 0% / 0% |
| 3:20 | 600 | 600 | 0 | 0% | 0.8 ms | 1.8 ms | 40% | 4.7% | 5 | 4.7% | 27 | 147 ms | 178 | 179 | 9.2% | 2 | 5.5% | 23% | 1/20 | 0.0 ms | 0/20 | – | 43/50 | 0.0 ms | 6.1% | 13% | 0% / 0% |
| 3:30 | 600 | 600 | 0 | 0% | 0.8 ms | 1.8 ms | 40% | 5.5% | 5 | 4.7% | 29 | 146 ms | 181 | 180 | 9.4% | 2 | 5.6% | 23% | 1/20 | 0.0 ms | 0/20 | – | 42/50 | 0.0 ms | 6.0% | 13% | 0% / 0% |
| 3:40 | 600 | 600 | 0 | 0% | 0.8 ms | 1.8 ms | 40% | 4.7% | 5 | 4.7% | 26 | 147 ms | 177 | 178 | 9.4% | 2 | 5.6% | 22% | 1/20 | 0.0 ms | 0/20 | – | 42/50 | 0.0 ms | 6.1% | 12% | 0% / 0% |
| 3:50 | 600 | 600 | 0 | 0% | 0.8 ms | 1.9 ms | 40% | 4.8% | 5 | 4.7% | 27 | 146 ms | 180 | 180 | 9.7% | 2 | 5.8% | 23% | 1/20 | 0.0 ms | 1/20 | – | 40/50 | 0.0 ms | 6.0% | 12% | 0% / 0% |
| 4:00 | 600 | 600 | 0 | 0% | 0.8 ms | 1.9 ms | 40% | 4.7% | 5 | 4.8% | 21 | 146 ms | 176 | 176 | 9.6% | 2 | 5.7% | 22% | 0/20 | 0.0 ms | 0/20 | – | 41/50 | 0.0 ms | 6.0% | 12% | 0% / 0% |
| 4:10 | 600 | 600 | 0 | 0% | 0.8 ms | 2.1 ms | 40% | 4.8% | 5 | 5.0% | 34 | 147 ms | 179 | 177 | 9.8% | 2 | 5.7% | 23% | 1/20 | 0.0 ms | 1/20 | – | 39/50 | 0.0 ms | 6.0% | 12% | 0% / 0% |
| 4:20 | 600 | 600 | 0 | 0% | 0.8 ms | 1.9 ms | 40% | 4.8% | 5 | 5.0% | 27 | 146 ms | 182 | 183 | 9.5% | 2 | 5.9% | 23% | 1/20 | 0.0 ms | 0/20 | – | 43/50 | 0.0 ms | 6.1% | 12% | 0% / 0% |
| 4:30 | 600 | 600 | 0 | 0% | 0.8 ms | 1.9 ms | 40% | 5.3% | 5 | 4.8% | 28 | 147 ms | 174 | 175 | 9.7% | 2 | 5.7% | 22% | 1/20 | 0.0 ms | 1/20 | – | 42/50 | 0.0 ms | 6.0% | 12% | 0% / 0% |
| 4:40 | 600 | 600 | 0 | 0% | 0.8 ms | 2.0 ms | 40% | 4.8% | 5 | 4.7% | 27 | 147 ms | 184 | 183 | 9.5% | 2 | 6.0% | 23% | 1/20 | 0.0 ms | 1/20 | – | 42/50 | 0.0 ms | 6.0% | 13% | 0% / 0% |
| 4:50 | 600 | 600 | 0 | 0% | 0.8 ms | 2.1 ms | 39% | 5.0% | 7 | 4.6% | 26 | 146 ms | 172 | 172 | 9.3% | 2 | 5.6% | 22% | 0/20 | 0.0 ms | 0/20 | – | 43/50 | 0.0 ms | 5.2% | 12% | 0% / 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 and Toxiproxy CPU are against their budgets. A queue's jobs are redis_stream_length (redis_exporter): jobs published and not yet finished, waiting or running, at the window's end; oldest is redis_stream_first_entry_id (redis_exporter): the oldest unfinished job's publish time, at most over the window; published is redis_stream_entries_added_total (redis_exporter) and finished loadlore_jobs_total (the workers' finished jobs, by result).

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: backed-up-queue-more-runners

# A shop's checkout. Each order is written to Postgres and its
# confirmation email goes on a queue, so checkout doesn't wait for the
# email provider. Two sender workers take the emails off the queue and
# hand each one to the provider.

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
      cpu_ms: 0.5
      calls:
        - { target: db, op: read, when: read }          # product pages
        - { target: db, op: write, when: write }        # the order
        - { target: emails, op: publish, when: write }  # its confirmation email
    pools: { db: 10, emails: 10 }

  emails:
    kind: queue
    resources: { cpu: 0.5, memory: 64m }

  sender:
    image: loadlore/synthetic
    role: worker
    replicas: 2
    resources: { cpu: 0.5, memory: 128m }
    consume: { from: emails, concurrency: 25 }   # was 5
    behavior:
      cpu_ms: 1
      calls:
        - { target: provider, op: http }   # hand the email to the provider
    pools: { provider: 25 }   # was 10: one connection per job runner

  provider:
    image: loadlore/synthetic
    resources: { cpu: 0.5, memory: 256m }
    behavior:
      cpu_ms: 0.05

  db:
    kind: postgres
    version: "17"
    resources: { cpu: 1, memory: 1g }
    dataset: { rows: 100000, row_bytes: 512 }

workload:
  entry: lb
  mix: { read: 0.7, write: 0.3 }
  payload_kb: 8   # an order, and so its email job
  profile:
    - hold: { rps: 150, for: 1m }                 # a normal morning
    - ramp: { from: 150, to: 600, over: 2m }      # the sale opens
    - hold: { rps: 600, for: 2m }

# The email provider is outside your system and takes about 150 ms to
# accept an email, modeled as network latency on the way to it for the
# whole run.
scenario:
  - { at: 0s, inject: latency, target: provider, add_ms: 150 }

assumptions:
  - { field: scenario, basis: guessed, why: "a transactional email API answers in about 150 ms" }
  - { field: workload.payload_kb, basis: guessed, why: "an order with its items and address, which the email job carries" }
  - { field: services.emails.resources.memory, basis: guessed, why: "a small managed Redis for background jobs" }
  - { field: workload.profile, basis: guessed, why: "a sale that quadruples traffic over two minutes" }
```
