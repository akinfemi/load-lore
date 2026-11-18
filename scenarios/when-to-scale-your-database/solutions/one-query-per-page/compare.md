# Compare: when-to-scale-your-database vs when-to-scale-your-database-one-query-per-page

_A: run `when-to-scale-your-database-20261117-212641` · B: run `when-to-scale-your-database-one-query-per-page-20261117-213423`_

| | A | B |
| --- | --- | --- |
| Design | when-to-scale-your-database | when-to-scale-your-database-one-query-per-page |
| Verdict | broke at 4:10 (2,990 rps offered) | broke at 5:10 (3,500 rps offered) |
| Most served | 2,872 rps at 4:00 | 3,656 rps at 5:30 |
| Worst 10-second p99 | 3.34 s | 1.77 s |
| Worst error rate | 0% | 0% |
| Diagnoses | Queueing knee (db CPU) from 3:50 | Queueing knee (db CPU) from 4:50 |
| Predictions held | 2 of 2 | 1 of 2 |
| Peak offered | 3,500 rps | 3,500 rps |
| Scale, seed | 1, 1 | 1, 1 |
| Host | 8 CPUs, 9.7 GiB, Docker 29.5.2 | 8 CPUs, 9.7 GiB, Docker 29.5.2 |
| Data | fresh copy | fresh copy |

<sub>Sources as in each run's report: served from k6_http_reqs_total{expected_response="true"} (k6: successful responses), p99 from k6_http_req_duration_seconds (k6 histogram, end to end), errors from k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6).</sub>

**Before reading the difference:**

- Both are single runs. The same design varies from run to run on one machine, so a small difference may be noise: `loadl run --repeat 3` on each design measures how much on this host.

## Verdicts

**A.** Broke 4:10 into the load, at 2,990 rps offered: p99 reached 595 ms (3.1 ms at low load). Most saturated then: api → db pool 96 of 96 connections in use, waiting on db (CPU 87%, CPU-throttled 16.8 s, 3.5 ms per call with a connection, 0.4 ms at low load). After the break the load generator couldn't send 29% of the arrivals the profile asked for (158,698 arrivals from 4:20; k6_dropped_iterations_total (k6: arrivals it couldn't start)): slow responses held its 4,000 VUs, so served throughput from then on is partly the generator's limit, not only the system's. It hadn't recovered when the load ended.

**B.** Broke 5:10 into the load, at 3,500 rps offered: p99 reached 648 ms (2.5 ms at low load). Most saturated then: api → db pool 92 of 96 connections in use, waiting on db (CPU 84%, CPU-throttled 20.3 s, 8.8 ms per call with a connection, 0.4 ms at low load). After the break the load generator couldn't send 3.1% of the arrivals the profile asked for (10,745 arrivals from 5:20; k6_dropped_iterations_total (k6: arrivals it couldn't start)): slow responses held its 4,000 VUs, so served throughput from then on is partly the generator's limit, not only the system's. It hadn't recovered when the load ended.

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
    y-axis "requests/s" 0 --> 4000
    line [156.667, 270, 383.333, 496.667, 610, 723.333, 836.667, 950, 1063.333, 1176.667, 1290, 1403.333, 1516.667, 1630, 1743.333, 1856.667, 1970, 2083.333, 2196.667, 2310, 2423.333, 2536.667, 2650, 2763.333, 2876.667, 2990, 3103.333, 3216.667, 3330, 3443.333, 3500, 3500, 3500, 3500, 3500, 3500, 3500, 3500, 3500, 3500, 3500, 3500]
    line [149.309, 266.065, 379.413, 492.673, 606.087, 719.42, 832.697, 946.053, 1059.304, 1172.661, 1286.073, 1399.337, 1512.701, 1626.124, 1739.402, 1852.731, 1966.028, 2078.785, 2193.87, 2305.419, 2419.398, 2532.706, 2645.67, 2759.918, 2872.3, 2845.747, 2499.575, 2498.886, 2509.922, 2494.806, 2403.238, 2210.386, 2432.508, 2409.493, 2550.973, 2434.68, 2445.083, 2429.681, 2353.448, 2378.539, 2414.461, 2485.87]
    line [150.42, 266.563, 379.943, 493.365, 606.558, 719.998, 833.229, 946.645, 1060.163, 1172.965, 1286.646, 1400.078, 1513.207, 1626.757, 1739.772, 1853.239, 1966.828, 2079.943, 2193.017, 2306.696, 2419.962, 2533.158, 2646.774, 2760.083, 2873.282, 2986.372, 3100.139, 3213.415, 3326.544, 3440.126, 3499.672, 3356.79, 3416.715, 3655.522, 3499.944, 3114.015, 3413.378, 3369.133, 3295.597, 3182.919, 3412.345, 3334.256]
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
    y-axis "ms" 0 --> 4000
    line [4.879, 3.688, 3.097, 3.002, 3, 3.073, 2.992, 2.986, 2.997, 2.967, 2.987, 3.102, 3.12, 3.279, 3.422, 3.411, 3.423, 3.369, 357.415, 3.844, 3.877, 4.448, 10.579, 34.566, 13.423, 595.363, 1901.259, 2445.075, 2393.578, 2548.622, 2768.541, 2867.222, 2574.209, 2708.093, 2592.994, 2747.355, 3038.948, 3073.071, 3316.966, 3335.182, 3083.322, 2957.882]
    line [3.777, 2.716, 2.478, 2.441, 2.425, 360.771, 2.23, 2.443, 2.582, 2.289, 2.237, 2.207, 2.172, 2.194, 2.291, 2.424, 2.473, 2.746, 2.633, 2.477, 2.594, 2.769, 2.677, 2.795, 2.748, 2.821, 2.954, 28.235, 3.945, 29.951, 19.52, 647.599, 1015.781, 838.873, 49.813, 838.755, 1073.479, 1128.174, 1320.863, 1765.541, 1683.369, 1563.958]
