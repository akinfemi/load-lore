# Twice the database's CPU

**Spoiler:** this is a variant of [The live vote](../../README.md): a fix you're likely to try. Run the starting design first.

The database gets 2 CPUs instead of 1; nothing else changes. At the break the database's CPU was near its limit, so more of it looks like the fix.

It still breaks, on hot rows, at about the same load. The CPU went on the contention for one row, not on more work: the row still takes its updates one at a time, and waiting backends spend whatever CPU they're given checking it again. The check keeps this true: `loadl scenario check` fails if twice the CPU ever holds.
