# Sixteen times the queue's memory

**Spoiler:** this is a variant of [The backed-up queue](../../README.md): a fix you're likely to try. Run the starting design first.

The queue gets 1 GB of memory instead of 64 MB; nothing else changes. Checkout failed when the queue was full, so a queue that can't fill looks like the fix.

Checkout holds the whole sale, and the report's verdict says so. But the workers still send only about 66 emails a second against 180 arriving, so the backlog grows for as long as the sale lasts. In the reference check, 19,100 emails were waiting when the load ended and the oldest had waited 107 s; at the workers' rate, the last of them goes out about five minutes after the sale ends. A buffer absorbs a burst only when the workers can catch up afterwards. Here it hides the lag instead of fixing it, and the queue fills anyway if the sale lasts long enough.

The check keeps this true: `loadl scenario check` fails if the bigger queue stops the consumer-lag diagnosis.
