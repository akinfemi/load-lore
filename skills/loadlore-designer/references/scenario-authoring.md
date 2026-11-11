# Writing a scenario

A scenario is a guided experiment: a starting design, a story, a question the learner must predict, and hints. The learner should discover the lesson by running it, not by reading it. `scenarios/single-postgres-breaking-point/` is the reference example.

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
```

## README.md

Keep it short. Sections, in this order:

1. **Title.** The scenario's question, such as "When does one Postgres fall over?", then one line with the level, the minutes and the concepts.
2. **The story.** Two or three sentences in plain terms, then the diagram: the exact output of `loadl diagram design.yaml` in a ```mermaid block. `loadl scenario check` fails when the diagram doesn't match the design, so regenerate it after every design change.
3. **The question.** What the learner must predict before running. Tell them to write it under `expect` in `design.yaml`, or to tell their AI assistant.
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
title: When does one Postgres fall over?
level: 1                       # 1 beginner, 2 intermediate, 3 advanced
concepts: [saturation, queueing knee, connection pools]
minutes: 45                    # rough time to work through it

# Available to Docker at --scale 1, Loadlore's own containers included.
# loadl run warns when the host has less, and suggests a --scale.
# reference and speed: the machine the scenario was checked on and the
# host speed index loadl run printed there; it warns on a much slower host.
host:
  cpus: 8
  memory: 8g
  reference: MacBook Air M4, Colima VM with 8 vCPUs
  speed: 1000

# What `loadl scenario check` verifies, at --scale 1.
check:
  problem:
    verdict: broke             # broke or held: what the starting design must do
    diagnoses: [queueing-knee] # rules that must fire
  solutions:
    one-query-per-page: {}     # helps: holds, or serves min_gain x the peak throughput or more
    # bigger-db: { min_gain: 1.5, diagnoses: [queueing-knee] }
```

Size `host` from the plan: the design's limits plus Loadlore's containers (`loadl run` prints the budgets). Breaking points move with core speed, so state the claims as relative results (broke or held, which rule fired, the gain from a fix), never as request rates. Rule ids are `pool-exhausted`, `queueing-knee` and `connection-storm` in this version.

## Solutions

At least one. Each is a folder with a full `design.yaml` (named `<scenario>-<solution>`), a `README.md` explaining what changed, why it helps and what it costs, and the `report.md` of a reference run. Where there are several reasonable fixes, include more than one; showing trade-offs is part of the lesson. A solution must not need more host than `meta.yaml` states.

## Checks before handing over

1. `loadl scenario check <folder> --static` passes: every file is there, every design validates, the README's diagram matches, and the claims name real solutions and rules.
2. `loadl scenario check <folder>` passes on a quiet host at `--scale 1`: the starting design shows the problem and each solution helps. Small scales may not reproduce it: a fraction of a CPU stalls under throttling and fails in its own way. When a solution's run and the starting design's measure host speed indexes more than 10% apart, the slower one runs again once the host has cooled; still apart, the check ends `INCONCLUSIVE` rather than passing or failing, since the host's speed could explain the difference. Let the machine cool, check it's on mains power, and run the check again.
3. The README contains no answer; reread it as a learner.
4. Every assumption in the designs is listed and plausible.

Run checks on an otherwise idle machine. On Colima or Docker Desktop, other work on the host steals CPU from the VM, which shows up inside it as higher CPU use and CPU throttling; the report then marks the run inconclusive.

## Turning a real incident into a scenario

Public postmortems make good scenarios. Model the system as described, reproduce the trigger as a `scenario` fault or workload spike, and check that the failure mode matches the postmortem's account. Credit and link the source postmortem in the README. Don't use internal or confidential incident details unless the user owns them and confirms they can be shared.
