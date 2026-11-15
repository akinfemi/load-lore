# Hints

Reveal them one at a time with `loadl hint featured-on-hacker-news`. Each one assumes you've run the design and have its report open.

## Read the timeline

The report's timeline lists each autoscaler decision, with the CPU it saw, and when each new replica took traffic. How many replicas did each decision ask for, and how long after it did the replica arrive?

## What one decision can ask for

The autoscaler wants `ceil(replicas × CPU ÷ target)`. Two replicas at 100% CPU, with a target of 70%: how many is that? Now remember a replica's CPU can't go above 100% of its limit, however many requests are queued for it.

So how many replicas could the first decision ask for, while the surge needed six? And when could the next decision ask for more?

## Where the time goes

Add up what stands between the surge and a new replica taking traffic: the wait for the next decision, the averaging window, the 30 seconds to be scheduled and pull the image, the app starting, the load balancer's health checks. Then multiply by the number of steps the CPU ceiling forces.

## A metric that keeps growing

Requests in flight, or p99, keep growing while a backlog builds; CPU stops at 100% of the limit. What would an autoscaler on in-flight requests ask for when every replica has a hundred requests queued?

## Ways out

1. Scale on a metric that grows with the backlog: requests in flight, or latency.
2. Keep headroom: run enough replicas before a surge you can expect, or scale ahead of it on a schedule.
3. Shed or queue what can't be served yet, so the replicas you have stay healthy while more arrive. This version of Loadlore can't model that.

The first two are worked in `solutions/`. Which one still works when the surge comes twice as fast?
