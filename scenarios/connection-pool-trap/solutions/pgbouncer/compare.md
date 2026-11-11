# Compare: connection-pool-trap vs connection-pool-trap-pgbouncer

_A: run `connection-pool-trap-20261111-120625` · B: run `connection-pool-trap-pgbouncer-20261111-121407`_

| | A | B |
| --- | --- | --- |
| Design | connection-pool-trap | connection-pool-trap-pgbouncer |
| Verdict | broke at 1:40 (555 rps offered) | held |
| Most served | 494 rps at 1:30 | 1,400 rps at 6:30 |
| Worst 10-second p99 | 6.16 s | 88 ms |
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

**A.** Broke 1:40 into the load, at 555 rps offered: p99 reached 1.44 s (88 ms at low load) and only 494 rps succeeded. Most saturated then: api → db pool 40 of 40 connections in use. After the break the load generator couldn't send 57% of the arrivals the profile asked for (202,821 arrivals from 1:50; k6_dropped_iterations_total (k6: arrivals it couldn't start)): slow responses held its 2,800 VUs, so served throughput from then on is partly the generator's limit, not only the system's.

**B.** Held the whole profile: up to 1,400 rps offered, 1,400 rps served at best, worst 10-second p99 88 ms, at most 0% errors.

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
    line [119.592, 164.246, 207.797, 251.041, 294.32, 337.507, 381.091, 424.285, 467.662, 493.769, 493.693, 491.992, 490.523, 489.764, 488.294, 488.761, 485.526, 486.92, 489.085, 488.216, 486.875, 487.263, 487.382, 486.313, 485.554, 486.855, 481.213, 487.307, 483.548, 484.048, 484.382, 487.596, 486.793, 485.421, 485.5, 486.347, 485.007, 484.619, 486.766, 485.707, 487.497, 485.39]
    line [120.078, 164.341, 207.89, 251.091, 294.374, 337.79, 381.132, 424.445, 467.794, 511.186, 554.73, 597.508, 641.161, 684.442, 727.84, 771.031, 814.492, 857.857, 901.344, 944.124, 987.918, 1031.207, 1074.436, 1117.68, 1161.502, 1204.115, 1247.886, 1291.141, 1334.517, 1377.638, 1400.21, 1399.876, 1399.872, 1400.035, 1400.035, 1400.219, 1399.821, 1400.014, 1399.759, 1400.369, 1399.864, 1399.986]
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
    line [88.351, 88.327, 88.312, 88.366, 88.306, 88.326, 88.288, 88.28, 88.274, 406.954, 1441.926, 2365.16, 3646.588, 5164.387, 6161.015, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.722, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.723, 6163.716, 6163.722, 6163.722]
    line [88.352, 88.314, 88.313, 88.313, 88.308, 88.307, 88.295, 88.294, 88.287, 88.282, 88.277, 88.282, 88.27, 88.271, 88.264, 88.26, 88.259, 88.263, 88.259, 88.262, 88.257, 88.259, 88.26, 88.262, 88.262, 88.261, 88.262, 88.264, 88.262, 88.261, 88.267, 88.269, 88.268, 88.268, 88.268, 88.269, 88.267, 88.268, 88.27, 88.268, 88.269, 88.269]
