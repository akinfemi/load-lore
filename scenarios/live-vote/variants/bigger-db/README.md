# Twice the database's CPU

**Spoiler:** this is a variant of [The live vote](../../README.md): a fix you're likely to try. Run the starting design first.

The database gets 2 CPUs instead of 1; nothing else changes. At the break the database's CPU was busy, at 62% and past 70% ten seconds later, so more of it looks like the fix.

It still breaks, on hot rows, at about the same load. In the reference check the starting design broke 1:00 into the load, at 688 requests a second offered, and twice the CPU broke at the same moment, at the same load, with the database's CPU at 33%. Where hot rows tip over can move from run to run, for both, so compare your own pair of runs, not these numbers. The row still takes its updates one at a time, and each vote holds it until its commit is flushed to disk, so the time a vote holds the row sets how many votes a second it takes, not the CPU. The waiting backends still burned CPU, but the second CPU went mostly unused. The check keeps this true: `loadl scenario check` fails if twice the CPU ever holds.
