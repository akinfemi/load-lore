# Loadlore

Loadlore is an open-source, terminal-first lab for system design. You describe a system in a YAML file: services, Postgres, Redis, a load balancer, queues, replicas, the load and the faults. `loadl` runs it on real Docker containers under that load, finds where it broke, and writes a report that explains why, citing the metric behind each fact.

It's for building the intuition that usually only comes from production incidents: what a connection pool, a hot row or a retry storm does to a real system, measured rather than drawn on a whiteboard.

```text
Broke 5:10 into the load, at 3,500 rps offered: p99 reached 940 ms (5.7 ms at low load).
Most saturated then: db CPU at 100% of its limit.

Diagnosis
  1. CPU bursts at the limit (db), from 1:10
  2. Queueing knee (db CPU), from 4:20
     median p99 by db CPU: below 50%: 5.9 ms; 70–80%: 88 ms; 90% and up: 1.32 s
     Little's law on api at 4:20: 3,098 rps × 7.1 ms = 22 in flight; measured 22
  3. Hot rows (api → db), from 5:10
     api's writes took 307 ms each, 136x their 2.3 ms at low load;
     24.7 backends waited on locks
     Try next: spread the writes over more rows...
```

<sub>Trimmed from a real report: [one query per page](scenarios/when-to-scale-your-database/solutions/one-query-per-page/report.md), a solution to the first scenario. Full reports add a diagram of the system at the break, charts, your predictions scored, a timeline, and every assumption and threshold.</sub>

## Install

```sh
brew install akinfemi/tap/loadlore                          # macOS
go install github.com/akinfemi/load-lore/cmd/loadl@latest   # anywhere with Go
```

Or download `loadl` for macOS or Linux (arm64 or amd64) from [Releases](https://github.com/akinfemi/load-lore/releases). Runs need Docker with Compose 2.23 or later: on Linux, the engine you have; on a Mac, [Colima](https://github.com/abiosoft/colima) (Docker Desktop and WSL2 aren't verified yet). [Set up](docs/how-to.md#1-set-up) has the details, and `loadl doctor` checks the machine and says how to fix what's missing.

## Quick start

```sh
loadl init when-to-scale-your-database   # copy the first scenario into ./when-to-scale-your-database
cd when-to-scale-your-database
loadl predict design.yaml                # guess where it breaks, before you run it
loadl run design.yaml                    # about ten minutes, with a live dashboard
loadl hint                               # when you want a nudge toward a fix
```

The run's report is in `.loadlore/runs/<run-id>/report.md`; `loadl report latest --print` shows it in the terminal.

## Scenarios

Guided experiments: a starting design that breaks, a story, questions to predict, hints and worked solutions. Start at the top.

| Scenario | Level | Teaches |
| --- | --- | --- |
| [When do you need to scale your database?](scenarios/when-to-scale-your-database/) | 1 | saturation, the queueing knee, connection pools |
| [The connection pool trap](scenarios/connection-pool-trap/) | 1 | pools per replica, `max_connections`, PgBouncer |
| [The slow dependency](scenarios/slow-dependency/) | 2 | timeouts, retry storms, circuit breakers |
| [The cache restart](scenarios/cache-restart/) | 2 | cold caches, the thundering herd, request coalescing |
| [The flaky network](scenarios/flaky-network/) | 2 | packet loss, TCP retransmits, timeouts sized to the tail |
| [Healthy but broken](scenarios/healthy-but-broken/) | 2 | shallow and deep health checks, partial failure |
| [The live vote](scenarios/live-vote/) | 2 | row locks, hot rows, contention that burns CPU |
| [Featured on Hacker News](scenarios/featured-on-hacker-news/) | 2 | autoscaling, scale-up lag, headroom |
| [Statement day](scenarios/statement-day/) | 2 | the buffer cache, working sets, a cloud disk's IOPS limit |
| [The backed-up queue](scenarios/backed-up-queue/) | 3 | job queues, consumer lag, Little's law for workers |
| [The flash sale](scenarios/flash-sale/) | 3 | hot rows under slow commits, taking writes off the request path |
| [The viral post](scenarios/viral-post/) | 3 | sharding by key, a hot key on one shard, caching popular reads |
| [The slow disk](scenarios/slow-disk/) | 3 | fsync and commit latency, group commit, `synchronous_commit` |
| [Stale orders](scenarios/stale-orders/) | 3 | read replicas, replication lag, read-your-writes |

`loadl scenarios` lists them in the terminal; they're built into `loadl`, so they work outside a checkout.

## Commands

```sh
loadl init                                     # questions, then a design file that validates; loadl init <scenario> copies one
loadl validate design.yaml                     # errors carry field paths; --json for tools
loadl diagram design.yaml                      # Mermaid; --ascii for the terminal
loadl schema --json                            # JSON Schema of the design file
loadl run design.yaml                          # run on local Docker; results in .loadlore/runs/<run-id>/
loadl status                                   # where the latest run is, or its verdict; pairs with run --detach
loadl watch                                    # the live dashboard for a run in progress
loadl report latest                            # rewrite a run's report.md; --print shows it as text, --json gives the analysis
loadl compare <run-a> <run-b>                  # two runs side by side, with the design diff
loadl scenarios                                # the scenario library
loadl predict design.yaml                      # answer the scenario's questions; they become expect lines
loadl hint                                     # the next hint for the scenario you're running
loadl scenario check scenarios/<name>          # run a scenario and its solutions; check its claims
loadl down                                     # remove anything a run left behind
loadl doctor                                   # check Docker, capacity, image, host speed and power, with fixes
loadl export                                   # write runs to a corpus for the simulator, one record each
loadl sweep                                    # stress one operation per component at several CPU sizes
```

Every command accepts `--json`.

## Learn more

- **[Using Loadlore on your own](docs/how-to.md)**: setup, working through a scenario, writing your own design, run options, predictions, reading the report, comparing runs, and getting numbers you can trust.
- **[Design file reference](skills/loadlore-designer/references/schema.md)**: every field, with defaults and the sources of the disk and network numbers.
- **[Designer skill](skills/loadlore-designer/)**: lets an AI agent turn a plain-language description into a validated design file. Its eval set runs with `go run ./cmd/skilleval` and makes paid model calls.
- **[Product spec](https://claude.ai/artifact/TEkM1GHG5mpmKXTcH3jJ6m)**: the source of truth for scope and behavior; its Build status section says what's built.

## Status

Early: the MVP is built and reviewed end to end, with fourteen scenarios and twelve diagnosis rules, each of which has fired in a real run. Runs are local Docker only for now. Results depend on your machine: slower cores break the same design at a lower load, so compare runs from the same machine.

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for local setup, the Docker behaviors the local driver depends on, adding a scenario, and releasing. Building needs Go (the version in `go.mod`; the toolchain downloads itself) and, for end-to-end tests, Docker with Compose v2.

```sh
make build        # bin/loadl
make test         # unit tests
make lint         # golangci-lint v2
make integration  # synthetic service against real Postgres and Redis (testcontainers)
make e2e          # end-to-end run on Docker (needs make image)
make image        # build loadlore/synthetic:dev, the image a source build's services run on
```

## License

[Apache-2.0](LICENSE)
