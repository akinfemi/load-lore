# Mapping real services onto loadlore/synthetic

`loadlore/synthetic` reproduces what a service does to the rest of the system: CPU time, memory, outbound calls, connection pools, timeouts and retries. It does not run business logic. A good model gets the calls and their counts right first, then CPU, then everything else.

## What to capture, in order of impact

1. **Outbound calls per request.** Number of database queries, cache operations and service calls, and whether they depend on reads vs writes. This is usually what decides where the system breaks.
2. **Pools and resilience settings.** Connection pool sizes, timeouts, retry counts and backoff. Copy them from the real code or config when available; defaults in real libraries are often what causes the failure.
3. **Concurrency model.** Threads, event loop or worker pool (see `concurrency` in `schema.md`). A Node.js service with heavy CPU work saturates one core per replica long before a Go service does.
4. **CPU time per request.** Matters most for compute-heavy endpoints.
5. **Memory per request.** Only matters for large payloads or high concurrency.

## Reading calls from code

| In the code | In `calls` |
| --- | --- |
| One ORM `find`/`get` by id | `{ target: db, op: read }` |
| A query inside a loop over N items | `{ target: db, op: read, count: N }`, with N from typical data, recorded as an assumption. Call out N+1 patterns to the user plainly; don't fix them. |
| `SELECT ... WHERE created_at > ...` with no index | `{ target: db, op: read, query: scan }` |
| Pagination / list endpoints | `{ target: db, op: read, query: range, rows: 50 }` |
| Insert or update inside a transaction | `{ target: db, op: write }` per statement |
| Cache lookup, then DB on miss, then cache fill | `{ target: cache, op: get, on_miss: [ {target: db, op: read}, {target: cache, op: set, ttl_s: ...} ] }` |
| Write to DB, then delete cache key | `{ target: db, op: write, when: write }`, `{ target: cache, op: del, when: write }` |
| HTTP or gRPC call to another internal service | `{ target: <service>, op: http }` plus a synthetic service for the target |
| Third-party API (payments, email) | A synthetic service with `cpu_ms: 0` standing in for it; latency faults model its behavior. Record as an assumption. |
| Publishing to a queue | `{ target: <queue>, op: publish, mode: async }` |
| Background consumer | A separate service with `role: worker` and `consume` |
| Auth middleware calling an auth service on every request | A call at the start of `calls`, `when: always` |

When a request's behavior depends on data (feature flags, branches), model the common path and mention the others to the user.

## Rough CPU starting points

These are order-of-magnitude guesses for one request on one modern core. Always record them with `basis: guessed` and say so to the user. Replace them with `loadl profile` measurements when the real service can be run.

| Handler does | `cpu_ms` starting point |
| --- | --- |
| Proxy or gateway, little logic | 0.2–1 |
| Typical JSON CRUD endpoint | 1–5 |
| Large JSON responses (hundreds of KB) | 5–20 |
| Server-side HTML templating | 5–30 |
| Password hashing (bcrypt, default cost) | 50–100 |
| Image resizing | 50–500 |
| PDF or report generation | 100–2000 |

Pick the low end unless the code suggests otherwise, and tell the user that this number is a guess they can correct.

## Resilience settings

- Copy real values when you find them: HTTP client timeouts, retry middleware, circuit breakers, pool sizes, database statement timeouts.
- If none are set in code, use the library defaults the real service would get (for example, many HTTP clients have no timeout at all; record `timeout_ms: 0` meaning none) rather than Loadlore's defaults, and record the basis. Missing timeouts are a common real-world failure, and the user should see it happen.
- Never add retries, breakers or bulkheads the user didn't ask for (ground rule 1).

## When the user has their own image

If the user wants to run their real service instead of the synthetic one, set `image` to their image and leave out `behavior`. They must expose Prometheus metrics for full reports; tell them which metrics are missing after the first run.
