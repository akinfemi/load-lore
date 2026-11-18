# Give the replica what the primary has

**Spoiler:** this is a worked answer to [Stale orders](../../README.md). Try your own first.

## What changed

The replica gets the primary's class of instance: its size, 1 CPU instead of 0.75 and 2 GB of memory instead of 512 MB, with `shared_buffers` to match, and a volume with provisioned IOPS instead of the baseline:

```yaml
resources: { cpu: 1, memory: 2g }   # was 0.75 CPU and 512m
config: { max_connections: 100, shared_buffers: 512MB }   # was 128MB
disk: { iops: 10000 }   # was {}: 3,000 IOPS
```

## Why it helps

The primary applies orders from 80 connections at once. The replica replays every one of those changes in a single process, one after another, and with 512 MB most of them touched a row whose page it didn't hold in memory: the replay waited for the disk, and the replica's own reads queued for the same disk. Lag grew with the order rate, and so did the share of stale pages.

Replaying also writes: the replica writes the WAL it receives to its own disk, in small pieces and synced as they arrive, and later the pages it changed. At the top of the ramp those writes alone approach 3,000 a second, the baseline volume's limit. A probe with the memory but the baseline disk kept up to about 1,650 requests a second and then fell 2.4 s behind in one window.

With the primary's memory, the replica holds the whole table: replaying a change needs no disk read, and its reads stop queuing. With provisioned IOPS, its writes have room. In the reference run it stayed at most 108 ms behind the primary through the ramp, and under 0.1% of the pages 200 ms after an order missed it until the last window, where 2.0% did.

## What it costs

- **Money.** The replica now costs as much as the primary. Replicas sized down to save money are where this lag usually comes from.
- **It has a ceiling.** One process still replays every write, and the replica writes everything the primary does. Write volume grows; a replica that keeps up today may not after the next sale.
- **Lag never goes to zero.** Asynchronous replication is always a little behind. A page that reads immediately after a write can still miss it; this one waits 200 ms.

## The reference run

`report.md` is the reference run on a MacBook Air M4 on mains power.
