# More connections

**Spoiler:** this is a worked answer to [The slow disk](../../README.md). Try your own first.

## What changed

Five times the api's database connections:

```yaml
pools:
  db: { size: 40, min: 40 }   # 2 replicas x 40 = 80 connections
```

Nothing else changed. In the connection pool trap scenario, more connections was a trap; here it's an answer, and the difference is what the connections wait on.

## Why it helps

The database's CPU is nearly idle while the disk is slow. Its commits wait on the disk, each holding a connection for as long as the slow writes and fsyncs take, and 16 connections can't cover 200 commits a second at that length, with the reads queued behind them.

More connections let more commits wait at once, and Postgres makes waiting together cheaper: commits that are waiting at the same moment share one flush of the write-ahead log (group commit), so the disk's delay is paid once for all of them, not once each. With 80 connections, enough commits share each flush that the database keeps up, and the reads always find a free connection. Each request is slower while the disk is slow, but it succeeds.

## What it costs

- Every connection is a Postgres backend process with its own memory: 80 cost more than 16, and this database's `max_connections` of 100 leaves little room for more replicas.
- It's slower: p99 goes from 2 ms to about 120 ms while the disk is slow, because every commit still waits on it.
- It works because the waits are on the disk. When the database is busy instead, more connections just queue more work on its CPU, as in the connection pool trap.

## Reference run

In the reference runs (8-vCPU machine, `--scale 1`, seed 1, host speed index 977 for the starting design and 1001 for this one), 18–25% of requests failed in each 10 seconds of the slow disk in the starting design. Here none failed; p99 was 123 to 270 ms while the disk was slow, against 2 ms with `synchronous_commit` off (`solutions/async-commit`), and the database's CPU stayed at most 5%. [`report.md`](report.md) is this run's report. Compare your own pair of runs, not these numbers.
