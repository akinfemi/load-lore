# A shallow check on a slow database

**Spoiler:** this is part of the answer to [Healthy but broken](../../README.md). Try the fix yourself first.

## What it is

The starting design, with its shallow `/healthz` check, and the fault of `variants/deep-check-slow-db` instead of the partition: from 1:01 the database is slow for a minute, every query 200 ms longer.

## What it shows

It holds: every request is served, about 200 ms slower while the database is slow. `/healthz` doesn't touch the database, so the slowness doesn't reach the load balancer's view of the replicas, and they all stay in rotation.

Put next to the starting design, it's the trade-off of health checks: the shallow check missed a replica that couldn't do its job at all, and it rode out a dependency that was only slow. The deep check catches the first, and with a timeout too tight for the second, turns it into an outage.

In the reference run (host speed index 1014), no request failed; p99 was 210 ms while the database was slow, against 2 ms before, and all three replicas stayed in rotation. With the tight deep check (`variants/deep-check-slow-db`), the same minute failed every request.
