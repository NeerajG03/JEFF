# memory/ — Persona memory and repo learnings

Manages the directory structure for persistent agent memory. Two types: persona memory (`JEFF_HOME/personas/<name>/memory/`) and repo learnings (`JEFF_HOME/learnings/<repo>/`).

## File roles

| File | What it does |
|------|-------------|
| `memory.go` | Dir paths, EnsureDir, LoadIndex, InstallLearnCommand, AutoCurate |

## Memory structure

- **Persona memory**: `personas/<name>/memory/MEMORY.md` (index) + detail files
- **Repo learnings**: `learnings/<repo>/INDEX.md` (index) + detail files
- Seed detection: heading + comment-only files = treated as empty (avoids false positives)

## /learn command

- `InstallLearnCommand()` bakes all paths into `.claude/commands/learn.md`
- The command runs: read scratchpad → classify → write to persona memory / repo learnings
- `AutoCurate()` runs claude non-interactively (`--dangerously-skip-permissions`)

## Index format

Both `MEMORY.md` and `INDEX.md` use: `- [Title](file.md) — one-line summary`

Detail files use frontmatter with `source` and `updated` fields.
