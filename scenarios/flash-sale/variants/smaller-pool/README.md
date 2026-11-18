# Fewer connections

**Spoiler:** this is a variant of [The flash sale](../../README.md): a fix you're likely to try. Run the starting design first.

Each api replica keeps 4 connections to the database instead of 40; nothing else changes. Up to 80 purchases waited on the shoe's row lock at once, so letting only 8 at a time touch it looks like the fix, and it's what the queue's workers do too.

It still breaks, one window later. In the reference check the starting design broke 1:20 into the load, at 742 rps, and this one 1:30 into the load, at 958 rps. With 8 connections, only 6.4 backends waited on the row's lock instead of 68, and the database's CPU stayed at 12% instead of 61%, since there was less contention to burn it. But the row still took one purchase at a time, and checkout still waited for each one. The purchases that couldn't get a connection waited in the api's pool instead of on the lock: a read or a write took about 500 ms, pool wait included, and checkout's p99 passed 1 s.

Fewer connections move the waiting out of the database; they don't make the row take more purchases a second, and they don't stop checkout from waiting for it. The queue's workers limit the writers the same way, which is why the row works well for them. The difference is that checkout no longer waits for the writes.

The check keeps this true: `loadl scenario check` fails if the smaller pool stops breaking on hot rows.
