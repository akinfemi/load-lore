# featured-on-hacker-news-scale-on-inflight: run report

_run `featured-on-hacker-news-scale-on-inflight-20261115-140046` · completed · 15 Nov 2026 14:00 UTC · scale 1 · seed 1_

## Verdict

**Broke.** Broke 1:20 into the load, at 1,089 rps offered: p99 reached 1.11 s (2.7 ms at low load) and only 903 rps succeeded. Most saturated then: api CPU at 100% of its limit. After the break the load generator couldn't send 12% of the arrivals the profile asked for (20,844 arrivals from 1:30; k6_dropped_iterations_total (k6: arrivals it couldn't start)): slow responses held its 3,600 VUs, so served throughput from then on is partly the generator's limit, not only the system's. It recovered at 2:20, 1:00 after the break, and stayed healthy to the end.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); saturation: container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit. Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 1:20

Measured in the 10-second window from 1:20 to 1:30 (the break). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 1,800 rps<br/>offered 1,089 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 14%<br/>2/6 replicas up"}}
    api["api ×2<br/>0.5 CPU · 256m<br/>autoscale 2–6 on 10 in flight<br/>CPU 100%<br/>in flight 396"]
    db[("db<br/>postgres 17<br/>1 CPU · 1g<br/>CPU 6.3%<br/>conns 21/100")]
    load -->|"90% read · 10% write<br/>903 rps · p99 1.11 s"| lb
    lb -->|"round-robin<br/>checks every 1s, out after 3<br/>903 rps · p99 1.47 s"| api
    api -->|"read<br/>pool 10<br/>902 calls/s · p99 498 ms<br/>in use 19/20, 378 waiting"| db
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
    y-axis "requests/s" 0 --> 2500
    line [0, 0, 0, 0, 0, 0, 0, 0, 2500, 2500, 0, 0, 2500, 0, 0, 0, 0, 0, 0]
    line [200, 200, 200, 200, 200, 200, 377.778, 733.333, 1088.889, 1444.444, 1755.556, 1800, 1800, 1800, 1800, 1800, 1800, 1800, 1800]
    line [196.695, 200.025, 199.838, 200.098, 199.973, 199.943, 371.767, 727.16, 903.244, 942.101, 948.677, 947.292, 1912.048, 1881.412, 1800.051, 1799.918, 1800.036, 1799.92, 1800.021]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 1:30 api 2 → 6 replicas (105.9 requests in flight per replica, target 10); 2:00 api-4 start; 2:00 api-5 start; 2:00 api-3 start; 2:00 api-2 start; 2:02 api-2 took traffic, 31.8 s after the decision; 2:02 api-3 took traffic, 31.8 s after the decision; 2:02 api-4 took traffic, 31.9 s after the decision; 2:02 api-5 took traffic, 31.9 s after the decision; 1:20 break.</sub>

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
    y-axis "ms" 0 --> 5000
    line [0, 0, 0, 0, 0, 0, 0, 0, 5000, 5000, 0, 0, 5000, 0, 0, 0, 0, 0, 0]
    line [1.688, 1.637, 1.7, 1.661, 1.695, 1.654, 1.371, 1.325, 567.931, 1752.844, 3672.664, 3833.746, 2152.189, 1.43, 1.379, 1.377, 1.377, 1.377, 1.375]
    line [2.289, 2.26, 2.447, 2.272, 2.426, 2.415, 1.769, 1.579, 1055.762, 2558.091, 3967.266, 3983.375, 3954.013, 1295.792, 1.969, 1.913, 1.847, 2.001, 2.029]
    line [2.765, 2.525, 2.736, 2.525, 2.753, 2.679, 2.116, 4.564, 1109.663, 2766.903, 3993.453, 3996.675, 3990.803, 1772.422, 3.894, 3.156, 2.581, 3.491, 3.975]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 1:30 api 2 → 6 replicas (105.9 requests in flight per replica, target 10); 2:00 api-4 start; 2:00 api-5 start; 2:00 api-3 start; 2:00 api-2 start; 2:02 api-2 took traffic, 31.8 s after the decision; 2:02 api-3 took traffic, 31.8 s after the decision; 2:02 api-4 took traffic, 31.9 s after the decision; 2:02 api-5 took traffic, 31.9 s after the decision; 1:20 break.</sub>

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
    line [0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0]
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 1:30 api 2 → 6 replicas (105.9 requests in flight per replica, target 10); 2:00 api-4 start; 2:00 api-5 start; 2:00 api-3 start; 2:00 api-2 start; 2:02 api-2 took traffic, 31.8 s after the decision; 2:02 api-3 took traffic, 31.8 s after the decision; 2:02 api-4 took traffic, 31.9 s after the decision; 2:02 api-5 took traffic, 31.9 s after the decision; 1:20 break.</sub>

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
    line [0, 0, 0, 0, 0, 0, 0, 0, 100, 100, 0, 0, 100, 0, 0, 0, 0, 0, 0]
    line [30.138, 28.972, 29.794, 29.566, 29.723, 29.458, 45.68, 85.319, 100.346, 99.912, 100.032, 100.004, 72.863, 74.03, 72.811, 72.636, 72.603, 72.511, 72.442]
    line [7.227, 6.354, 5.91, 6.535, 5.711, 5.658, 4.316, 5.001, 6.268, 6.25, 6.293, 6.242, 11.026, 9.541, 8.834, 8.714, 8.81, 8.664, 8.768]
    line [9.053, 8.687, 9.465, 8.468, 9.609, 8.811, 7.732, 10.395, 14.118, 17.752, 13.4, 13.036, 27.328, 24.148, 21.211, 22.054, 23.646, 20.716, 20.84]
