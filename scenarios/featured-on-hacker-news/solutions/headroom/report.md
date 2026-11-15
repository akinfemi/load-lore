# featured-on-hacker-news-headroom: run report

_run `featured-on-hacker-news-headroom-20261115-135703` · completed · 15 Nov 2026 13:57 UTC · scale 1 · seed 1_

## Verdict

**Held.** Held the whole profile: up to 1,800 rps offered, 1,806 rps served at best, worst 10-second p99 70 ms, at most 0% errors.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6). Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 2:00

Measured in the 10-second window from 2:00 to 2:10 (the busiest window: the most successful responses). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 1,800 rps<br/>offered 1,800 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 23%<br/>4/6 replicas up"}}
    api["api ×4<br/>0.5 CPU · 256m<br/>autoscale 4–6 on CPU 70%<br/>CPU 100%<br/>in flight 48"]
    db[("db<br/>postgres 17<br/>1 CPU · 1g<br/>CPU 11%<br/>conns 41/100")]
    load -->|"90% read · 10% write<br/>1,806 rps · p99 70 ms"| lb
    lb -->|"round-robin<br/>checks every 1s, out after 3<br/>1,805 rps · p99 70 ms"| api
    api -->|"read<br/>pool 10<br/>1,799 calls/s · p99 15 ms<br/>in use 22/40, 13 waiting"| db
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb,db ok
    class api hot
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
    x-axis "seconds" 0 --> 180
    y-axis "requests/s" 0 --> 2000
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2000, 0, 0, 2000, 0, 0, 0, 0, 0]
    line [200, 200, 200, 200, 200, 200, 377.778, 733.333, 1088.889, 1444.444, 1755.556, 1800, 1800, 1800, 1800, 1800, 1800, 1800, 1800]
    line [196.825, 200.123, 199.829, 200.093, 199.879, 200.13, 372.061, 727.554, 1083.143, 1438.564, 1752.656, 1793.627, 1806.375, 1799.892, 1800.22, 1799.87, 1799.916, 1800.173, 1799.946]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 1:45 api 4 → 6 replicas (CPU 91% of its limit, target 70%); 2:15 api-5 start; 2:15 api-4 start; 2:17 api-4 took traffic, 31.8 s after the decision; 2:17 api-5 took traffic, 31.8 s after the decision.</sub>

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
    x-axis "seconds" 0 --> 180
    y-axis "ms" 0 --> 80
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 80, 0, 0, 80, 0, 0, 0, 0, 0]
    line [1.648, 1.632, 1.626, 1.633, 1.712, 1.637, 1.367, 1.329, 1.333, 1.338, 1.5, 11.998, 12.765, 2.941, 1.375, 1.373, 1.379, 1.374, 1.379]
    line [2.296, 2.39, 2.29, 2.391, 2.347, 2.362, 1.83, 1.572, 1.623, 1.675, 49.965, 58.564, 59.665, 58.9, 1.91, 1.917, 1.924, 1.912, 1.949]
    line [2.772, 2.755, 2.667, 2.692, 2.801, 2.987, 2.405, 2.342, 2.984, 2.664, 64.366, 69.035, 70.036, 70.369, 2.656, 2.662, 2.707, 2.926, 2.829]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 1:45 api 4 → 6 replicas (CPU 91% of its limit, target 70%); 2:15 api-5 start; 2:15 api-4 start; 2:17 api-4 took traffic, 31.8 s after the decision; 2:17 api-5 took traffic, 31.8 s after the decision.</sub>

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
    x-axis "seconds" 0 --> 180
    y-axis "% of requests" 0 --> 1
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 1, 0, 0, 0, 0, 0]
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 1:45 api 4 → 6 replicas (CPU 91% of its limit, target 70%); 2:15 api-5 start; 2:15 api-4 start; 2:17 api-4 took traffic, 31.8 s after the decision; 2:17 api-5 took traffic, 31.8 s after the decision.</sub>

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
    x-axis "seconds" 0 --> 180
    y-axis "% of limit" 0 --> 100
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 100, 0, 0, 0, 0, 0]
    line [15.143, 15.492, 15.25, 15.338, 15.444, 15.568, 24.089, 44.126, 64.878, 85.804, 99.849, 99.97, 100.025, 68.704, 72.491, 72.678, 72.681, 72.654, 72.723]
    line [4.821, 5.794, 6.304, 5.843, 5.139, 5.806, 4.427, 4.902, 5.89, 6.978, 10.522, 10.702, 10.598, 10.265, 8.513, 8.673, 8.512, 8.713, 8.75]
    line [8.586, 8.772, 8.317, 8.522, 8.761, 8.519, 8.18, 12.148, 17.374, 20.152, 22.302, 23.324, 23.307, 23.537, 22.935, 23.153, 27.886, 27.722, 27.522]
