# Plugin Packages v1

Gastown plugins are versioned packages rooted at `plugins/<name>/`.

## Layout

```text
plugins/<name>/
├── plugin.md
└── run.sh        # optional
```

`plugin.md` is required. `run.sh` is optional and lets a plugin execute as a
script instead of pure markdown instructions.

## `plugin.md` frontmatter

```toml
+++
name = "anomaly-investigation"
description = "Investigate high-activity rigs with no completions"
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
```

## Contract fields

- `name`: unique plugin identifier
- `description`: operator-facing summary
- `version`: plugin content/schema version
- `api_version`: Gastown package contract version, currently `v1`
- `min_gastown_version`: minimum supported `gt` version
- `permissions`: declarative capability list for auditing and future enforcement

## Commands

- `gt plugin list`
- `gt plugin show <name>`
- `gt plugin sync`
- `gt plugin install <name|path|git-url>`
- `gt plugin upgrade <name>`

Town-level runtime plugins live in `~/gt/plugins/`. Rig-level plugins live in
`<rig>/plugins/` and take precedence when names collide.
