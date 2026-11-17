# Hints

## Where to look

Read the verdict, then the CPU chart. Which component was at its limit when it broke, and what were the components next to it doing?

## Two shards, one busy

The report names a hot shard. Look up how many of the api's calls went to each shard just before the break. The keys are split evenly between the shards, half each. Is the traffic?

## Where a key lives

A key lives on one shard: the shard its id hashes to. Six page views in ten are of one post, so six in ten of the reads go to its shard, wherever that is, plus that shard's half of everything else. Work out the busy shard's share of the reads with two shards, then with four. How much later do twice the shards let it break? What's the most any number of shards could buy?

## The share sets it

With a share s of the traffic on one key and n shards, the hot shard takes s + (1 − s) / n of the reads. More shards shrink only the second part. The hot key's share stays on one shard whatever n is, so the break can move by at most (s + (1 − s) / 2) / s from two shards: 1.33x here. At a smaller share more shards do buy more, up to the same limit.

## Directions

Take the hot key's reads off the database. The viral post is the same page for every viewer, and a page that popular is exactly what a cache answers best. Or give the reads of that one post more capacity of their own. `solutions/` has a worked answer.