```

<sub>Blue: api. Orange: db. Green: lb (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 1:30 api 2 → 6 replicas (105.9 requests in flight per replica, target 10); 2:00 api-4 start; 2:00 api-5 start; 2:00 api-3 start; 2:00 api-2 start; 2:02 api-2 took traffic, 31.8 s after the decision; 2:02 api-3 took traffic, 31.8 s after the decision; 2:02 api-4 took traffic, 31.9 s after the decision; 2:02 api-5 took traffic, 31.9 s after the decision; 1:20 break.</sub>

## Diagnosis

### 1. Connection storm (api → db), from 1:10

At 1:10, api's pools opened 10 connections to db within 10 seconds, at 733 rps offered, and p99 reached 1.11 s (2.7 ms at low load). The pools open connections only when callers need them, so when a short slowdown leaves requests waiting, every replica opens more at once. Each new Postgres connection starts a server process and authenticates, which takes db's CPU from queries just when it is short of it.

Evidence:

- 10 new connections to db from 1:10 to 1:20: from 6 open to 8 of 20 <sub>(loadlore_pool_connect_seconds (new connections the pools opened, and how long each took))</sub>
- p99 1.11 s at 1:20, against 2.7 ms at low load <sub>(k6_http_req_duration_seconds (k6 histogram, end to end))</sub>
- each new connection took 6.7 ms to open at 1:10, against 4.9 ms for the ones opened before <sub>(loadlore_pool_connect_seconds (new connections the pools opened, and how long each took))</sub>
- db CPU at 5.0% of its limit at 1:10 <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>

**Try next:** Open the connections before the load arrives: give the pool a minimum (pools: { db: { size: 10, min: 10 } }) and re-run to compare a warm pool with this cold one. PgBouncer in front of db also keeps server connections open.

### 2. Autoscale lag (api), from 1:20

At the break, 1:20, api ran 2 of up to 6 replicas, their CPU at 100% of the limit: the load grew faster than the autoscaler added capacity. It reached 6 at 1:30.

Evidence:

- api CPU at 100% of its limit at 1:20, over 2 replicas <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- autoscale 1:30 api 2 → 6 replicas (105.9 requests in flight per replica, target 10) <sub>(events.jsonl (Loadlore's autoscaler))</sub>
- 2:02 api-2 took traffic, 31.8 s after the decision <sub>(events.jsonl (Loadlore's autoscaler))</sub>
- 2:02 api-3 took traffic, 31.8 s after the decision <sub>(events.jsonl (Loadlore's autoscaler))</sub>
- 2:02 api-4 took traffic, 31.9 s after the decision <sub>(events.jsonl (Loadlore's autoscaler))</sub>
- 2:02 api-5 took traffic, 31.9 s after the decision <sub>(events.jsonl (Loadlore's autoscaler))</sub>

**Try next:** Let api's autoscaler react sooner and in bigger steps: decide more often (interval_s), and keep more replicas running before a spike you expect (min).

### 3. Queueing knee (db, slower without running out of CPU), from 1:20

From 1:20, p99 stayed above 13 ms (2.7 ms at low load): db answered more slowly while its CPU stayed at most 6.3% of its limit, so callers held their connections longer and queued for them. The slowdown wasn't its CPU.

Evidence:

- 1:20 to 1:50: p99 1.11 s, 2.77 s, 3.99 s <sub>(k6_http_req_duration_seconds (k6 histogram, end to end))</sub>
- api CPU at 100% of its limit at 1:20: not where requests queued, since most of their time went to calls to db <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- api's requests spent 53% of their time in calls to db at 1:20: 1 calls per request × 315 ms each, against 593 ms per request <sub>(loadlore_calls_total (the caller's outbound call attempts); loadlore_call_duration_seconds (the caller's histogram, pool wait included); loadlore_request_duration_seconds (the service's own histogram))</sub>
- api's calls to db took 9.7 ms each once they had a connection at 1:20, against 0.3 ms at low load <sub>(loadlore_call_duration_seconds minus loadlore_pool_wait_seconds, per call (time with a connection))</sub>
- db CPU at most 6.3% of its limit from 1:20 to 1:50 <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>

**Try next:** Find what db waited on besides CPU, such as disk writes or locks, in its own logs and metrics, then re-run the ramp.

<sub>Rules version 12. Rules are deterministic and offline; their thresholds are listed below.</sub>

## Predictions

1 of 1 scored prediction held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `p99_ms < 500 until load_rps >= 1000` | **held** | 4.6 ms | 0:00 to 1:20, when load_rps >= 1,000 rps first held | source: k6_http_req_duration_seconds (k6 histogram, end to end) |

Loads and request rates depend on this machine: slower cores break the same design at a lower load, so a load guess can miss even when the reasoning behind it was right.

## Timeline

| Time | Event |
| --- | --- |
| 1:30 | autoscale: api 2 → 6 replicas (105.9 requests in flight per replica, target 10) |
| 2:00 | container: api-4 start |
| 2:00 | container: api-5 start |
| 2:00 | container: api-3 start |
| 2:00 | container: api-2 start |
| 2:02 | replica ready: api-2 took traffic, 31.8 s after the decision |
| 2:02 | replica ready: api-3 took traffic, 31.8 s after the decision |
| 2:02 | replica ready: api-4 took traffic, 31.9 s after the decision |
| 2:02 | replica ready: api-5 took traffic, 31.9 s after the decision |

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
| services.api.autoscale | 10 requests in flight per replica, 2 to 6 replicas (starts with 2), every 15 s, scale-up cooldown 0 s, scale-down cooldown 300 s | as a Kubernetes HorizontalPodAutoscaler: it wants ceil(replicas taking traffic × metric ÷ target), ignores changes within 10%, and goes on deciding while new replicas start, adding more if it wants more; it scales down only once none are starting. Unset values are Kubernetes' defaults: 15 s, no wait to scale up, 300 s before scaling down |
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
- Host speed index 986 (hash 946, memory 1021, syscalls 993): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (986) was within 5.0% of the run it's compared with (987)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 197,605 requests, 21,387 dropped arrivals, 0% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261115-140046-api-0-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261115-140046-api-1-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261115-140046-api-2-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261115-140046-api-3-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261115-140046-api-4-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261115-140046-api-5-1 | loadlore/synthetic:dev |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261115-140046-db-1 | postgres:17.11 |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261115-140046-lb-1 | haproxy:3.2.26 |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261115-140046-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261115-140046-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261115-140046-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261115-140046-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-featured-on-hacker-news-scale-on-inflight-20261115-140046-lore-toxiproxy-1 | ghcr.io/shopify/toxiproxy:2.13.0 |

</details>

## Per-window data

<details><summary>19 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | api replicas | db CPU | db conns | lb CPU | lb up | api→db pool | wait | k6 CPU | Toxiproxy CPU | k6 / Toxiproxy stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 200 | 197 | 0 | 0% | 1.7 ms | 2.8 ms | 30% | 2 | 7.2% | 5 | 9.1% | 2 | 0/20 | 0.0 ms | 6.7% | 9.6% | 1.0% / 0% |
| 0:10 | 200 | 200 | 0 | 0% | 1.6 ms | 2.5 ms | 29% | 2 | 6.4% | 5 | 8.7% | 2 | 0/20 | 0.0 ms | 5.6% | 6.6% | 0% / 0% |
| 0:20 | 200 | 200 | 0 | 0% | 1.7 ms | 2.7 ms | 30% | 2 | 5.9% | 5 | 9.5% | 2 | 0/20 | 0.0 ms | 5.3% | 7.1% | 0% / 0% |
| 0:30 | 200 | 200 | 0 | 0% | 1.7 ms | 2.5 ms | 30% | 2 | 6.5% | 5 | 8.5% | 2 | 0/20 | 0.0 ms | 5.5% | 6.6% | 0% / 0% |
| 0:40 | 200 | 200 | 0 | 0% | 1.7 ms | 2.8 ms | 30% | 2 | 5.7% | 5 | 9.6% | 2 | 0/20 | 0.0 ms | 5.6% | 7.2% | 0% / 0% |
| 0:50 | 200 | 200 | 0 | 0% | 1.7 ms | 2.7 ms | 29% | 2 | 5.7% | 5 | 8.8% | 2 | 0/20 | 0.0 ms | 5.6% | 7.1% | 0% / 0% |
| 1:00 | 378 | 372 | 0 | 0% | 1.4 ms | 2.1 ms | 46% | 2 | 4.3% | 7 | 7.7% | 2 | 0/20 | 0.0 ms | 5.1% | 6.8% | 0% / 0% |
| 1:10 | 733 | 727 | 0 | 0% | 1.3 ms | 4.6 ms | 85% | 2 | 5.0% | 9 | 10% | 2 | 1/20 | 0.0 ms | 7.0% | 11% | 0% / 0% |
| 1:20 | 1,089 | 903 | 543 | 0% | 568 ms | 1.11 s | 100% | 2 | 6.3% | 21 | 14% | 2 | 19/20 +378 | 306 ms | 9.0% | 15% | 0% / 0% |
| 1:30 | 1,444 | 942 | 2,855 | 0% | 1.75 s | 2.77 s | 100% | 2 | 6.2% | 21 | 18% | 2 | 20/20 +378 | 397 ms | 15% | 18% | 2.0% / 0% |
| 1:40 | 1,756 | 949 | 7,854 | 0% | 3.67 s | 3.99 s | 100% | 2 | 6.3% | 21 | 13% | 2 | 18/20 +378 | 395 ms | 7.4% | 13% | 0% / 0% |
| 1:50 | 1,800 | 947 | 8,501 | 0% | 3.83 s | 4.00 s | 100% | 2 | 6.2% | 21 | 13% | 2 | 18/20 +379 | 394 ms | 9.0% | 12% | 1.7% / 0% |
| 2:00 | 1,800 | 1,912 | 1,633 | 0% | 2.15 s | 3.99 s | 73% | 6 | 11% | 31 | 27% | 2 | 17/60 +378 | 195 ms | 14% | 24% | 0.3% / 0% |
| 2:10 | 1,800 | 1,881 | 0 | 0% | 1.4 ms | 1.77 s | 74% | 6 | 9.5% | 46 | 24% | 6 | 13/60 +378 | 28 ms | 16% | 27% | 2.0% / 0% |
| 2:20 | 1,800 | 1,800 | 0 | 0% | 1.4 ms | 3.9 ms | 73% | 6 | 8.8% | 49 | 21% | 6 | 3/60 | 0.0 ms | 16% | 26% | 2.0% / 0% |
| 2:30 | 1,800 | 1,800 | 0 | 0% | 1.4 ms | 3.2 ms | 73% | 6 | 8.7% | 49 | 22% | 6 | 2/60 | 0.0 ms | 16% | 25% | 1.0% / 0% |
| 2:40 | 1,800 | 1,800 | 0 | 0% | 1.4 ms | 2.6 ms | 73% | 6 | 8.8% | 49 | 24% | 6 | 2/60 | 0.0 ms | 15% | 25% | 0.5% / 0% |
| 2:50 | 1,800 | 1,800 | 0 | 0% | 1.4 ms | 3.5 ms | 73% | 6 | 8.7% | 52 | 21% | 6 | 2/60 | 0.0 ms | 15% | 25% | 0.5% / 0% |
| 3:00 | 1,800 | 1,800 | 0 | 0% | 1.4 ms | 4.0 ms | 72% | 6 | 8.8% | 52 | 21% | 6 | 2/60 | 0.0 ms | 16% | 25% | 1.0% / 0% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 and Toxiproxy CPU are against their budgets.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: featured-on-hacker-news-scale-on-inflight

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
    replicas: 2
    resources: { cpu: 0.5, memory: 256m }
    behavior:
      cpu_ms: 1
      calls:
        - { target: db, op: read }
      resilience:
        timeout_ms: 2000
    pools: { db: 10 }
    autoscale: { max: 6, metric: inflight, target: 10 }   # was metric: cpu, target: 0.7

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