```

<sub>Blue: A. Orange: B (k6_http_req_duration_seconds (k6 histogram, end to end)). Windows without data are drawn at 0.</sub>

<details><summary>Per-window numbers, side by side</summary>

| t | offered A | offered B | served A | served B | p99 A | p99 B | errors A | errors B |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 122 | 122 | 120 | 120 | 88 ms | 88 ms | 0% | 0% |
| 0:10 | 165 | 165 | 164 | 164 | 88 ms | 88 ms | 0% | 0% |
| 0:20 | 208 | 208 | 208 | 208 | 88 ms | 88 ms | 0% | 0% |
| 0:30 | 252 | 252 | 251 | 251 | 88 ms | 88 ms | 0% | 0% |
| 0:40 | 295 | 295 | 294 | 294 | 88 ms | 88 ms | 0% | 0% |
| 0:50 | 338 | 338 | 338 | 338 | 88 ms | 88 ms | 0% | 0% |
| 1:00 | 382 | 382 | 381 | 381 | 88 ms | 88 ms | 0% | 0% |
| 1:10 | 425 | 425 | 424 | 424 | 88 ms | 88 ms | 0% | 0% |
| 1:20 | 468 | 468 | 468 | 468 | 88 ms | 88 ms | 0% | 0% |
| 1:30 | 512 | 512 | 494 | 511 | 407 ms | 88 ms | 0% | 0% |
| 1:40 | 555 | 555 | 494 | 555 | 1.44 s | 88 ms | 0% | 0% |
| 1:50 | 598 | 598 | 492 | 598 | 2.37 s | 88 ms | 0% | 0% |
| 2:00 | 642 | 642 | 491 | 641 | 3.65 s | 88 ms | 0% | 0% |
| 2:10 | 685 | 685 | 490 | 684 | 5.16 s | 88 ms | 0% | 0% |
| 2:20 | 728 | 728 | 488 | 728 | 6.16 s | 88 ms | 0% | 0% |
| 2:30 | 772 | 772 | 489 | 771 | 6.16 s | 88 ms | 0% | 0% |
| 2:40 | 815 | 815 | 486 | 814 | 6.16 s | 88 ms | 0% | 0% |
| 2:50 | 858 | 858 | 487 | 858 | 6.16 s | 88 ms | 0% | 0% |
| 3:00 | 902 | 902 | 489 | 901 | 6.16 s | 88 ms | 0% | 0% |
| 3:10 | 945 | 945 | 488 | 944 | 6.16 s | 88 ms | 0% | 0% |
| 3:20 | 988 | 988 | 487 | 988 | 6.16 s | 88 ms | 0% | 0% |
| 3:30 | 1,032 | 1,032 | 487 | 1,031 | 6.16 s | 88 ms | 0% | 0% |
| 3:40 | 1,075 | 1,075 | 487 | 1,074 | 6.16 s | 88 ms | 0% | 0% |
| 3:50 | 1,118 | 1,118 | 486 | 1,118 | 6.16 s | 88 ms | 0% | 0% |
| 4:00 | 1,162 | 1,162 | 486 | 1,162 | 6.16 s | 88 ms | 0% | 0% |
| 4:10 | 1,205 | 1,205 | 487 | 1,204 | 6.16 s | 88 ms | 0% | 0% |
| 4:20 | 1,248 | 1,248 | 481 | 1,248 | 6.16 s | 88 ms | 0% | 0% |
| 4:30 | 1,292 | 1,292 | 487 | 1,291 | 6.16 s | 88 ms | 0% | 0% |
| 4:40 | 1,335 | 1,335 | 484 | 1,335 | 6.16 s | 88 ms | 0% | 0% |
| 4:50 | 1,378 | 1,378 | 484 | 1,378 | 6.16 s | 88 ms | 0% | 0% |
| 5:00 | 1,400 | 1,400 | 484 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 5:10 | 1,400 | 1,400 | 488 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 5:20 | 1,400 | 1,400 | 487 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 5:30 | 1,400 | 1,400 | 485 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 5:40 | 1,400 | 1,400 | 486 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 5:50 | 1,400 | 1,400 | 486 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 6:00 | 1,400 | 1,400 | 485 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 6:10 | 1,400 | 1,400 | 485 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 6:20 | 1,400 | 1,400 | 487 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 6:30 | 1,400 | 1,400 | 486 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 6:40 | 1,400 | 1,400 | 487 | 1,400 | 6.16 s | 88 ms | 0% | 0% |
| 6:50 | 1,400 | 1,400 | 485 | 1,400 | 6.16 s | 88 ms | 0% | 0% |

</details>

## Diagnosis

### A

- **Connection pool exhausted (api → db), from 1:30.** From 1:30, every connection in api's pool to db was in use and requests waited for one, while db still had CPU to spare. Requests were queueing for a connection, not for the database.

### B

No diagnosis rule fired.

## Predictions

| Prediction | A | B |
| --- | --- | --- |
| `p99_ms < 300 until load_rps >= 1000` | broken, worst 6.16 s (broke at 1:30) | held, worst 88 ms |
| `error_rate < 1% throughout` | held, worst 0% | held, worst 0% |

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
```
