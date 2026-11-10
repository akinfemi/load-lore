# Hints

Reveal them one at a time with `loadl hint`. Each one assumes you've run the design and have its report open.

## Start with the verdict

The first line of the report says when the system broke, at what offered load, and which signal crossed the line: p99, errors, or throughput. Put it next to your prediction. Were you early or late, and by how much?

Then look at the latency chart around that moment. Did p99 creep up over minutes, or bend sharply over a few windows?

## A full pool: cause or symptom?

At the break, the api's connection pool is full. That can mean two different things:

- The pool is the limit: there aren't enough connections, the database has CPU to spare, and requests queue for a connection.
- The pool is a symptom: the database is slow, so each request holds its connection longer, and the pool fills up.

Loadlore has a rule for each case. Which one fired in your report, and which metric ruled the other out? The per-window table has the column you need.

## Where latency lifts off

Find the diagnosis evidence that groups p99 by the database's CPU use. Latency stays flat for a long time, then climbs steeply over a narrow band of utilization.

That bend is queueing: when a server is busy a fraction ρ of the time, the time spent waiting grows roughly like 1 / (1 − ρ). At 50% busy, waits are short. At 90% they're several times longer, and they keep growing faster as ρ approaches 1. Which band does your report show the bend in?

## What does one request cost the database?

Look at the edge from api to db in the diagram. How many statements does each request send to Postgres? Count the reads, and add the writes for the share of requests that write.

Multiply that by the offered load at the break from your report: that's the statement rate the database could no longer keep up with on its CPU.

For a primary-key lookup, how much of the work is per row, and how much is per statement: the network round trip, parsing, planning, starting the executor and sending the result?

## Ways out, and how to test one

There are three broad ways to move the breaking point:

1. Give the database more CPU. Does your host have it to spare, tooling included?
2. Send fewer statements per request for the same data.
3. Stop some requests from reaching the database at all, for example with a cache.

Pick one. Before you run it, add an `expect` line that says where you think the new design will break. Run it, then put the two runs side by side with `loadl compare <first-run> <new-run>`.

One worked answer is in `solutions/`.