```

<sub>Blue: api. Orange: db. Green: lb (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 1:45 api 4 → 6 replicas (CPU 91% of its limit, target 70%); 2:15 api-5 start; 2:15 api-4 start; 2:17 api-4 took traffic, 31.8 s after the decision; 2:17 api-5 took traffic, 31.8 s after the decision.</sub>

## Diagnosis

### 1. Connection storm (api → db), from 1:40

At 1:40, api's pools opened 26 connections to db within 10 seconds, at 1,756 rps offered, and p99 reached 69 ms (2.8 ms at low load). The pools open connections only when callers need them, so when a short slowdown leaves requests waiting, every replica opens more at once. Each new Postgres connection starts a server process and authenticates, which takes db's CPU from queries just when it is short of it.

Evidence:

- 26 new connections to db from 1:40 to 1:50: from 12 open to 40 of 40 <sub>(loadlore_pool_connect_seconds (new connections the pools opened, and how long each took))</sub>
- p99 69 ms at 1:50, against 2.8 ms at low load <sub>(k6_http_req_duration_seconds (k6 histogram, end to end))</sub>
- each new connection took 5.0 ms to open at 1:40, against 3.4 ms for the ones opened before <sub>(loadlore_pool_connect_seconds (new connections the pools opened, and how long each took))</sub>
- up to 9 requests waiting for a connection at 1:40 <sub>(loadlore_pool_pending (callers waiting for a connection))</sub>
- db CPU at 11% of its limit at 1:40 <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- api's calls to db took 1.5 ms each once they had a connection at 1:40, against 0.3 ms at low load <sub>(loadlore_call_duration_seconds minus loadlore_pool_wait_seconds, per call (time with a connection))</sub>

**Try next:** Open the connections before the load arrives: give the pool a minimum (pools: { db: { size: 10, min: 10 } }) and re-run to compare a warm pool with this cold one. PgBouncer in front of db also keeps server connections open.

### 2. Queueing knee (db, slower without running out of CPU), from 1:40

From 1:40, p99 stayed above 13 ms (2.8 ms at low load): db answered more slowly while its CPU stayed at most 11% of its limit, so callers held their connections longer and queued for them. The slowdown wasn't its CPU.

Evidence:

- 1:40 to 2:10: p99 64 ms, 69 ms, 70 ms <sub>(k6_http_req_duration_seconds (k6 histogram, end to end))</sub>
- api CPU at 100% of its limit at 1:40: not where requests queued, since most of their time went to calls to db <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- api's requests spent 51% of their time in calls to db at 1:50: 1 calls per request × 3.7 ms each, against 7.3 ms per request <sub>(loadlore_calls_total (the caller's outbound call attempts); loadlore_call_duration_seconds (the caller's histogram, pool wait included); loadlore_request_duration_seconds (the service's own histogram))</sub>
- api's calls to db took 2.4 ms each once they had a connection at 1:50, against 0.3 ms at low load <sub>(loadlore_call_duration_seconds minus loadlore_pool_wait_seconds, per call (time with a connection))</sub>
- db CPU at most 11% of its limit from 1:40 to 2:10 <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>

**Try next:** Find what db waited on besides CPU, such as disk writes or locks, in its own logs and metrics, then re-run the ramp.

<sub>Rules version 12. Rules are deterministic and offline; their thresholds are listed below.</sub>

## Predictions

1 of 1 scored prediction held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 500 until load_rps >= 1000` | **held** | 3.0 ms | 0:00 to 1:20, when load_rps >= 1,000 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |

