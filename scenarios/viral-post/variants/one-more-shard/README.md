# One more shard

**Spoiler:** this is a variant of [The viral post](../../README.md): the fix someone on the team proposes. Run the starting design first.

The posts are split across three shards instead of two, a third of a million each, with the same resources per shard; nothing else changes. One shard ran out of CPU while the other idled, so spreading the posts thinner looks like the fix.

It still breaks, on the same shard, about 12% later. In the reference check the starting design broke 2:30 into the load, at 2,525 page views a second, and three shards broke 2:50 in, at 2,825. The viral post still lives on db0, and its six views in ten still all go there: db0 took 74% of the reads with three shards, against 80% with two, and ran at 100% of its CPU while db1 and db2 sat at about a quarter of theirs.

The arithmetic: with a share s of the reads on one key and n shards, its shard takes s + (1 − s) ÷ n. More shards shrink only the second part. At s = 0.6 that's 80% with two shards, 73% with three and 70% with four, and never below 60%, so from two shards no number of shards moves the break by more than a third. A smaller share leaves more to gain: in test runs of an earlier sizing of this scenario (six reads a page, up to 3,400 a second), with the hot post at 45% of views a third shard still ran its hot shard out of CPU near the peak, and with it at 30% three shards held. The lesson isn't that a shard never helps; it's that a shard can't split a key, so the hot key's share sets a limit no number of shards gets past.

The check keeps this true: `loadl scenario check` fails if the third shard stops breaking on a hot shard.
