# A deep check on a slow database

**Spoiler:** this is part of the answer to [Healthy but broken](../../README.md). Try the fix yourself first.

## What it is

The solution's deep health check, with a tight timeout:

```yaml
health_check: { path: /readyz, interval_ms: 1000, timeout_ms: 150, fall: 2, rise: 2 }
```

and a different fault: no partition, but from 1:01 the database is slow for a minute, every query 200 ms longer. Slow, but working.

## What it shows

It breaks, completely: every request fails for about a minute.

Each replica's `/readyz` pings the database, the ping now takes over 200 ms, and the check gives up at 150. Every replica fails its check at the same moment, because they all depend on the same database, and the load balancer takes all three out. With no replica in rotation, it refuses every request, though every replica could have served them, 200 ms slower. When the database speeds up, the checks pass again and the replicas come back.

[`variants/shallow-check-slow-db`](../shallow-check-slow-db/) is the same slow database behind the starting design's shallow check: it held, every request served, p99 about 210 ms.

A deep check is worth having only with a timeout the dependency meets even on a bad day, and with the load balancer keeping some replicas when every check fails at once: the cause is then more likely shared than in every replica.

In the reference run (host speed index 1012), the load balancer had no replica in rotation from 1:00 to 2:10: every request failed from 1:10 to 2:00, after 68% in the window the slowness started in. Behind the shallow check (`variants/shallow-check-slow-db`, host speed index 1014), the same minute failed none.
