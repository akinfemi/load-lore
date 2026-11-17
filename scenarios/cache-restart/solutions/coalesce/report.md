# cache-restart-coalesce: run report

_run `cache-restart-coalesce-20261117-153908` · completed · 17 Nov 2026 15:39 UTC · scale 1 · seed 1_

## Verdict

**Broke.** Broke 3:00 into the load, at 1,500 rps offered: p99 reached 1.09 s (16 ms at low load), 26% of requests failed and only 1,085 rps succeeded. Of api's failed requests, 52% failed because calls to db ran past their timeout. Most saturated then: db CPU at 89% of its limit. Around then: restart cache at 3:01. It recovered at 3:50, 0:50 after the break, and stayed healthy to the end.

Caveat: at 3:00 the load generator hit its CPU limit and was stalled in 2.0% of its scheduling periods (container_cpu_cfs_throttled_periods_total ÷ container_cpu_cfs_periods_total (cAdvisor)); work caught in a stall waits for the next 100 ms period, which can show up in p99.

<sub>Sources: offered load: the load profile in plan.json (offered arrival rate); served: k6_http_reqs_total{expected_response="true"} (k6: successful responses); p99: k6_http_req_duration_seconds (k6 histogram, end to end); errors: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); saturation: container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit. Windows are 10 seconds; what counts as broken is under [Thresholds](#thresholds).</sub>

## Diagram at 3:00

Measured in the 10-second window from 3:00 to 3:10 (the break). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90% or more, amber from 70%, green below.

```mermaid
flowchart LR
    load(["load<br/>up to 1,500 rps<br/>offered 1,500 rps"])
    lb{{"lb<br/>load balancer<br/>0.5 CPU · 256m<br/>CPU 14%<br/>3/3 replicas up"}}
    api["api ×3<br/>0.5 CPU · 256m<br/>CPU 50%<br/>in flight 441"]
    cache[("cache<br/>redis 8<br/>0.5 CPU · 256m<br/>CPU 5.2%<br/>hit rate 71%")]
    db[("db<br/>postgres 17<br/>1 CPU · 1g<br/>CPU 89%<br/>conns 31/100")]
    load -->|"100% read · 0% write<br/>1,471 rps · p99 1.09 s · 26% errors"| lb
    lb -->|"round-robin<br/>checks every 2s, out after 3<br/>1,469 rps · p99 1.45 s · 26% errors"| api
    api -->|"get, set on miss<br/>pool 20<br/>1,506 calls/s · p99 55 ms · 0.2% errors<br/>in use 2/60, 13 waiting"| cache
    api -->|"read scan on miss<br/>pool 10<br/>158 calls/s · p99 1.49 s · 96% errors<br/>in use 23/30, 218 waiting"| db
    classDef ok fill:#d4edda,stroke:#28a745,color:#000
    classDef warm fill:#fff3cd,stroke:#d39e00,color:#000
    classDef hot fill:#f8d7da,stroke:#dc3545,color:#000
    class lb,cache ok
    class api,db warm
```

<sub>Sources: offered from the load profile in plan.json (offered arrival rate); load edge from k6_http_reqs_total (k6: all responses) and k6_http_req_duration_seconds (k6 histogram, end to end); CPU from container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit; calls/s from loadlore_calls_total (the caller's outbound call attempts) and their p99 from loadlore_call_duration_seconds (the caller's histogram, pool wait included); pools from loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections and loadlore_pool_pending (callers waiting for a connection); lb traffic from haproxy_backend_http_requests_total (HAProxy), replicas up from haproxy_backend_active_servers (HAProxy), and the lb edge's p99 from loadlore_request_duration_seconds (the service's own histogram); conns from pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter).</sub>

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
    y-axis "requests/s" 0 --> 2000
    line [2000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [72.083, 196.25, 320.417, 444.583, 568.75, 692.917, 817.083, 941.25, 1065.417, 1189.583, 1313.75, 1437.917, 1500, 1500, 1500, 1500, 1500, 1500, 1500, 1500, 1500, 1500, 1500, 1500, 1500, 1500, 1500, 1500, 1500, 1500]
    line [64.826, 189.203, 313.65, 437.675, 561.853, 685.961, 810.208, 934.331, 1058.523, 1182.621, 1306.823, 1430.986, 1499.842, 1500, 1499.998, 1500, 1499.998, 1500.003, 1084.658, 1083.734, 1121.289, 1231.995, 1443.974, 1499.95, 1499.952, 1500.096, 1500.002, 1500, 1500.049, 1499.901]
```

<sub>Grey: offered load (the load profile in plan.json (offered arrival rate)). Blue: successful responses (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Pink spikes mark 0:00 stall; 3:01 cache kill; 3:01 cache die (exit code 0); 3:01 restart cache; 3:01 cache start; 3:01 cache restart; 3:00 break.</sub>

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
    y-axis "ms" 0 --> 1200
    line [1200, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1200, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [1.725, 1.631, 1.509, 1.319, 1.311, 1.304, 1.302, 1.292, 1.292, 1.288, 1.285, 1.28, 1.273, 1.277, 1.273, 1.275, 1.276, 1.276, 1.339, 1.32, 1.325, 1.306, 1.268, 1.265, 1.274, 1.276, 1.274, 1.276, 1.277, 1.278]
    line [18.911, 1.907, 1.751, 1.44, 1.421, 1.38, 1.381, 1.377, 1.38, 1.377, 1.379, 1.378, 1.376, 1.379, 1.376, 1.378, 1.377, 1.377, 1053.519, 1048.72, 1048.417, 1041.267, 126.754, 1.373, 1.376, 1.376, 1.374, 1.378, 1.376, 1.377]
    line [21.969, 15.971, 1.817, 1.505, 1.691, 1.493, 1.562, 1.485, 1.602, 1.483, 1.62, 1.714, 1.541, 1.847, 1.558, 1.854, 1.547, 1.693, 1090.224, 1082.15, 1082.09, 1080.66, 1053.609, 1.445, 1.541, 1.5, 1.451, 1.656, 1.483, 1.505]
```

<sub>Green: p50. Orange: p95. Red: p99 (k6_http_req_duration_seconds (k6 histogram, end to end)). Pink spikes mark 0:00 stall; 3:01 cache kill; 3:01 cache die (exit code 0); 3:01 restart cache; 3:01 cache start; 3:01 cache restart; 3:00 break.</sub>

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
    y-axis "% of requests" 0 --> 30
    line [30, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 30, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 26.257, 27.87, 25.381, 18.402, 4.638, 0, 0, 0, 0, 0, 0, 0]
```

<sub>Failed requests as a share of all responses (k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)), timeouts included. Pink spikes mark 0:00 stall; 3:01 cache kill; 3:01 cache die (exit code 0); 3:01 restart cache; 3:01 cache start; 3:01 cache restart; 3:00 break.</sub>

| Service | Failed requests | Share of its failures | Cause | From calls to | First seen | What it means |
| --- | --- | --- | --- | --- | --- | --- |
| api | 9,440 | 62% | `timeout` | db | 3:00 | calls to db ran past their timeout |
| api | 5,901 | 38% | `pool_timeout` | db | 3:00 | they timed out waiting for a pool connection to db |
| api | 3 | <0.1% | `conn_reset` | cache | 3:00 | connections to cache were reset or closed mid-call |

<sub>Why requests failed, over the whole run (loadlore_request_errors_total and loadlore_call_errors_total, by cause). A request that failed after retries counts once, by its last attempt's cause.</sub>

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
    x-axis "seconds" 0 --> 290
    y-axis "% of limit" 0 --> 100
    line [100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    line [3.997, 7.343, 10.281, 13.119, 16.498, 19.826, 23.178, 26.606, 30.093, 33.487, 36.993, 40.428, 42.353, 42.577, 42.364, 42.453, 42.304, 42.303, 49.802, 51.196, 51.194, 50.285, 46.197, 42.158, 42.183, 42.133, 42.11, 42.17, 42.24, 41.853]
    line [1.861, 2.018, 2.265, 2.349, 2.713, 2.761, 3.259, 3.516, 3.708, 3.981, 4.294, 4.433, 4.832, 5.022, 4.965, 4.915, 4.82, 4.539, 5.222, 5.335, 5.352, 5.366, 5.351, 4.685, 4.779, 4.666, 4.757, 4.671, 4.752, 4.757]
    line [21.121, 11.361, 3.852, 3.186, 2.634, 2.585, 2.548, 2.714, 2.602, 2.654, 2.506, 2.674, 2.531, 2.649, 2.488, 2.628, 2.573, 2.535, 88.593, 100.02, 100.201, 99.462, 61.34, 2.517, 2.582, 2.691, 2.531, 2.641, 2.56, 2.651]
    line [3.037, 5.478, 6.047, 5.739, 4.876, 6.653, 9.22, 7.438, 11.04, 8.199, 10.9, 10.823, 10.599, 11.739, 10.62, 12.361, 13.302, 10.736, 13.738, 12.708, 12.999, 12.818, 12.167, 10.479, 10.482, 11.53, 10.263, 13.979, 10.262, 11.305]
```

<sub>Blue: api. Orange: cache. Green: db. Purple: lb (container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window). Pink spikes mark 0:00 stall; 3:01 cache kill; 3:01 cache die (exit code 0); 3:01 restart cache; 3:01 cache start; 3:01 cache restart; 3:00 break.</sub>

## Diagnosis

### 1. Connection storm (api → db), from 3:00

At 3:00, api's pools opened 788 connections to db within 10 seconds, at 1,500 rps offered, and p99 reached 1.09 s (16 ms at low load). The pools open connections only when callers need them, so when a short slowdown leaves requests waiting, every replica opens more at once. Each new Postgres connection starts a server process and authenticates, which takes db's CPU from queries just when it is short of it.

Evidence:

- 788 new connections to db from 3:00 to 3:10: from 5 open to 23 of 30 <sub>(loadlore_pool_connect_seconds (new connections the pools opened, and how long each took))</sub>
- p99 1.09 s at 3:00, against 16 ms at low load <sub>(k6_http_req_duration_seconds (k6 histogram, end to end))</sub>
- each new connection took 102 ms to open at 3:00, against 3.5 ms for the ones opened before <sub>(loadlore_pool_connect_seconds (new connections the pools opened, and how long each took))</sub>
- up to 218 requests waiting for a connection at 3:00 <sub>(loadlore_pool_pending (callers waiting for a connection))</sub>
- db CPU at 89% of its limit at 3:00 <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- db was CPU-throttled for 55.6 s (summed over its threads) in the window from 3:00, while its average CPU was 89% of its limit: bursts of work hit the 1-CPU limit and waited for the next period <sub>(container_cpu_cfs_throttled_seconds_total (cAdvisor))</sub>
- api's calls to db took 50 ms each once they had a connection at 3:00, against 13 ms at low load <sub>(loadlore_call_duration_seconds minus loadlore_pool_wait_seconds, per call (time with a connection))</sub>

**Try next:** Open the connections before the load arrives: give the pool a minimum (pools: { db: { size: 10, min: 10 } }) and re-run to compare a warm pool with this cold one. PgBouncer in front of db also keeps server connections open.

### 2. Cold cache (cache, api → db), from 3:00

At 3:00, cache's hit rate fell to 71% from 100%, 29% of lookups missing, and api's reads from db reached 158 rps, against none before. The keys the cache used to answer now miss together, so their reads all go to the database at once: a thundering herd.

Evidence:

- cache hit rate 71% at 3:00, against 100% before <sub>(redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter))</sub>
- 158 rps from api to db at 3:00, against none before <sub>(loadlore_calls_total to the database (the caller's own counts))</sub>
- db CPU at 89% of its limit at 3:00 <sub>(container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- cache kill at 3:01 <sub>(events.jsonl)</sub>

**Try next:** Keep a cold cache from sending every miss to db at once: coalesce concurrent misses for the same key (single-flight), so one read fills it for everyone, or warm cache before it takes traffic.

### 3. Queueing knee (db CPU), from 3:00

From 3:00, p99 stayed above 48 ms (16 ms at low load) with db CPU at 89%. Near saturation requests wait behind each other, so latency grows non-linearly, not in proportion to load.

Evidence:

- median end-to-end p99 by db CPU: below 50%: 1.6 ms (25 windows); 50–70%: 1.05 s (1 window); 80–90%: 1.09 s (1 window); 90% and up: 1.08 s (3 windows) <sub>(k6_http_req_duration_seconds (k6 histogram, end to end); container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- 3:00 to 3:30: p99 1.09 s, 1.08 s, 1.08 s with db CPU 89%, 100%, 100% <sub>(k6_http_req_duration_seconds (k6 histogram, end to end); container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit)</sub>
- db was CPU-throttled for 55.6 s (summed over its threads) in the window from 3:00, while its average CPU was 89% of its limit: bursts of work hit the 1-CPU limit and waited for the next period <sub>(container_cpu_cfs_throttled_seconds_total (cAdvisor))</sub>
- Little's law on api at 3:00: 1,471 rps × 212 ms mean time in the service = 312 in flight; measured 276 <sub>(loadlore_requests_inflight (summed over replicas); rate and mean from loadlore_request_duration_seconds (the service's own histogram))</sub>

**Try next:** Add capacity or cut per-request work on db, then re-run the ramp.

<sub>Rules version 19. Rules are deterministic and offline; their thresholds are listed below.</sub>

## Predictions

0 of 1 scored prediction held.

| Prediction | Result | Worst value | Checked over | Notes |
| --- | --- | --- | --- | --- |
| `error_rate < 1% after 3m40s` | **broken** at 3:40 | 4.6% | from 3:40 to the end | source: k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6) |

A broken prediction isn't a wrong answer: it shows where your picture of the system and the run differ, and that gap is what to look into.

## Timeline

| Time | Event |
| --- | --- |
| 0:00 | stall: only 90% of the offered rate was served for 10 s, then recovered; too short to count as a break |
| 3:01 | container: cache kill |
| 3:01 | container: cache die (exit code 0) |
| 3:01 | fault started: restart cache |
| 3:01 | container: cache start |
| 3:01 | container: cache restart |

<sub>From events.jsonl: the fault timeline and Docker container events. Stalls are from the per-window numbers below, judged by the thresholds.</sub>

## Assumptions

### From the design

The design lists no assumptions.

### Chosen by Loadlore

| Field | Value | Why |
| --- | --- | --- |
| services.cache.config.maxmemory | 192m | 75% of the container's 256m memory limit, as managed Redis usually does; the rest is for fragmentation and client buffers. Set maxmemory: 0 for stock Redis (no cap) |
| services.cache.config.maxmemory-policy | allkeys-lru | evict the least recently used keys when the cache is full, as managed Redis usually does |
| workload.timeout_ms | 10s | the default; k6 gives up on a request after this long and counts it as failed |
| workload.slo.p99_ms | 500 ms | the default; a break on p99 alone needs p99 at or above this as well as 10x the low-load p99, so latency users would still accept isn't called a break |
| network.profile | same-zone: 0.3 ms round trip | the default: every service in one availability zone. AWS says a round trip between two instances in the same zone is generally sub-millisecond with enhanced networking, and publishes no exact figure, so the 0.3 ms round trip Loadlore adds between any two containers is its own estimate, inside that bound. Applied between any two of the stack's containers, the load generator and the entry point included, as half the round trip on each side's outgoing packets; Loadlore's own tooling isn't slowed. Not scaled with --scale. Source: AWS Architecture Blog, Improving performance and reducing cost using Availability Zone Affinity: https://aws.amazon.com/blogs/architecture/improving-performance-and-reducing-cost-using-availability-zone-affinity/ |
| resources.cpu_period_ms | 100 ms for api, cache, db, lb | Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU |
| services.db.disk | laptop: the Docker host's | no disk block: a read that misses db's memory goes to the Docker host's disk, which on a laptop mostly answers from the host's own memory, fast and uncapped, and a commit's fsync returns in a fraction of a millisecond. Add disk: { profile: gp3 } for a cloud volume's waits and caps |
| services.lb timeouts | connect 5s, server 60s | HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there |
| services.api.pools.db.min | 0 | the default, as pgxpool's: each replica opens connections only when callers need them, so a pool fills as load rises, and a burst opens many at once |
| services.api.pools.db.hold | call | the default, as pgxpool's: a request takes a connection for each call and hands it back straight away |

### How the run was planned

- load generator: 4000 VUs started before the load, the scenario's measured prealloc_vus 4000 at --scale 1, more than 1500 rps × the SLO's 500 ms
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

## Environment

- loadl dev, profile 5m0s, scale 1, seed 1
- Host: 8 CPUs, 9.7 GiB memory, Ubuntu 24.04.4 LTS (aarch64, kernel 6.8.0-117-generic), cgroup v2
- Docker 29.5.2, Compose 5.1.1
- Host speed index 1001 (hash 943, memory 1057, syscalls 1006): one CPU's work against the reference machine's 1000 (MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load
- Cool-down: none needed; the first benchmark (1001) was within 5.0% of the run it's compared with (988)
- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen
- Data: a fresh copy of the cached seeded dataset
- Load generator summary (k6): 360,521 requests, 0 dropped arrivals, 4.3% failed

<details><summary>Containers and images</summary>

| Container | Image |
| --- | --- |
| loadlore-cache-restart-coalesce-20261117-153908-api-0-1 | loadlore/synthetic:dev |
| loadlore-cache-restart-coalesce-20261117-153908-api-1-1 | loadlore/synthetic:dev |
| loadlore-cache-restart-coalesce-20261117-153908-api-2-1 | loadlore/synthetic:dev |
| loadlore-cache-restart-coalesce-20261117-153908-cache-1 | redis:8.2 |
| loadlore-cache-restart-coalesce-20261117-153908-db-1 | postgres:17.11 |
| loadlore-cache-restart-coalesce-20261117-153908-lb-1 | haproxy:3.2.26 |
| loadlore-cache-restart-coalesce-20261117-153908-lore-cadvisor-1 | ghcr.io/google/cadvisor:v0.60.6 |
| loadlore-cache-restart-coalesce-20261117-153908-lore-k6-1 | grafana/k6:2.3.0 |
| loadlore-cache-restart-coalesce-20261117-153908-lore-pgexp-db-1 | quay.io/prometheuscommunity/postgres-exporter:v0.20.1 |
| loadlore-cache-restart-coalesce-20261117-153908-lore-prometheus-1 | prom/prometheus:v3.16.0 |
| loadlore-cache-restart-coalesce-20261117-153908-lore-redisexp-cache-1 | oliver006/redis_exporter:v1.97.0 |

</details>

## Per-window data

<details><summary>30 windows of 10 seconds</summary>

| t | offered | served | dropped | errors | p50 | p99 | api CPU | cache CPU | cache hit rate | db CPU | db conns | lb CPU | lb up | api→cache pool | wait | api→db pool | wait | k6 CPU | k6 stalled |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 72 | 65 | 0 | 0% | 1.7 ms | 22 ms | 4.0% | 1.9% | 81% | 21% | 5 | 3.0% | 3 | 0/60 | – | 1/30 | 0.0 ms | 7.9% | 5.0% |
| 0:10 | 196 | 189 | 0 | 0% | 1.6 ms | 16 ms | 7.3% | 2.0% | 96% | 11% | 6 | 5.5% | 3 | 1/60 | – | 1/30 | 0.0 ms | 3.6% | 0% |
| 0:20 | 320 | 314 | 0 | 0% | 1.5 ms | 1.8 ms | 10% | 2.3% | 100% | 3.9% | 6 | 6.0% | 3 | 1/60 | – | 0/30 | 0.0 ms | 4.3% | 0% |
| 0:30 | 445 | 438 | 0 | 0% | 1.3 ms | 1.5 ms | 13% | 2.3% | 100% | 3.2% | 7 | 5.7% | 3 | 1/60 | – | 0/30 | – | 4.2% | 0% |
| 0:40 | 569 | 562 | 0 | 0% | 1.3 ms | 1.7 ms | 16% | 2.7% | 100% | 2.6% | 6 | 4.9% | 3 | 1/60 | – | 0/30 | – | 7.3% | 1.0% |
| 0:50 | 693 | 686 | 0 | 0% | 1.3 ms | 1.5 ms | 20% | 2.8% | 100% | 2.6% | 6 | 6.7% | 3 | 1/60 | – | 0/30 | – | 5.7% | 0% |
| 1:00 | 817 | 810 | 0 | 0% | 1.3 ms | 1.6 ms | 23% | 3.3% | 100% | 2.5% | 6 | 9.2% | 3 | 0/60 | – | 0/30 | – | 8.3% | 2.0% |
| 1:10 | 941 | 934 | 0 | 0% | 1.3 ms | 1.5 ms | 27% | 3.5% | 100% | 2.7% | 6 | 7.4% | 3 | 1/60 | – | 0/30 | – | 7.2% | 0% |
| 1:20 | 1,065 | 1,059 | 0 | 0% | 1.3 ms | 1.6 ms | 30% | 3.7% | 100% | 2.6% | 6 | 11% | 3 | 2/60 | – | 0/30 | – | 9.8% | 1.0% |
| 1:30 | 1,190 | 1,183 | 0 | 0% | 1.3 ms | 1.5 ms | 33% | 4.0% | 100% | 2.7% | 6 | 8.2% | 3 | 1/60 | – | 0/30 | – | 8.2% | 0% |
| 1:40 | 1,314 | 1,307 | 0 | 0% | 1.3 ms | 1.6 ms | 37% | 4.3% | 100% | 2.5% | 6 | 11% | 3 | 2/60 | – | 0/30 | – | 11% | 1.0% |
| 1:50 | 1,438 | 1,431 | 0 | 0% | 1.3 ms | 1.7 ms | 40% | 4.4% | 100% | 2.7% | 6 | 11% | 3 | 2/60 | – | 0/30 | – | 11% | 1.0% |
| 2:00 | 1,500 | 1,500 | 0 | 0% | 1.3 ms | 1.5 ms | 42% | 4.8% | 100% | 2.5% | 6 | 11% | 3 | 2/60 | – | 0/30 | – | 9.1% | 0% |
| 2:10 | 1,500 | 1,500 | 0 | 0% | 1.3 ms | 1.8 ms | 43% | 5.0% | 100% | 2.6% | 6 | 12% | 3 | 2/60 | – | 0/30 | – | 12% | 1.0% |
| 2:20 | 1,500 | 1,500 | 0 | 0% | 1.3 ms | 1.6 ms | 42% | 5.0% | 100% | 2.5% | 6 | 11% | 3 | 1/60 | – | 0/30 | – | 9.3% | 0% |
| 2:30 | 1,500 | 1,500 | 0 | 0% | 1.3 ms | 1.9 ms | 42% | 4.9% | 100% | 2.6% | 6 | 12% | 3 | 2/60 | – | 0/30 | – | 11% | 2.0% |
| 2:40 | 1,500 | 1,500 | 0 | 0% | 1.3 ms | 1.5 ms | 42% | 4.8% | 100% | 2.6% | 6 | 13% | 3 | 2/60 | – | 0/30 | – | 9.7% | <0.1% |
| 2:50 | 1,500 | 1,500 | 0 | 0% | 1.3 ms | 1.7 ms | 42% | 4.5% | 100% | 2.5% | 6 | 11% | 3 | 1/60 | – | 0/30 | – | 11% | 1.0% |
| 3:00 | 1,500 | 1,085 | 0 | 26% | 1.3 ms | 1.09 s | 50% | 5.2% | 71% | 89% | 31 | 14% | 3 | 2/60 +13 | 22 ms | 23/30 +218 | 935 ms | 12% | 2.0% |
| 3:10 | 1,500 | 1,084 | 0 | 28% | 1.3 ms | 1.08 s | 51% | 5.3% | 72% | 100% | 22 | 13% | 3 | 2/60 | – | 24/30 +196 | 960 ms | 10% | 0% |
| 3:20 | 1,500 | 1,121 | 0 | 25% | 1.3 ms | 1.08 s | 51% | 5.4% | 75% | 100% | 21 | 13% | 3 | 1/60 | – | 24/30 +173 | 951 ms | 13% | 1.0% |
| 3:30 | 1,500 | 1,232 | 0 | 18% | 1.3 ms | 1.08 s | 50% | 5.4% | 82% | 99% | 28 | 13% | 3 | 4/60 | – | 26/30 +144 | 923 ms | 10% | 0% |
| 3:40 | 1,500 | 1,444 | 0 | 4.6% | 1.3 ms | 1.05 s | 46% | 5.4% | 95% | 61% | 29 | 12% | 3 | 1/60 | – | 30/30 +93 | 759 ms | 13% | 2.0% |
| 3:50 | 1,500 | 1,500 | 0 | 0% | 1.3 ms | 1.4 ms | 42% | 4.7% | 100% | 2.5% | 29 | 10% | 3 | 2/60 | – | 0/30 | – | 9.3% | 0% |
| 4:00 | 1,500 | 1,500 | 0 | 0% | 1.3 ms | 1.5 ms | 42% | 4.8% | 100% | 2.6% | 29 | 10% | 3 | 1/60 | – | 0/30 | – | 11% | 1.0% |
| 4:10 | 1,500 | 1,500 | 0 | 0% | 1.3 ms | 1.5 ms | 42% | 4.7% | 100% | 2.7% | 29 | 12% | 3 | 2/60 | – | 0/30 | – | 11% | 1.0% |
| 4:20 | 1,500 | 1,500 | 0 | 0% | 1.3 ms | 1.5 ms | 42% | 4.8% | 100% | 2.5% | 29 | 10% | 3 | 2/60 | – | 0/30 | – | 9.5% | 0% |
| 4:30 | 1,500 | 1,500 | 0 | 0% | 1.3 ms | 1.7 ms | 42% | 4.7% | 100% | 2.6% | 29 | 14% | 3 | 1/60 | – | 0/30 | – | 12% | 2.0% |
| 4:40 | 1,500 | 1,500 | 0 | 0% | 1.3 ms | 1.5 ms | 42% | 4.8% | 100% | 2.6% | 29 | 10% | 3 | 2/60 | – | 0/30 | – | 9.6% | 0% |
| 4:50 | 1,500 | 1,500 | 0 | 0% | 1.3 ms | 1.5 ms | 42% | 4.8% | 100% | 2.7% | 29 | 11% | 3 | 1/60 | – | 0/30 | – | 11% | 1.1% |

Columns: offered is the load profile in plan.json (offered arrival rate); served is k6_http_reqs_total{expected_response="true"} (k6: successful responses); dropped is k6_dropped_iterations_total (k6: arrivals it couldn't start); errors are k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6); p50 and p99 are from k6_http_req_duration_seconds (k6 histogram, end to end); CPU is container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit, mean over the window; conns are pg_stat_activity_count and pg_settings_max_connections (postgres_exporter); hit rate is from redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter); up is haproxy_backend_active_servers (HAProxy); pools are loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections (peak in use/size, +peak waiting), wait is the mean from loadlore_pool_wait_seconds (time to get a connection); k6 CPU is against its budget.

</details>

## Design file

The exact `design.yaml` this run used:

```yaml
version: 1
name: cache-restart-coalesce

# Solution: request coalescing on the cache lookup. When requests in one
# api replica miss the same product at the same time, the first computes
# the review summary and fills the cache, and the others wait for its
# answer, so the database runs the query about once per product per
# replica instead of once per request.

services:
  lb:
    kind: lb
    target: api
    health_check: { interval_ms: 2000, fall: 3, rise: 2 }

  api:
    image: loadlore/synthetic
    replicas: 3
    resources: { cpu: 0.5, memory: 256m }
    behavior:
      cpu_ms: 0.3
      calls:
        - target: cache
          op: get
          coalesce: true
          on_miss:
            - { target: db, op: read, query: scan }
            - { target: cache, op: set, ttl_s: 600 }
      resilience:
        timeout_ms: 1000
    pools:
      cache: 20
      db: 10   # 3 replicas x 10 = 30 connections

  cache:
    kind: redis
    resources: { cpu: 0.5, memory: 256m }

  db:
    kind: postgres
    version: "17"
    resources: { cpu: 1, memory: 1g }
    config: { max_connections: 100, shared_buffers: 256MB }
    dataset: { rows: 140000, row_bytes: 512 }

workload:
  entry: lb
  profile:
    - ramp: { from: 10, to: 1500, over: 2m }
    - hold: { rps: 1500, for: 3m }
  mix: { read: 1.0 }
  keys: { distribution: zipf, skew: 1.1, count: 200 }

# Redis restarts at 3:01, as a deploy or a node replacement would restart
# it. It keeps nothing on disk, so it comes back empty.
scenario:
  - { at: 3m1s, inject: restart, target: cache }

expect:
  - error_rate < 1% after 3m40s
```
