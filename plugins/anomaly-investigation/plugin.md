+++
name = "anomaly-investigation"
description = "Investigate rigs that show high activity without completions and summarize the likely bottleneck"
version = 1
api_version = "v1"
min_gastown_version = "0.0.0"
permissions = ["events:read", "beads:read", "metrics:read"]

[gate]
type = "manual"

[tracking]
labels = ["plugin:anomaly-investigation", "category:observability"]
digest = true

[execution]
timeout = "10m"
notify_on_failure = true
severity = "medium"
+++

# Anomaly Investigation

Investigate a rig after `gt patrol remediate` detects a sustained burst of
activity without any `done` events.

## Goal

Produce a short diagnosis that answers:

1. Which agents are active or unhealthy right now?
2. Which convoy/bead/work item appears blocked?
3. Whether the likely remedy is `nudge`, `handoff`, `restart`, or `escalate`
4. What evidence supports that conclusion

## Inputs

- The rig name is provided in the dispatch context.
- Recent event counts are attached by the caller when available.

## Procedure

1. Inspect current health.

```bash
gt feed --problems --rig "$GT_RIG" --limit 20
gt patrol scan --rig "$GT_RIG" --json
```

2. Inspect recent agent activity and unfinished work.

```bash
bd list --json -n 40
bd activity --json -n 60
```

3. Look for:

- polecats with hooked work and no progress
- repeated `nudge`, `handoff`, or `escalate` events
- a convoy or merge queue gate that is accumulating attempts without a `done`
- evidence that the rig is healthy and the anomaly was a false positive

4. Record a concise summary with the suspected bottleneck and the next action.

## Output

Create a result that includes:

- `diagnosis`: one sentence
- `blocked_target`: rig / polecat / bead / convoy if known
- `recommended_action`: `nudge`, `handoff`, `restart`, `escalate`, or `observe`
- `evidence`: 2-4 concrete observations from commands above
