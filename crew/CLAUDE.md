# crew/ — Multi-agent orchestration

Manages Orchestrator and Session records in SQLite (`jeff.db`). Controls worker lifecycle via tmux panes/windows.

## Types

- **Orchestrator** — tmux session running the orchestrator agent
- **Session** — worker agent in a tmux window, tied to a gig task
- **Message** — stored in the inbox and delivered to the worker's tmux pane
- **Store** — SQLite-backed store (`jeff.db` in JEFF_HOME)

## File roles

| File | What it does |
|------|-------------|
| `crew.go` | Core types, DB schema, Store open/close, migrations |
| `lifecycle.go` | Start/Stop/Send/Capture — all state mutations |
| `tmux.go` | Tmux primitives (HasSession, SendCommand, NewWindow) |
| `cleanup.go` | Reconcile DB sessions against live tmux windows |

## Key behaviors

- DB: `modernc.org/sqlite` (no CGO), file: `$JEFF_HOME/jeff.db`
- Session status flow: `starting → running → done / failed / stopped`
- `Send()` stores the message in the inbox then delivers it to the worker's pane. Pass `interrupt=true` to Ctrl-C the agent before delivery.
- SendCommand splits paste + Enter into two separate tmux calls (paste must complete before Enter) — includes a 500ms delay for Gemini agent compatibility
- Cleanup = mark DB sessions failed if tmux window gone (not vice versa). Kill rules:
  - Kills a window ONLY if it has no DB row AND its pane is dead (remain-on-exit leftover). A live pane is NEVER killed — a DB↔tmux mismatch (renamed window, diverged JEFF_HOME, start race) must not take down running workers.
  - Refuses the orphan sweep entirely when the DB has zero sessions and zero orchestrators (`SkippedNoState`) — that's a wrong-DB signal, not a garbage signal.
  - A `list-windows` error for a session skips that session (no stale-marking, no kills) — same transient-probe rule as Refresh (gig-0c51).
- Refresh() validates both worker and orchestrator tmux sessions
- Tests use in-memory SQLite (`:memory:`)

## Extending

- New message behavior: add delivery logic in `lifecycle.go`
- New session field: add to Session struct, update CREATE TABLE in `crew.go`, add migration
- New tmux op: add to `tmux.go` — all tmux interaction goes through this file
