# One query per page

**Spoiler:** this is a worked answer to [When do you need to scale your database?](../../README.md). Try your own first.

## What changed

One line in the api's behavior. Each request used to fetch its page of three items with three queries, one round trip each:

```yaml
- { target: db, op: read, count: 3 }
```

Now it fetches all three by id in one statement (`SELECT … WHERE id = ANY($1)`):

```yaml
- { target: db, op: read, query: batch, rows: 3 }
```

Everything else is the same: the same 0.4-CPU Postgres, the same pools, the same ramp.

## Why it helps

The starting design broke when Postgres ran out of CPU. For a lookup by primary key, most of that CPU isn't spent finding the row. It goes on everything around it, once per statement: receiving the query, binding its parameters, starting the executor, sending the result back, and the network round trip. Three statements per page pay that three times; one statement pays it once. In a pgbench run on a 0.5-CPU Postgres (this scenario's earlier size) with reads only, it managed 10,951 single-row lookups per second, but only 5,830 batches of three rows: fetching three rows in one statement cost about twice as much as fetching one, not three times.

In the reference runs (8-vCPU machine, `--scale 1`, seed 1, host speed index 1033 and 984), the starting design broke at 3:30, at 2,537 rps offered, with Postgres at 100% CPU; the most it served in any 10-second window was 2,764 requests per second. This design got through the rest of the ramp, with Postgres at 48% CPU at 3:30 and 93% when the load reached 3,500 at 5:00, and served 3,391 of the 3,500 requests a second there. It broke 10 seconds into the hold, at 5:10, with Postgres out of CPU again. Its best window served 3,619, 1.31× the starting design's peak. Peaks vary from run to run: compare your own pair of runs, not these numbers.

A database benchmark predicts more. With pgbench on a 0.5-CPU Postgres and the same mix of 10% writes, one batched statement served 6,537 pages per second against 4,187 for three single-row queries (1.56×). In the full run, the api and the load balancer add their own work, each hop adds the network's round trip, and the database is CPU-throttled in bursts (next section).

## Bursts at the peak

It isn't smooth near the peak, or before it. p99 rose to 514 ms at 1:10, with Postgres at 37% CPU, and to 37–112 ms in several windows from 4:00 to 4:50, while every offered request was still served: bursts of queries use up the database's CPU quota early in each 100 ms scheduling period, and Postgres waits for the next one.

Small CPU limits are bursty, so compare peak throughput and the shape of the curves, not just the break time. Fewer connections in the pool would likely smooth these bursts, which is the subject of the connection pool scenario.

## What it costs

- The handler changes: it collects the ids first, then fetches them together. In real code that often means replacing a loop of lookups (the N+1 pattern) with one query.
- A slow page now waits for one bigger query instead of three small ones, so its latency is less spread out but not always lower.
- The way you write the batch matters. A first attempt here used a range (`WHERE id >= $1 ORDER BY id LIMIT 3`), which peaked at 3,129 rps (1.12×) in a run from before the database moved to a gp3 disk. With pgbench and writes, the range served 4,935 pages per second against 6,537 for the batch; without writes the two were close (5,583 and 5,830). We haven't confirmed why. A likely cause: Postgres plans a range query with a parameterized `LIMIT` afresh on every call, and estimating a range near the low end of the index means reading the index there, where the most popular rows sit and keep being updated.

## Reference run

[report.md](report.md) is this design's report from the run described above, and [compare.md](compare.md) puts it next to the starting design's reference run. Compare it with your own run of the starting design:

```sh
loadl run scenarios/when-to-scale-your-database/solutions/one-query-per-page/design.yaml
loadl compare <your-first-run> latest -o compare.md
```