Loads and request rates depend on this machine: slower cores break the same design at a lower load, so a load guess can miss even when the reasoning behind it was right.

## Timeline

| Time | Event |
| --- | --- |
| 1:45 | autoscale: api 4 → 6 replicas (CPU 91% of its limit, target 70%) |
| 2:15 | container: api-5 start |
| 2:15 | container: api-4 start |
| 2:17 | replica ready: api-4 took traffic, 31.8 s after the decision |
| 2:17 | replica ready: api-5 took traffic, 31.8 s after the decision |

<sub>From events.jsonl: the fault timeline and Docker container events.</sub>

## Assumptions

### From the design

| Field | Value | Basis | Why |
| --- | --- | --- | --- |
| `services.api.behavior.cpu_ms` |  | guessed | a JSON handler with some work in it: a replica of 0.5 CPU serves about 350 requests a second |
| `workload.profile` |  | guessed | a front-page surge: nine times the usual traffic in under a minute |
| `services.api.autoscale.startup_s` |  | guessed | a new pod waits about 30 s to be scheduled and pull its image |

3 of these are guessed, not measured: the result depends on them.

### Chosen by Loadlore

| Field | Value | Why |
| --- | --- | --- |
| workload.timeout_ms | 10s | the default; k6 gives up on a request after this long and counts it as failed |
| workload.slo.p99_ms | 500 ms | the default; a break on p99 alone needs p99 at or above this as well as 10x the low-load p99, so latency users would still accept isn't called a break |
| resources.cpu_period_ms | 100 ms for api, db, lb | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.api.autoscale | 70% CPU, 4 to 6 replicas (starts with 4), every 15 s, scale-up cooldown 0 s, scale-down cooldown 300 s | as a Kubernetes HorizontalPodAutoscaler: it wants ceil(replicas taking traffic × metric ÷ target), ignores changes within 10%, and goes on deciding while new replicas start, adding more if it wants more; it scales down only once none are starting. Unset values are Kubernetes' defaults: 15 s, no wait to scale up, 300 s before scaling down |
| services.api.autoscale.startup_s | 30 s | set in the design |
| services.db.disk | the Docker host's | no disk block: a read that misses db's memory goes to the Docker host's disk, which on a laptop mostly answers from the host's own memory, fast and uncapped. Add disk: {} for a cloud volume's wait and IOPS cap |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |
| services.api.pools.db.min | 0 | the default, as pgxpool's: each replica opens connections only when callers need them, so a pool fills as load rises, and a burst opens many at once |
| services.api.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- Loadlore's own containers get fixed budgets: load generator 1.5 CPU (peak 1800 rps), Toxiproxy 1 CPU (about 3600 proxied calls/s at peak), Prometheus 0.1, cAdvisor 0.1, exporters 0.1 each

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
- **Autoscale lag:** the run broke while an autoscaled service's replicas were at 90% or more of their CPU limit: below its max, the autoscaler hadn't caught up; at its max (autoscale ceiling), it had nothing left to add.
- **Hot rows:** for two windows in a row, 2 or more of a Postgres's backends waiting on locks on average, while a caller's writes to it took 3x their low-load time or more (and 2 ms more). Reads of the same rows may slow too, and the summary says when the database's CPU per request reached 3x its low-load value: the contention burning CPU. A queueing knee on that database's CPU at the same time then points at the rows. Windows with a disk_slow fault on the database don't count.

## Environment

