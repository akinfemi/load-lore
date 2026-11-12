# Using Loadlore on your own

This guide covers everything with `loadl` alone, in a terminal: setting up, working through a scenario, writing and running your own designs, reading the report, and comparing changes. The [designer skill](../skills/loadlore-designer/) can do much of this for you from a chat with an AI agent, but nothing here needs it.

## 1. Set up

Loadlore runs your design on real containers, so it needs Docker. There are no published releases yet; build from source.

1. **Docker** with Compose 2.23 or later, on cgroup v2. On a Mac, use [Colima](https://github.com/abiosoft/colima). A dedicated profile keeps runs away from your other containers:

   ```sh
   colima start loadlore --cpu 8 --memory 10 --activate=false
   export DOCKER_HOST=unix://$HOME/.colima/loadlore/docker.sock
   ```

   Put the `export` in your shell profile, or run it in each terminal you use for Loadlore. On Linux, the Docker engine you already have works. Docker Desktop and WSL2 aren't verified yet.

2. **Go**, the version in `go.mod` (the toolchain downloads itself). Then, from a checkout of this repo:

   ```sh
   make build    # bin/loadl
   make image    # the loadlore/synthetic:dev image every design's services run on
   export PATH="$PWD/bin:$PATH"
   ```

3. **Check the machine:**

   ```sh
   loadl doctor
   ```

   It checks Docker, Compose, the CPUs and memory Docker can use against what each scenario needs, the synthetic image, the host speed index, and on a Mac the power settings. Each problem comes with its fix. A failed check means runs won't start; a warning means they will, but the numbers may differ from the scenario READMEs.

## 2. Work through a scenario

Scenarios are guided experiments: a starting design that breaks, a story, questions to answer before you run it, hints, and worked solutions. Seven are built:

| Scenario | Level | Teaches |
| --- | --- | --- |
| [When do you need to scale your database?](../scenarios/when-to-scale-your-database/) | 1 | saturation, the queueing knee, connection pools |
| [The connection pool trap](../scenarios/connection-pool-trap/) | 1 | pools per replica, `max_connections`, PgBouncer |
| [The slow dependency](../scenarios/slow-dependency/) | 2 | timeouts, retry storms, circuit breakers |
| [The cache restart](../scenarios/cache-restart/) | 2 | cold caches, the thundering herd, request coalescing |
| [The flaky network](../scenarios/flaky-network/) | 2 | packet loss, TCP retransmits, fan-out, timeouts sized to the tail |
| [Healthy but broken](../scenarios/healthy-but-broken/) | 2 | shallow and deep health checks, partial failure |
| [The slow disk](../scenarios/slow-disk/) | 3 | fsync and commit latency, connection pools, group commit, `synchronous_commit` |

`loadl scenarios` lists them; `loadl scenarios retry` searches names, titles and concepts.

The loop for each one:

1. **Read the scenario's README.** It tells the story, draws the system, and asks the questions.

2. **Predict.** Write down what you think will happen before you run it. That's most of what you learn from it.

   ```sh
   loadl predict scenarios/connection-pool-trap/design.yaml
   ```

   It asks the scenario's questions one by one and writes your answers into the `expect` block at the bottom of `design.yaml`. The report scores them after the run. You can also edit `expect` by hand (see [section 5](#5-predictions-the-expect-block)).

3. **Run it.**

   ```sh
   loadl run scenarios/connection-pool-trap/design.yaml
   ```

   In a terminal you get a live dashboard: the topology colored by how saturated each part is, load and latency and errors over time with faults and the break marked, and each component's numbers. Ctrl-C stops the run and cleans up. A run takes 5 to 10 minutes; the README says how long.

   Once the run sees the design break, it keeps the load on for 60 seconds so the report can diagnose the break, then stops. `--full` runs the whole profile anyway.

4. **Read the report.**

   ```sh
   loadl report latest --print | less
   ```

   `--print` shows it as text with small charts. The full report is `report.md` in the run's folder, with Mermaid diagrams and charts that render on GitHub or in any Markdown viewer. [Section 6](#6-read-the-report) walks through it.

5. **Stuck? Take a hint.**

   ```sh
   loadl hint
   ```

   Each call reveals the next hint for the scenario you last ran. `--show 2` shows hint 2 again; `--reset` hides them all.

6. **Change one thing and run again.** Copy the design, change what you think will fix it, write a new prediction, run it, and compare the two runs:

   ```sh
   cp scenarios/connection-pool-trap/design.yaml my-fix.yaml
   # edit my-fix.yaml
   loadl run my-fix.yaml
   loadl compare <first-run-id> latest -o compare.md
   ```

7. **Check the worked answers.** Each scenario's `solutions/` folder has fixes with their reasoning, costs and reference reports; `variants/` has fixes that look right and aren't. They give the game away, so try your own first.

## 3. Write your own design

The quickest start is `loadl init`: it asks for a name, an api's replicas and CPU, its datastore (Postgres, Redis in front of Postgres, or none), the peak load, how long to ramp to it, and an optional fault, then writes a commented `design.yaml` that validates. The answers can be flags instead (`loadl init --name checkout --replicas 3 --store cache --rps 800`), and `loadl init <scenario>` copies a library scenario into a folder of its own, to work through outside a checkout.

A design is one YAML file: the services, the load, the faults, and your predictions. The [design file reference](../skills/loadlore-designer/references/schema.md) has every field. A small one:

```yaml
version: 1
name: my-api

services:
  lb:
    kind: lb               # HAProxy, round-robin across the api's replicas
    target: api

  api:
    image: loadlore/synthetic
    replicas: 2
    resources: { cpu: 0.5, memory: 256m }
    behavior:
      cpu_ms: 1            # CPU time per request
      calls:               # what each request does, in order
        - { target: cache, op: get, when: read, on_miss: [ { target: db, op: read }, { target: cache, op: set, ttl_s: 60 } ] }
        - { target: db, op: write, when: write }
      resilience: { timeout_ms: 1000 }
    pools: { db: 10, cache: 10 }   # connections per replica

  cache:
    kind: redis
    resources: { cpu: 0.5, memory: 128m }

  db:
    kind: postgres
    resources: { cpu: 1, memory: 512m }
    dataset: { rows: 20000, row_bytes: 256 }   # seeded before the run

workload:
  entry: lb
  profile:
    - ramp: { from: 50, to: 300, over: 30s }
    - hold: { rps: 300, for: 30s }
  mix: { read: 0.9, write: 0.1 }
  keys: { distribution: zipf, skew: 1.1 }

scenario:
  - { at: 20s, inject: latency, target: db, add_ms: 50, for: 10s }
  - { at: 45s, inject: kill, target: "api[1]" }

expect:
  - p99_ms < 200 until load_rps >= 250
  - cache.hit_rate > 50% after 30s
```

That's [`examples/smoke.yaml`](../examples/smoke.yaml), a one-minute run. [`examples/`](../examples/) has a single-Postgres design and one with PgBouncer to start from.

**The parts:**

- **Services.** Your application services run `loadlore/synthetic`, a service that does what `behavior` says: burns CPU, allocates memory, and makes calls to other services in order (`read`/`write` to Postgres or PgBouncer, `get`/`set`/`del` to Redis, `http` to another service). Datastores and infrastructure are a `kind`: `postgres`, `redis`, `pgbouncer`, `lb`. Every service gets CPU and memory limits from `resources`; they're what make it break at a realistic load.
- **Pools and resilience.** `pools` sets each replica's connections to each target, and how long a request keeps one (`hold: request` holds it for the whole request, as Rails and Django do). `resilience` sets timeouts, retries with backoff, a circuit breaker and a bulkhead. Copy them from your real code: their defaults are a common cause of real outages.
- **Workload.** The offered load over time, as `ramp` and `hold` steps in requests per second, the read/write mix and how keys are chosen. Arrivals are open: they keep coming at the profile's rate whether or not the system keeps up, as real users do.
- **Scenario.** Faults at set times: `latency`, `bandwidth`, `timeout` and `reset` on the connections into a service; `kill`, `pause` and `restart` of its containers (`api[1]` is one replica). `loss` drops a share of the target's reply packets (TCP retransmits them, about 200 ms each). `partition` cuts two services, or one replica and a service, apart (`between: ["api[1]", db]`): their calls hang until they time out. `disk_slow` makes every write and fsync of a Postgres's disk `add_ms` slower (the kernel's dm-delay).
- **Expect.** Your predictions; see [section 5](#5-predictions-the-expect-block).

**Check it before you run it:**

```sh
loadl validate my-api.yaml        # errors name the field, such as services.api.behavior.calls[1].target
loadl diagram my-api.yaml --ascii # the topology, drawn from the file
loadl schema --json > schema.json # JSON Schema, for editor completion
```

Point your editor's YAML language server at the schema and you get completion and inline errors while you type.

**Then run it** with `loadl run my-api.yaml`. Before starting anything, the run prints what each service and Loadlore's own containers get, and warns when the design doesn't fit the CPUs Docker has. `--scale 0.5` shrinks resources and load together, so a big design fits a small machine, but breaking points don't scale perfectly: compare runs at the same scale.

## 4. Run options

| Flag | What it does |
| --- | --- |
| `--full` | Run the whole profile, even after a break. Use it when you compare designs by their peaks. |
| `--seed N` | Fix the random keys and mix. Without it every run gets a new seed. Use the same seed for runs you compare; scenario checks use 1. |
| `--repeat N` | Run N times with the same seed and write the spread of break time, peak and p99 to `.loadlore/series/<id>/series.md`. `compare` then says when a difference is within the spread. |
| `--detach` | Start in the background and print the run id. `loadl status` shows its progress and, at the end, its verdict; `loadl watch` opens the dashboard on it. |
| `--watch=false` | Plain progress lines instead of the dashboard. |
| `--keep` | Leave the stack running after the load, to poke at it. `loadl down` removes it. |
| `--reuse-data` | Skip the fresh copy of the seeded data. Faster, but earlier runs' writes carry over; the report says which. |
| `--scale F` | Shrink resources and load together. |

Every command takes `--json`, for scripts.

## 5. Predictions: the `expect` block

One prediction per line. The report scores each one as held or broken, with the measured value.

```yaml
expect:
  - break_load_rps within 25% of 1000   # where it breaks: right if within 25% either way
  - errors_load_rps none                # no load where 1% or more of requests fail
  - p99_ms < 300 until load_rps >= 800  # fast until the offered load reaches 800
  - error_rate < 1% after 2m20s         # recovers after a fault
  - db.cpu < 90% throughout
  - note: "the database gives out first" # not scored, shown in the report
```

- End-to-end metrics: `p50_ms`, `p95_ms`, `p99_ms`, `error_rate`, `rps` (served), and `load_rps` (offered) in conditions.
- Per component: `<service>.cpu`, `.memory`, `.p99_ms`, `.error_rate`, `.inflight`; `.connections` for Postgres; `.hit_rate` for Redis.
- For "where will it break?", use `break_load_rps` and `errors_load_rps` with `within N% of X` or `none`. An `until load_rps >= X` line only breaks when your guess is too high; these break when it's too low too.

The [reference](../skills/loadlore-designer/references/schema.md#expect) has the full grammar.

## 6. Read the report

The report is made from the run folder alone, so `loadl report <run>` can rewrite it any time. Its parts, top to bottom:

- **Verdict.** One of:
  - **Broke**, at a time and offered load, with the signals that crossed the line and what was most saturated then, and whether it recovered afterwards (healthy from some point to the end of the load, at least three windows). A break is a shortfall (under 90% of the offered rate served) or errors (1% or more of requests) for two 10-second windows in a row, or p99 at least 10× its low-load value and over the design's latency target (`workload.slo.p99_ms`, default 500 ms) for three. A single bad window is a stall: it shows in the timeline, but the run still held.
  - **Held** the whole profile.
  - **Inconclusive**: the run can't be judged, and the report says why. Usually Loadlore's own tooling was short of CPU (the load generator or Toxiproxy), the load generator couldn't start requests fast enough before anything strained, or the host slept or stalled during the load. It still shows what the run looks like taken at face value. Quiet the machine and run again.
- **Diagram** of the system at the moment that matters (the break, or the busiest window), colored by saturation.
- **Charts**: offered and served load, latency percentiles, errors, and each component's CPU and pools over time.
- **Diagnosis**: what went wrong, by deterministic rules that run offline. Each comes with its evidence, the metric each fact came from, and what to try next. The rules:

  | Rule | What it means |
  | --- | --- |
  | Pool exhausted | A caller's connection pool was full and requests waited for a connection, while the database had CPU to spare: they queued for connections, not for the database. |
  | Queueing knee | A component neared saturation and latency grew much faster than load. |
  | Connection storm | Pools to Postgres opened many connections at once as latency jumped; each new connection costs the database a process and a login just when it's short of CPU. |
  | Retry storm | Retries multiplied the calls to a failing or slow dependency, keeping it overloaded. |
  | Cold cache | A cache's misses multiplied, as after a restart, and they all went to the database at once. |
  | Working set doesn't fit | Postgres read from outside its memory while latency climbed. |
  | Hot rows | Writes queued on row locks while reads stayed fast. |

  Two diagnoses can both be true, one causing the other; the report says when it sees that.
- **Why requests failed**, under the diagnosis: failed requests counted by cause (timeouts, refused connections, `max_connections`, an open circuit breaker…).
- **Predictions**: each `expect` line, held or broken, with the measured value.
- **Timeline**: faults, container deaths, stalls and the break, in order.
- **Assumptions**: what came from your design, what Loadlore chose for you, and how it sized the run.
- **Thresholds**: every number the verdict and the rules use.
- **Environment**, **per-window data** and the exact **design file**.

`loadl report <run> --json` prints the whole analysis as JSON.

## 7. Compare runs

```sh
loadl compare <run-a> <run-b> -o compare.md
```

Run A is the baseline. The comparison shows both verdicts, load and latency over time side by side, the diagnoses and predictions of each, and the diff between the two designs. It warns when the two runs' host speed differs by more than 10%, and when a difference is within a `--repeat` series' spread.

`<run>` is a run id, a unique prefix of one, a run folder, or `latest`.

## 8. Get numbers you can trust

Loadlore measures real containers on your machine, so the machine is part of the experiment.

- **Keep the machine quiet during a run.** A browser, a build or a test suite takes CPU from the containers, and inside a VM they can't tell. A run that looks wrong this way is often reported as inconclusive; close things and run again.
- **On a laptop, plug in and turn off Low Power Mode.** On battery with Low Power Mode on, the reference Mac's speed halved and the first scenario broke at half the load. `loadl doctor` checks it.
- **Keep the lid open.** `loadl run` stops the Mac from going to sleep when idle, but a closed lid still sleeps it, and the run is lost; the report then calls it inconclusive.
- **Compare runs from the same machine, at the same scale and seed.** CPU limits are fractions of your cores, so a faster machine breaks at a higher load. Before a run, Loadlore measures a host speed index (the reference Mac is 1000) and warns when yours is much lower than a scenario's reference.
- **Expect some spread.** Two runs of the same design differ. For a decision that matters, use `--repeat 3`.

## 9. Files and cleanup

Each run gets a folder, `.loadlore/runs/<run-id>/`, in the directory you ran it from:

| File | What it holds |
| --- | --- |
| `report.md`, `analysis.json` | the report and the analysis it's made from |
| `design.yaml` | the exact design that ran |
| `plan.json` | resources after scaling, and why each was chosen |
| `compose.yaml`, `load.js` | the generated Docker Compose file and k6 script; you can read or rerun them |
| `events.jsonl` | the load starting and ending, faults, container deaths |
| `metrics/` | every Prometheus series, exported before teardown |
| `logs/`, `containers.json` | each container's logs and final state |
| `run.json`, `k6-summary.json`, `status.json`, `progress.jsonl` | run metadata, the load generator's summary, live progress |

Teardown removes the run's containers on success, failure and Ctrl-C. If something is left over, as after a crash:

```sh
loadl down        # remove every container, network and volume Loadlore made
loadl down --all  # and the cached seeded datasets
```

## 10. When something goes wrong

| Symptom | What to do |
| --- | --- |
| A run won't start | `loadl doctor` says what's missing and how to fix it. |
| "needs N CPUs available to Docker" | Give Docker more CPUs (on Colima, `colima stop loadlore` then `colima start loadlore --cpu N`), or run with the `--scale` the warning suggests. |
| The verdict is inconclusive | Read why in the verdict. Quiet the machine, plug in, keep the lid open, and run again. |
| A result far from the scenario README's | Compare your host speed index with the README's reference machine; check power and other load on the machine. |
| `loadl doctor` fails on the image | `make image` in the repo builds `loadlore/synthetic:dev`. |
| Containers left behind | `loadl down`. |
| Want to look inside a running stack | `loadl run --keep`, then use `docker ps` and `docker logs` with the same `DOCKER_HOST`, and `loadl down` when done. |
