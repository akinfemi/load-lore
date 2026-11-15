# Scale on requests in flight

**Spoiler:** this is a worked answer to [Featured on Hacker News](../../README.md). Try your own first.

## What changed

The autoscaler holds requests in flight per replica at 10, instead of CPU at 70%:

```yaml
autoscale: { max: 6, metric: inflight, target: 10 }   # was metric: cpu, target: 0.7
```

## Why it helps

CPU stops at 100% of a replica's limit. With a 70% target, a decision over saturated replicas asks for at most 1 ÷ 0.7 = 1.43 times as many, so two replicas become three, and the next step waits until the third is serving: about 30 seconds of start-up each time. In the reference check, the starting design's replicas arrived at 2:02 and 2:47, and the sixth, asked for at 3:00, hadn't arrived when the load ended at 3:15: it never recovered.

Requests in flight keep growing while a backlog builds. When the surge lands, each replica has a hundred or more requests waiting, ten times the target, so the first decision asks for all six. They still take 30 seconds to start, and the run still breaks as the surge lands, at 1:20, but all six took traffic at 2:02 and it recovered at 2:20, instead of not at all.

## What it costs

- **A metric to export and trust.** CPU comes from the platform; in-flight requests come from the app, or from the load balancer, and the autoscaler has to read them.
- **A target to tune.** 10 in flight suits this api's 1 ms of CPU per request. A slower endpoint holds more in flight at the same load, so the right target depends on what the replicas do.
- **It still breaks.** Nothing reactive beats the time a new replica takes to start. For a surge faster than that, see `solutions/headroom`.

## The reference run

`report.md` is the reference run on a MacBook Air M4 on mains power.
