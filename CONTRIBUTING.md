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
- **In a VM, host load leaks into the measurements.** On Colima or Docker Desktop, work on the host (a browser, a build, another test run) takes CPU time from the VM's vCPUs, and the guest can't tell: containers appear to use more CPU and get CFS-throttled at loads they normally handle. A run of the first scenario's solution taken while headless Chrome rendered charts on the host showed db CPU at 33% at 182 rps (10% on a quiet host) and k6 throttled in up to 18% of its periods; the report marked it inconclusive. Keep the host quiet during runs.
- **On Linux, loadl samples the host's CPU itself.** When Docker's kernel is this machine's (same kernel release and hostname), loadl reads `/proc/stat` every second during the load, and the analysis uses steal time and the CPU used outside the run's containers to name why tooling was short of CPU. Inside Colima or Docker Desktop that can't be seen, so the report says "possibly".
- **A laptop's power mode moves every number.** On battery with Low Power Mode on, the M4 Mac's speed index roughly halves and the first scenario breaks at about half the load. Reference runs need mains power with Low Power Mode off (`pmset -g batt; pmset -g | grep lowpowermode`).
- **Colima's clock can run slightly fast.** After one restart, 20 s inside the VM took 19.86 s on the Mac, and k6 finished a 60 s profile in 59 s. The analysis counts a last window that is at least 90% covered, over the seconds it covers.
- **Mixed core types add noise.** An M4 MacBook Air has 4 performance and 6 efficiency cores, so an 8-vCPU Colima VM can't keep every vCPU on a performance core, and which containers land on the slower cores is up to macOS. Results on it vary more between runs than on a machine with uniform cores.
- **Breaking points depend on core speed.** CPU limits are fractions of the host's cores. Pre-flight runs `loadlore/synthetic bench` in a 1-CPU container and prints a host speed index against the reference machine (1000); the same scenario broke at about 2,960 rps on the M4 Mac and at 672 rps on an 8-vCPU shared-CPU cloud VM. Compare runs from the same machine.

## Adding a scenario

A scenario is a folder in `scenarios/`; the format is in [`skills/loadlore-designer/references/scenario-authoring.md`](skills/loadlore-designer/references/scenario-authoring.md). `loadl` embeds the folder, so `loadl scenarios` and `loadl hint` work outside a checkout. `go test ./internal/scenario/` and the `scenarios` workflow run the static checks on every scenario. The workflow's full check, which runs every design, is started by hand on a runner as large as the scenarios' `meta.yaml` asks for: standard GitHub-hosted runners are too small (see the comment in the workflow). Run `loadl scenario check <folder>` at `--scale 1` on an idle machine before opening a pull request.

Don't count on small scales to reproduce a scenario. At `--scale 0.25` the first scenario's Postgres gets 0.125 CPU, CFS throttling alone puts its low-load p99 at 85–130 ms, and it breaks at the very start of the ramp instead of at its queueing knee. A 10 ms CFS period made it worse, not better.
