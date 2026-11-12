# Hints

Reveal them one at a time with `loadl hint healthy-but-broken`. Each one assumes you've run the design and have its report open.

## Read the verdict

What share of requests failed during the partition? With three replicas behind a round-robin load balancer, what share does each one get?

## Ask what the check checks

Look at the load balancer's healthy replicas in the per-window table. Did it ever take the third one out? What does `/healthz` have to do to answer, and what does a request have to do?

## Shallow and deep

A shallow health check answers "is the process up?". A deep one answers "can it do its job?", by checking what the job needs: here, the database. The synthetic service has both: `/healthz` is shallow, and `/readyz` pings its dependencies before it answers.

## How fast

With a check every 2 seconds and 3 failures to take a replica out, how long does a broken replica keep its share of traffic, at best? What would a check every second and 2 failures give?

## The other side

A deep check makes every replica's health depend on the database. What happens if the database itself is slow for a minute, slower than the check's timeout, but still answering queries? Work it out before you look at `variants/deep-check-slow-db`.

## The way out

Point the load balancer at `/readyz`, checked often enough that a broken replica leaves quickly. Predict what the run will show in `expect`, run it, and compare. It's worked in `solutions/`. Then think about the check's timeout: what should it be, given how slow the database can get and still be useful?
