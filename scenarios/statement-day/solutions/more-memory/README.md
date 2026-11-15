# Give the database enough memory

**Spoiler:** this is a worked answer to [Statement day](../../README.md). Try your own first.

## What changed

The database gets 4 GB of memory instead of 512 MB, and `shared_buffers` 1 GB instead of 128 MB:

```yaml
resources: { cpu: 1, memory: 4g }   # was 512m
config: { max_connections: 100, shared_buffers: 1GB }   # was 128MB
```

## Why it helps

Postgres keeps the pages it has read in memory: its own `shared_buffers`, then the operating system's page cache, which counts against the container's memory limit too. The table and its index are about 2.5 GB. With 512 MB, most of a random customer's pages had been pushed out by the time anyone asked for them again, so about 1.4 page reads per request went outside `shared_buffers`, most of them to the disk. At 3,000 reads a second the disk was full at about 2,200 requests a second, and every request after that queued for it. The database's CPU was at a quarter of its limit.

With 4 GB the whole table fits. Each customer's pages are read from the disk once, the first time anyone asks, and from memory after that, so the reads that reach the disk fall as the run goes on. The memory starts empty: Loadlore empties the cache after copying the table in, the way a restart would. So the hit rate climbs through the run instead of starting high: in the reference run, `shared_buffers` answered 81% of block reads in the first 30 seconds and 86% by the end, and the rest came mostly from the page cache. The disk saw fewer reads at 3,200 requests a second than the starting design did at 2,200.

A rule of thumb gives `shared_buffers` about a quarter of the memory and leaves the rest to the page cache. Both count: a page in either is a read that doesn't wait for the disk.

## What it costs

- **Money.** Memory is most of a database instance's price. Four times the memory is roughly the next two instance sizes up.
- **It stops working as the table grows.** Twice the customers is twice the table, and the memory has to grow with it. Statement data grows every month.
- **A cold start still reads at disk speed.** After a restart or a failover the cache is empty, and the first wave of traffic reads from the disk until it fills.

## The reference run

`report.md` is the reference run on a MacBook Air M4 on mains power. It has one slow stretch, p99 402 ms at 2:00, while Toxiproxy (the proxy Loadlore puts between services) was stalled at its CPU limit; the report notes it as a caveat. The database's reads stayed under a millisecond each, so the working-set diagnosis doesn't fire.
