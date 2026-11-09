# Writing a scenario

A scenario is a guided experiment: a starting design, a story, a question the learner must predict, and hints. The learner should discover the lesson by running it, not by reading it.

## Folder layout

```
scenarios/<scenario-name>/
  README.md        # story, question, what to try
  design.yaml      # starting design (validated)
  hints.md         # progressive hints
  meta.yaml        # level, concepts, time, host requirements
  solutions/
    <solution-name>/
      design.yaml  # an improved design
      NOTES.md     # what changed and why it works
```

## README.md

Keep it under a page. Sections, in this order:

1. **The situation.** Two or three sentences of story in plain terms ("Your store's product page is about to be featured in a newsletter going to 2 million people.").
2. **The system.** The Mermaid diagram from `loadl diagram design.yaml`, and one line per component.
3. **The question.** What the learner must predict before running ("At what request rate does p99 latency pass 500 ms, and which component causes it?"). Tell them to write it under `expect`, or to tell their AI assistant.
4. **Run it.** The exact command: `loadl run design.yaml`.
5. **Then try.** One or two open prompts that lead toward the lesson without naming the fix ("What happens if you double the api replicas? Why?").

Never put the answer, the bottleneck, or the fix in the README.

## hints.md

Three to five hints, each a level more direct, separated by `---` so `loadl hint` can reveal one at a time.

1. Where to look ("Watch the database panel during the ramp.").
2. What the evidence means ("Connections are at their limit while database CPU is low. What are requests waiting for?").
3. The concept ("This is pool exhaustion. Requests queue for a connection, not for the database.").
4. A direction, not a full solution ("Is there a way to share fewer connections between more replicas?").

## meta.yaml

```yaml
title: The connection pool trap
level: 1                     # 1 beginner, 2 intermediate, 3 advanced
concepts: [connection-pools, queueing, pgbouncer]
minutes: 20
host: { cpu: 4, memory: 8g }  # minimum to run at scale 1
modes: [local]               # modes the scenario is tested in
```

## Solutions

At least one. Each solution is a full `design.yaml` plus `NOTES.md` explaining what changed, why it works, and what it costs (more components, more memory, new failure modes). Where there are several reasonable fixes, include more than one; showing trade-offs is part of the lesson.

## Checks before handing over

1. `loadl validate` passes for the starting design and every solution.
2. `loadl scenario check <folder>` passes. It runs the starting design and each solution in local mode and checks that the starting design shows the problem the README describes and that each solution improves the stated metric.
3. The README contains no answer; reread it as a learner.
4. Every assumption in the designs is listed and plausible.

## Turning a real incident into a scenario

Public postmortems make good scenarios. Model the system as described, reproduce the trigger as a `scenario` fault or workload spike, and check that the failure mode matches the postmortem's account. Credit and link the source postmortem in the README. Don't use internal or confidential incident details unless the user owns them and confirms they can be shared.
