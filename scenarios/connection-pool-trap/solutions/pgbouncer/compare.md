# Compare: connection-pool-trap vs connection-pool-trap-pgbouncer

_A: run `connection-pool-trap-20261118-085050` · B: run `connection-pool-trap-pgbouncer-20261118-085831`_

| | A | B |
| --- | --- | --- |
| Design | connection-pool-trap | connection-pool-trap-pgbouncer |
| Verdict | broke at 1:40 (555 rps offered) | held |
| Most served | 490 rps at 2:40 | 1,400 rps at 6:50 |
| Worst 10-second p99 | 6.16 s | 90 ms |
| Worst error rate | 0% | 0% |
| Diagnoses | Connection pool exhausted (api → db) from 1:30 | none fired |
| Predictions held | 1 of 2 | 2 of 2 |
| Peak offered | 1,400 rps | 1,400 rps |
| Scale, seed | 1, 1 | 1, 1 |
| Host | 8 CPUs, 9.7 GiB, Docker 29.5.2 | 8 CPUs, 9.7 GiB, Docker 29.5.2 |
| Data | fresh copy | fresh copy |

<sub>Sources as in each run's report: served from k6_http_reqs_total{expected_response="true"} (k6: successful responses), p99 from k6_http_req_duration_seconds (k6 histogram, end to end), errors from k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6).</sub>

**Before reading the difference:**

- Both are single runs. The same design varies from run to run on one machine, so a small difference may be noise: `loadl run --repeat 3` on each design measures how much on this host.

## Verdicts

**A.** Broke 1:40 into the load, at 555 rps offered: p99 reached 1.47 s (88 ms at low load) and only 490 rps succeeded. Most saturated then: api → db pool 40 of 40 connections in use. After the break the load generator couldn't send 57% of the arrivals the profile asked for (203,060 arrivals from 1:50; k6_dropped_iterations_total (k6: arrivals it couldn't start)): slow responses held its 2,800 VUs, so served throughput from then on is partly the generator's limit, not only the system's. It hadn't recovered when the load ended.

**B.** Held the whole profile: up to 1,400 rps offered, 1,400 rps served at best, worst 10-second p99 90 ms, at most 0% errors.

## Charts

```mermaid
---
config:
  themeVariables:
    xyChart:
      backgroundColor: "transparent"
      plotColorPalette: "#9e9e9e, #1f77b4, #ff7f0e"
---
xychart-beta
    title "Throughput"
    x-axis "seconds" 0 --> 410
    y-axis "requests/s" 0 --> 1500
    line [121.667, 165, 208.333, 251.667, 295, 338.333, 381.667, 425, 468.333, 511.667, 555, 598.333, 641.667, 685, 728.333, 771.667, 815, 858.333, 901.667, 945, 988.333, 1031.667, 1075, 1118.333, 1161.667, 1205, 1248.333, 1291.667, 1335, 1378.333, 1400, 1400, 1400, 1400, 1400, 1400, 1400, 1400, 1400, 1400, 1400, 1400]
    line [117.541, 163.489, 207.06, 250.309, 293.734, 337.017, 380.414, 423.519, 467.093, 489.853, 489.948, 489.946, 490.014, 489.808, 490.389, 489.785, 490.449, 488.756, 489.534, 489.715, 487.486, 487.702, 485.803, 486.65, 485.837, 484.934, 485.227, 485.53, 482.288, 483.543, 485.302, 484.637, 484.638, 481.648, 481.06, 480.709, 482.355, 479.344, 483.489, 479.768, 487.22, 482.388]
    line [118.065, 163.647, 207.33, 250.407, 293.753, 337.145, 380.489, 423.953, 467.186, 510.536, 553.824, 597.195, 640.385, 683.843, 727.269, 770.309, 813.91, 857.191, 901.163, 943.11, 987.14, 1030.608, 1073.936, 1117.201, 1160.593, 1203.704, 1246.894, 1290.645, 1334.028, 1376.998, 1400.025, 1399.946, 1399.969, 1399.914, 1400.132, 1400.175, 1399.893, 1400.089, 1399.802, 1400.115, 1399.866, 1400.228]
```

