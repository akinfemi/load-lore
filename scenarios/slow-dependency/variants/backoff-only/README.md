# Backoff with jitter, still three retries

**Spoiler:** this is part of the answer to [The slow dependency](../../README.md). Try the fix yourself first.

## What it is

The starting design with exponential backoff and full jitter between the api's retries:

```yaml
resilience:
  timeout_ms: 250
  retries: { max: 3, backoff: exponential, base_ms: 100, jitter: true }
```

The waits before the three retries are random, up to 100, 200 and 400 ms. Nothing else changed. It's the fix most guides suggest first for a retry storm.

## What it shows

It breaks just like the starting design, and doesn't recover either.

Backoff decides when a retry is sent, not whether it is. Each failing request still makes four calls to pricing, now spread over about a second and a half instead of one. Once pricing has a backlog and every call fails, the api sends 150 new requests a second, each of which makes four calls sooner or later: 600 calls a second, against the about 400 pricing can serve. The spreading helps against a burst, such as every client retrying in the same instant pricing wakes up, but in a steady overload the rate is the same.

The report's retry storm diagnosis gives the count: four calls per request, three quarters of them retries. To end the storm, the number of calls per failing request has to come down, with fewer retries, or to zero for a while, with a circuit breaker; both are in [`solutions/`](../../solutions/).

In the reference run (host speed index 1037), it broke at 2:00 like the starting design, with 4.0 calls to pricing per request from 2:10 to the end, pricing at 100% of its CPU, and every request failing. p99 was 1.67 s instead of 1.09 s: the same four attempts, with the waits between them added.
