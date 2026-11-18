# Loadlore

Loadlore is an open-source, terminal-first lab for system design. You describe a system in a YAML design file, run it on real Docker containers under realistic load and faults, and get a report that explains where and why it broke.

The CLI is `loadl`.

> **Status:** early development. The MVP is built and reviewed end to end, and waits only on the v0.1 release. You can validate a design, run it on local Docker (Linux, or macOS with Colima) with a live dashboard, read its report and work through fourteen scenarios: [When do you need to scale your database?](scenarios/when-to-scale-your-database/), [The connection pool trap](scenarios/connection-pool-trap/), [The slow dependency](scenarios/slow-dependency/), [The cache restart](scenarios/cache-restart/), [The flaky network](scenarios/flaky-network/), [Healthy but broken](scenarios/healthy-but-broken/), [The live vote](scenarios/live-vote/), [Statement day](scenarios/statement-day/), [The slow disk](scenarios/slow-disk/), [Stale orders](scenarios/stale-orders/), [Featured on Hacker News](scenarios/featured-on-hacker-news/), [The backed-up queue](scenarios/backed-up-queue/), [The flash sale](scenarios/flash-sale/) and [The viral post](scenarios/viral-post/). Twelve diagnosis rules are built: pool exhausted, queueing knee, connection storm, retry storm, cold cache, working set doesn't fit, hot rows, replication lag, autoscale lag, consumer lag, hot shard and CPU bursts at the limit, and each has fired in a real run. A stall of the Docker host's own disk is flagged as the machine, not the design. Designs can have streaming read replicas (`kind: postgres-replica`), with replication lag and stale reads measured, autoscaled services (`autoscale`) that add and drain replicas during the run, job queues (`kind: queue`, on Redis Streams) with workers (`role: worker`) that take their jobs and retry failed ones, and data sharded by key across Postgres services (`shards`), with fan-out reads. Postgres disks pick a profile (laptop, gp3, io2) with numbers from AWS's specs, and the network between containers one too (local, same-zone, the default, or cross-zone). Only connections a fault acts on go through Toxiproxy. `run --quick` starts the climb near the last run's break. Not built yet: restarting only the services a design change touched, and releases (build from source for now). Docker Desktop and WSL2 aren't verified. The spec's "Build status" section has the details.

