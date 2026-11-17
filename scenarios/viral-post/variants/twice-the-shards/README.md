# Twice the shards

**Spoiler:** this is a variant of [The viral post](../../README.md): the fix someone on the team proposes. Run the starting design first.

The posts are split across four shards instead of two, a quarter of a million each, with the same resources per shard; nothing else changes. One shard ran out of CPU while the other idled, so doubling the shards looks like the fix.

It still breaks, on the same shard, about 8% later. In the reference check the starting design broke 2:40 into the load, at 2,675 page views a second, and four shards broke 3:10 in, at the 2,900 peak, ten seconds into the hold. The viral post still lives on db0, and its six views in ten still all go there: db0 took 71% of the reads with four shards, against 80% with two, and ran at 99% of its CPU while the others sat at about a fifth of theirs.

The arithmetic: with a share s of the reads on one key and n shards, its shard takes s + (1 − s) ÷ n. More shards shrink only the second part. At s = 0.6 that's 80% with two shards, 73% with three and 70% with four, and never below 60%, so doubling the shards can move the break by about 14% at most, and no number of shards by more than a third. A smaller share leaves more to gain: in test runs of an earlier sizing of this scenario (six reads a page, up to 3,400 a second), with the hot post at 45% of views a third shard still ran its hot shard out of CPU near the peak, and with it at 30% three shards held. The lesson isn't that a shard never helps; it's that a shard can't split a key, so the hot key's share sets a limit no number of shards gets past.

The check keeps this true: `loadl scenario check` fails if four shards stop breaking on a hot shard.
