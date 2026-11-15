# Hints

Reveal them one at a time with `loadl hint stale-orders`. Each one assumes you've run the design and have its report open.

## Read the verdict to the end

The run held: no errors, and pages stayed fast. Read the sentence after "Held", then the replication-lag diagnosis. How many of the confirmation pages showed the customer's order?

## How a replica keeps up

The replica gets a copy of everything the primary writes, its write-ahead log (WAL), and replays it: the same changes, applied in the same order. A commit on the primary doesn't wait for that. So the replica is always a little behind, and a read from it can miss a write the primary already made.

Find the replica's lag in the report's table, and the confirmation page's timing in `design.yaml`. When does a page that loads 200 ms after the order see it?

## Why it falls behind

The primary applies orders from 80 connections at once. The replica replays all of them in one process, one change after another, and each change to a row whose page isn't in the replica's memory waits for its disk first. The replica's own reads queue for the same disk.

Replaying also writes. The replica writes the WAL it receives to its own disk, in small pieces as they arrive, and later the pages it changed, against a cap of 3,000 writes a second. The primary's disk isn't capped in this lab.

Compare the replica's memory with the primary's, and the size of the table. Then look at the replica's time per block read as the load rises.

## What a second replica does

Each replica replays every write the primary makes. A second one takes half the reads, but none of the replaying. Predict what happens to the lag, then run `variants/two-replicas` and compare.

## Ways out

1. Read what a request just wrote from the primary: read-your-writes. The pages that follow a write go to the primary; everything else stays on the replica.
2. Let the replica keep up: give it what the primary has, the memory to hold the rows its writes touch and the CPU to replay them.
3. Tell the client how far the write got and have it wait for the replica to catch up to it, or keep the order in the client's session until the replica has it. This version of Loadlore can't model those.

The first two are worked in `solutions/`. Which one still holds when orders double?
