# A timeout sized to the tail

**Spoiler:** this is a worked answer to [The flaky network](../../README.md). Try your own first.

## What changed

The api gives each call to inventory 1 second instead of 150 ms:

```yaml
resilience:
  timeout_ms: 1000
```

Nothing else changed.

## Why it helps

When a packet of inventory's reply is lost, TCP on inventory's side notices that it wasn't acknowledged and sends it again, after its retransmission timeout: at least 200 ms on Linux. The call doesn't fail; it takes a little over 200 ms instead of 2. A 150 ms timeout gives up before the resend arrives, and turns a slow call into a failed page. With 10 calls a page, about 1 page in 10 had at least one such call.

A timeout sized to the tail, the slowest calls you're willing to wait for rather than the typical one, lets the resend arrive. The page that lost a packet is slower, by about 200 ms, and it succeeds.

## What it costs

- Those pages are slow: p99 goes from about 3 ms to about 225 ms while the loss lasts. Here that's within the 500 ms target; with more lost packets per page, or a slower dependency, it may not be.
- A long timeout holds a connection and a worker for as long as it waits. If inventory really were down, every page would wait the full second before failing, and a service with a fixed number of workers can run out of them. Long timeouts go with a bulkhead or a circuit breaker, so a dead dependency fails fast.

## Reference run

In the reference runs (8-vCPU machine, `--scale 1`, seed 1, host speed index 1008 for the starting design and 987 for this one), between 8.8% and 10.8% of pages failed in each 10 seconds of the loss with the 150 ms timeout, against 1 − 0.99¹⁰ ≈ 9.6% predicted. Here none failed; p99 rose from 3 ms to 223–225 ms while the loss lasted, and the median stayed at 2.7 ms. [`report.md`](report.md) is this run's report. Compare your own pair of runs, not these numbers.
