# Writing a scenario

A scenario is a guided experiment: a starting design, a story, a question the learner must predict, and hints. The learner should discover the lesson by running it, not by reading it. `scenarios/when-to-scale-your-database/` is the reference example.

## Folder layout

```
scenarios/<scenario-name>/
  README.md        # story, question, how to run, what to try, the diagram
  design.yaml      # the starting design; its name is the folder's name
  hints.md         # progressive hints, one "## " section each
  meta.yaml        # level, concepts, minutes, host resources, check claims
  solutions/
    <solution-name>/
      README.md    # what changed, why it helps, what it costs
      design.yaml  # the improved design
      report.md    # the report from a reference run of it (copied from .loadlore/runs/<id>/)
  variants/        # optional
    <variant-name>/
      README.md    # what the variant is and what it shows
      design.yaml  # a design the learner is likely to try
```

## README.md

Keep it short. Sections, in this order:

1. **Title.** The scenario's question, such as "When do you need to scale your database?", then one line with the level, the minutes and the concepts.
2. **The story.** Two or three sentences in plain terms, then the diagram: the exact output of `loadl diagram design.yaml` in a ```mermaid block. `loadl scenario check` fails when the diagram doesn't match the design, so regenerate it after every design change.
3. **The question.** What the learner must predict before running. Tell them to run `loadl predict design.yaml`, which asks the questions in `meta.yaml`'s `predict` block, or to write the lines under `expect` themselves, or to tell their AI assistant.
4. **Run it.** The exact command, how long it takes, and the host it needs (from `meta.yaml`), with the `--scale` advice for smaller machines. Mention `loadl hint`.
5. **What to try next.** Open directions that don't single out the fix, and a pointer to `solutions/` with a spoiler warning.

Never put the answer, the bottleneck or the fix in the README.

## hints.md

Three to five hints. Each starts with a `## ` heading, which `loadl hint` shows as its title; text before the first heading is not a hint. Each hint is a level more direct:

1. Where to look ("Read the verdict, then the latency chart around that moment.").
2. What the evidence means ("The pool is full. Is it the cause or a symptom? Which rule fired?").
3. The concept ("Latency bends sharply as utilization approaches 1: queueing.").
4. The arithmetic ("How many statements does one request send?").
5. Directions, not a full solution, ending with a pointer to `solutions/`.

Hints refer to the learner's own report, not to numbers from a reference run.

## meta.yaml

```yaml
title: When do you need to scale your database?
level: 1                       # 1 beginner, 2 intermediate, 3 advanced
concepts: [saturation, queueing knee, connection pools]
minutes: 45                    # rough time to work through it

# Available to Docker at --scale 1, Loadlore's own containers included.
# loadl run warns when the host has less, and suggests a --scale.
# reference and speed: the machine the scenario was checked on and the
# host speed index loadl run printed there; it warns on a much slower host.
# Add them once the full check passes; until then --static only warns.
host:
  cpus: 8
  memory: 8g
  reference: MacBook Air M4, Colima VM with 8 vCPUs
  speed: 1000

# Load generator VUs started before the load, at --scale 1, measured: the
# larger of the peak rate x slo.p99_ms and 1.5 x the most VUs busy at once
# before a break in any of the check's runs. loadl scenario check prints
# what its runs call for; copy it here with the run it came from.
load:
  prealloc_vus: 1750
  peak_vus: 956
  from: when-to-scale-your-database-one-query-per-page-20261111-161415

# What `loadl scenario check` verifies, at --scale 1.
check:
  problem:
    verdict: broke             # broke or held: what the starting design must do
    diagnoses: [queueing-knee] # rules that must fire
    # signals: [latency]       # break signals that must show at the break
    # recovers: false          # healthy again by the end of the load (true), or still broken (false)
  solutions:
    one-query-per-page: {}     # helps: holds, or serves min_gain x the peak throughput or more
    # bigger-db: { min_gain: 1.5, diagnoses: [queueing-knee] }
  variants:                    # one claim per variants/ folder; verdict required
    # bigger-pools: { verdict: broke, signals: [errors] }
```

Arrivals are open, and each needs a free VU. VUs started mid-run take time to start and drop arrivals meanwhile, which in a stall lightens the load just when it matters, so `load` sizes them from measurement. Leave it out until the first check has run, then copy the numbers the check prints. Designs outside a scenario get the peak rate × `slo.p99_ms`.

`predict` lists the README's questions for `loadl predict`. Each takes an answer of a kind, `rps` (an offered load, or `never`), `service` (one of the design's services) or `text`, and becomes the `expect` line in `expect`, with `{answer}` where the answer goes; an `rps` question also gives the `never` line. A question the grammar can't score becomes a quoted `note`:

