# Keep headroom

**Spoiler:** this is a worked answer to [Featured on Hacker News](../../README.md). Try your own first.

## What changed

The api runs four replicas at least, instead of two:

```yaml
replicas: 4   # was 2
autoscale: { min: 4, max: 6, metric: cpu, target: 0.7 }   # was min 2
```

## Why it helps

An autoscaler reacts after the load arrives: the next decision, then the new replicas' start-up. Four replicas carry about 1,400 requests a second on their own, so when the surge lands they absorb it while the autoscaler adds the last two. The run holds the whole profile; at the top, p99 rose to 70 ms for the 40 seconds before the new replicas arrived.

## What it costs

- **Money, all the time.** Two extra replicas run every hour of every day, for a surge that may come once a month. Headroom is insurance.
- **Guessing the surge.** Four is enough for nine times the usual traffic here. A bigger surge outruns it again.
- **The better version is a schedule.** When the surge is expected, a launch or a campaign, scale up ahead of it and back down after, and keep the minimum low the rest of the time.

## The reference run

`report.md` is the reference run on a MacBook Air M4 on mains power.