- loadl dev, profile 3m15s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 999 (hash 976, memory 1019, syscalls 1004): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (999) was within 5.0% of the run it's compared with (987)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 218,994 requests, 0 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-featured-on-hacker-news-headroom-20261115-135703-api-0-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-headroom-20261115-135703-api-1-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-headroom-20261115-135703-api-2-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-headroom-20261115-135703-api-3-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-headroom-20261115-135703-api-4-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-headroom-20261115-135703-api-5-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-headroom-20261115-135703-db-1 | postgres:17.11 |
| loadlore-featured-on-hacker-news-headroom-20261115-135703-lb-1 | haproxy:3.2.26 |
| loadlore-featured-on-hacker-news-headroom-20261115-135703-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-featured-on-hacker-news-headroom-20261115-135703-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-featured-on-hacker-news-headroom-20261115-135703-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-featured-on-hacker-news-headroom-20261115-135703-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-featured-on-hacker-news-headroom-20261115-135703-lore-toxiproxy-1 | ghcr.io/shopify/toxiproxy:2.13.0 |

</details>

## Per-window data

<details><summary>19 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | api replicas | db CPU | db conns | lb CPU | lb up | api→db pool | wait | k6 CPU | Toxiproxy CPU | k6 / Toxiproxy stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 200 | 197 | 0 | 0% | 1.6 ms | 2.8 ms | 15% | 4 | 4.8% | 5 | 8.6% | 4 | 1/40 | 0.0 ms | 6.6% | 9.3% | 1.0% / 0% |
| 0:10 | 200 | 200 | 0 | 0% | 1.6 ms | 2.8 ms | 15% | 4 | 5.8% | 7 | 8.8% | 4 | 1/40 | 0.0 ms | 5.4% | 7.0% | 0% / 0% |
| 0:20 | 200 | 200 | 0 | 0% | 1.6 ms | 2.7 ms | 15% | 4 | 6.3% | 8 | 8.3% | 4 | 0/40 | 0.0 ms | 4.7% | 6.6% | 0% / 0% |
| 0:30 | 200 | 200 | 0 | 0% | 1.6 ms | 2.7 ms | 15% | 4 | 5.8% | 8 | 8.5% | 4 | 1/40 | 0.0 ms | 5.6% | 6.9% | 0% / 0% |
| 0:40 | 200 | 200 | 0 | 0% | 1.7 ms | 2.8 ms | 15% | 4 | 5.1% | 8 | 8.8% | 4 | 1/40 | 0.0 ms | 5.2% | 7.1% | 0% / 0% |
| 0:50 | 200 | 200 | 0 | 0% | 1.6 ms | 3.0 ms | 16% | 4 | 5.8% | 8 | 8.5% | 4 | 1/40 | 0.0 ms | 5.4% | 6.8% | 0% / 0% |
| 1:00 | 378 | 372 | 0 | 0% | 1.4 ms | 2.4 ms | 24% | 4 | 4.4% | 9 | 8.2% | 4 | 0/40 | 0.0 ms | 5.2% | 7.3% | 0% / 0% |
| 1:10 | 733 | 728 | 0 | 0% | 1.3 ms | 2.3 ms | 44% | 4 | 4.9% | 9 | 12% | 4 | 1/40 | 0.0 ms | 7.2% | 11% | 0% / 0% |
| 1:20 | 1,089 | 1,083 | 0 | 0% | 1.3 ms | 3.0 ms | 65% | 4 | 5.9% | 10 | 17% | 4 | 1/40 | 0.0 ms | 9.7% | 16% | 0% / 0% |
| 1:30 | 1,444 | 1,439 | 0 | 0% | 1.3 ms | 2.7 ms | 86% | 4 | 7.0% | 13 | 20% | 4 | 2/40 | 0.0 ms | 12% | 20% | 0% / 0% |
| 1:40 | 1,756 | 1,753 | 0 | 0% | 1.5 ms | 64 ms | 100% | 4 | 11% | 41 | 22% | 4 | 17/40 +9 | 0.7 ms | 14% | 23% | 0% / 0% |
| 1:50 | 1,800 | 1,794 | 0 | 0% | 12 ms | 69 ms | 100% | 4 | 11% | 41 | 23% | 4 | 17/40 +10 | 1.3 ms | 14% | 22% | 0% / 0% |
| 2:00 | 1,800 | 1,806 | 0 | 0% | 13 ms | 70 ms | 100% | 4 | 11% | 41 | 23% | 4 | 22/40 +13 | 1.5 ms | 14% | 22% | 0% / 0% |
| 2:10 | 1,800 | 1,800 | 0 | 0% | 2.9 ms | 70 ms | 69% | 6 | 10% | 44 | 24% | 4 | 15/60 +11 | 1.0 ms | 14% | 23% | 0% / 0% |
| 2:20 | 1,800 | 1,800 | 0 | 0% | 1.4 ms | 2.7 ms | 72% | 6 | 8.5% | 45 | 23% | 6 | 1/60 | 0.0 ms | 15% | 25% | 0% / 0% |
| 2:30 | 1,800 | 1,800 | 0 | 0% | 1.4 ms | 2.7 ms | 73% | 6 | 8.7% | 45 | 23% | 6 | 3/60 | 0.0 ms | 15% | 25% | 0% / 0% |
| 2:40 | 1,800 | 1,800 | 0 | 0% | 1.4 ms | 2.7 ms | 73% | 6 | 8.5% | 45 | 28% | 6 | 3/60 | 0.0 ms | 15% | 25% | 0% / 0% |
| 2:50 | 1,800 | 1,800 | 0 | 0% | 1.4 ms | 2.9 ms | 73% | 6 | 8.7% | 46 | 28% | 6 | 2/60 | 0.0 ms | 15% | 25% | 0% / 0% |
| 3:00 | 1,800 | 1,800 | 0 | 0% | 1.4 ms | 2.8 ms | 73% | 6 | 8.8% | 46 | 28% | 6 | 3/60 | 0.0 ms | 14% | 25% | 0% / 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 and Toxiproxy CPU are against their budgets.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: featured-on-hacker-news-headroom

