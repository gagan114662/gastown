# Management UI + Chroma Agent Memory — Design

**Date:** 2026-03-27
**Status:** Approved

## Goal

Add two capabilities missing from Gastown that Paperclip provides:

1. **Management UI** — a browser-based React dashboard for spawning agents, assigning beads, configuring rigs, and viewing live agent transcripts
2. **Agent memory via Chroma** — semantic search over transcripts, beads, and docs so every new polecat starts with relevant past context

---

## Architecture

### New components

```
gastown_2/
  ui/                          # Vite + React + TypeScript app
    src/
      pages/                   # one file per page
      components/              # shared UI components
      api/                     # typed fetch wrappers
    package.json
    vite.config.ts

  internal/
    chromadb/                  # new Go package
      client.go                # Chroma HTTP client
      embed.go                 # embedding pipeline
      query.go                 # semantic search helpers
    web/
      manage.go                # new REST endpoints /api/manage/
```

### Deployment

- `gt start` launches **three** processes: dolt (existing), gt daemon (existing), chroma server (new, port 8000)
- Go server embeds built `ui/dist/` and serves at `/manage`
- Existing convoy dashboard at `/` is unchanged
- Dev mode: `pnpm dev` in `ui/` with Vite proxy to local gt daemon

---

## Management UI

### Tech stack

- Vite + React + TypeScript (mirrors Paperclip's `ui/` package)
- Served from Go binary (embedded static files, no separate process in production)
- Same CSRF token auth pattern as existing dashboard
- SSE (`EventSource`) for live agent transcript streaming

### Pages

| Page | Route | Purpose |
|------|-------|---------|
| Dashboard | `/manage` | Town health overview — active agents, recent beads, spend |
| Polecats | `/manage/polecats` | List all agents with status |
| Polecat Detail | `/manage/polecats/:id` | Live transcript stream, injected context, stop/restart |
| New Polecat | `/manage/polecats/new` | Spawn agent — task input with live similarity search |
| Beads | `/manage/beads` | Work items — view, assign, close |
| Rigs | `/manage/rigs` | Project containers — view, configure model tier |
| Costs | `/manage/costs` | Spend by agent/rig/day |
| Activity | `/manage/activity` | Audit trail feed |
| Memory | `/manage/memory` | Semantic search across transcripts, beads, docs |

### REST API (new endpoints under `/api/manage/`)

| Method | Path | Action |
|--------|------|--------|
| GET | `/api/manage/polecats` | List polecats with status |
| POST | `/api/manage/polecats` | Spawn new polecat |
| POST | `/api/manage/polecats/:id/stop` | Stop agent |
| GET | `/api/manage/polecats/:id/stream` | SSE — live transcript |
| GET | `/api/manage/beads` | List beads |
| POST | `/api/manage/beads/:id/assign` | Assign bead to agent |
| POST | `/api/manage/beads/:id/close` | Close bead |
| GET | `/api/manage/rigs` | List rigs |
| PATCH | `/api/manage/rigs/:id` | Update rig config (model tier etc.) |
| GET | `/api/manage/costs` | Cost summary JSON |
| GET | `/api/manage/activity` | Activity feed |
| GET | `/api/manage/memory/search` | Semantic search query |

### Live transcript streaming

Agents write transcripts to `~/.claude/projects/.../*.jsonl`. The Go server watches these with `fsnotify` and pushes new events via SSE. React uses native `EventSource` — no extra libraries needed.

```
Agent (Claude Code)
  → writes ~/.claude/projects/.../transcript.jsonl
    → Go fsnotify watcher
      → parse via internal/agentlog/claudecode.go
        → SSE stream → React EventSource → live transcript panel
```

---

## Chroma Agent Memory

### Deployment

- Chroma runs as a local server (Python/Rust binary) started by `gt start`
- Port 8000 (configurable)
- Go talks to Chroma via its HTTP API (`internal/chromadb/client.go`)
- No API key, no internet required — uses local sentence-transformers embeddings

### Collections

| Collection | Content | Metadata |
|-----------|---------|----------|
| `transcripts` | Agent session chunks (by conversation turn) | rig, agent_id, role, date, bead_id |
| `beads` | Title + description + outcome | rig, status, assigned_to, closed_at |
| `docs` | CLAUDE.md, README, docs/**/*.md chunks | rig, file_path, last_modified |

### Embedding triggers

```
Session ends (Stop hook)
  → parse ~/.claude/projects/.../transcript.jsonl
  → chunk by conversation turn
  → embed into "transcripts" collection

Bead created/updated/closed
  → embed title + description + outcome
  → into "beads" collection

Rig doc changes (git hook on rig repo)
  → embed CLAUDE.md, README, docs/**/*.md
  → into "docs" collection
```

### Agent memory injection flow

When `gt assign` spawns a polecat:

1. Query `transcripts` → top 5 semantically related past sessions
2. Query `beads` → top 5 related work items
3. Query `docs` → top 3 relevant doc chunks
4. Write summary to polecat's CLAUDE.md context section before launch

```markdown
## Relevant past work
- 2026-03-20: polecat-42 fixed JWT expiry in auth middleware (bead #234)
- 2026-03-15: polecat-31 investigated session token storage (transcript)
- Docs: internal/auth/README.md — token lifecycle diagram
```

This directly solves "agents lose context on restart."

### Management UI — Chroma additions

**Memory page** (`/manage/memory`)
- Semantic search box across all collections
- Results grouped by type (transcripts, beads, docs)
- Click result → opens source (bead detail, polecat transcript, doc file)

**Context panel on Polecat Detail**
- "Why this context?" — shows what Chroma injected at spawn time
- Similarity scores for each injected item
- Manually add/remove context items before spawning

**Spawn flow enhancement** (`/manage/polecats/new`)
- Live similarity search as you type the task description
- "Similar work found" warning if duplicates detected
- One-click to assign an existing bead instead of creating new work

---

## Build & dev workflow

```bash
# Install UI deps
cd ui && pnpm install

# Dev mode (React hot reload + Go backend)
cd ui && pnpm dev          # Vite dev server on :5173, proxies /api to :8080
gt start                   # Go daemon + dolt + chroma

# Production build
cd ui && pnpm build        # outputs to ui/dist/
go build ./...             # Go embeds ui/dist/
```

---

## Out of scope (this iteration)

- Per-agent budget enforcement (hard spend caps)
- Multi-town management from one UI
- Chroma Cloud / remote Chroma deployment
- Embedding non-Claude agents (Copilot, Codex transcripts)
