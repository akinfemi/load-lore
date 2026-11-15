# Hints

Reveal them one at a time with `loadl hint statement-day`. Each one assumes you've run the design and have its report open.

## Read the diagnosis

The report names a working set that doesn't fit, and a queueing knee on the database that wasn't its CPU. Find how busy the database's CPU was at the break. If it wasn't the CPU, what was every request waiting for?

## Where a read comes from

Postgres keeps the table's pages it has read recently in memory: its own `shared_buffers`, then the operating system's page cache, which counts against the container's memory too. A read whose page is there takes a fraction of a millisecond. A read whose page isn't goes to the disk.

The table and its index are about 2.5 GB, and the database has 512 MB. On a normal day most requests are for a few busy customers, whose pages stay in memory. Today, any customer is as likely as any other. What share of reads can memory answer? The report's buffer hit rate shows it.

## What the disk can do

The disk is a cloud volume: each read from it takes about a millisecond, and it serves at most 3,000 reads a second. The report's assumptions list both.

Each statement read touches a page or two of the index and one of the table. At the break, how many reads a second went to the disk? How many requests a second does that cap allow, at the hit rate you found?

## What more CPU buys

If the database's CPU was mostly idle, how much later would twice the CPU break? Predict it, then run `variants/bigger-db` and compare.

## Ways out

1. Give the database enough memory that the table and its index fit, and `shared_buffers` a fair share of it.
2. Give it a disk that serves more reads a second. Cloud volumes sell IOPS separately.
3. Keep the clicks off the database: render each statement once, before the email goes out, into a file served from object storage. Loadlore can't model object storage yet.

The first two are worked in `solutions/`. Work out what each costs, and which one still holds when the bank has twice as many customers.
