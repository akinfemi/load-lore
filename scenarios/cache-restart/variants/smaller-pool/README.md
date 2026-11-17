# A smaller database pool

**Spoiler:** this is a variant of [The cache restart](../../README.md): a fix you're likely to try. Run the starting design first.

Half the api's database connections:

```yaml
pools:
  cache: 20
  db: 5    # 3 replicas x 5 = 15 connections
```

Nothing else changes. The report said requests timed out waiting for a database connection, and the database's one CPU was the limit, so fewer connections looks backwards but reasonable: fewer copies of the query share the CPU, so each should finish inside the 1-second timeout and fill the cache.

It helps, slowly. In the starting design about a third of requests fail from the restart to the end of the load: the herd never ends. With 15 connections the failures fall instead, from about a quarter of requests to none, as the cache refills, but it takes minutes, not seconds. Across three runs of the reference check on 17 Nov, one recovered 1:10 after the restart, and two were still failing 6% and 13% of requests when the load ended two minutes after it.

Why so slowly: the herd hasn't gone anywhere, it's waiting in the pool instead of inside the database. Hundreds of requests queue for 15 connections, each waits most of its second for one, and a request that gets a connection with little time left is stopped mid-query and fills nothing. Only the requests that get a connection early enough fill the cache, so it refills a product at a time. Run it twice and the recovery time moves by a minute or more.

What ends a herd quickly is fewer copies of the work: one query per product, shared by everyone who missed it, which is `solutions/coalesce`; it recovered within a minute. A smaller pool helps alongside something that stops the waiting from eating the timeout, such as giving up on a pool wait early, so a request fails fast instead of starting a query it can't finish.

The check keeps the break true: `loadl scenario check` fails if the smaller pool stops breaking on the cold cache.
