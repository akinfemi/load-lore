# Loadlore

Loadlore is an open-source, terminal-first lab for system design. You describe a system in a YAML design file, run it on real Docker containers under realistic load and faults, and get a report that explains where and why it broke.

The CLI is `loadl`.

> **Status:** early development. Nothing here is usable yet.

- **Spec:** [Loadlore — Product Spec](https://claude.ai/artifact/TEkM1GHG5mpmKXTcH3jJ6m) is the source of truth for scope and behavior.
- **AI skill:** [`skills/loadlore-designer/`](skills/loadlore-designer/) turns plain-language descriptions into validated design files.

## Commands so far

```sh
loadl validate examples/single-postgres.yaml   # errors carry field paths; --json for tools
loadl diagram examples/single-postgres.yaml    # Mermaid; --ascii for the terminal
loadl schema --json                            # JSON Schema of the design file
loadl run examples/smoke.yaml                  # run on local Docker; results in .loadlore/runs/<run-id>/
loadl down                                     # remove anything a run left behind
```

Every command accepts `--json`.

`loadl run` needs Docker with Compose 2.23 or later and the synthetic service image (`make image`). It uses the current Docker context, or `DOCKER_HOST`. On Colima, give the VM enough CPUs for the design; the pre-flight check says when a design doesn't fit and which `--scale` does.

A run folder holds the exact `design.yaml`, the generated `compose.yaml` and `load.js`, `plan.json` (resources after scaling, and why), `events.jsonl` (load, faults, container deaths), `metrics/` (every series, exported from Prometheus before teardown), `logs/`, `containers.json`, `k6-summary.json` and `run.json`.

## Development

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
