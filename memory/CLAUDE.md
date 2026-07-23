# memory/ — JEFF memory subsystem

This package is in a **dual-API transitional state** during EPIC `gig-1d33` (memory subsystem v1).

- **Legacy /learn API** (in `memory.go`, `memory_test.go`): persona memory + repo learnings + `/learn` slash command. Function names: `PersonaMemoryDir`, `RepoLearningsDir`, `EnsurePersonaDir`, `EnsureRepoDir`, `LoadPersonaMemory`, `LoadRepoLearnings`, `InstallLearnCommand`, `AutoCurate`. **Phased out by Worker E** (`gig-1d33.6`).
- **v1 API** (everything else in this package): typed schema, propose/curate/store/queue primitives, cobra command tree under `cmd/jeff/memory/`. New work uses this API.

The two APIs share a Go package but no function names collide. They will coexist until E migrates callers and deletes the legacy file.

## v1 API — file roles

| File | What it does | Owner |
|------|-------------|-------|
| `types.go` | `MemoryType`, `ScopeKind`, `Bucket`, path resolvers, `EnsureLayout` | FND |
| `disable.go` | `Disabled(jeffHome)` gate — checks `JEFF_MEMORY_DISABLE` env var and `jeff.json memory.disabled` | R3 |
| `frontmatter.go` | YAML frontmatter parse/write — worker-facing 3-field schema + canonical (bi-temporal) schema | FND |
| `proposals.go` | `proposals/<persona>/<task>/*.md` CRUD | FND |
| `queue.go` | `queue/sessions/*.json` CRUD + `archive/<iso-week>/queue/` rollover; dedupes by (task, transcript) | FND |
| `store.go` | Canonical-memory access (`ListEntries`, `ReadEntry`, `EntryFilter`), write helpers (`WriteCanonical`, `Invalidate`, `Supersede`). Skips corrupt entries with stderr warning. Refuses duplicate non-core writes. | FND |
| `inject.go` | Memory-pack composition + CLAUDE.md/GEMINI.md addendum. Index capped at 30 entries per scope. Atomically writes via tmp+rename. | A (stub) |
| `suppress.go` | Native-memory suppression (Claude/Gemini/opencode) | A (stub) |
| `curate.go` | marlowe curation loop, supersede semantics, archive sweep, `.last-curated` stamp, retention sweep. No longer archives on agent error (inputs preserved for retry). `CurateOptions.Auto` removed. | C |
| `doc.go` | `jeff memory doc` content | D (stub) |
| `init.go` | Initialize/Update/Migrate (retires legacy /learn API) | E (stub) |

`cmd/jeff/memory/*.go` mirrors this with one cobra subcommand per file. The root `Cmd` is wired into `cmd/jeff/main.go`.

## v1 layout (created by `EnsureLayout`)

```
JEFF_HOME/
├── memory/                       canonical — only marlowe writes
│   ├── personas/<name>/{GOAL.md,core.md,procedural/,semantic/,episodic/}
│   ├── repos/<repo>/{core.md,procedural/,semantic/,episodic/}
│   ├── projects/<key>/{core.md,procedural/,semantic/,episodic/}
│   └── orchestrator/             marlowe's own memory
├── proposals/<persona>/<task>/*.md   workers write here
├── queue/sessions/*.json             SessionEnd hook drops here
├── transcripts/<task>/               copies of session transcripts
└── archive/<iso-week>/               processed proposals + queue entries
```

## Schema

Worker-facing — 3 fields, mirrors Claude Code:

```yaml
---
name: <slug>
description: <one-liner>
type: user | feedback | project | reference
---
<body>
```

Canonical (marlowe-enriched, bi-temporal) adds: `status`, `scope`, `goal_served`, `importance`, `valid_from`, `valid_to`, `supersedes`, `superseded_by`, `verifier`, `provenance`, `source`. Soft-invalidate via `valid_to` — entries are never deleted.

## Permission model (enforced in B's `add` command)

| Persona | `JEFF_MEMORY_CAN_ADD` | Allowed ops |
|---|---|---|
| jenko, schmidt, eric, hardy, dickson | unset / 0 | `propose`, `list`, `show`, `status`, `doc` |
| marlowe | `1` | all of the above + `add`, `curate` |

## Disable gate

The memory subsystem can be disabled via `JEFF_MEMORY_DISABLE=1` env var or `jeff.json` `memory.disabled: true`. When disabled:
- Session-start and session-end hooks are no-ops.
- Pickup omits legacy persona-memory and repo-learnings sections from CLAUDE.md.
- `jeff memory propose` returns an error.
- `memory/disable.go` provides the `Disabled(jeffHome string) bool` helper. The `memory` package must NOT import the root `jeff` package (import cycle risk).

## Corrupt-file resilience

- `ListEntries` and `ListScope` skip entries that fail to parse, emitting a `fmt.Fprintf(os.Stderr, "warning: …")` and continuing. Only I/O errors on the directory walk itself remain fatal.
- `listQueueItems` similarly skips malformed or unreadable JSON files.

## Duplicate-write refusal

- `WriteCanonical` refuses to overwrite an existing non-core entry. Returns an error mentioning `--supersede`.
- The `core` bucket (a single well-known file per scope) is exempt — it is meant to be rewritten.
- `Supersede` writes a new entry then invalidates the old one; covered by existing tests.

## Queue dedupe

- `WriteQueueEntry` keys on `<task>-<transcriptBase>.json` when a transcript path is available. Same task + same transcript → idempotent overwrite.
- Falls back to `<task>-<unixnano>.json` when transcript is empty.

## Index cap

- `buildMemoryIndex` caps each scope's bullets at 30, sorted by `importance` desc then `valid_from` desc. Overflow is reported as "…and N more — run 'jeff memory list --scope <scope>'".

## Retention sweep

- After a successful curate pass, `sweepRetention(home)` removes:
  - `transcripts/` files older than 28 days (by mtime).
  - `queue/sessions/*-start.log` files older than 7 days.
- Does NOT touch `archive/` or canonical memory.

## Conventions

- No CGO. SQLite (when needed) uses `modernc.org/sqlite`.
- Tests use `t.TempDir()` — never touch a real `JEFF_HOME`.
- Frontmatter parser must round-trip cleanly (worker- and canonical-schema both).
- New canonical writes are owned by Worker C only — `store.go` is read-only here.

## See also

- `exports/memory-research/specs/EPIC.md` — full design
- `exports/memory-research/specs/FND.md` — this package's spec
- `exports/memory-research/00-synthesis.md` — TL;DR
