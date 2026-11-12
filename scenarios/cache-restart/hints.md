# Hints

Reveal them one at a time with `loadl hint cache-restart`. Each one assumes you've run the design and have its report open.

## Find the restart

Find Redis's restart in the timeline, then look at the cache's hit rate and the database's CPU in the per-window table before and after it. The restart itself takes a second or two. How long do the errors go on?

## Read the diagnosis

The report names a cold cache. Look at its evidence: the share of lookups that missed, and the database's reads. Then look at why requests failed: they waited for a database connection, or their query ran past its timeout. How long does one review summary take when the database is idle, and how long after the restart?

## Count the duplicates

The popular products get most of the traffic. Right after the restart, every request for the most popular one misses, and each runs the same query for the same answer. How many copies can run at once, with 30 connections? And what happens to a copy that's still running when its 1-second timeout passes?

## Why it never ends

A query the api gives up on is stopped, and fills nothing. With 30 copies sharing the database's one CPU, each takes longer than its timeout, so none finishes, and the next requests miss and start them again. The database stays busy with answers no one receives. Nothing in that loop needs Redis to be empty any more; it only needs the queries to keep timing out.

## Why the start of the run was fine

The cache was empty at 0:00 too. How many requests a second arrived then, and how many at 3:01? At 10 a second, a miss runs alone and finishes in time.

## Ways out

1. Make the requests that miss the same product at the same time share one query: the first runs it and fills the cache, the others wait for its answer. That's request coalescing, or single-flight; in the design it's one setting on the cache lookup.
2. Let fewer queries run at once, so each finishes inside its timeout and fills the cache. Which setting decides how many run at once?

Pick one, predict what the run will show in `expect`, run it, and compare. Both are worked in `solutions/`. Then think about what each costs, and what you'd do if the catalog were a hundred times bigger.
