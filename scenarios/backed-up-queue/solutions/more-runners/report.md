# backed-up-queue-more-runners: run report

_run `backed-up-queue-more-runners-20261118-003422` · completed · 18 Nov 2026 00:34 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 600 rps offered, 600 rps served at best, worst 10-second p99 3.0 ms, at most 0% errors.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 3:40

Measured in the 10-second window from 3:40 to 3:50 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 600 rps<br/>offered 600 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 7.2%<br/>2/2 replicas up"}}
    api["api ×2<br/>0.5 CPU · 256m<br/>CPU 39%<br/>in flight 2"]
    db[("db<br/>postgres 17<br/>1 CPU · 1g<br/>CPU 4.4%<br/>conns 5/100")]
    emails[("emails<br/>queue · Redis Streams<br/>0.5 CPU · 64m<br/>CPU 3.9%<br/>36 jobs")]
    sender["sender ×2<br/>worker<br/>0.5 CPU · 128m<br/>CPU 22%<br/>177 jobs/s, 41/50 busy"]
    provider["provider<br/>0.5 CPU · 256m<br/>CPU 5.2%<br/>in flight 0"]
    load -->|"70% read · 30% write<br/>600 rps · p99 2.3 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>600 rps · p99 2.0 ms"| api
    api -->|"read on reads, write on writes<br/>pool 10<br/>600 calls/s · p99 0.9 ms<br/>in use 1/20"| db
    api -->|"publish on writes<br/>pool 10<br/>177 calls/s · p99 0.5 ms<br/>in use 2/20"| emails
    emails -->|"consume, 25 at a time<br/>177 jobs/s"| sender
    sender -->|"http<br/>pool 25<br/>177 calls/s · p99 200 ms<br/>in use 41/50"| provider
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb,api,db,emails,provider ok
    class sender warm
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
    x-axis "seconds" 0 --> 290
    y-axis "requests/s" 0 --> 800
    line [800, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [150, 150, 150, 150, 150, 150, 168.75, 206.25, 243.75, 281.25, 318.75, 356.25, 393.75, 431.25, 468.75, 506.25, 543.75, 581.25, 600, 600, 600, 600, 600, 600, 600, 600, 600, 600, 600, 600]
    line [149.477, 149.974, 149.92, 150.046, 150.082, 149.925, 168.67, 205.969, 243.604, 281.177, 318.704, 355.85, 393.71, 430.951, 468.68, 506.022, 543.699, 581.014, 600.062, 599.996, 599.997, 600.083, 600.331, 599.531, 600.058, 600.013, 600.127, 599.791, 600.078, 599.973]
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
    y-axis "ms" 0 --> 4
    line [4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [1.755, 1.628, 1.634, 1.633, 1.637, 1.625, 1.625, 1.6, 1.596, 1.589, 1.587, 1.578, 1.582, 1.561, 1.565, 1.549, 1.552, 1.532, 1.53, 1.531, 1.534, 1.531, 1.526, 1.532, 1.515, 1.53, 1.553, 1.521, 1.536, 1.533]
    line [2.464, 2.287, 2.315, 2.304, 2.337, 2.29, 2.261, 2.191, 2.21, 2.177, 2.137, 2.12, 2.116, 2.086, 2.097, 2.081, 2.072, 2.055, 2.049, 2.046, 2.041, 2.035, 2.034, 2.041, 2.019, 2.037, 2.052, 2.013, 2.043, 2.092]
    line [2.99, 2.627, 2.693, 2.584, 2.73, 2.573, 2.507, 2.437, 2.465, 2.43, 2.32, 2.406, 2.318, 2.373, 2.379, 2.32, 2.32, 2.384, 2.317, 2.407, 2.303, 2.336, 2.294, 2.318, 2.289, 2.309, 2.398, 2.306, 2.373, 2.506]
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
    line [11.604, 11.503, 11.535, 11.368, 11.436, 11.435, 12.43, 14.451, 16.798, 19.029, 21.381, 23.811, 26.179, 28.448, 30.808, 33.212, 35.59, 37.746, 39.078, 39.038, 39.079, 39.109, 38.917, 39.135, 38.964, 39.034, 39.078, 38.982, 39.037, 37.874]
    line [3.393, 3.624, 3.312, 4.106, 3.692, 3.511, 3.326, 3.296, 3.412, 3.538, 3.625, 3.791, 3.936, 3.949, 4.082, 4.254, 4.327, 4.457, 4.372, 4.623, 4.409, 4.936, 4.43, 4.525, 4.342, 4.52, 4.44, 4.872, 4.561, 4.745]
    line [2.733, 2.754, 2.761, 2.729, 2.541, 2.592, 2.612, 2.663, 2.661, 2.88, 2.943, 3.156, 3.288, 3.367, 3.576, 3.585, 3.851, 3.942, 3.912, 4.008, 3.978, 4.049, 3.858, 3.979, 3.95, 3.897, 3.988, 3.918, 4.218, 3.882]
    line [3.311, 3.236, 3.376, 3.355, 3.256, 3.237, 3.237, 3.498, 3.762, 4.107, 4.537, 4.9, 5.275, 5.661, 6.025, 6.538, 6.904, 7.203, 7.368, 7.283, 7.352, 7.206, 7.189, 7.303, 7.251, 7.293, 7.376, 7.271, 7.242, 7.109]
    line [2.582, 2.476, 2.577, 2.578, 2.519, 2.384, 2.631, 2.754, 2.963, 3.095, 3.443, 3.615, 3.883, 4.033, 4.308, 4.604, 4.876, 5.135, 5.109, 5.169, 5.227, 5.213, 5.223, 5.326, 5.05, 5.263, 5.284, 4.996, 5.354, 5.068]
    line [6.894, 6.636, 6.791, 7.038, 6.763, 6.634, 7.442, 8.788, 9.568, 10.908, 12.422, 13.613, 15.238, 15.738, 17.307, 18.673, 21.005, 21.488, 21.996, 22.58, 22.213, 22.399, 21.949, 22.382, 21.894, 22.123, 22.633, 21.733, 22.773, 21.157]
```

<sub>Blue: api. Orange: db. Green: emails. Purple: lb. Brown: provider. Pink: sender (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 0:00 latency +150 ms on connections into provider.</sub>

## Diagnosis

No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version 22; thresholds below).

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
| network.profile | same-zone: 0.3 ms round trip | the default: every service in one availability zone. AWS says a round trip between two instances in the same zone is generally sub-millisecond with enhanced networking, and publishes no exact figure, so the 0.3 ms round trip Loadlore adds between any two containers is its own estimate, inside that bound. Applied between any two of the stack's containers, the load generator and the entry point included, as half the round trip on each side's outgoing packets; Loadlore's own tooling isn't slowed. Not scaled with --scale. Source: AWS Architecture Blog, Improving performance and reducing cost using Availability Zone Affinity: https://aws.amazon.com/blogs/architecture/improving-performance-and-reducing-cost-using-availability-zone-affinity/ |
| resources.cpu_period_ms | 100 ms for api, db, emails, lb, provider, sender | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.db.disk | laptop: the Docker host's | no disk block: a read that misses db's memory goes to the Docker host's disk, which on a laptop mostly answers from the host's own memory, fast and uncapped, and a commit's fsync returns in a fraction of a millisecond. Add disk: { profile: gp3 } for a cloud volume's waits and caps |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |
| services.sender.consume.concurrency | 25 | set in the design: each replica runs up to 25 jobs from emails at once |
| services.sender.consume.retries | none | not set in the design: a job whose calls fail is dropped, not retried. Real job systems retry (Sidekiq up to 25 times), so add retries to see the backlog retries make |
| services.api.pools.db.min | 0 | the default, as pgxpool's: each replica opens connections only when callers need them, so a pool fills as load rises, and a burst opens many at once |
| services.api.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- Loadlore's own containers get fixed budgets: load generator 1.5 CPU (peak 600 rps), Toxiproxy 1 CPU (about 0 calls/s at peak on the 1 connection it proxies for faults), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each

## Thresholds

- **Broke:** 2 windows in a row with at least 1.0% of requests failing or successful responses below 90% of the offered rate, or 3 windows in a row with any of those or p99 at least 10x the low-load p99 (the median p99 of windows offered at most 25% of the peak, before errors or a shortfall in 2 windows in a row) and at least the SLO, 500 ms (workload.slo.p99_ms): 500 ms in this run. A shorter episode is shown as a stall in the timeline, not a break.
- **Doubtful:** a break on p99 alone, when the load generator was stalled at its CPU limit in 1.0% or more of its scheduling periods in one of the break's windows and the entry service's own p99 (loadlore_request_duration_seconds (the service's own histogram)) was below the limit in one of them. The verdict stays what it was.
- **Inconclusive:** before the break (or anywhere, if nothing broke), the load generator dropped more than 1.0% of a window's arrivals (more than 5.0% in a window where the system itself failed 1.0% or more of requests; up to that it's a caveat that gives the share), or the load generator or Toxiproxy used 90% or more of its CPU budget, or was stalled at its CPU limit in 5.0% or more of its scheduling periods (from 1.0% it's noted as a caveat). The first 10 s of load are a warm-up and don't count: the load generator starts its VUs and connections open then. A cause is named only with evidence: on a Linux host, the hypervisor giving 2.0% or more of CPU time to other virtual machines (steal), or other programs using 10% or more of the host's CPUs (at least 0.5); inside a VM on this machine, only as a possibility. A fault that couldn't be injected, or an autoscaler decision that couldn't be carried out, also makes a run inconclusive: it measured a different design.
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
- **Hot shard:** for two windows in a row, a caller sending calls to shards by key, one shard taking 1.5x the calls of the next busiest shard or more while it was busy (CPU 80% or more, or its calls 3x their low-load time and 1 ms more) and another shard's CPU under 50%. A queueing knee on that shard at the same time then points at its popular keys.
- **CPU bursts at the limit:** a stall no other rule covers and no fault made, in which a component was CPU-throttled for 3 s or more in a window (summed over its threads) while its average CPU stayed under 90% of its limit: bursts of work used up a CFS period's quota early and waited for the next period.
- **Host disk stalls:** for 2 seconds or more, a Postgres on the Docker host's own disk read no blocks and committed at most 10% of its median over the 10 s before (50 a second or more), while 5 or more queries waited on disk reads or WAL flushes and its exporter answered. Near the break it makes the run inconclusive; anywhere else it's a caveat, and its windows are flagged and left out of exports.

## Environment

- loadl dev, profile 5m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 986 (hash 942, memory 1041, syscalls 977): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (986) was within 5.0% of the run it's compared with (981)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 125,999 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-backed-up-queue-more-runners-20261118-003422-api-0-1 | loadlore/synthetic:dev |
| loadlore-backed-up-queue-more-runners-20261118-003422-api-1-1 | loadlore/synthetic:dev |
| loadlore-backed-up-queue-more-runners-20261118-003422-db-1 | postgres:17.11 |
| loadlore-backed-up-queue-more-runners-20261118-003422-emails-1 | redis:8.2 |
| loadlore-backed-up-queue-more-runners-20261118-003422-lb-1 | haproxy:3.2.26 |
| loadlore-backed-up-queue-more-runners-20261118-003422-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-backed-up-queue-more-runners-20261118-003422-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-backed-up-queue-more-runners-20261118-003422-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-backed-up-queue-more-runners-20261118-003422-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-backed-up-queue-more-runners-20261118-003422-lore-redisexp-emails-1 | oliver006/redis_exporter:v1.97.0 |
| loadlore-backed-up-queue-more-runners-20261118-003422-lore-toxiproxy-1 | ghcr.io/shopify/toxiproxy:2.13.0 |
| loadlore-backed-up-queue-more-runners-20261118-003422-provider-0-1 | loadlore/synthetic:dev |
| loadlore-backed-up-queue-more-runners-20261118-003422-sender-0-1 | loadlore/synthetic:dev |
| loadlore-backed-up-queue-more-runners-20261118-003422-sender-1-1 | loadlore/synthetic:dev |

</details>

## Per-window data

<details><summary>30 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | emails CPU | emails jobs | oldest | published/s | finished/s | lb CPU | lb up | provider CPU | sender CPU | api→db pool | wait | api→emails pool | wait | sender→provider pool | wait | k6 CPU | Toxiproxy CPU | k6 / Toxiproxy stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 150 | 149 | 0 | 0% | 1.8 ms | 3.0 ms | 12% | 3.4% | 5 | 2.7% | 4 | 145 ms | 44 | 44 | 3.3% | 2 | 2.6% | 6.9% | 1/20 | 0.0 ms | 0/20 | – | 14/50 | 0.0 ms | 3.1% | 0.9% | 0% / 0% |
| 0:10 | 150 | 150 | 0 | 0% | 1.6 ms | 2.6 ms | 12% | 3.6% | 5 | 2.8% | 4 | 138 ms | 43 | 43 | 3.2% | 2 | 2.5% | 6.6% | 1/20 | 0.0 ms | 0/20 | – | 9/50 | 0.0 ms | 2.3% | 0.8% | 0% / 0% |
| 0:20 | 150 | 150 | 0 | 0% | 1.6 ms | 2.7 ms | 12% | 3.3% | 5 | 2.8% | 5 | 145 ms | 44 | 44 | 3.4% | 2 | 2.6% | 6.8% | 1/20 | 0.0 ms | 0/20 | – | 13/50 | 0.0 ms | 2.2% | 0.8% | 0% / 0% |
| 0:30 | 150 | 150 | 0 | 0% | 1.6 ms | 2.6 ms | 11% | 4.1% | 5 | 2.7% | 7 | 145 ms | 45 | 44 | 3.4% | 2 | 2.6% | 7.0% | 1/20 | 0.0 ms | 0/20 | – | 12/50 | 0.0 ms | 2.2% | 0.8% | 0% / 0% |
| 0:40 | 150 | 150 | 0 | 0% | 1.6 ms | 2.7 ms | 11% | 3.7% | 5 | 2.5% | 7 | 145 ms | 44 | 44 | 3.3% | 2 | 2.5% | 6.8% | 1/20 | 0.0 ms | 1/20 | – | 13/50 | 0.0 ms | 2.2% | 0.8% | 0% / 0% |
| 0:50 | 150 | 150 | 0 | 0% | 1.6 ms | 2.6 ms | 11% | 3.5% | 5 | 2.6% | 6 | 145 ms | 43 | 43 | 3.2% | 2 | 2.4% | 6.6% | 1/20 | 0.0 ms | 1/20 | – | 9/50 | 0.0 ms | 2.2% | 0.8% | 0% / 0% |
| 1:00 | 169 | 169 | 0 | 0% | 1.6 ms | 2.5 ms | 12% | 3.3% | 5 | 2.6% | 9 | 141 ms | 51 | 51 | 3.2% | 2 | 2.6% | 7.4% | 1/20 | 0.0 ms | 1/20 | – | 12/50 | 0.0 ms | 2.3% | 0.9% | 0% / 0% |
| 1:10 | 206 | 206 | 0 | 0% | 1.6 ms | 2.4 ms | 14% | 3.3% | 5 | 2.7% | 12 | 146 ms | 64 | 64 | 3.5% | 2 | 2.8% | 8.8% | 1/20 | 0.0 ms | 0/20 | – | 17/50 | 0.0 ms | 2.6% | 1.0% | 0% / 0% |
| 1:20 | 244 | 244 | 0 | 0% | 1.6 ms | 2.5 ms | 17% | 3.4% | 5 | 2.7% | 7 | 145 ms | 70 | 70 | 3.8% | 2 | 3.0% | 9.6% | 0/20 | 0.0 ms | 1/20 | – | 21/50 | 0.0 ms | 2.8% | 1.0% | 0% / 0% |
| 1:30 | 281 | 281 | 0 | 0% | 1.6 ms | 2.4 ms | 19% | 3.5% | 5 | 2.9% | 13 | 144 ms | 82 | 82 | 4.1% | 2 | 3.1% | 11% | 0/20 | 0.0 ms | 0/20 | – | 29/50 | 0.0 ms | 3.2% | 1.2% | 0% / 0% |
| 1:40 | 319 | 319 | 0 | 0% | 1.6 ms | 2.3 ms | 21% | 3.6% | 5 | 2.9% | 18 | 144 ms | 96 | 96 | 4.5% | 2 | 3.4% | 12% | 1/20 | 0.0 ms | 1/20 | – | 27/50 | 0.0 ms | 3.5% | 1.4% | 0% / 0% |
| 1:50 | 356 | 356 | 0 | 0% | 1.6 ms | 2.4 ms | 24% | 3.8% | 5 | 3.2% | 16 | 146 ms | 105 | 105 | 4.9% | 2 | 3.6% | 14% | 1/20 | 0.0 ms | 1/20 | – | 33/50 | 0.0 ms | 3.6% | 1.5% | 0% / 0% |
| 2:00 | 394 | 394 | 0 | 0% | 1.6 ms | 2.3 ms | 26% | 3.9% | 5 | 3.3% | 15 | 146 ms | 120 | 118 | 5.3% | 2 | 3.9% | 15% | 1/20 | 0.0 ms | 0/20 | – | 37/50 | 0.0 ms | 3.9% | 1.6% | 0% / 0% |
| 2:10 | 431 | 431 | 0 | 0% | 1.6 ms | 2.4 ms | 28% | 3.9% | 5 | 3.4% | 13 | 146 ms | 125 | 126 | 5.7% | 2 | 4.0% | 16% | 1/20 | 0.0 ms | 0/20 | – | 34/50 | 0.0 ms | 4.0% | 1.7% | 0% / 0% |
| 2:20 | 469 | 469 | 0 | 0% | 1.6 ms | 2.4 ms | 31% | 4.1% | 5 | 3.6% | 21 | 146 ms | 136 | 136 | 6.0% | 2 | 4.3% | 17% | 1/20 | 0.0 ms | 0/20 | – | 33/50 | 0.0 ms | 4.4% | 1.8% | 0% / 0% |
| 2:30 | 506 | 506 | 0 | 0% | 1.5 ms | 2.3 ms | 33% | 4.3% | 5 | 3.6% | 24 | 145 ms | 149 | 149 | 6.5% | 2 | 4.6% | 19% | 1/20 | 0.0 ms | 1/20 | – | 39/50 | 0.0 ms | 4.7% | 2.0% | 0% / 0% |
| 2:40 | 544 | 544 | 0 | 0% | 1.6 ms | 2.3 ms | 36% | 4.3% | 5 | 3.9% | 27 | 147 ms | 168 | 169 | 6.9% | 2 | 4.9% | 21% | 0/20 | 0.0 ms | 1/20 | – | 45/50 | 0.0 ms | 4.9% | 2.2% | 0% / 0% |
| 2:50 | 581 | 581 | 0 | 0% | 1.5 ms | 2.4 ms | 38% | 4.5% | 5 | 3.9% | 25 | 146 ms | 173 | 172 | 7.2% | 2 | 5.1% | 21% | 1/20 | 0.0 ms | 0/20 | – | 39/50 | 0.0 ms | 5.1% | 2.3% | 0% / 0% |
| 3:00 | 600 | 600 | 0 | 0% | 1.5 ms | 2.3 ms | 39% | 4.4% | 5 | 3.9% | 28 | 145 ms | 177 | 176 | 7.4% | 2 | 5.1% | 22% | 1/20 | 0.0 ms | 0/20 | – | 44/50 | 0.0 ms | 5.3% | 2.3% | 0% / 0% |
| 3:10 | 600 | 600 | 0 | 0% | 1.5 ms | 2.4 ms | 39% | 4.6% | 5 | 4.0% | 25 | 147 ms | 181 | 182 | 7.3% | 2 | 5.2% | 23% | 1/20 | 0.0 ms | 0/20 | – | 43/50 | 0.0 ms | 5.2% | 2.4% | 0% / 0% |
| 3:20 | 600 | 600 | 0 | 0% | 1.5 ms | 2.3 ms | 39% | 4.4% | 5 | 4.0% | 21 | 145 ms | 179 | 180 | 7.4% | 2 | 5.2% | 22% | 1/20 | 0.0 ms | 1/20 | – | 37/50 | 0.0 ms | 5.3% | 2.4% | 0% / 0% |
| 3:30 | 600 | 600 | 0 | 0% | 1.5 ms | 2.3 ms | 39% | 4.9% | 5 | 4.0% | 27 | 145 ms | 180 | 179 | 7.2% | 2 | 5.2% | 22% | 1/20 | 0.0 ms | 1/20 | – | 44/50 | 0.0 ms | 5.1% | 2.3% | 0% / 0% |
| 3:40 | 600 | 600 | 0 | 0% | 1.5 ms | 2.3 ms | 39% | 4.4% | 5 | 3.9% | 27 | 145 ms | 177 | 177 | 7.2% | 2 | 5.2% | 22% | 1/20 | 0.0 ms | 2/20 | – | 41/50 | 0.0 ms | 5.2% | 2.3% | 0% / 0% |
| 3:50 | 600 | 600 | 0 | 0% | 1.5 ms | 2.3 ms | 39% | 4.5% | 5 | 4.0% | 19 | 145 ms | 181 | 180 | 7.3% | 2 | 5.3% | 22% | 2/20 | 0.0 ms | 0/20 | – | 43/50 | 0.0 ms | 5.2% | 2.4% | 0% / 0% |
| 4:00 | 600 | 600 | 0 | 0% | 1.5 ms | 2.3 ms | 39% | 4.3% | 5 | 3.9% | 27 | 146 ms | 175 | 176 | 7.3% | 2 | 5.1% | 22% | 1/20 | 0.0 ms | 0/20 | – | 41/50 | 0.0 ms | 5.1% | 2.3% | 0% / 0% |
| 4:10 | 600 | 600 | 0 | 0% | 1.5 ms | 2.3 ms | 39% | 4.5% | 5 | 3.9% | 25 | 146 ms | 179 | 178 | 7.3% | 2 | 5.3% | 22% | 1/20 | 0.0 ms | 1/20 | – | 44/50 | 0.0 ms | 5.2% | 2.4% | 0% / 0% |
| 4:20 | 600 | 600 | 0 | 0% | 1.6 ms | 2.4 ms | 39% | 4.4% | 5 | 4.0% | 32 | 145 ms | 182 | 183 | 7.4% | 2 | 5.3% | 23% | 1/20 | 0.0 ms | 1/20 | – | 32/50 | 0.0 ms | 5.1% | 2.4% | 0% / 0% |
| 4:30 | 600 | 600 | 0 | 0% | 1.5 ms | 2.3 ms | 39% | 4.9% | 5 | 3.9% | 28 | 145 ms | 174 | 173 | 7.3% | 2 | 5.0% | 22% | 2/20 | 0.0 ms | 1/20 | – | 43/50 | 0.0 ms | 5.2% | 2.3% | 0% / 0% |
| 4:40 | 600 | 600 | 0 | 0% | 1.5 ms | 2.4 ms | 39% | 4.6% | 5 | 4.2% | 30 | 147 ms | 185 | 185 | 7.2% | 2 | 5.4% | 23% | 2/20 | 0.0 ms | 0/20 | – | 37/50 | 0.0 ms | 5.2% | 2.4% | 0% / 0% |
| 4:50 | 600 | 600 | 0 | 0% | 1.5 ms | 2.5 ms | 38% | 4.7% | 8 | 3.9% | 31 | 145 ms | 171 | 171 | 7.1% | 2 | 5.1% | 21% | 2/20 | 0.0 ms | 1/20 | – | 43/50 | 0.0 ms | 4.2% | 2.3% | 0% / 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 and Toxiproxy CPU are against their budgets. A queue's jobs are redis_stream_length (redis_exporter): jobs published and not yet finished, waiting or running, at the window's end; oldest is redis_stream_first_entry_id (redis_exporter): the oldest unfinished job's publish time, at most over the window; published is redis_stream_entries_added_total (redis_exporter), less loadlore_jobs_requeued_total (retries put back) and finished loadlore_jobs_total (the workers' finished jobs, by result).

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
