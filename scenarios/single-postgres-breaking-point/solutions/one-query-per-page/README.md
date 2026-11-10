# One query per page

**Spoiler:** this is a worked answer to [When does one Postgres fall over?](../../README.md). Try your own first.

## What changed

One line in the api's behavior. Each request used to fetch its page of three items with three queries, one round trip each:

```yaml
- { target: db, op: read, count: 3 }
```

Now it fetches all three by id in one statement (`SELECT … WHERE id = ANY($1)`):

```yaml
- { target: db, op: read, query: batch, rows: 3 }
```

Everything else is the same: the same 0.5-CPU Postgres, the same pools, the same ramp.

## Why it helps

The starting design broke when Postgres ran out of CPU. For a lookup by primary key, most of that CPU isn't spent finding the row. It goes on everything around it, once per statement: receiving the query, binding its parameters, starting the executor, sending the result back, and the network round trip. Three statements per page pay that three times; one statement pays it once. In a pgbench run on the same 0.5-CPU Postgres with reads only, it managed 10,951 single-row lookups per second, but only 5,830 batches of three rows: fetching three rows in one statement cost about twice as much as fetching one, not three times.

In the reference runs (8-vCPU machine, `--scale 1`, seed 1), the most the system served in any 10-second window went from 2,791 requests per second to 3,422 (1.23×), and Postgres reached 98% CPU 20 seconds later in the ramp (3:10 instead of 2:50).

The gain is smaller than a database benchmark predicts. With pgbench on the same 0.5-CPU Postgres and the same mix of 10% writes, one batched statement served 6,537 pages per second against 4,187 for three single-row queries (1.56×). In the full run, the api, the load balancer and Toxiproxy add their own latency, and the database is CPU-throttled in bursts (next section).

## Why the verdict says it broke earlier

The verdict marks this design as broken at 1:50, at 1,978 rps, while the starting design held until 2:50. That isn't because one query is slower. At about 1:30, the api's connection pools grew from about 20 connections to 96 at once. From then on, bursts of up to 96 active queries used up the database's CPU quota early in each 100 ms scheduling period, and Postgres sat stalled until the next one. p50 stayed at about 1.3 ms, but p99 jumped to 100–550 ms in some windows while the database averaged 60–80% CPU. The report shows it as CPU throttling under the queueing-knee diagnosis.

The starting design had the same storm at 1:50 but recovered for a while. Small CPU limits are bursty, so compare peak throughput and the shape of the curves, not just the break time. Fewer connections in the pool would likely smooth it out; that's the subject of the connection pool scenario.

## What it costs

- The handler changes: it collects the ids first, then fetches them together. In real code that often means replacing a loop of lookups (the N+1 pattern) with one query.
- A slow page now waits for one bigger query instead of three small ones, so its latency is less spread out but not always lower.
- The way you write the batch matters. A first attempt here used a range (`WHERE id >= $1 ORDER BY id LIMIT 3`), which peaked at 3,129 rps (1.12×). With pgbench and writes, the range served 4,935 pages per second against 6,537 for the batch; without writes the two were close (5,583 and 5,830). We haven't confirmed why. A likely cause: Postgres plans a range query with a parameterized `LIMIT` afresh on every call, and estimating a range near the low end of the index means reading the index there, where the most popular rows sit and keep being updated.

## Reference run

[report.md](report.md) is this design's report from the run described above, and [compare.md](compare.md) puts it next to the starting design's reference run. Compare it with your own run of the starting design:

```sh
loadl run scenarios/single-postgres-breaking-point/solutions/one-query-per-page/design.yaml
loadl compare <your-first-run> latest -o compare.md
```
