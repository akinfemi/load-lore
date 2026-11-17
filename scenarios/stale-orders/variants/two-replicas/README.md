# A second replica

**Spoiler:** this is a variant of [Stale orders](../../README.md): a fix you're likely to try. Run the starting design first.

The replica service runs two replicas instead of one, each the same size, and the api spreads its reads across them. When a database falls behind, adding more of it is the usual move.

It doesn't help: the pages after an order are stale as often, or more. In the reference check, up to 75% of them, against 33% with one replica. Each replica replays every write the primary makes; a second one takes half the reads, but none of the replaying. Both fall behind together. The check keeps this true: `loadl scenario check` fails if two replicas ever stop the replication-lag diagnosis.
