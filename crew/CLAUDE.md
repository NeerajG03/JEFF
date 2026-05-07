# crew/ — Multi-agent orchestration

Manages Orchestrator and Session records in SQLite (`jeff.db`). Controls worker lifecycle via tmux panes/windows.

## Types

- **Orchestrator** — tmux session running the orchestrator agent
- **Session** — worker agent in a tmux window, tied to a gig task
- **Message / MessageType** — nudge | status | divert | normal
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
- Messages delivered differently by type — see MessageType constants in `crew.go`
- SendCommand splits paste + Enter into two separate tmux calls (paste must complete before Enter) — includes a 500ms delay for Gemini agent compatibility
- Cleanup = mark DB sessions stopped if tmux window gone (not vice versa)
- Refresh() validates both worker and orchestrator tmux sessions
- Tests use in-memory SQLite (`:memory:`)

## Extending

- New message type: add constant in `crew.go` + delivery case in `lifecycle.go`
- New session field: add to Session struct, update CREATE TABLE in `crew.go`, add migration
- New tmux op: add to `tmux.go` — all tmux interaction goes through this file
