# connection-pool-trap-release-between-queries: run report

_run `connection-pool-trap-release-between-queries-20261118-090607` · completed · 18 Nov 2026 09:06 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 1,400 rps offered, 1,400 rps served at best, worst 10-second p99 88 ms, at most 0% errors.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 6:20

Measured in the 10-second window from 6:20 to 6:30 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 1,400 rps<br/>offered 1,400 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 18%<br/>4/4 replicas up"}}
    api["api ×4<br/>0.6 CPU · 512m<br/>CPU 31%<br/>in flight 115"]
    db[("db<br/>postgres 17<br/>1 CPU · 1g<br/>disk gp3 · 3,000 IOPS<br/>CPU 8.7%<br/>conns 17/100")]
    payments["payments<br/>0.5 CPU · 256m<br/>CPU 32%<br/>in flight 1"]
    load -->|"90% read · 10% write<br/>1,400 rps · p99 88 ms"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>1,400 rps · p99 100 ms"| api
    api -->|"read, write on writes<br/>pool 10<br/>1,538 calls/s · p99 3.8 ms<br/>in use 2/40"| db
    api -->|"http<br/>pool 50<br/>1,400 calls/s · p99 100 ms<br/>in use 114/200"| payments
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb,api,db,payments ok
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
    x-axis "seconds" 0 --> 410
    y-axis "requests/s" 0 --> 1500
    line [1500, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [121.667, 165, 208.333, 251.667, 295, 338.333, 381.667, 425, 468.333, 511.667, 555, 598.333, 641.667, 685, 728.333, 771.667, 815, 858.333, 901.667, 945, 988.333, 1031.667, 1075, 1118.333, 1161.667, 1205, 1248.333, 1291.667, 1335, 1378.333, 1400, 1400, 1400, 1400, 1400, 1400, 1400, 1400, 1400, 1400, 1400, 1400]
    line [117.825, 163.542, 207.196, 250.486, 293.704, 337.049, 380.61, 423.684, 467.025, 510.411, 553.807, 597.085, 640.323, 683.906, 727.276, 770.297, 813.869, 856.821, 901.206, 943.081, 987.091, 1030.577, 1073.856, 1116.992, 1160.497, 1203.66, 1247.106, 1290.35, 1333.807, 1377.246, 1399.848, 1400.197, 1399.815, 1399.734, 1400.235, 1400.024, 1399.978, 1399.945, 1400.308, 1399.763, 1399.912, 1400.13]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 0:00 latency +80 ms on connections into payments.</sub>

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
    x-axis "seconds" 0 --> 410
    y-axis "ms" 0 --> 100
    line [100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [84.741, 84.72, 84.72, 84.723, 84.726, 84.723, 84.721, 84.72, 84.72, 84.722, 84.72, 84.72, 84.721, 84.72, 84.72, 84.721, 84.721, 84.72, 84.721, 84.72, 84.72, 84.72, 84.721, 84.72, 84.721, 84.721, 84.721, 84.72, 84.721, 84.722, 84.727, 84.722, 84.722, 84.721, 84.722, 84.721, 84.722, 84.721, 84.721, 84.721, 84.722, 84.723]
    line [88.061, 88.022, 88.022, 88.027, 88.031, 88.026, 88.023, 88.022, 88.022, 88.024, 88.022, 88.022, 88.023, 88.022, 88.022, 88.022, 88.023, 88.022, 88.023, 88.022, 88.022, 88.022, 88.023, 88.022, 88.022, 88.023, 88.022, 88.022, 88.023, 88.024, 88.034, 88.025, 88.024, 88.023, 88.025, 88.022, 88.025, 88.022, 88.023, 88.023, 88.025, 88.026]
    line [88.356, 88.315, 88.315, 88.321, 88.325, 88.319, 88.317, 88.315, 88.315, 88.318, 88.315, 88.315, 88.316, 88.315, 88.315, 88.316, 88.317, 88.315, 88.317, 88.315, 88.315, 88.315, 88.317, 88.315, 88.316, 88.316, 88.316, 88.315, 88.317, 88.318, 88.327, 88.318, 88.318, 88.316, 88.319, 88.316, 88.319, 88.316, 88.317, 88.317, 88.319, 88.319]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 0:00 latency +80 ms on connections into payments.</sub>

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
    x-axis "seconds" 0 --> 410
    y-axis "% of requests" 0 --> 1
    line [1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 0:00 latency +80 ms on connections into payments.</sub>

```mermaid
---
config:
  themeVariables:
    xyChart:
      backgroundColor: "transparent"
      plotColorPalette: "#f4cccc, #1f77b4, #ff7f0e, #2ca02c, #9467bd"
---
xychart-beta
    title "CPU against each limit"
    x-axis "seconds" 0 --> 410
    y-axis "% of limit" 0 --> 100
    line [100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [3.917, 4.695, 5.364, 6.287, 7.19, 7.88, 8.722, 9.421, 10.312, 11.13, 11.985, 12.831, 13.688, 14.424, 15.334, 16.11, 16.965, 17.77, 18.724, 19.529, 20.455, 21.282, 22.228, 23.119, 24.028, 24.849, 25.837, 26.705, 27.52, 30.501, 34.751, 35.509, 36.066, 30.733, 31.621, 30.817, 30.862, 30.999, 31.017, 30.731, 30.881, 29.887]
    line [3.337, 3.483, 3.452, 3.676, 3.81, 3.939, 3.907, 4.115, 4.23, 4.312, 4.518, 4.648, 4.792, 4.81, 5.135, 5.295, 5.264, 5.52, 5.492, 5.848, 5.861, 6.135, 6.042, 6.444, 6.474, 6.691, 6.687, 7.029, 7.159, 8.913, 11.026, 11.276, 11.58, 8.401, 8.923, 8.593, 8.498, 8.647, 8.694, 8.584, 8.355, 8.183]
    line [3.256, 3.045, 3.383, 3.795, 4.278, 4.621, 5.101, 5.416, 5.873, 6.281, 6.669, 7.304, 7.514, 8.046, 8.377, 8.972, 9.303, 9.889, 10.272, 10.699, 11.176, 11.752, 12.119, 12.693, 13.143, 13.841, 14.088, 14.737, 15.084, 18.343, 23.836, 24.807, 25.719, 18.15, 19.366, 18.186, 18.262, 18.367, 18.385, 17.957, 18.303, 17.814]
    line [4.096, 5.024, 5.728, 6.532, 7.703, 8.341, 9.088, 9.976, 10.891, 11.726, 12.643, 13.47, 14.422, 15.325, 16.126, 17.287, 18.173, 19.016, 19.846, 20.657, 21.345, 22.184, 23.122, 23.913, 24.884, 25.35, 26.321, 27.208, 27.771, 31.567, 37.032, 38.082, 38.842, 31.427, 32.633, 31.609, 31.688, 31.541, 31.736, 31.366, 31.493, 30.292]
```

<sub>Blue: api. Orange: db. Green: lb. Purple: payments (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 0:00 latency +80 ms on connections into payments.</sub>

## Diagnosis

No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version 23; thresholds below).

## Predictions

2 of 2 scored predictions held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 300 until load_rps >= 1000` | **held** | 88 ms | 0:00 to 3:30, when load_rps >= 1,000 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |
| `error_rate < 1% throughout` | **held** | 0% | the whole run | source: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6) |

Loads and request rates depend on this machine: slower cores break the same design at a lower load, so a load guess can miss even when the reasoning behind it was right.

## Timeline

| Time | Event |
| --- | --- |
| 0:00 | fault started: latency +80 ms on connections into payments |
| 7:00 | fault ended: latency +80 ms on connections into payments |

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
| resources.cpu_period_ms | 100 ms for api, db, lb, payments | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.db.disk | gp3: 1 ms per read, 1 ms per write, 3000 IOPS, 125 MiB/s | a gp3 volume at its baseline: 3,000 IOPS and 125 MiB/s, included in its price, and up to 80,000 IOPS provisioned. AWS publishes no exact latency, only single-digit milliseconds, so the 1 ms Loadlore adds to every read, write and fsync is its own estimate: the low end of that range, so the disk is flattered rather than handicapped. Reads and writes are capped separately, so a mixed load can reach up to twice the IOPS. Provisioned throughput isn't modelled. Source: AWS, Amazon EBS General Purpose SSD volumes: https://docs.aws.amazon.com/ebs/latest/userguide/general-purpose.html |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |
| services.api.pools.db.min | 0 | the default, as pgxpool's: each replica opens connections only when callers need them, so a pool fills as load rises, and a burst opens many at once |
| services.api.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- Loadlore's own containers get fixed budgets: load generator 1.5 CPU (peak 1400 rps), Toxiproxy 1 CPU (about 1400 calls/s at peak on the 1 connection it proxies for faults), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each

## Thresholds

- **Broke:** 2 windows in a row with at least 1.0% of requests failing or successful responses below 90% of the offered rate, or 3 windows in a row with any of those or p99 at least 10x the low-load p99 (the median p99 of windows offered at most 25% of the peak, before errors or a shortfall in 2 windows in a row) and at least the SLO, 500 ms (workload.slo.p99_ms): 883 ms in this run. A shorter episode is shown as a stall in the timeline, not a break.
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
- **Host disk stalls:** for 2 seconds or more, a Postgres read no blocks and committed at most 10% of its median over the 10 s before (50 a second or more), while 5 or more queries waited on disk reads or WAL flushes and its exporter answered; on a modelled disk it had also been reading 20 blocks a second or more (median), which its caps can't stop for seconds; not under a disk_slow fault on it. Near the break it makes the run inconclusive; anywhere else it's a caveat, and its windows are flagged and left out of exports.

## Environment

- loadl dev, profile 7m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 986 (hash 917, memory 1032, syscalls 1012): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (986) was within 5.0% of the run it's compared with (986)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 392,995 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-connection-pool-trap-release-between-queries-20261118-090607-api-0-1 | loadlore/synthetic:dev |
| loadlore-connection-pool-trap-release-between-queries-20261118-090607-api-1-1 | loadlore/synthetic:dev |
| loadlore-connection-pool-trap-release-between-queries-20261118-090607-api-2-1 | loadlore/synthetic:dev |
| loadlore-connection-pool-trap-release-between-queries-20261118-090607-api-3-1 | loadlore/synthetic:dev |
| loadlore-connection-pool-trap-release-between-queries-20261118-090607-db-1 | postgres:17.11 |
| loadlore-connection-pool-trap-release-between-queries-20261118-090607-lb-1 | haproxy:3.2.26 |
| loadlore-connection-pool-trap-release-between-queries-20261118-090607-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-connection-pool-trap-release-between-queries-20261118-090607-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-connection-pool-trap-release-between-queries-20261118-090607-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-connection-pool-trap-release-between-queries-20261118-090607-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-connection-pool-trap-release-between-queries-20261118-090607-lore-toxiproxy-1 | ghcr.io/shopify/toxiproxy:2.13.0 |
| loadlore-connection-pool-trap-release-between-queries-20261118-090607-payments-0-1 | loadlore/synthetic:dev |

</details>

## Per-window data

<details><summary>42 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | db CPU | db conns | lb CPU | lb up | payments CPU | api→db pool | wait | api→payments pool | wait | k6 CPU | Toxiproxy CPU | k6 / Toxiproxy stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 122 | 118 | 0 | 0% | 85 ms | 88 ms | 3.9% | 3.3% | 6 | 3.3% | 4 | 4.1% | 1/40 | 0.0 ms | 12/200 | 0.0 ms | 3.4% | 1.8% | 0% / 0% |
| 0:10 | 165 | 164 | 0 | 0% | 85 ms | 88 ms | 4.7% | 3.5% | 9 | 3.0% | 4 | 5.0% | 1/40 | 0.0 ms | 15/200 | 0.0 ms | 2.2% | 2.3% | 0% / 0% |
| 0:20 | 208 | 207 | 0 | 0% | 85 ms | 88 ms | 5.4% | 3.5% | 9 | 3.4% | 4 | 5.7% | 1/40 | 0.0 ms | 19/200 | 0.0 ms | 2.6% | 2.8% | 0% / 0% |
| 0:30 | 252 | 250 | 0 | 0% | 85 ms | 88 ms | 6.3% | 3.7% | 9 | 3.8% | 4 | 6.5% | 1/40 | 0.0 ms | 22/200 | 0.0 ms | 2.8% | 3.3% | 0% / 0% |
| 0:40 | 295 | 294 | 0 | 0% | 85 ms | 88 ms | 7.2% | 3.8% | 9 | 4.3% | 4 | 7.7% | 1/40 | 0.0 ms | 26/200 | 0.0 ms | 3.2% | 3.8% | 0% / 0% |
| 0:50 | 338 | 337 | 0 | 0% | 85 ms | 88 ms | 7.9% | 3.9% | 9 | 4.6% | 4 | 8.3% | 1/40 | 0.0 ms | 28/200 | 0.0 ms | 3.9% | 4.1% | 0% / 0% |
| 1:00 | 382 | 381 | 0 | 0% | 85 ms | 88 ms | 8.7% | 3.9% | 9 | 5.1% | 4 | 9.1% | 1/40 | 0.0 ms | 32/200 | 0.0 ms | 3.6% | 4.4% | 0% / 0% |
| 1:10 | 425 | 424 | 0 | 0% | 85 ms | 88 ms | 9.4% | 4.1% | 9 | 5.4% | 4 | 10.0% | 1/40 | 0.0 ms | 36/200 | 0.0 ms | 4.2% | 4.8% | 0% / 0% |
| 1:20 | 468 | 467 | 0 | 0% | 85 ms | 88 ms | 10% | 4.2% | 10 | 5.9% | 4 | 11% | 2/40 | 0.0 ms | 40/200 | 0.0 ms | 4.4% | 5.2% | 0% / 0% |
| 1:30 | 512 | 510 | 0 | 0% | 85 ms | 88 ms | 11% | 4.3% | 10 | 6.3% | 4 | 12% | 2/40 | 0.0 ms | 43/200 | 0.0 ms | 4.9% | 5.6% | 0% / 0% |
| 1:40 | 555 | 554 | 0 | 0% | 85 ms | 88 ms | 12% | 4.5% | 10 | 6.7% | 4 | 13% | 2/40 | 0.0 ms | 47/200 | 0.0 ms | 5.1% | 5.9% | 0% / 0% |
| 1:50 | 598 | 597 | 0 | 0% | 85 ms | 88 ms | 13% | 4.6% | 10 | 7.3% | 4 | 13% | 1/40 | 0.0 ms | 48/200 | 0.0 ms | 5.3% | 6.4% | 0% / 0% |
| 2:00 | 642 | 640 | 0 | 0% | 85 ms | 88 ms | 14% | 4.8% | 10 | 7.5% | 4 | 14% | 1/40 | 0.0 ms | 54/200 | 0.0 ms | 5.7% | 6.7% | 0% / 0% |
| 2:10 | 685 | 684 | 0 | 0% | 85 ms | 88 ms | 14% | 4.8% | 10 | 8.0% | 4 | 15% | 2/40 | 0.0 ms | 57/200 | 0.0 ms | 5.9% | 7.1% | 0% / 0% |
| 2:20 | 728 | 727 | 0 | 0% | 85 ms | 88 ms | 15% | 5.1% | 10 | 8.4% | 4 | 16% | 3/40 | 0.0 ms | 60/200 | 0.0 ms | 5.9% | 7.4% | 0% / 0% |
| 2:30 | 772 | 770 | 0 | 0% | 85 ms | 88 ms | 16% | 5.3% | 11 | 9.0% | 4 | 17% | 3/40 | 0.0 ms | 63/200 | 0.0 ms | 6.8% | 7.8% | 0% / 0% |
| 2:40 | 815 | 814 | 0 | 0% | 85 ms | 88 ms | 17% | 5.3% | 13 | 9.3% | 4 | 18% | 4/40 | 0.0 ms | 68/200 | 0.0 ms | 7.2% | 8.2% | 0% / 0% |
| 2:50 | 858 | 857 | 0 | 0% | 85 ms | 88 ms | 18% | 5.5% | 13 | 9.9% | 4 | 19% | 2/40 | 0.0 ms | 71/200 | 0.0 ms | 7.4% | 8.5% | 0% / 0% |
| 3:00 | 902 | 901 | 0 | 0% | 85 ms | 88 ms | 19% | 5.5% | 13 | 10% | 4 | 20% | 3/40 | 0.0 ms | 74/200 | 0.0 ms | 7.9% | 8.8% | 0% / 0% |
| 3:10 | 945 | 943 | 0 | 0% | 85 ms | 88 ms | 20% | 5.8% | 13 | 11% | 4 | 21% | 1/40 | 0.0 ms | 79/200 | 0.0 ms | 8.0% | 9.1% | 0% / 0% |
| 3:20 | 988 | 987 | 0 | 0% | 85 ms | 88 ms | 20% | 5.9% | 13 | 11% | 4 | 21% | 2/40 | 0.0 ms | 81/200 | 0.0 ms | 8.1% | 9.3% | 0% / 0% |
| 3:30 | 1,032 | 1,031 | 0 | 0% | 85 ms | 88 ms | 21% | 6.1% | 13 | 12% | 4 | 22% | 2/40 | 0.0 ms | 85/200 | 0.0 ms | 8.5% | 9.6% | 0% / 0% |
| 3:40 | 1,075 | 1,074 | 0 | 0% | 85 ms | 88 ms | 22% | 6.0% | 13 | 12% | 4 | 23% | 2/40 | 0.0 ms | 88/200 | 0.0 ms | 8.9% | 10.0% | 0% / 0% |
| 3:50 | 1,118 | 1,117 | 0 | 0% | 85 ms | 88 ms | 23% | 6.4% | 13 | 13% | 4 | 24% | 1/40 | 0.0 ms | 91/200 | 0.0 ms | 9.1% | 10% | 0% / 0% |
| 4:00 | 1,162 | 1,160 | 0 | 0% | 85 ms | 88 ms | 24% | 6.5% | 13 | 13% | 4 | 25% | 3/40 | 0.0 ms | 95/200 | 0.0 ms | 9.0% | 11% | 0% / 0% |
| 4:10 | 1,205 | 1,204 | 0 | 0% | 85 ms | 88 ms | 25% | 6.7% | 14 | 14% | 4 | 25% | 2/40 | 0.0 ms | 99/200 | 0.0 ms | 9.1% | 11% | 0% / 0% |
| 4:20 | 1,248 | 1,247 | 0 | 0% | 85 ms | 88 ms | 26% | 6.7% | 14 | 14% | 4 | 26% | 2/40 | 0.0 ms | 103/200 | 0.0 ms | 9.7% | 11% | 0% / 0% |
| 4:30 | 1,292 | 1,290 | 0 | 0% | 85 ms | 88 ms | 27% | 7.0% | 14 | 15% | 4 | 27% | 3/40 | 0.0 ms | 107/200 | 0.0 ms | 10% | 12% | 0% / 0% |
| 4:40 | 1,335 | 1,334 | 0 | 0% | 85 ms | 88 ms | 28% | 7.2% | 15 | 15% | 4 | 28% | 3/40 | 0.0 ms | 110/200 | 0.0 ms | 10% | 12% | 0% / 0% |
| 4:50 | 1,378 | 1,377 | 0 | 0% | 85 ms | 88 ms | 31% | 8.9% | 15 | 18% | 4 | 32% | 2/40 | 0.0 ms | 113/200 | 0.0 ms | 12% | 14% | 0% / 0% |
| 5:00 | 1,400 | 1,400 | 0 | 0% | 85 ms | 88 ms | 35% | 11% | 16 | 24% | 4 | 37% | 2/40 | 0.0 ms | 114/200 | 0.0 ms | 15% | 18% | 0% / 0% |
| 5:10 | 1,400 | 1,400 | 0 | 0% | 85 ms | 88 ms | 36% | 11% | 16 | 25% | 4 | 38% | 2/40 | 0.0 ms | 115/200 | 0.0 ms | 16% | 19% | 0% / 0% |
| 5:20 | 1,400 | 1,400 | 0 | 0% | 85 ms | 88 ms | 36% | 12% | 16 | 26% | 4 | 39% | 2/40 | 0.0 ms | 114/200 | 0.0 ms | 16% | 20% | 0% / 0% |
| 5:30 | 1,400 | 1,400 | 0 | 0% | 85 ms | 88 ms | 31% | 8.4% | 16 | 18% | 4 | 31% | 2/40 | 0.0 ms | 114/200 | 0.0 ms | 12% | 14% | 0% / 0% |
| 5:40 | 1,400 | 1,400 | 0 | 0% | 85 ms | 88 ms | 32% | 8.9% | 16 | 19% | 4 | 33% | 2/40 | 0.0 ms | 114/200 | 0.0 ms | 13% | 15% | 0% / 0% |
| 5:50 | 1,400 | 1,400 | 0 | 0% | 85 ms | 88 ms | 31% | 8.6% | 16 | 18% | 4 | 32% | 2/40 | 0.0 ms | 114/200 | 0.0 ms | 12% | 14% | 0% / 0% |
| 6:00 | 1,400 | 1,400 | 0 | 0% | 85 ms | 88 ms | 31% | 8.5% | 16 | 18% | 4 | 32% | 3/40 | 0.0 ms | 114/200 | 0.0 ms | 12% | 14% | 0% / 0% |
| 6:10 | 1,400 | 1,400 | 0 | 0% | 85 ms | 88 ms | 31% | 8.6% | 16 | 18% | 4 | 32% | 2/40 | 0.0 ms | 114/200 | 0.0 ms | 12% | 14% | 0% / 0% |
| 6:20 | 1,400 | 1,400 | 0 | 0% | 85 ms | 88 ms | 31% | 8.7% | 17 | 18% | 4 | 32% | 2/40 | 0.0 ms | 114/200 | 0.0 ms | 13% | 14% | 0% / 0% |
| 6:30 | 1,400 | 1,400 | 0 | 0% | 85 ms | 88 ms | 31% | 8.6% | 17 | 18% | 4 | 31% | 2/40 | 0.0 ms | 115/200 | 0.0 ms | 12% | 14% | 0% / 0% |
| 6:40 | 1,400 | 1,400 | 0 | 0% | 85 ms | 88 ms | 31% | 8.4% | 17 | 18% | 4 | 31% | 2/40 | 0.0 ms | 115/200 | 0.0 ms | 12% | 14% | 0% / 0% |
| 6:50 | 1,400 | 1,400 | 0 | 0% | 85 ms | 88 ms | 30% | 8.2% | 17 | 18% | 4 | 30% | 3/40 | 0.0 ms | 115/200 | 0.0 ms | 10% | 14% | 0% / 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 and Toxiproxy CPU are against their budgets.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: connection-pool-trap-release-between-queries

# The starting design with connections released between queries.
#
# A checkout service: every request reads the cart from Postgres, then asks
# the payment provider to authorize the card, and one in ten also writes
# the order. Like Rails and Django, the api's framework checks out a
# database connection when a request first needs one and keeps it until the
# response.

services:
  lb:
    kind: lb
    target: api
    health_check: { interval_ms: 2000, fall: 3, rise: 2 }

  api:
    image: loadlore/synthetic
    replicas: 4
    resources: { cpu: 0.6, memory: 512m }
    behavior:
      cpu_ms: 0.3
      calls:
        - { target: db, op: read }                          # the cart
        - { target: payments, op: http }                    # authorize the card
        - { target: db, op: write, when: write }            # the order
    pools:
      db: { size: 10, hold: call }      # take a connection for each query, then hand it back
      payments: 50

  payments:
    image: loadlore/synthetic
    resources: { cpu: 0.5, memory: 256m }
    behavior:
      cpu_ms: 0.05

  db:
    kind: postgres
    version: "17"
    resources: { cpu: 1, memory: 1g }
    config: { max_connections: 100, shared_buffers: 256MB }
    dataset: { rows: 1000000, row_bytes: 512 }
    disk: { profile: gp3 }

workload:
  entry: lb
  profile:
    - ramp: { from: 100, to: 1400, over: 5m }
    - hold: { rps: 1400, for: 2m }
  mix: { read: 0.9, write: 0.1 }
  keys: { distribution: zipf, skew: 1.1, count: 1000000 }

# The payment provider is outside your system and answers in about 80 ms,
# modeled as network latency on the way to it for the whole run.
scenario:
  - { at: 0s, inject: latency, target: payments, add_ms: 80 }

# Your predictions, scored in the report. Replace these with your own
# before you run it.
expect:
  - p99_ms < 300 until load_rps >= 1000
  - error_rate < 1% throughout
```
