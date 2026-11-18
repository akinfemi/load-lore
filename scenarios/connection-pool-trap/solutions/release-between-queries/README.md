# Release the connection between queries

**Spoiler:** this is a worked answer to [The connection pool trap](../../README.md). Try your own first.

## What changed

One word in the api's pool to the database:

```yaml
db: { size: 10, hold: request }
```

became

```yaml
db: { size: 10, hold: call }
```

Each query now takes a connection from the pool and hands it back as soon as it's done, instead of keeping it from the request's first query until the response. In a real app that means not holding a database connection, or a transaction, open while waiting on something else: here, the payment provider. Rails, for example, can release its connection between queries, and Django does by default unless a view runs inside a transaction.

## Why it helps

In the starting design, a request holds its database connection for about 80 ms, nearly all of it waiting on the payment provider. 40 connections held 80 ms each can serve at most about 500 requests a second, however idle the database is: the starting design's reference run served at most 490, with its database at 5% CPU.

Now a connection is held only while a query runs, about a millisecond. By Little's law, the number held at once is the rate of queries times the time each is held: 1,400 requests a second send about 1,540 queries, which keep one or two connections busy. The reference run showed 2 to 3 of the 40 in use at 1,400 requests a second, and the wait for one was 0.0 ms. The same 40 connections that ran out at about 500 requests a second now have room for many times 1,400. The wait moved to where it costs little: a request still waits 80 ms for the payment provider, but it holds only a connection to the payment provider while it does.

In the reference runs (8-vCPU machine, `--scale 1`, seed 1, host speed index 986 and 986), the most served in any 10-second window went from 490 requests per second to 1,400, the top of the load profile, with p99 at 88 ms at worst, no errors, and the database at 12% CPU at most. 1,400 is where the profile stops, not where this design does. Compare your own pair of runs, not these numbers.

Postgres still had 17 connections open at the peak, the exporter's among them, while 2 to 3 were in use: the pools keep the connections they've opened, and each one is a Postgres backend process even while it's idle. Here that's cheap; with many services each keeping a pool, it adds up.

## What it costs

- The two queries of a request may run on different connections, so they can't share a transaction. If the order write must be atomic with the cart read, that has to be designed for, for example by re-checking the cart inside the write's own transaction.
- It fixes this service only. Every other service and job that talks to the database has to behave the same way, which is the argument for PgBouncer in front of the database (the other worked answer): it enforces the sharing for everyone.

## Reference run

[report.md](report.md) is this design's report from the run described above, and [compare.md](compare.md) puts it next to the starting design's reference run.

```sh
loadl run scenarios/connection-pool-trap/solutions/release-between-queries/design.yaml
loadl compare <your-first-run> latest -o compare.md
```
