# Cache the posts

**Spoiler:** this is a worked answer to [The viral post](../../README.md). Try your own first.

## What changed

A post page is looked up in Redis first. Only on a miss does the api read the seven rows from the post's shard, and then it keeps the page in Redis for a minute:

```yaml
api:
  calls:
    - target: cache
      op: get
      on_miss:
        - { shards: [db0, db1], op: read, count: 7 }   # was every page view
        - { target: cache, op: set, ttl_s: 60 }

cache:
  kind: redis
  resources: { cpu: 0.2, memory: 256m }
```

## Why it helps

The viral post lives on one shard, db0, because its id hashes there. Six page views in ten are of it, and db0 also holds half of every other post, so it took 80% of the api's reads: 0.6 + 0.4 ÷ 2. In the starting design's reference check, db0 ran out of CPU 2:40 into the load, at 2,675 page views a second, while db1 sat at about a third of its CPU.

Adding shards can't move that post. With n shards its shard takes 0.6 + 0.4 ÷ n of the reads: 73% with three, 70% with four, and never less than 60%, however many there are. So from two shards, shards alone can move the break by at most a third (80% ÷ 60%), and doubling to four shards bought 8% in the reference check ([variants/twice-the-shards](../../variants/twice-the-shards/)).

A cache takes the viral post off the shards altogether. It's the same page for every viewer, so after its first miss every view of it is answered by Redis. In the reference check the cache answered 62–63% of lookups: the viral post's 60% and some of the others. What reached the shards was the long tail of other posts, spread evenly by the hash, and db0 and db1 each ran at about a third of their CPU at the 2,900-a-second peak. It held the whole profile. Its p99 stayed in single-digit milliseconds except for one 10-second stall at 4:10, at 521 ms, with db0 at half its CPU; the same short stall shows up now and then in other scenarios' runs, and the run recovered from it at once.

## What it costs

- **Staleness.** A cached page is up to a minute old. For a post's text that's fine; for its like count it shows: a viewer can like the post and not see the count move. Shorten the TTL, or update the cached page when the count changes (write-through), or keep fast-changing fields out of the cached page.
- **A cold start.** When Redis restarts or the page expires, every viewer of the viral post misses at once and goes to db0 together: a thundering herd on the one shard least able to take it. [The cache restart](../../../cache-restart/) is about this; `coalesce: true` on the `get` lets one miss per replica read the shard while the others wait for it.
- **Another service.** Redis needs its own memory, monitoring and failover. Here it used about 40% of 0.2 CPU at the peak.

Other ways out, not modelled here: a read replica for the hot shard alone, or the application spotting hot keys and keeping them in its own memory. Writes to a hot key, such as likes on the viral post, are a different problem with their own fix: see [The live vote](../../../live-vote/).
