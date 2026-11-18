# PgBouncer in transaction mode

**Spoiler:** this is a worked answer to [The connection pool trap](../../README.md). Try your own first.

## What changed

A PgBouncer sits between the api and the database, and the api's pools point at it instead of at Postgres:

```yaml
pgb:
  kind: pgbouncer
  target: db
  pool_mode: transaction
  default_pool_size: 20
  config: { max_client_conn: 500 }
```

```yaml
pools:
  pgb: { size: 100, hold: request }
```

The api still holds a connection for the whole request, through the call to the payment provider. What changed is what that connection costs: it's a client connection to PgBouncer, not a Postgres backend.

## Why it helps

In the starting design, a request holds its database connection for about 80 ms, nearly all of it waiting on the payment provider. 40 connections held 80 ms each can serve at most about 500 requests a second, however idle the database is: the starting design's reference run served at most 490, with its database at 5% CPU.

With PgBouncer, the api still holds a connection for the whole request, but now it's one of up to 400 client connections. By Little's law, the number held at once is the arrival rate times the time each is held: at 1,400 requests a second and about 82 ms each, about 115. The reference run showed 115 of the 400 in use. PgBouncer, in transaction mode, lends one of its 20 server connections only while a query runs, about a millisecond, so at the same 1,400 requests a second it had 2 to 4 of them busy. Postgres saw 10 connections in all, and its CPU stayed under 10%.

In the reference runs (8-vCPU machine, `--scale 1`, seed 1, host speed index 986 and 1017), the most served in any 10-second window went from 490 requests per second to 1,400, the top of the load profile: p99 was 88 ms in every window but the first, at 90 ms, and there were no errors. 1,400 is where the profile stops, not where this design does. Compare your own pair of runs, not these numbers.

## What it costs

- One more component to run, size and watch. PgBouncer is single-threaded, so one instance tops out on one core.
- In transaction mode, anything that lives on a server connection between transactions doesn't carry over: session settings (`SET`), advisory locks, `LISTEN`, temporary tables. Prepared statements work, because PgBouncer tracks them for its clients.
- Requests still queue somewhere when the database is the limit: now in PgBouncer, for one of its 20 server connections, instead of in the api's pools. The report shows those clients waiting.
- `max_client_conn` is a limit too. 4 replicas × 100 is 400 client connections, plus the exporter's; at PgBouncer's default of 100, most of them would be refused.

## Reference run

[report.md](report.md) is this design's report from the run described above, and [compare.md](compare.md) puts it next to the starting design's reference run.

```sh
loadl run scenarios/connection-pool-trap/solutions/pgbouncer/design.yaml
loadl compare <your-first-run> latest -o compare.md
```