# A small product's api, autoscaled on CPU as the platform's defaults
# suggest. It runs two replicas for its usual traffic. Then a post about
# the product reaches the front page of Hacker News.

services:
  lb:
    kind: lb
    target: api
    health_check: { interval_ms: 1000, fall: 3, rise: 2 }

  api:
    image: loadlore/synthetic
    replicas: 4   # was 2
    resources: { cpu: 0.5, memory: 256m }
    behavior:
      cpu_ms: 1
      calls:
        - { target: db, op: read }
      resilience:
        timeout_ms: 2000
    pools: { db: 10 }
    autoscale: { min: 4, max: 6, metric: cpu, target: 0.7 }   # was min 2: room for a surge before it scales

  db:
    kind: postgres
    version: "17"
    resources: { cpu: 1, memory: 1g }
    config: { max_connections: 100 }
    dataset: { rows: 100000, row_bytes: 256 }

workload:
  entry: lb
  profile:
    - hold: { rps: 200, for: 1m }                  # a normal afternoon
    - ramp: { from: 200, to: 1800, over: 45s }    # the post reaches the front page
    - hold: { rps: 1800, for: 1m30s }

assumptions:
  - { field: services.api.behavior.cpu_ms, basis: guessed, why: "a JSON handler with some work in it: a replica of 0.5 CPU serves about 350 requests a second" }
  - { field: workload.profile, basis: guessed, why: "a front-page surge: nine times the usual traffic in under a minute" }
  - { field: services.api.autoscale.startup_s, basis: guessed, why: "a new pod waits about 30 s to be scheduled and pull its image" }

# Your predictions, scored in the report. The line here is an example;
# loadl predict design.yaml replaces it with your answers.
expect:
  - p99_ms < 500 until load_rps >= 1000
```