<sub>Grey: offered (A). Blue: served, A. Orange: served, B (k6_http_reqs_total{expected_response="true"} (k6: successful responses)). Windows without data are drawn at 0.</sub>

```mermaid
---
config:
  themeVariables:
    xyChart:
      backgroundColor: "transparent"
      plotColorPalette: "#1f77b4, #ff7f0e"
---
xychart-beta
    title "p99 latency, end to end"
    x-axis "seconds" 0 --> 410
    y-axis "ms" 0 --> 8000
    line [89.878, 88.32, 88.315, 88.318, 88.315, 88.317, 88.315, 88.317, 88.318, 407.975, 1471.812, 2363.169, 3640.289, 5158.333, 6160.225, 6163.723, 6163.723, 6163.723, 6163.723, 6163.706, 6163.704, 6163.723, 6163.723, 6163.723, 6163.719, 6163.72, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723]
    line [89.928, 88.324, 88.319, 88.318, 88.32, 88.319, 88.317, 88.315, 88.318, 88.315, 88.319, 88.315, 88.322, 88.318, 88.317, 88.318, 88.316, 88.318, 88.316, 88.32, 88.316, 88.319, 88.316, 88.317, 88.319, 88.317, 88.321, 88.32, 88.317, 88.321, 88.32, 88.318, 88.316, 88.33, 88.316, 88.317, 88.319, 88.319, 88.327, 88.319, 88.318, 88.319]
```

<sub>Blue: A. Orange: B (k6_http_req_duration_seconds (k6 histogram, end to end)). Windows without data are drawn at 0.</sub>

<details><summary>Per-window numbers, side by side</summary>

| t | offered A | offered B | served A | served B | p99 A | p99 B | errors A | errors B |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 122 | 122 | 118 | 118 | 90 ms | 90 ms | 0% | 0% |
| 0:10 | 165 | 165 | 163 | 164 | 88 ms | 88 ms | 0% | 0% |
| 0:20 | 208 | 208 | 207 | 207 | 88 ms | 88 ms | 0% | 0% |
| 0:30 | 252 | 252 | 250 | 250 | 88 ms | 88 ms | 0% | 0% |
| 0:40 | 295 | 295 | 294 | 294 | 88 ms | 88 ms | 0% | 0% |
| 0:50 | 338 | 338 | 337 | 337 | 88 ms | 88 ms | 0% | 0% |
| 1:00 | 382 | 382 | 380 | 380 | 88 ms | 88 ms | 0% | 0% |
| 1:10 | 425 | 425 | 424 | 424 | 88 ms | 88 ms | 0% | 0% |
| 1:20 | 468 | 468 | 467 | 467 | 88 ms | 88 ms | 0% | 0% |
| 1:30 | 512 | 512 | 490 | 511 | 408 ms | 88 ms | 0% | 0% |
| 1:40 | 555 | 555 | 490 | 554 | 1.47 s | 88 ms | 0% | 0% |
| 1:50 | 598 | 598 | 490 | 597 | 2.36 s | 88 ms | 0% | 0% |
| 2:00 | 642 | 642 | 490 | 640 | 3.64 s | 88 ms | 0% | 0% |
| 2:10 | 685 | 685 | 490 | 684 | 5.16 s | 88 ms | 0% | 0% |
| 2:20 | 728 | 728 | 490 | 727 | 6.16 s | 88 ms | 0% | 0% |
| 2:30 | 772 | 772 | 490 | 770 | 6.16 s | 88 ms | 0% | 0% |
| 2:40 | 815 | 815 | 490 | 814 | 6.16 s | 88 ms | 0% | 0% |
| 2:50 | 858 | 858 | 489 | 857 | 6.16 s | 88 ms | 0% | 0% |
| 3:00 | 902 | 902 | 490 | 901 | 6.16 s | 88 ms | 0% | 0% |
| 3:10 | 945 | 945 | 490 | 943 | 6.16 s | 88 ms | 0% | 0% |
| 3:20 | 988 | 988 | 487 | 987 | 6.16 s | 88 ms | 0% | 0% |
| 3:30 | 1,032 | 1,032 | 488 | 1,031 | 6.16 s | 88 ms | 0% | 0% |
| 3:40 | 1,075 | 1,075 | 486 | 1,074 | 6.16 s | 88 ms | 0% | 0% |
| 3:50 | 1,118 | 1,118 | 487 | 1,117 | 6.16 s | 88 ms | 0% | 0% |
| 4:00 | 1,162 | 1,162 | 486 | 1,161 | 6.16 s | 88 ms | 0% | 0% |
| 4:10 | 1,205 | 1,205 | 485 | 1,204 | 6.16 s | 88 ms | 0% | 0% |
| 4:20 | 1,248 | 1,248 | 485 | 1,247 | 6.16 s | 88 ms | 0% | 0% |
| 4:30 | 1,292 | 1,292 | 486 | 1,291 | 6.16 s | 88 ms | 0% | 0% |
| 4:40 | 1,335 | 1,335 | 482 | 1,334 | 6.16 s | 88 ms | 0% | 0% |
| 4:50 | 1,378 | 1,378 | 484 | 1,377 | 6.16 s | 88 ms | 0% | 0% |
| 5:00 | 1,400 | 1,400 | 485 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 5:10 | 1,400 | 1,400 | 485 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 5:20 | 1,400 | 1,400 | 485 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 5:30 | 1,400 | 1,400 | 482 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 5:40 | 1,400 | 1,400 | 481 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 5:50 | 1,400 | 1,400 | 481 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 6:00 | 1,400 | 1,400 | 482 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 6:10 | 1,400 | 1,400 | 479 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 6:20 | 1,400 | 1,400 | 483 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 6:30 | 1,400 | 1,400 | 480 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 6:40 | 1,400 | 1,400 | 487 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 6:50 | 1,400 | 1,400 | 482 | 1,400 | 6.16 s | 88 ms | 0% | 0% |

