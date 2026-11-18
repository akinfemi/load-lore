# A circuit breaker

**Spoiler:** this is a worked answer to [The slow dependency](../../README.md). Try your own first.

## What changed

Each api replica gets a circuit breaker on its calls to pricing:

```yaml
resilience:
  timeout_ms: 250
  retries: { max: 3 }
  circuit_breaker: { error_rate: 0.5, window_s: 5, open_s: 2 }
```

When half or more of a replica's calls to pricing over the last 5 seconds have failed (and there were at least 20), the breaker opens: for the next 2 seconds the replica doesn't call pricing at all, and fails those requests at once. Then it lets one trial call through. If it succeeds, the breaker closes and calls flow again; if it fails, it stays open for another 2 seconds. The retries are the same three as before.

## Why it helps

In the starting design the pause leaves pricing with a backlog. Every call then waits longer than its 250 ms, fails and is retried, and four calls per request, 600 a second, is more than the about 400 pricing can serve: the retries keep it overloaded long after the pause is over.

The breaker cuts that loop. A few hundred milliseconds into the pause, most calls have failed, and the breakers open. Requests now fail fast, without calling pricing, and without retrying, since a call the breaker rejects isn't retried. Pricing gets no new work for two seconds and finishes its backlog. The trial call finds it answering in a few milliseconds, the breakers close, and the 150 calls a second it can easily serve resume.

The requests during the pause and while the breakers are open still fail: the api has no other way to price an item. The difference is that the outage ends a few seconds after the pause instead of never.

## What it costs

- While a breaker is open, every request through that replica fails, even ones pricing could have served. A breaker that opens too easily turns small blips into short outages.
- The settings are a trade: a short `open_s` recovers sooner but tries a struggling dependency again sooner; a long one gives it more time and fails more requests meanwhile.
- The three retries are still there. When pricing is slow without failing, or fails less than half the time, the breaker stays closed and they multiply its load as before. Fewer retries, or a retry budget, alongside a breaker is the common setup.

## Reference run

In the reference runs (8-vCPU machine, `--scale 1`, seed 1, host speed index 966 for the starting design and 977 for this one), the 10 seconds with the pause had 35% of requests failing here, against 89% in the starting design, and pricing's CPU stayed at 42%, about its usual 41%: the breakers kept the retries away from it. From the next window on, no request failed. The starting design never recovered: 4.0 calls per request, pricing at 100% of its CPU and every request failing until the load ended. [`report.md`](report.md) is this run's report. Compare your own pair of runs, not these numbers.

During the pause, requests took up to a second, and up to 149 of the load generator's VUs were busy at once. The scenario starts enough before the load (`load` in `meta.yaml`), so it sent every arrival the profile asked for, and the pause was absorbed at the full load. In an earlier run with fewer VUs up front, it couldn't start 2.6% of that window's arrivals, and the result was the same: 33% of requests failed in the window, and none after it.