```

<sub>Blue: A. Orange: B (k6_http_req_duration_seconds (k6 histogram, end to end)). Windows without data are drawn at 0.</sub>

<details><summary>Per-window numbers, side by side</summary>

| t | offered A | offered B | served A | served B | p99 A | p99 B | errors A | errors B |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0:00 | 157 | 157 | 149 | 150 | 4.9 ms | 3.8 ms | 0% | 0% |
| 0:10 | 270 | 270 | 266 | 267 | 3.7 ms | 2.7 ms | 0% | 0% |
| 0:20 | 383 | 383 | 379 | 380 | 3.1 ms | 2.5 ms | 0% | 0% |
| 0:30 | 497 | 497 | 493 | 493 | 3.0 ms | 2.4 ms | 0% | 0% |
| 0:40 | 610 | 610 | 606 | 607 | 3.0 ms | 2.4 ms | 0% | 0% |
| 0:50 | 723 | 723 | 719 | 720 | 3.1 ms | 361 ms | 0% | 0% |
| 1:00 | 837 | 837 | 833 | 833 | 3.0 ms | 2.2 ms | 0% | 0% |
| 1:10 | 950 | 950 | 946 | 947 | 3.0 ms | 2.4 ms | 0% | 0% |
| 1:20 | 1,063 | 1,063 | 1,059 | 1,060 | 3.0 ms | 2.6 ms | 0% | 0% |
| 1:30 | 1,177 | 1,177 | 1,173 | 1,173 | 3.0 ms | 2.3 ms | 0% | 0% |
| 1:40 | 1,290 | 1,290 | 1,286 | 1,287 | 3.0 ms | 2.2 ms | 0% | 0% |
| 1:50 | 1,403 | 1,403 | 1,399 | 1,400 | 3.1 ms | 2.2 ms | 0% | 0% |
| 2:00 | 1,517 | 1,517 | 1,513 | 1,513 | 3.1 ms | 2.2 ms | 0% | 0% |
| 2:10 | 1,630 | 1,630 | 1,626 | 1,627 | 3.3 ms | 2.2 ms | 0% | 0% |
| 2:20 | 1,743 | 1,743 | 1,739 | 1,740 | 3.4 ms | 2.3 ms | 0% | 0% |
| 2:30 | 1,857 | 1,857 | 1,853 | 1,853 | 3.4 ms | 2.4 ms | 0% | 0% |
| 2:40 | 1,970 | 1,970 | 1,966 | 1,967 | 3.4 ms | 2.5 ms | 0% | 0% |
| 2:50 | 2,083 | 2,083 | 2,079 | 2,080 | 3.4 ms | 2.7 ms | 0% | 0% |
| 3:00 | 2,197 | 2,197 | 2,194 | 2,193 | 357 ms | 2.6 ms | 0% | 0% |
| 3:10 | 2,310 | 2,310 | 2,305 | 2,307 | 3.8 ms | 2.5 ms | 0% | 0% |
| 3:20 | 2,423 | 2,423 | 2,419 | 2,420 | 3.9 ms | 2.6 ms | 0% | 0% |
| 3:30 | 2,537 | 2,537 | 2,533 | 2,533 | 4.4 ms | 2.8 ms | 0% | 0% |
| 3:40 | 2,650 | 2,650 | 2,646 | 2,647 | 11 ms | 2.7 ms | 0% | 0% |
| 3:50 | 2,763 | 2,763 | 2,760 | 2,760 | 35 ms | 2.8 ms | 0% | 0% |
| 4:00 | 2,877 | 2,877 | 2,872 | 2,873 | 13 ms | 2.7 ms | 0% | 0% |
| 4:10 | 2,990 | 2,990 | 2,846 | 2,986 | 595 ms | 2.8 ms | 0% | 0% |
| 4:20 | 3,103 | 3,103 | 2,500 | 3,100 | 1.90 s | 3.0 ms | 0% | 0% |
| 4:30 | 3,217 | 3,217 | 2,499 | 3,213 | 2.45 s | 28 ms | 0% | 0% |
| 4:40 | 3,330 | 3,330 | 2,510 | 3,327 | 2.39 s | 3.9 ms | 0% | 0% |
| 4:50 | 3,443 | 3,443 | 2,495 | 3,440 | 2.55 s | 30 ms | 0% | 0% |
| 5:00 | 3,500 | 3,500 | 2,403 | 3,500 | 2.77 s | 20 ms | 0% | 0% |
| 5:10 | 3,500 | 3,500 | 2,210 | 3,357 | 2.87 s | 648 ms | 0% | 0% |
| 5:20 | 3,500 | 3,500 | 2,433 | 3,417 | 2.57 s | 1.02 s | 0% | 0% |
| 5:30 | 3,500 | 3,500 | 2,409 | 3,656 | 2.71 s | 839 ms | 0% | 0% |
| 5:40 | 3,500 | 3,500 | 2,551 | 3,500 | 2.59 s | 50 ms | 0% | 0% |
| 5:50 | 3,500 | 3,500 | 2,435 | 3,114 | 2.75 s | 839 ms | 0% | 0% |
| 6:00 | 3,500 | 3,500 | 2,445 | 3,413 | 3.04 s | 1.07 s | 0% | 0% |
| 6:10 | 3,500 | 3,500 | 2,430 | 3,369 | 3.07 s | 1.13 s | 0% | 0% |
| 6:20 | 3,500 | 3,500 | 2,353 | 3,296 | 3.32 s | 1.32 s | 0% | 0% |
| 6:30 | 3,500 | 3,500 | 2,379 | 3,183 | 3.34 s | 1.77 s | 0% | 0% |
| 6:40 | 3,500 | 3,500 | 2,414 | 3,412 | 3.08 s | 1.68 s | 0% | 0% |
| 6:50 | 3,500 | 3,500 | 2,486 | 3,334 | 2.96 s | 1.56 s | 0% | 0% |

</details>

## Diagnosis

### A

- **Queueing knee (db CPU), from 3:50.** From 3:50, p99 stayed above 13 ms (3.1 ms at low load) with db CPU at 72%. Near saturation requests wait behind each other, so latency grows non-linearly, not in proportion to load.

### B

- **Queueing knee (db CPU), from 4:50.** From 4:50, p99 stayed above 12 ms (2.5 ms at low load) with db CPU at 73%. Near saturation requests wait behind each other, so latency grows non-linearly, not in proportion to load.

## Predictions

| Prediction | A | B |
| --- | --- | --- |
| `break_load_rps within 25% of 3000` | held, worst 2,990 rps at 4:10 | not in this design |
| `errors_load_rps none` | held, worst no failures | not in this design |
| `p99_ms < 300 until load_rps >= 3000` | not in this design | broken, worst 361 ms (broke at 0:50) |
| `error_rate < 1% throughout` | not in this design | held, worst 0% |

## Design changes

```diff
--- A/design.yaml
+++ B/design.yaml
@@ -1,5 +1,5 @@
 version: 1
-name: when-to-scale-your-database
+name: when-to-scale-your-database-one-query-per-page
 
 # A feed service: every request returns a page of three items from Postgres,
 # and one request in ten also updates an item.
@@ -17,7 +17,7 @@
     behavior:
       cpu_ms: 0.5
       calls:
-        - { target: db, op: read,  count: 3 }              # the page's three items, one query each
+        - { target: db, op: read,  query: batch, rows: 3 }  # the page's three items in one query
         - { target: db, op: write, count: 1, when: write }
     pools: { db: { size: 24, min: 24 } }   # opened at start; 4 replicas x 24 = 96 connections, under max_connections
 
@@ -36,8 +36,7 @@
   mix: { read: 0.9, write: 0.1 }
   keys: { distribution: zipf, skew: 1.1, count: 5000000 }
 
-# Your predictions, scored in the report. Replace these with your own
-# before you run it.
+# The starting design's predictions, kept so the two reports compare.
 expect:
-  - break_load_rps within 25% of 3000
-  - errors_load_rps none
+  - p99_ms < 300 until load_rps >= 3000
+  - error_rate < 1% throughout
```
