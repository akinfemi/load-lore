# Count the votes in Redis

**Spoiler:** this is a worked answer to [The live vote](../../README.md). Try your own first.

## What changed

A vote no longer updates the contestant's row in Postgres. It increments the contestant's key in a new Redis, `tally`:

```yaml
- { target: tally, op: set, when: write }  # was: { target: db, op: write, when: write }
```

The results screen still reads the tallies from Postgres. In a real system a small job copies Redis's totals into Postgres every few seconds: ten writes every few seconds, which this design leaves out. Loadlore's synthetic service has no increment operation, so a `set` of the key stands in for `INCR`; both are one in-memory operation on one key.

## Why it helps

The tally was one row, and a row takes its updates one at a time: each one locks the row until its commit. At the break, about 2,400 votes a second queued for the favourite's row, and the contention burned the database's CPU, so twice the CPU barely moved the break (`variants/bigger-db`).

Redis runs commands one at a time too, but an increment holds nothing: no lock to wait for, no commit, no old version of the row left behind. It takes microseconds, so one key takes hundreds of thousands a second. Postgres is left with the reads, which don't take the row's lock.

## What it costs

- **Durability.** Votes counted in Redis and not yet copied are lost if Redis restarts, unless it writes them to disk (append-only file), which costs throughput. For a vote, decide how many lost votes are acceptable; for money, don't count this way.
- **A second system** to run, monitor and keep available. If Redis is down, votes fail.
- **Results lag.** The screen shows the totals as of the last copy, a few seconds old.

Other ways out, which this version of Loadlore can't model because they change which rows the traffic touches: spread each tally over several rows (picked at random per vote, summed on read), or insert each vote as its own row and count later.

## The reference run

`report.md` is the reference run on a MacBook Air M4 on mains power.
