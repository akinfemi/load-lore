# Buy a faster disk

**Spoiler:** this is a worked answer to [Statement day](../../README.md). Try your own first.

## What changed

The disk serves 10,000 reads a second instead of 3,000. Cloud volumes sell IOPS separately from size: a gp3 volume can be provisioned up to 16,000.

```yaml
disk: { iops: 10000 }   # was {}: 3,000 IOPS, gp3's baseline
```

Nothing else changes: the database still has 512 MB, and each read from the disk still waits about a millisecond.

## Why it helps

The working set still doesn't fit: about 70% of block reads come from `shared_buffers`, as before, and at the top of the ramp the database read about 4,800 blocks a second from outside them. What changed is the disk's limit. In the starting design about 1.4 page reads per request went outside `shared_buffers`, so 3,000 reads a second ran out at about 2,200 requests a second. With 10,000, the same reads per request take the load to about 7,000 requests a second before the disk is full, past the top of the ramp. No diagnosis fires: nothing queued.

Each request still waits for its disk reads, about a millisecond each, so the page is slower than if its rows were in memory. It just doesn't queue.

## What it costs

- **Money, every month, for the peak.** Provisioned IOPS are billed whether they're used or not, so a disk sized for the first of the month pays for it on every other day too.
- **It moves the limit, it doesn't remove it.** The break moves with the IOPS: twice the customers clicking needs about twice the reads. Memory makes the reads go away; IOPS make them cheaper to wait for.
- **Latency.** Every read that misses memory still waits for the disk. A page that reads two rows waits twice.

## The reference run

`report.md` is the reference run on a MacBook Air M4 on mains power.
