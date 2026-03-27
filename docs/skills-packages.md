# Skill Packages v1

Canonical skills live under `skills/<name>/` and sync into runtime-specific
directories such as `.claude/skills`.

## Layout

```text
skills/<name>/
├── skill.toml
├── SKILL.md
└── fixtures/
    └── *.json
```

## Files

- `skill.toml`: package metadata used by `gt skills test` and `gt skills audit`
- `SKILL.md`: the instructions shipped to the runtime
- `fixtures/*.json`: lightweight regression fixtures that assert expected phrases
  still exist in the skill text

## Example

```toml
name = "agent-ci"
description = "Debug CI failures locally with Docker."
version = "1.0.0"
providers = ["claude", "codex", "copilot"]
```

## Commands

- `gt skills sync`
- `gt skills test`
- `gt skills audit`

The canonical package is the source of truth. Runtime copies are treated as
derived artifacts and are checked for drift during `gt skills audit`.
