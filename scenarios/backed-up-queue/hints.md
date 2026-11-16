# Hints

Reveal them one at a time with `loadl hint backed-up-queue`. Each one assumes you've run the design and have its report open.

## Read the per-window table

Find the queue's columns: jobs waiting, the oldest one's age, and jobs published and finished a second. When did published first run ahead of finished? What happened to checkout's errors some time later, and what did the queue look like then?

## What a full queue does

The queue holds its jobs in memory, and it never throws one away to make room. Once it's full, what can it do with the next order's email? And what happens to the checkout request that was publishing it?

## How fast can the workers go?

A worker runs a few jobs at once, and each job waits about 150 ms on the provider. So one job runner sends about 1 ÷ 0.15 emails a second. How many runners are there in all, and how many emails a second is that? How many arrive at the peak, with three requests in ten being orders?

Throughput is jobs at once ÷ time per job: Little's law, for workers.

## Waiting, or working?

Look at the workers' CPU. If they're mostly idle, what are their job runners doing for those 150 ms? Would more of them on the same replicas help? What else would each new runner need, to call the provider at the same time as the others?

## Ways out

1. Let each worker run more jobs at once, with enough connections to the provider for all of them.
2. Run more worker replicas.
3. Give the queue more memory.

One of these is worked in `solutions/`. Only one of them gets the emails out within a minute without adding machines; run the third and look at the oldest job before you trust it.
