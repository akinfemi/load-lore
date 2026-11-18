# synchronous_commit off

**Spoiler:** this is a worked answer to [The slow disk](../../README.md). Try your own first.

## What changed

One setting on the database:

```yaml
config: { synchronous_commit: "off" }
```

Nothing else changed.

## Why it helps

With `synchronous_commit` on, the default, a commit returns only after its changes are written to the write-ahead log and fsynced: two waits on the disk, each 30 ms longer while it's slow. The request holds its pool connection all that time, and the 16 connections, which serve the reads too, run out.

With it off, a commit returns once its changes are in the log's memory buffer. The WAL writer flushes the buffer in the background, a few times a second. The disk is just as slow, but nothing waits for it: commits take as long as they did before, connections are held for a moment, and the reads never notice.

## What it costs

- Durability. If the server crashes, the commits from the last few hundred milliseconds (up to three times `wal_writer_delay`, 200 ms by default) are lost, though the clients were told they succeeded. The database stays consistent; it just goes back slightly in time. For orders that's a business decision, not a technical one; for a payment ledger it usually isn't acceptable.
- It can be set per transaction, so a design can keep durable commits for what needs them and relax the rest.
- It hides the slow disk: nothing in the api's numbers shows it. Watch the database's own write latency.

## Reference run

In the reference runs (8-vCPU machine, `--scale 1`, seed 1, host speed index 977 for the starting design and 990 for this one), 18–25% of requests failed in each 10 seconds of the slow disk in the starting design, with p99 from 2.4 to 5.2 s and the database's CPU at most 17%. Here none failed, and p99 stayed at 2 ms throughout: nothing waited for the disk. [`report.md`](report.md) is this run's report. Compare your own pair of runs, not these numbers.
