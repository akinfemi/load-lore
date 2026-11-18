# Decide every 5 seconds

**Spoiler:** this is a variant of [Featured on Hacker News](../../README.md): a fix you're likely to try. Run the starting design first.

The autoscaler decides every 5 seconds instead of every 15; nothing else changes. If it reacted too slowly, reacting more often looks like the fix.

It still lags. Each decision over saturated replicas can still ask for only 1.43 times the replicas that are serving, and the next step still waits for those to start, about 30 seconds. Deciding more often adds decisions that have nothing new to ask for. In the reference check it asked for its sixth replica at 2:35 instead of 3:00, and still broke. The check keeps this true: `loadl scenario check` fails if deciding every 5 seconds stops the autoscale-lag diagnosis.
