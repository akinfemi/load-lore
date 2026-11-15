# Read your own writes from the primary

**Spoiler:** this is a worked answer to [Stale orders](../../README.md). Try your own first.

## What changed

The confirmation page reads the order from the primary instead of the replica. Everything else still reads the replica:

```yaml
- { target: db, op: read, when: write, mode: async, after_ms: 200 }   # was: target: ro
```

## Why it helps

The replica was always somewhat behind, and under the sale's load it fell seconds behind: a page reading it 200 ms after the order usually got the table as it was before the order. The primary has every committed write, so a read there after the commit sees it.

In the reference run, under 0.1% of the pages after an order missed it. The replica still lags, by up to 3.7 s at the top of the ramp, and the report still says so: browsing reads a slightly old catalogue. For most pages that's fine. The rule this applies is "read your own writes": a page that shows what the same customer just changed reads the primary, and the rest can read a replica.

## What it costs

- **Load on the primary.** Every order now costs the primary a read as well as a write. Here that's half the requests; if the primary was added to because it was busy, this gives some of that back.
- **Routing logic in the app.** Something has to know which reads must see a write: a flag on the session for a few seconds after a write, or reads by the same user going to the primary. Get it wrong and the stale pages come back.
- **The lag is still there.** Anything else that reads the replica right after a write, a second service or an email job, can still see old data.

## The reference run

`report.md` is the reference run on a MacBook Air M4 on mains power.
