# Hints

Reveal them one at a time with `loadl hint slow-disk`. Each one assumes you've run the design and have its report open.

## Read the diagnosis

The report names an exhausted connection pool. Look at the database's CPU at the same time. Is the database busy? What are the connections doing?

## What a commit waits for

By default, Postgres returns from a commit only once its changes are in the write-ahead log on disk: it writes the log and calls fsync, and waits for both. With 30 ms added to every write and every fsync, how long does a commit take? How long does the request that made it hold its connection?

## Reads that never touch the disk

The reads need a connection too, from the same pool. If the pool's 16 connections are all held by commits waiting on the disk, what happens to a read that arrives?

## Little's law again

At 200 writes a second, each holding a connection for the length of a slow commit, how many connections are busy at once? Compare that with 16.

## Ways out

1. Stop commits from waiting on the disk: with `synchronous_commit` off, a commit returns as soon as its changes are in memory, and the WAL writer flushes them a moment later. What do you lose if the server crashes?
2. Let more commits wait at once. Commits that wait at the same time share one flush of the log, group commit, so the disk's delay is paid once for all of them. What does each connection cost the database?

Twice the CPU is in `variants/more-db-cpu`; work out what it would change before you look. Both fixes are worked in `solutions/`.