- **Spec:** [Loadlore — Product Spec](https://claude.ai/artifact/TEkM1GHG5mpmKXTcH3jJ6m) is the source of truth for scope and behavior.
- **AI skill:** [`skills/loadlore-designer/`](skills/loadlore-designer/) turns plain-language descriptions into validated design files. Its eval set ([`evals/`](skills/loadlore-designer/evals/)) runs with `go run ./cmd/skilleval`, and in CI on changes to the skill once an `ANTHROPIC_API_KEY` secret is set; it makes paid model calls.
- **Using it on your own:** [`docs/how-to.md`](docs/how-to.md) walks through setup, a scenario, writing your own design, and reading the report, with `loadl` alone.

## Commands so far

```sh
loadl init                                     # questions, then a design file that validates; loadl init <scenario> copies one
loadl validate examples/single-postgres.yaml   # errors carry field paths; --json for tools
loadl diagram examples/single-postgres.yaml    # Mermaid; --ascii for the terminal
loadl schema --json                            # JSON Schema of the design file
loadl run examples/smoke.yaml                  # run on local Docker; results in .loadlore/runs/<run-id>/
loadl status                                   # where the latest run is, or its verdict; pairs with run --detach
loadl watch                                    # the live dashboard for a run in progress
loadl report latest                            # rewrite a run's report.md; --print shows it as text with small charts, --json gives the analysis
loadl compare <run-a> <run-b>                  # two runs side by side, with the design diff
loadl scenarios                                # the scenario library
loadl predict scenarios/<name>/design.yaml     # answer the scenario's questions; they become expect lines
loadl hint                                     # the next hint for the scenario you're running
loadl scenario check scenarios/<name>          # run a scenario and its solutions; check its claims
loadl down                                     # remove anything a run left behind
loadl doctor                                   # check Docker, capacity, image, host speed and power, with fixes
```

Every command accepts `--json`.

`loadl run` needs Docker with Compose 2.23 or later and the synthetic service image (`make image`). `loadl doctor` checks all of it, and says how to fix what's missing. It uses the current Docker context, or `DOCKER_HOST`. On Colima, give the VM enough CPUs for the design; the pre-flight check says when a design doesn't fit and which `--scale` does.

A run folder holds the exact `design.yaml`, the generated `compose.yaml` and `load.js`, `plan.json` (resources after scaling, and why), `events.jsonl` (load, faults, container deaths), `metrics/` (every series, exported from Prometheus before teardown), `logs/`, `containers.json`, `k6-summary.json`, `run.json`, and the `report.md` and `analysis.json` that `loadl run` writes at the end. Reports are made from the folder alone, so `loadl report` can regenerate them at any time.

Scenarios are guided experiments in [`scenarios/`](scenarios/): a starting design, a story, a question to predict, hints and worked solutions. Start with [When do you need to scale your database?](scenarios/when-to-scale-your-database/), then [The connection pool trap](scenarios/connection-pool-trap/). [The slow dependency](scenarios/slow-dependency/) (level 2) is about timeouts, retry storms and circuit breakers: a two-second pause that turns into an outage that never ends. [The cache restart](scenarios/cache-restart/) (level 2) is about cold caches, the thundering herd and request coalescing. [The flaky network](scenarios/flaky-network/) (level 2) loses 1% of a dependency's packets: TCP's resends, fan-out, and timeouts sized to the tail. [Healthy but broken](scenarios/healthy-but-broken/) (level 2) cuts one replica off from the database behind a shallow health check, and shows what a deep one fixes and risks. [The live vote](scenarios/live-vote/) (level 2) sends nine votes in ten to one row: row locks, hot rows, and contention that burns the CPU, so twice the CPU doesn't help. [The slow disk](scenarios/slow-disk/) (level 3) makes Postgres's disk 30 ms slower: commits waiting on fsync, connections held longer, group commit and `synchronous_commit`.

The report's diagnosis rules are deterministic and offline, and every report lists their thresholds. Hot rows needs lock-wait data from Postgres, which Loadlore adds to `postgres_exporter` with a custom query; a run where that metric is missing says so instead of skipping the rule silently.

In a terminal, `loadl run` shows a live dashboard: the topology colored by saturation, load, latency and errors over the profile with faults and the break marked, and each component's numbers. Ctrl-C on it stops the run, with teardown; `--watch=false` gives plain progress lines.

A run stops its load 60 s after it sees the design break, by the same rule as the report's verdict, so a broken design costs minutes less; `--full` runs the whole profile. `--quick` goes further for the next try: it starts the climb near where the last run broke, with a 30 s warm-up, and keeps the profile's slope from there.

Each run starts from a fresh copy of the seeded dataset. `--reuse-data` skips the copy for quick iteration; the report then says which earlier runs wrote to the data.

On macOS, `loadl run` keeps the Mac from idle-sleeping while it runs (`caffeinate`). It can't stop the sleep a closed lid causes, so keep the lid open for unattended runs. A run whose load took far longer on the wall clock than its profile, as when the host slept, is reported as inconclusive.

A run is also inconclusive when Loadlore's own tooling may have set the limit rather than the design: before the break, the load generator dropped arrivals (more than 1%, or more than 5% where the system was already failing requests), the load generator or Toxiproxy ran short of CPU, or Toxiproxy held up new connections on an edge. Toxiproxy carries only the connections a fault acts on, and opens each upstream connection in turn, so one slow handshake stalls the rest; the analysis finds this in its log. The report says which, and what it looks like taken at face value.

Results vary from run to run, and a laptop slows as it heats up. `loadl run --repeat 3` runs a design three times with the same seed and writes the spread of break time, peak served and p99 at the break to `.loadlore/series/<id>/series.md`. Runs after the first wait until the host speed index is back within 5% of the first run's. `loadl compare` says when a difference is within a series' spread, and flags runs whose speed index differs by more than 10%.

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for local setup and the Docker behaviors the local driver depends on.

Requires Go (the version in `go.mod`; the toolchain downloads itself) and, for end-to-end tests, Docker with Compose v2.

```sh
make build   # bin/loadl
make test    # unit tests
make lint    # golangci-lint v2
make integration  # synthetic service against real Postgres and Redis (testcontainers)
make e2e     # end-to-end run on Docker (needs make image)
make image   # build loadlore/synthetic:dev
```

## License

[Apache-2.0](LICENSE)
