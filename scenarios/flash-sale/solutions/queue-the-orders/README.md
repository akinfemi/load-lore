# Queue the purchases

**Spoiler:** this is a worked answer to [The flash sale](../../README.md). Try your own first.

## What changed

Checkout no longer writes the stock. It puts each purchase on a queue and answers at once, and a worker takes the purchases off and updates the stock, 5 at a time:

```yaml
api:
  calls:
    - { target: db, op: read, when: read }
    - { target: orders, op: publish, when: write }   # was a write to db

orders:
  kind: queue
  resources: { cpu: 0.5, memory: 256m }

fulfil:
  role: worker
  consume: { from: orders }   # 5 jobs at a time, Sidekiq's default
  calls:
    - { target: db, op: write }
```

## Why it helps

A purchase's write took 2.4 ms at low load, and the shoe's row takes writes one at a time, so it takes about 420 a second: 1 ÷ 2.4 ms. In the starting design's reference check it broke at 742 rps, when about 420 purchases of the shoe arrived a second (742 × 3/5 × 19/20). The rest waited on the row's lock, up to 68 backends at once, holding their connections and their requests until checkout's p99 passed 6 s.

With the queue, checkout's only work on a purchase is to add a job. In the reference check it held the whole drop, up to 1,500 rps, with a worst p99 of 23 ms. The worker took at most 498 purchases a second off the queue, against 903 published at the top, so the queue grew to 34,276 purchases by the time the shoe sold out. Then traffic fell to 200 rps, and the backlog went down by about 380 a second: the worker's 500, less the 120 new purchases. The queue was empty about 90 s after the drop ended. The oldest purchase waited 69.1 s, inside the business's two minutes.

## What it doesn't do

It doesn't make the row faster. The worker took at most 498 purchases a second, and each write waited 9.3 ms against 2.3 ms at low load, with 3.3 backends waiting on locks: its 5 runners queue for the row too, only fewer of them. What the queue changes is who waits: purchases wait in the queue, where waiting costs a few kilobytes of memory, instead of in checkout, where it costs a connection, a request and a buyer staring at a spinner.

That's why more job runners don't help either. In test runs of a bigger drop (2,000 rps at the top), 8 runners and 50 runners both took about 495 purchases a second; with 50, each write waited 101 ms in line for the row instead of 16 ms, with 45 backends waiting on locks instead of 7. The consumer-lag diagnosis says so and advises against more runners.

## What it costs

- **Buyers wait to hear.** Checkout answers "order received" at once; whether the buyer got a pair is known only when the worker has taken it from the stock, up to the oldest job's wait later. The app has to say "pending" until then and tell the buyer after, by email or a status page.
- **It can oversell.** While purchases wait in the queue, the shop has promised them, and the stock row doesn't know yet. With a limited stock, the queue can hold more purchases than there are shoes, and the last buyers get a "sorry, sold out" after checkout said yes. Loadlore's stock never runs out, so the run doesn't show this. The usual answers are to hold the stock in a fast counter that checkout decrements before it queues the purchase (a Redis `DECR` that refuses at zero), and keep the queue for the slow, durable part; or to stop taking purchases once the queue holds as many as there's stock.
- **A backlog has to drain.** The queue absorbs a burst only when the row's pace catches up afterwards. A drop that stayed at its peak would grow the queue until it was full, and checkout would fail again, as in [The backed-up queue](../../../backed-up-queue/README.md).

Batching is what would make the row faster: one update that takes 50 from the stock costs about one commit, not 50. This version of Loadlore runs one write per job, so it can't model that yet.

## The reference run

`report.md` is the reference run on a MacBook Air M4 on mains power. In the starting design's run, checkout broke 1:20 into the load, at 742 requests a second offered, on hot rows; with the queue it held, and the oldest purchase waited 69.1 s.
