# Hints

Reveal them one at a time with `loadl hint live-vote`. Each one assumes you've run the design and have its report open.

## Read the diagnosis

The report names two things from the same moment: hot rows, and a queueing knee on the database's CPU. The CPU really was busy. Read the knee's "Try next" and the hot-rows evidence: what was the database's CPU doing?

## What a vote does

Each vote runs one statement: update the favourite's row, then commit. Postgres locks a row while a transaction updates it and keeps the lock until the commit has been flushed to disk. A second vote for the same contestant has to wait for the first.

How many votes a second land on the favourite's row at the load where it broke? Take the offered load from the verdict, four in five requests are votes, nine in ten votes are for the favourite.

## One at a time

The favourite's row takes its updates one after another, however many connections and CPUs there are. Look up how long a write took at low load. How many updates a second can one row take, at that speed? What happens to the waiting votes as the rate gets close?

Past a point, the waiting isn't idle. Each waiting backend wakes when the row is released and checks it again, and every update leaves an old version of the row behind for the next one to step over. Compare the database's CPU per request before and after the break.

## What more CPU buys

If the CPU went on the contention rather than on more work, how much later would twice the CPU break? Predict it, then run `variants/bigger-db` and compare.

## Ways out

1. Count the votes somewhere that takes increments to one key cheaply, with no row lock and no commit to wait for, and copy the totals into Postgres now and then.
2. Spread one tally over several rows, picked at random per vote and summed on read.
3. Record each vote as its own row and count them later.

This version of Loadlore can model the first: the workload's keys stand for the rows, so the other two would change the traffic itself. The first is worked in `solutions/`.
