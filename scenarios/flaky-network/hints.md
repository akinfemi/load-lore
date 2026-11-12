# Hints

Reveal them one at a time with `loadl hint flaky-network`. Each one assumes you've run the design and have its report open.

## Read the verdict

When did the errors start, and when did they stop? Compare with the fault in the timeline. Then look at why requests failed, in the report's table of causes.

## One call, one packet

Inventory's CPU didn't move, and its calls take a couple of milliseconds. What happens to a call when one packet of its reply is lost? TCP doesn't tell the program: it waits, and sends the packet again. How long does it wait before it does?

## Ten calls, not one

If 1% of calls lose a packet, what share of pages, which make 10 calls each, lose at least one? Work out 1 − 0.99¹⁰. Compare it with the error rate in the report. This is fan-out: the more calls a request makes, the more of the tail of each one it collects.

## The timeout and the tail

On Linux, TCP waits at least 200 ms before resending a lost packet. A call that needs a resend takes a little more than that. Which calls does a 150 ms timeout fail? Which would a 200 ms one?

## Ways out

1. Size the timeout to the tail, not to the median: long enough for a resend, so a lost packet costs time instead of an error.
2. Keep the short timeout, and retry a call that ran past it once: the retry is very likely not to lose a packet too.

Each has a cost. Predict what the run will show in `expect`, run it, and compare. Both are worked in `solutions/`, and `variants/timeout-200ms` is the trap in between.
