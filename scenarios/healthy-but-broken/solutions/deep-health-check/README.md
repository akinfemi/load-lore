# A deep health check

**Spoiler:** this is a worked answer to [Healthy but broken](../../README.md). Try your own first.

## What changed

The load balancer asks each replica's `/readyz` instead of `/healthz`, every second, and takes a replica out after 2 failed checks:

```yaml
health_check: { path: /readyz, interval_ms: 1000, fall: 2, rise: 2 }
```

Nothing else changed. `/readyz` answers only once the replica has pinged each of its dependencies, here the database, within the check's timeout (1 s, the default).

## Why it helps

`/healthz` asks the replica whether its process is up, and the cut-off replica's is. Requests sent to it wait for a database that never answers and fail at their 1-second timeout, a third of all traffic for as long as the partition lasts, while every health check passes.

`/readyz` asks whether the replica can do its job. On the cut-off replica the database ping hangs, the check fails, and two failed checks a second apart take it out of rotation. The other two replicas take its traffic, and the requests that went to it in the meantime are the only ones that fail. When the partition ends, two passing checks put it back.

## What it costs

- Every replica's health now depends on its dependencies. If the database is slow for a while, slower than the check's timeout, every replica fails its check at once, and the load balancer takes them all out: `variants/deep-check-slow-db` shows that. Give the check a timeout the dependency can always meet when it's merely slow, and consider keeping the last replicas in rotation whatever their checks say: an answer that may fail is better than none.
- More checks: a ping to each dependency every second from every replica, for every load balancer that checks it.
- A deep check only knows what it checks. A replica that can reach the database but has, say, a broken cache still passes.

## Reference run

In the reference runs (8-vCPU machine, `--scale 1`, seed 1, host speed index 1005 for the starting design and 1024 for this one), the starting design's load balancer kept all three replicas in rotation through the partition, and 33.3% of requests failed in every 10 seconds of it, at the 1-second timeout. Here it was down to two replicas within the window the partition started in: 11.8% of requests failed in that window, and none after. [`report.md`](report.md) is this run's report. Compare your own pair of runs, not these numbers.