</details>

## Diagnosis

### A

- **Connection pool exhausted (api → db), from 1:30.** From 1:30, every connection in api's pool to db was in use and requests waited for one, while db still had CPU to spare. Requests were queueing for a connection, not for the database.

### B

No diagnosis rule fired.

## Predictions

| Prediction | A | B |
| --- | --- | --- |
| `break_load_rps within 25% of 1000` | broken, worst 555 rps at 1:40 | not in this design |
| `errors_load_rps none` | held, worst no failures | not in this design |
| `p99_ms < 300 until load_rps >= 1000` | not in this design | held, worst 90 ms |
| `error_rate < 1% throughout` | not in this design | held, worst 0% |

## Design changes

```diff
--- A/design.yaml
+++ B/design.yaml
@@ -1,6 +1,8 @@
 version: 1
-name: connection-pool-trap
+name: connection-pool-trap-pgbouncer
 
+# The starting design with PgBouncer between api and db, in transaction mode.
+#
 # A checkout service: every request reads the cart from Postgres, then asks
 # the payment provider to authorize the card, and one in ten also writes
 # the order. Like Rails and Django, the api's framework checks out a
@@ -20,13 +22,21 @@
     behavior:
       cpu_ms: 0.3
       calls:
-        - { target: db, op: read }                          # the cart
+        - { target: pgb, op: read }                         # the cart
         - { target: payments, op: http }                    # authorize the card
-        - { target: db, op: write, when: write }            # the order
+        - { target: pgb, op: write, when: write }           # the order
     pools:
-      db: { size: 10, hold: request }   # 4 replicas x 10 = 40 connections
+      pgb: { size: 100, hold: request }  # client connections to PgBouncer: 4 x 100 = 400
       payments: 50
 
+  pgb:
+    kind: pgbouncer
+    target: db
+    pool_mode: transaction        # a server connection per transaction, not per client
+    default_pool_size: 20         # what db sees: 20 connections
+    resources: { cpu: 0.5, memory: 128m }
+    config: { max_client_conn: 500 }
+
   payments:
     image: loadlore/synthetic
     resources: { cpu: 0.5, memory: 256m }
@@ -57,5 +67,5 @@
 # Your predictions, scored in the report. Replace these with your own
 # before you run it.
 expect:
-  - break_load_rps within 25% of 1000
-  - errors_load_rps none
+  - p99_ms < 300 until load_rps >= 1000
+  - error_rate < 1% throughout
```
