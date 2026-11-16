# Hints

Reveal them one at a time with `loadl hint flash-sale`. Each one assumes you've run the design and have its report open.

## Read the diagnosis

The report names hot rows. The database's CPU was busy too, but it wasn't full. Look at the hot-rows evidence: how long did a purchase's write take at low load, and how long when it broke? How many backends were waiting on locks?

## One row, one write at a time

Postgres locks a row while a transaction updates it and keeps the lock until the commit, and the commit waits for the disk. A second purchase of the shoe waits for the first to commit.

Take a write's time at low load. If writes to the shoe's row go through one after another, how many a second can it take? How many purchases of the shoe arrive at the load where it broke: three in five requests are purchases, nineteen in twenty of those are for the shoe?

## Fewer at once?

Forty connections per replica means up to 80 purchases waiting on the row. Would 4 per replica make the row any faster? Where would the other purchases wait instead, and for how long, against the api's 2 s timeout? Predict it, then run `variants/smaller-pool`.

## Who has to wait?

A buyer needs to know their purchase was taken, but does checkout have to wait while the row takes it? What could checkout do with a purchase instead, and what would take the purchases from there and update the row?

## How far behind?

If the row takes only so many purchases a second, a queue in front of it grows during the drop's minute at the top. How many purchases a second pile up, for how long? After the shoe sells out, how fast does the backlog go down? Is the oldest purchase applied within the business's two minutes?

## Ways out

1. Put each purchase on a queue and answer at once; a worker updates the stock a few at a time.
2. Hold the stock somewhere that takes decrements without a row lock or a commit, such as a Redis counter, and write the orders to Postgres afterwards.
3. Apply many purchases in one write: one update takes N from the stock.

The first is worked in `solutions/`. This version of Loadlore can model the second only roughly and the third not at all, since a worker's job is one write.
