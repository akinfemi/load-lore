# One retry

**Spoiler:** this is a worked answer to [The flaky network](../../README.md). Try your own first.

## What changed

The api keeps its 150 ms timeout, and retries a call that ran past it once, at once:

```yaml
resilience:
  timeout_ms: 150
  retries: { max: 1 }
```

Nothing else changed.

## Why it helps

A call whose reply lost a packet would wait about 200 ms for TCP's resend. Here it gives up at 150 ms, and the api sends the call again, on a new connection. With 1% of packets lost, the second try loses one too only about 1 time in 100, so a page fails only when the same call is unlucky twice: the error rate falls from about 10% to a fraction of a percent. The page that lost a packet takes about 150 ms more.

## What it costs

- Every retry is a second call to inventory. Here that's 1 call in 100 more; if inventory were slow because it was overloaded, rather than because the network was, retries would add load just when it can least take it. The slow dependency scenario is that case.
- A retry is only safe for calls that can be repeated without harm. A read is; a payment isn't, unless the dependency recognizes a repeated request.
- The timeout still has to be right. Much shorter, and calls that are merely a little slow would be retried too.

## Reference run

In the reference runs (8-vCPU machine, `--scale 1`, seed 1, host speed index 1008 for the starting design and 985 for this one), between 8.8% and 10.8% of pages failed in each 10 seconds of the loss without the retry. Here at most 0.2% did; p99 was 161 ms, the 150 ms timeout plus a quick second try, against 223–225 ms for a 1 s timeout that waits for TCP's resend instead (`solutions/timeout-for-the-tail`). [`report.md`](report.md) is this run's report. Compare your own pair of runs, not these numbers.
