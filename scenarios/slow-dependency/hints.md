# Hints

Reveal them one at a time with `loadl hint slow-dependency`. Each one assumes you've run the design and have its report open.

## When did it end?

Find the pause in the timeline, then the first window after it with no errors. Is there one? The pause lasted two seconds; how long did the outage last?

## What is pricing doing?

Look at pricing's CPU in the per-window table before the pause and after it. The users send the same 150 requests a second the whole time. Where does the extra work come from?

## Count the calls

The retry storm diagnosis gives the calls to pricing per user request. With up to three retries, what's the most a single request can make? How many calls a second does that make at 150 requests a second, against the roughly 400 pricing can serve?

## Why doesn't it recover?

A call that waits more than 250 ms fails, and is retried at once. Once pricing has a backlog, how long does a call wait? And what happens to the work pricing does for a call whose caller has already given up?

The trigger is gone after two seconds, but the extra load the retries create is enough to keep pricing overloaded on its own. Systems that stay broken after their trigger goes away are called metastable.

## Two diagnoses

The report also diagnoses a queueing knee on pricing's CPU. Both are true. Which one caused the other, and which fix is cheaper: capacity for four times the real load, or fewer calls while pricing struggles?

## Ways out

1. Retry less: work out the most retries per request that still fits in what pricing can serve.
2. Stop calling pricing for a moment when most calls fail, so it can drain its backlog: a circuit breaker.

Backoff alone spreads retries out in time. Before you rely on it, count how many calls a failing request still makes. `variants/backoff-only` is that design.

Pick one, predict what the run will show in `expect`, run it, and compare. Both fixes are worked in `solutions/`.
