# A second replica

**Spoiler:** this is a variant of [Stale orders](../../README.md): a fix you're likely to try. Run the starting design first.

The replica service runs two replicas instead of one, each the same size, and the api spreads its reads across them. When a database falls behind, adding more of it is the usual move.

It doesn't fix it: the pages after an order are still stale, somewhat less often. In the reference check, up to 29% of them, against 40% with one replica; the share varies between checks, and another check the same night gave 44% against 62%. Each replica replays every write the primary makes; a second one takes half the reads, but none of the replaying. Both fall behind together, up to 1.7 s, as far as the one replica did. The check keeps this true: `loadl scenario check` fails if two replicas ever stop the replication-lag diagnosis.
