# Run more jobs at once

**Spoiler:** this is a worked answer to [The backed-up queue](../../README.md). Try your own first.

## What changed

Each sender replica runs 25 jobs at once instead of 5, with a connection to the provider for each:

```yaml
consume: { from: emails, concurrency: 25 }   # was 5 (Sidekiq's default)
pools: { provider: 25 }                       # was 10
```

## Why it helps

A sender job is mostly waiting: about 1 ms of CPU, then 150 ms for the provider to accept the email. A job runner that waits can't take another job, so the workers' throughput is the jobs they run at once divided by the time each takes, Little's law: 10 runners ÷ 0.15 s is about 66 emails a second. At the peak about 180 orders a second arrive, so the queue grew by more than 100 jobs a second until it was full, and from then on it refused new emails and the checkouts that published them failed.

With 25 runners per replica, 50 in all, the workers can send about 330 emails a second, more than the peak needs, and their CPU stays low because they're still mostly waiting. In the reference check the queue never held more than 41 jobs, the oldest waited at most 148 ms, the workers' CPU stayed under a quarter of its limit, and checkout held the whole sale.

The pool has to grow too. Each runner holds a connection to the provider for its 150 ms; with 25 runners and 10 connections, 15 of them would queue for a connection, and throughput would stop at 10 runners' worth.

## What it costs

- **Load on the provider.** 50 runners can send up to 50 emails at once. A real provider has rate limits; past them it answers with errors, and the jobs fail. Check the limit before raising concurrency.
- **Memory and connections per replica.** Each runner is a thread or a coroutine with its own buffers, and its own connection.
- **It only works while the workers wait.** If each job used its 150 ms on CPU instead, more runners on the same replicas would only share the same CPU, and you'd need more replicas.

Adding replicas also works here: six replicas of 5 runners send about 200 emails a second. It costs three times the CPU for workers that sit nearly idle.

## The reference run

`report.md` is the reference run on a MacBook Air M4 on mains power. In the starting design's run, checkout broke 2:50 into the load, at 581 requests a second offered, when the queue was full.
