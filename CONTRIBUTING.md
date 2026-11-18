# Contributing

The [spec](https://claude.ai/artifact/TEkM1GHG5mpmKXTcH3jJ6m) is the source of truth for scope and behavior. Discuss spec changes there, as comments on the section they affect.

## Setup

- Go: the version in `go.mod` (the toolchain downloads itself).
- Docker with Compose 2.23 or later, on cgroup v2. Verified on Colima (Apple Silicon, Ubuntu 24.04 VM); Docker Desktop and WSL2 aren't verified yet.
- `make image` builds `loadlore/synthetic:dev`, which a source build of `loadl run` uses by default (a release uses `ghcr.io/akinfemi/loadlore-synthetic:<its version>`).

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

The designer skill has an eval set, `skills/loadlore-designer/evals/prompts.yaml`: each case is a user prompt given to Claude Code, headless, in a scratch workspace with the skill installed. Code decides the checks it can (the design validates, which services it has, nothing unrequested added, files present, no cloud run), and a judge model answers the rest yes or no. `go run ./cmd/skilleval` runs it with `claude` and `loadl` on `PATH`, and writes `skilleval-out/report.md`. **Every case and judged check is a paid model call**, capped by `--agent-usd` and `--judge-usd`; `-only id,id` runs a few. The agent may only use `loadl`'s commands that don't start containers, and the scenario case's check is `--static`. The `skill-evals` workflow runs it on pull requests that touch the skill, the design schema or the harness, once an `ANTHROPIC_API_KEY` secret is set; until then it skips itself.

## Docker behaviors the local driver depends on

These were found while building the local driver. Keep them in mind when changing it.

- **Detect OOM kills from `State.OOMKilled` or the `oom` event, not the exit code.** When the kernel kills a child process inside a container, the container can still exit 0. The driver records `oom` events in `events.jsonl` and `oom_killed` in `containers.json`.
- **`docker update --cpus 0` doesn't remove a CPU limit.** A container must be recreated to lift one. Faults never change limits in place for this reason.
- **Colima mounts only the home directory into the VM by default.** Anything a container needs is passed inline (Compose `configs: content:`), never bind-mounted from a temporary directory.
- **Remove volumes with their containers (`docker rm -v`).** Otherwise anonymous volumes pile up. Teardown removes everything labeled `io.loadlore.run=<run-id>`, and `loadl down` everything labeled `io.loadlore=1`; seeded dataset caches (`io.loadlore.cache`) stay unless `loadl down --all`.
- **A killed container sends no FIN or RST.** Connections to it hang until a timeout, as with a host that loses power. That is realistic, and it is why in-flight requests to a killed replica fail even behind a health-checked `lb`.
- **In a VM, host load leaks into the measurements.** On Colima or Docker Desktop, work on the host (a browser, a build, another test run) takes CPU time from the VM's vCPUs, and the guest can't tell: containers appear to use more CPU and get CFS-throttled at loads they normally handle. A run of the first scenario's solution taken while headless Chrome rendered charts on the host showed db CPU at 33% at 182 rps (10% on a quiet host) and k6 throttled in up to 18% of its periods; the report marked it inconclusive. Keep the host quiet during runs.
- **Packet faults run in privileged helpers.** `loss` and `partition` run the synthetic image's `netem` command in a helper container that shares the target's network namespace, with `NET_ADMIN`. `disk_slow` puts Postgres's data on an ext4 filesystem on a dm-delay device over a loop device, made by a privileged Alpine helper that installs `device-mapper`, `e2fsprogs` and `util-linux` with `apk` at run time (so it needs network), and mounted by the Docker host's kernel as `/dev/dm-<minor>`: `dmsetup` creates `/dev/mapper/<name>` only inside the helper. Reloading the delay table suspends the device for a moment, which can fail a few requests (0.5% in one window of a test). A Postgres `disk` block uses the same device with a read delay set from the start (kept when `disk_slow` reloads the table) and a direct-I/O loop device (without it, the loop reads its image one request at a time per cgroup through the host's page cache, which held Postgres to about 4,000 reads a second on Colima whatever the IOPS cap), caps the container's IOPS on `/dev/dm-<minor>` with Compose's `blkio_config`, and empties the Docker host's page cache after the dataset copy. Without that, the copied data stays cached, charged to the copying helper rather than to Postgres, and reads ignore Postgres's memory limit. Teardown, and `loadl down`, remove the device and its loop device before the volumes, then `fstrim` the Docker root so a VM's disk image (Colima's) gives the space back to the Mac: a slow-disk run writes its dataset twice. Loss also leaves SYN packets alone, for the reason below.
- **Toxiproxy dials upstream in its accept loop.** In v2.13.0 (and on its main branch), each proxy accepts a client and then calls `net.Dial` to the upstream before accepting the next, with no timeout. A dial that waits, on an upstream whose accept queue is full, that drops handshake packets or that has vanished from the network, holds up every new connection on that caller-to-target edge; a real network would delay only that one. A refused connection fails fast and doesn't stall it. Seen in the recorded runs: lost SYN-ACKs stalled the flaky network's edge for a second at a time (hence loss skipping SYN packets); the slow dependency's api → pricing edge stalls for 1–3 s from about 2:33, once pricing is saturated; slow disk's api → db edge for about a second during the fault, both after their breaks. The analysis finds these stalls in Toxiproxy's log ("Accepted client" lines: a gap of a second or more on a busy edge, then a queue draining under 0.5 ms apart, where HAProxy's own reconnect bursts are about 1.3 ms apart) and makes a run inconclusive when one comes by the end of the break's first window; after a break the system didn't recover from, `loadl scenario check` won't judge claims that rest on not recovering. A scenario that overloads a target past its accept queue, or black-holes one, needs that guard to hold. The fix would be a Toxiproxy that dials per client; we haven't forked it.
- **The lock-wait metric depends on a deprecated exporter flag.** `postgres_exporter` has no lock-wait metric, so the local driver mounts a custom query (`pg_lock_waits_waiting`, sessions with `wait_event_type = 'Lock'`) and loads it with `PG_EXPORTER_EXTEND_QUERY_PATH`, the environment form of `--extend.query-path`, which v0.20.1 marks deprecated. The hot-rows rule needs it. Before bumping `ImagePGExporter`, run a design with Postgres and check `metrics/pg_lock_waits_waiting.json` is there; the analysis also warns when the exporter's other metrics arrive without it.
- **On Linux, loadl samples the host's CPU itself.** When Docker's kernel is this machine's (same kernel release and hostname), loadl reads `/proc/stat` every second during the load, and the analysis uses steal time and the CPU used outside the run's containers to name why tooling was short of CPU. Inside Colima or Docker Desktop that can't be seen, so the report says "possibly".
- **A laptop's power mode moves every number.** On battery with Low Power Mode on, the M4 Mac's speed index roughly halves and the first scenario breaks at about half the load. Reference runs need mains power with Low Power Mode off (`pmset -g batt; pmset -g | grep lowpowermode`).
- **A closed lid sleeps the Mac mid-run.** `loadl run` holds a `caffeinate -i` assertion, which stops idle sleep but not Clamshell Sleep. When the lid closed ten seconds into a 4m30s load, the VM froze with the Mac, the load took 2h7m on the wall clock, and its metrics stopped at 0:15. The analysis now marks a run inconclusive when the load takes more than its profile plus 10% (at least 30 s) on the wall clock. Check `ioreg -r -k AppleClamshellState -d 4 | grep AppleClamshellState` says No before a long unattended series.
- **Colima's clock can run slightly fast.** After one restart, 20 s inside the VM took 19.86 s on the Mac, and k6 finished a 60 s profile in 59 s. The analysis counts a last window that is at least 90% covered, over the seconds it covers.
- **Mixed core types add noise.** An M4 MacBook Air has 4 performance and 6 efficiency cores, so an 8-vCPU Colima VM can't keep every vCPU on a performance core, and which containers land on the slower cores is up to macOS. Results on it vary more between runs than on a machine with uniform cores.
- **Breaking points depend on core speed.** CPU limits are fractions of the host's cores. Pre-flight runs `loadlore/synthetic bench` in a 1-CPU container and prints a host speed index against the reference machine (1000); the same scenario broke at about 2,960 rps on the M4 Mac and at 672 rps on an 8-vCPU shared-CPU cloud VM. Compare runs from the same machine.

## Adding a scenario

A scenario is a folder in `scenarios/`; the format is in [`skills/loadlore-designer/references/scenario-authoring.md`](skills/loadlore-designer/references/scenario-authoring.md). `loadl` embeds the folder, so `loadl scenarios` and `loadl hint` work outside a checkout. `go test ./internal/scenario/` and the `scenarios` workflow run the static checks on every scenario. The workflow's full check, which runs every design, is started by hand on a runner as large as the scenarios' `meta.yaml` asks for: standard GitHub-hosted runners are too small (see the comment in the workflow). Run `loadl scenario check <folder>` at `--scale 1` on an idle machine before opening a pull request.

Don't count on small scales to reproduce a scenario. At `--scale 0.25` the first scenario's Postgres gets 0.125 CPU, CFS throttling alone puts its low-load p99 at 85–130 ms, and it breaks at the very start of the ramp instead of at its queueing knee. A 10 ms CFS period made it worse, not better.

## Releasing

Push a tag such as `v0.1.0` from `main`. The `release` workflow runs the tests, pushes the synthetic image for that version to `ghcr.io/akinfemi/loadlore-synthetic` for amd64 and arm64, and only then runs GoReleaser ([`.goreleaser.yaml`](.goreleaser.yaml)). GoReleaser publishes `loadl` for macOS and Linux to GitHub Releases and updates the cask in `akinfemi/homebrew-tap`. A release build of `loadl` defaults to the image of its own version and pulls it when it's missing; a `go install …@v0.1.0` build does the same. A pre-release tag (`v0.2.0-rc.1`) is marked as one and leaves the tap alone.

The tap update uses the `HOMEBREW_TAP_GITHUB_TOKEN` secret, a fine-grained token with write access to the tap's contents only. A new GHCR package starts private, so after the first release set `loadlore-synthetic` to public in its package settings.

The macOS binaries are signed and notarized with Apple when five secrets are set, so Gatekeeper opens them whether they came from Homebrew or a browser download. Without the secrets the release still works, unsigned: the cask then clears macOS's quarantine flag on install, and a browser download needs `xattr -d com.apple.quarantine loadl`. Setting them up takes a paid Apple Developer Program membership, once:

1. **A Developer ID Application certificate.** In Keychain Access, Certificate Assistant → Request a Certificate From a Certificate Authority, saved to disk. Upload it at [developer.apple.com → Certificates → +](https://developer.apple.com/account/resources/certificates/add) as Developer ID Application (only the account holder can), download the `.cer`, open it to add it to the keychain, then export the certificate with its private key as a `.p12` with a password.
2. **An App Store Connect API key** for notarization: [App Store Connect → Users and Access → Integrations → Team Keys](https://appstoreconnect.apple.com/access/integrations/api), with Developer access. Download the `.p8` (it's offered once) and note its key ID and the issuer ID above the list.
3. **The secrets:**

   ```sh
   R=akinfemi/load-lore
   base64 -i Certificates.p12 | gh secret set MACOS_SIGN_P12 -R $R
   gh secret set MACOS_SIGN_PASSWORD -R $R          # paste the .p12 password
   base64 -i AuthKey_ABC123.p8 | gh secret set MACOS_NOTARY_KEY -R $R
   gh secret set MACOS_NOTARY_KEY_ID -R $R --body ABC123
   gh secret set MACOS_NOTARY_ISSUER_ID -R $R       # paste the issuer ID
   ```

The certificate lasts five years; the API key doesn't expire until revoked.

To check the config without publishing anything: `goreleaser check`, then `goreleaser release --snapshot --clean`, which builds everything into `dist/`.
