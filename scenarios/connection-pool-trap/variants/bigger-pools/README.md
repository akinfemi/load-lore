# Bigger pools

**Spoiler:** this is part of the answer to [The connection pool trap](../../README.md). Try the fix yourself first.

## What it is

The starting design with each api replica's database pool three times bigger:

```yaml
pools:
  db: { size: 30, hold: request }   # 4 replicas x 30 = 120 connections
```

Nothing else changed. It's the fix most people try first, because the report says the pools are full.

## What it shows

It gets further than the starting design, then breaks with errors instead of slow responses.

The pools open connections as they need them, so at first the extra size is real capacity: more requests can hold a connection through the call to the payment provider at once. But 4 × 30 is 120, and Postgres allows 100, one of which the metrics exporter needs. Once the pools have opened 99 between them, Postgres refuses the next connection (`sorry, too many clients already` in its log), and the request that asked for it fails. From there, served throughput stays flat while the offered load keeps rising, and the rest fails: 15% of requests at the peak in a run on the reference machine (`meta.yaml`). The report's table of error causes names it: `too_many_conns`, from api's calls to db. When the exporter reconnects it is refused too, so the `db conns` column goes blank.

The limit is still the number of connections, not the work they do. But refusing a connection isn't free: Postgres starts a backend process for every attempt before it can turn it away. In that run the database refused about 200 connections a second at the peak, one for each failed request, and its CPU went from 9% to 25% while it served about the same number of queries. On a busy host the refusals can come in bursts, and in one such run the database reached 85% and the report diagnosed a queueing knee on its CPU. Read that diagnosis with the errors: the database is busy turning connections away.

Making the pools bigger moved the limit from the api's pools to the database's `max_connections`. Raising that too would cost a Postgres backend process, and its memory, for every connection that mostly sits waiting on the payment provider.

The fixes in [`solutions/`](../../solutions/) change what a waiting request holds, not how many it may hold.
