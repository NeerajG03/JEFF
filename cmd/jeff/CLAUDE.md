# cmd/jeff/ — CLI (cobra, thin wrapper)

Cobra commands that wrap SDK packages. All business logic lives in packages — CLI only handles flag parsing, output formatting, and error display.

## Key files

| File | Commands |
|------|---------|
| `main.go` | Root cmd, PersistentPreRunE (config load), AddCommand wiring |
| `pickup_cmd.go` | `jeff pickup` — most complex (hooks, skills, worktrees, CLAUDE.md) |
| `crew_cmd.go` | `jeff crew start/stop/list/send/status/events/capture/cleanup` |
| `orchestrator_cmd.go` | `jeff orchestrator start/attach/stop/info/list` |
| `ship_cmd.go` | `jeff ship` |
| `done_cmd.go` | `jeff done` |
| `dashboard_cmd.go` | `jeff dashboard` |
| `taskctx.go` | `detectTaskContext()` — resolve task ID from CWD |

## Helpers (use these, don't reinvent)

- `openGigStore()` — always `defer store.Close()`
- `launchAgent(dir, agent)` — exec agent tool in directory
- `writeTaskClaudeMD()` — generates task CLAUDE.md on pickup
- `detectTaskContext()` — resolves task ID from CWD symlink

## Rules

- No business logic in cmd/ — put it in a package with tests
- Config (`cfg *jeff.Config`) loaded in PersistentPreRunE — skip-list commands that don't need it
- gig via SDK (`import gig`), not CLI shell-out
- See `docs/adding-commands.md` for step-by-step guide
