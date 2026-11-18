# A 200 ms timeout

**Spoiler:** this is part of the answer to [The flaky network](../../README.md). Try the fix yourself first.

## What it is

The starting design with the timeout raised from 150 ms to 200 ms, a third more:

```yaml
resilience:
  timeout_ms: 200
```

Nothing else changed. It's the fix that comes from looking at the report and seeing calls fail at 150 ms.

## What it shows

It breaks just like the starting design: about 1 page in 10 fails while the loss lasts.

The calls that fail aren't a little slow; they're waiting for TCP to resend a lost packet, and TCP on Linux waits at least 200 ms before it does. The resent packet arrives just after that, so a 200 ms timeout still gives up first. Raising a timeout a little, to just above the slowest calls you've seen, doesn't help when the slow calls come from a fixed timer you can't see in the application. Size it to the tail with room to spare, or retry: both are in [`solutions/`](../../solutions/).

In the reference run (host speed index 984), between 9.0% and 10.6% of pages failed in each 10 seconds of the loss, against 8.6% to 9.7% with 150 ms: p99 sat at 208 ms, the calls running into the new timeout instead of the old one.
