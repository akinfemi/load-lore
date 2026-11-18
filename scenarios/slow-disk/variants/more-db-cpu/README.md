# More CPU for the database

**Spoiler:** this is part of the answer to [The slow disk](../../README.md). Try the fix yourself first.

## What it is

The starting design with twice the database's CPU:

```yaml
resources: { cpu: 2, memory: 1g }
```

Nothing else changed. It's the fix that fits a database that's slow because it's busy.

## What it shows

It breaks just like the starting design: the pool runs out, and about a fifth of requests fail while the disk is slow.

The database's CPU was never the limit: in the starting design it was at most 22% busy while the disk was slow. Its commits wait on the disk, and a second core doesn't make an fsync return sooner. The report's diagnosis is the same, an exhausted pool, and the database's CPU column says why more of it can't help.

In the reference run (host speed index 991), 17–25% of requests failed in each 10 seconds of the slow disk, as in the starting design (21–28%), and the database's CPU, now twice as much of it, was at most 7% busy.
