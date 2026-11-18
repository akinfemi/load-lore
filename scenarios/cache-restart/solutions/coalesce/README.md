# Request coalescing

**Spoiler:** this is a worked answer to [The cache restart](../../README.md). Try your own first.

## What changed

One setting on the api's cache lookup:

```yaml
- target: cache
  op: get
  coalesce: true
  on_miss:
    - { target: db, op: read, query: scan }
    - { target: cache, op: set, ttl_s: 600 }
```

When requests in one api replica miss the same product at the same time, the first runs the `on_miss` calls, computing the review summary in Postgres and storing it in Redis, and the others wait for its answer instead of each running the query. It's what Go's `singleflight`, or a cache library's loader, does. Nothing else changed: the same 10 database connections per replica, the same 1-second timeout.

## Why it helps

Right after the restart, every lookup misses, and 1,500 requests a second arrive, most of them for a handful of popular products. In the starting design, each miss takes one of the api's 30 database connections to run the same slow count over the reviews table. Thirty copies share the database's one CPU, so each takes longer than its 1-second timeout; the api gives up on all of them, and none fills the cache. The next requests miss too, and the herd never ends.

With coalescing, each api replica runs the query once per product, however many requests are waiting for it. Fewer queries run at once, the popular products' ones finish inside their timeout, and each fills the cache for every request after it. The herd ends once the popular products are back, and the rest follow.

The restart still hurts: the requests during Redis's own restart fail, and the herd lasts until the popular products are cached again, with the database at 100% meanwhile. The difference is that it ends.

## What it costs

- Waiting requests are only as fast as the query they wait on: when it fails or times out, all of them fail together.
- Coalescing works within one process. Three replicas still run up to three copies of each query; with many replicas, that adds up, and a shared lock or a refresh in the background comes into play.
- It doesn't help with misses for many different keys at once, such as a catalog much bigger than its hot set: those are different queries, and the database has to run each one.

## Reference run

In the reference runs (8-vCPU machine, `--scale 1`, seed 1, host speed index 990 for the starting design and 1010 for this one), the restart broke both: about 23% of requests failed here in the first 20 seconds after it, 32% in the starting design. Here the failures fell to 14% in the fourth window and 0.5% in the fifth, and the run was healthy from 3:50 to the end, with the database back at 3% of its CPU. The starting design never recovered: about 34% of requests failed in every window until the load ended, with the database at 100%. [`report.md`](report.md) is this run's report. Compare your own pair of runs, not these numbers.
