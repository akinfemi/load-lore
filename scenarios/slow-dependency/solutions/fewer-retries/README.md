# One retry, with backoff

**Spoiler:** this is a worked answer to [The slow dependency](../../README.md). Try your own first.

## What changed

The api retries a failed call to pricing once instead of three times, after a random wait of up to 50 ms:

```yaml
resilience:
  timeout_ms: 250
  retries: { max: 1, backoff: exponential, base_ms: 50, jitter: true }
```

Nothing else changed.

## Why it helps

Pricing can serve about 400 calls a second: one CPU, about 2.5 ms of it per call. Users send 150 requests a second. What decides whether pricing recovers after the pause is how many calls it gets for each of them while it's in trouble.

In the starting design every call that waits more than 250 ms fails and is tried again, up to four calls per request: 600 a second, half again what pricing can serve. Once the pause leaves pricing with a backlog, every call waits longer than 250 ms, so every call fails and is retried, and pricing spends its CPU on calls whose callers have already given up. Nothing in that loop needs the pause any more. The outage feeds itself until the load ends.

With one retry, a failing request makes at most two calls: 300 a second, which still fits in the 400. Pricing gets through its backlog faster than it grows, calls start finishing inside 250 ms again, and the retries stop. The requests caught by the pause still fail, but the outage ends within seconds of it.

The wait before the retry matters less than the count. It spreads the retries out, so they don't all land on pricing in the same instant it wakes up, but it doesn't change how many there are; `variants/backoff-only` shows that.

## What it costs

- A request that fails once and then fails its retry reaches the user as an error. With three retries, a single unlucky failure was hidden; with one, two in a row aren't.
- The rule of thumb behind it: at the worst moment, a dependency gets its normal load times the attempts per request. Here that's 2 × 150 against 400. Add replicas or callers, or grow the load, and the sum changes: a retry budget, which caps retries at a share of all calls, keeps it true as things grow.

## Reference run

In the reference runs (8-vCPU machine, `--scale 1`, seed 1, host speed index 1012 for the starting design and 1015 for this one), the 10 seconds with the pause had 56% of requests failing here and 89% in the starting design. The difference is what came after. Here pricing's CPU went to 62% in that window and back to 41%, calls per request back to 1.0, and no request failed for the rest of the run. The starting design never recovered: 4.0 calls per request, three of them retries, pricing at 100% of its CPU and every request failing until the load ended. [`report.md`](report.md) is this run's report. Compare your own pair of runs, not these numbers.