```yaml
predict:
  - ask: At what offered load, in requests per second, will p99 pass 500 ms? Answer never if you think it stays fast.
    answer: rps
    expect: p99_ms < 500 until load_rps >= {answer}
    never: p99_ms < 500 throughout
  - ask: Which component gives out first?
    answer: service
    expect: 'note: "{answer} gives out first"'
```

Size `host` from the plan: the design's limits plus Loadlore's containers (`loadl run` prints the budgets). Breaking points move with core speed, so state the claims as relative results (broke or held, which rule fired, the gain from a fix), never as request rates. Rule ids are `pool-exhausted`, `queueing-knee`, `connection-storm`, `retry-storm`, `cold-cache`, `working-set` and `hot-rows` in this version. Break signals are `latency`, `errors` and `throughput`; a claim with `signals` needs `verdict: broke`.

A scenario whose problem is a fault under steady load (a pause, a kill, a slow dependency) needs solutions that hold: the load is flat, so a fix can't serve more at its peak than the starting design did before the fault, and `helps` falls back to the verdict. A short episode of errors or slow responses, one 10-second window, is a stall in the timeline, not a break, so a fix that absorbs the fault within one window holds. Put the fault inside a window (at 2m1s rather than 2m) and keep it short, and remember that the synthetic service has no fallback: requests that need a failing dependency fail during the fault whatever the fix. `scenarios/slow-dependency` is the worked example. When even a good fix can't absorb the fault in one window, claim recovery instead: the starting design `recovers: false`, each solution `recovers: true`. A solution then helps when it recovers and the starting design doesn't; a run recovers when every window from some point after its break to the end of the load, at least three of them, is healthy. `scenarios/cache-restart` works this way.

## Solutions

At least one. Each is a folder with a full `design.yaml` (named `<scenario>-<solution>`), a `README.md` explaining what changed, why it helps and what it costs, and the `report.md` of a reference run. Where there are several reasonable fixes, include more than one; showing trade-offs is part of the lesson. A solution must not need more host than `meta.yaml` states.

## Variants

A variant is a design the learner is likely to try that must show a stated outcome. Usually it is the obvious fix walking into a trap: the api's pools made 3x bigger run into Postgres `max_connections`, and the run breaks with errors.

Add one for the fix the hints lead the learner to try, when its failure is part of the lesson. The check then keeps that failure true. Without one, a change to the load profile can quietly turn the trap into a fix, and nothing notices.

Each variant is a folder in `variants/` with a full `design.yaml` (named `<scenario>-<variant>`) and a `README.md` saying what the variant is and what it shows. Like solutions, it's a spoiler.

Every variant needs a claim in `meta.yaml`, and the claim needs a verdict:

```yaml
check:
  variants:
    bigger-pools: { verdict: broke, signals: [errors] }
```

`diagnoses` and `signals` work as they do for the problem. Variants run after the solutions, each once the host has cooled to the starting design's speed. A variant's claim doesn't depend on another run, so it never runs again. Hints may point at a variant once the learner has tried the fix ("variants/bigger-pools shows what happens.").

## Checks before handing over

1. `loadl scenario check <folder> --static` passes: every file is there, every design validates, the README's diagram matches, and the claims name real solutions, variants, rules and signals.
2. `loadl scenario check <folder>` passes on a quiet host at `--scale 1`: the starting design shows the problem, each solution helps, and each variant shows its claim. Small scales may not reproduce it: a fraction of a CPU stalls under throttling and fails in its own way. When a solution's run and the starting design's measure host speed indexes more than 10% apart, the slower one runs again once the host has cooled; still apart, the check ends `INCONCLUSIVE` rather than passing or failing, since the host's speed could explain the difference. Let the machine cool, check it's on mains power, and run the check again. A claim that a run doesn't recover (`recovers: false`, or a solution that helps by recovering where the starting design didn't) is also inconclusive when Toxiproxy stalled new connections after that run's break: Toxiproxy dials each upstream in turn, so a slow dial holds up a whole edge, which can keep a system down that would otherwise come back. Avoid designs whose fault leaves a target unable to finish handshakes for long (a full accept queue, a black hole) when the claim is about recovery.
3. The README contains no answer; reread it as a learner.
4. Every assumption in the designs is listed and plausible.

Run checks on an otherwise idle machine. On Colima or Docker Desktop, other work on the host steals CPU from the VM, which shows up inside it as higher CPU use and CPU throttling; the report then marks the run inconclusive.

## Turning a real incident into a scenario

Public postmortems make good scenarios. Model the system as described, reproduce the trigger as a `scenario` fault or workload spike, and check that the failure mode matches the postmortem's account. Credit and link the source postmortem in the README. Don't use internal or confidential incident details unless the user owns them and confirms they can be shared.
