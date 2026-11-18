# A second replica

**Spoiler:** this is a variant of [Stale orders](../../README.md): a fix you're likely to try. Run the starting design first.

The replica service runs two replicas instead of one, each the same size, and the api spreads its reads across them. When a database falls behind, adding more of it is the usual move.

It doesn't fix it: the pages after an order are still stale. In the reference check, up to 35% of them, against 64% with one replica, but the share varies between checks, and an earlier check the same day gave 80% against 67%. Each replica replays every write the primary makes; a second one takes half the reads, but none of the replaying. Both still fall behind, up to 1.1 s, less far than the one replica's 4.0 s but well past the 200 ms a confirmation page waits. The check keeps this true: `loadl scenario check` fails if two replicas ever stop the replication-lag diagnosis.
