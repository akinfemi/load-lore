# Twice the database's CPU

**Spoiler:** this is a variant of [Statement day](../../README.md): a fix you're likely to try. Run the starting design first.

The database gets 2 CPUs instead of 1; nothing else changes. When a database slows down under load, more CPU is the usual first move.

It still breaks, on the working set, at the same load: 2,492 requests a second offered in the reference check, as for the starting design. The database's CPU was mostly idle, a quarter of one CPU at the break and 14% of two with this variant: its backends were waiting for the disk, which serves the same 3,000 reads a second whatever the CPU. The check keeps this true: `loadl scenario check` fails if twice the CPU ever holds.
