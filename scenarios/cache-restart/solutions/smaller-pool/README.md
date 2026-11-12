# A smaller database pool

**Spoiler:** this is a worked answer to [The cache restart](../../README.md). Try your own first.

## What changed

Half the api's database connections:

```yaml
pools:
  cache: 20
  db: 5    # 3 replicas x 5 = 15 connections
```

Nothing else changed. It looks backwards: the report says requests timed out waiting for a database connection, and the fix is fewer of them.

## Why it helps

The database has one CPU. In the starting design, after the restart, 30 copies of the review summary's count over the reviews table run at once on it, so each takes about 30 times as long as it would alone, longer than the api's 1-second timeout. The api gives up on every one, Postgres stops them, and none fills the cache: the herd never ends.

With 15 connections, half as many queries share the CPU, so each finishes in about half the time, inside the timeout. Every one that finishes fills the cache for its product, and the next requests for it hit. The requests that wait for a connection still time out while the herd lasts, but the cache fills, and the herd ends.

The pool was never the limit: the database's CPU was. A pool bigger than what the database can run at once doesn't add capacity; it adds queueing inside the database, where the api can't see it, until the timeouts make the work worthless.

## What it costs

- It still breaks: the herd lasts until the popular products are back in the cache, and requests fail meanwhile.
- The right size depends on the queries. A pool sized for this slow count is too small for fast lookups, and on a database with more CPUs, more can run at once.
- It doesn't remove the duplicate queries, only stops them from drowning each other. `solutions/coalesce` removes them.

## Reference run

In the reference runs (8-vCPU machine, `--scale 1`, seed 1, host speed index 1016 for the starting design and 1000 for this one), 19% of requests failed here in the 10 seconds after the restart and 8% in the next, against 23% in the starting design; from 3:20 the run was healthy to the end, with the database back at 3% of its CPU. The starting design never recovered: about 22% of requests failed in every window until the load ended, with the database at 100%. Here it recovered sooner than with coalescing (`solutions/coalesce`, healthy from 3:40), since fewer queries at once is what lets each finish. [`report.md`](report.md) is this run's report. Compare your own pair of runs, not these numbers.
