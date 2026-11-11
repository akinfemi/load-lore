# Hints

Reveal them one at a time with `loadl hint connection-pool-trap`. Each one assumes you've run the design and have its report open.

## Start with the verdict and the database

The first line of the report says when it broke and which signal crossed the line. Now look at the database's CPU in the per-window table at that moment. Is the database the part that's struggling?

## Which rule fired?

Read the diagnosis. A full pool can be a symptom of a slow database, or the limit itself. Which case does the report name, and which metric rules out the other?

## How long is a connection busy?

Find the evidence that says how long each connection was held, and how long its queries took with it. Where does the rest of that time go?

With 40 connections each held for that long, how many requests per second can the pools serve at most? Compare it with the load at the break.

## The obvious fix, and its trap

Bigger pools would let more requests hold a connection at once. Before you try it, add up what the database would see: replicas × pool size, against `max_connections` (and the metrics exporter holds one of those). What happens when a pool asks for one more connection than the database allows? Try it and read the errors; `variants/bigger-pools` is that design, with what it shows.

## Two ways out

1. Don't hold a database connection while you wait on something else: take one for each query and hand it back.
2. Put a connection pooler between the api and the database, so the api can hold many cheap client connections while the database sees only a few, each used only for a transaction.

Pick one, predict the new breaking point in `expect`, run it, and compare. Both are worked in `solutions/`.
