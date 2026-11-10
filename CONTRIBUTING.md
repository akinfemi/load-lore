# Contributing

The [spec](https://claude.ai/artifact/TEkM1GHG5mpmKXTcH3jJ6m) is the source of truth for scope and behavior. Discuss spec changes there, as comments on the section they affect.

## Setup

- Go: the version in `go.mod` (the toolchain downloads itself).
- Docker with Compose 2.23 or later, on cgroup v2. Verified on Colima (Apple Silicon, Ubuntu 24.04 VM); Docker Desktop and WSL2 aren't verified yet.
- `make image` builds `loadlore/synthetic:dev`, which `loadl run` uses by default.

On Colima, a dedicated profile keeps experiments away from other work:

```sh
colima start loadlore --cpu 8 --memory 10 --activate=false
export DOCKER_HOST=unix://$HOME/.colima/loadlore/docker.sock
```

## Tests

```sh
make test         # unit tests, with the race detector
make lint         # golangci-lint v2
make integration  # synthetic service against real Postgres and Redis (testcontainers)
make e2e          # runs examples/smoke.yaml end to end on Docker
make golden       # regenerate golden files after an intended output change
```

## Docker behaviors the local driver depends on

These were found while building the local driver. Keep them in mind when changing it.

- **Detect OOM kills from `State.OOMKilled` or the `oom` event, not the exit code.** When the kernel kills a child process inside a container, the container can still exit 0. The driver records `oom` events in `events.jsonl` and `oom_killed` in `containers.json`.
- **`docker update --cpus 0` doesn't remove a CPU limit.** A container must be recreated to lift one. Faults never change limits in place for this reason.
- **Colima mounts only the home directory into the VM by default.** Anything a container needs is passed inline (Compose `configs: content:`), never bind-mounted from a temporary directory.
- **Remove volumes with their containers (`docker rm -v`).** Otherwise anonymous volumes pile up. Teardown removes everything labeled `io.loadlore.run=<run-id>`, and `loadl down` everything labeled `io.loadlore=1`; seeded dataset caches (`io.loadlore.cache`) stay unless `loadl down --all`.
- **A killed container sends no FIN or RST.** Connections to it hang until a timeout, as with a host that loses power. That is realistic, and it is why in-flight requests to a killed replica fail even behind a health-checked `lb`.
