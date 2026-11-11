# Loadlore

Loadlore is an open-source, terminal-first lab for system design. You describe a system in a YAML design file, run it on real Docker containers under realistic load and faults, and get a report that explains where and why it broke.

The CLI is `loadl`.

> **Status:** early development, pre-MVP. You can validate a design, run it on local Docker (Linux, or macOS with Colima), read its report and work through the first scenario, [When does one Postgres fall over?](scenarios/single-postgres-breaking-point/). Not built yet: four of the six diagnosis rules, three of the four MVP scenarios, the live dashboard, `loadl init`, PgBouncer, and the loss, partition and slow-disk faults. Docker Desktop and WSL2 aren't verified. The spec's "Build status" section has the details.

- **Spec:** [Loadlore — Product Spec](https://claude.ai/artifact/TEkM1GHG5mpmKXTcH3jJ6m) is the source of truth for scope and behavior.
- **AI skill:** [`skills/loadlore-designer/`](skills/loadlore-designer/) turns plain-language descriptions into validated design files.

## Commands so far

```sh
loadl validate examples/single-postgres.yaml   # errors carry field paths; --json for tools
loadl diagram examples/single-postgres.yaml    # Mermaid; --ascii for the terminal
loadl schema --json                            # JSON Schema of the design file
loadl run examples/smoke.yaml                  # run on local Docker; results in .loadlore/runs/<run-id>/
loadl report latest                            # rewrite a run's report.md; --print shows it, --json gives the analysis
loadl compare <run-a> <run-b>                  # two runs side by side, with the design diff
loadl scenarios                                # the scenario library
loadl hint                                     # the next hint for the scenario you're running
loadl scenario check scenarios/<name>          # run a scenario and its solutions; check its claims
loadl down                                     # remove anything a run left behind
```

Every command accepts `--json`.

`loadl run` needs Docker with Compose 2.23 or later and the synthetic service image (`make image`). It uses the current Docker context, or `DOCKER_HOST`. On Colima, give the VM enough CPUs for the design; the pre-flight check says when a design doesn't fit and which `--scale` does.

A run folder holds the exact `design.yaml`, the generated `compose.yaml` and `load.js`, `plan.json` (resources after scaling, and why), `events.jsonl` (load, faults, container deaths), `metrics/` (every series, exported from Prometheus before teardown), `logs/`, `containers.json`, `k6-summary.json`, `run.json`, and the `report.md` and `analysis.json` that `loadl run` writes at the end. Reports are made from the folder alone, so `loadl report` can regenerate them at any time.

Scenarios are guided experiments in [`scenarios/`](scenarios/): a starting design, a story, a question to predict, hints and worked solutions. Start with [When does one Postgres fall over?](scenarios/single-postgres-breaking-point/).

Each run starts from a fresh copy of the seeded dataset. `--reuse-data` skips the copy for quick iteration; the report then says which earlier runs wrote to the data.

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
