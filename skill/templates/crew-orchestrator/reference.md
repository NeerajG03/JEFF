# Crew Command Reference

Full command documentation, messaging details, workflow patterns, and known gotchas for `jeff crew` and `jeff orchestrator`.

## Architecture

```
jeff-N (tmux session)
├── tab 1: orchestrator  — you live here (Claude Code with crew skill)
├── tab 2: gig-abc1      — worker 1
├── tab 3: gig-def2      — worker 2
└── tab N: ...
```

**Never use raw tmux commands** — always use `jeff` commands. The tmux session is managed entirely by jeff.

## Orchestrator Sessions

```bash
# Start a new orchestrator session (auto-names jeff-1, jeff-2, ...)
jeff orchestrator start

# Start with a friendly name (session becomes jeff-work)
jeff orchestrator start --name work

# List all orchestrators
jeff orchestrator list

# Stop an orchestrator and all its workers
jeff orchestrator stop <id>

# Attach to a running orchestrator
jeff orchestrator attach jeff-1
```

## Starting Workers

```bash
# Start a worker (full pickup: claim, workspace, worktrees, hooks, skills)
jeff crew start <gig-id> --persona jenko --repos backend

# Start with multiple repos
jeff crew start <gig-id> --persona jenko --repos backend,frontend

# Start with model override
jeff crew start <gig-id> --persona eric --model opus

# Resume a previously stopped worker (workspace already exists)
jeff crew resume <gig-id>
```

Workers automatically:
- Detect which orchestrator session they're in (from `$TMUX` → `jeff-N` session name)
- Get hooks injected at pickup time (heartbeat, stop signal, inbox check, etc.)
- Send a heartbeat on every tool call (updates `last_seen` for stall detection)
- Signal the orchestrator on stop (via the Stop hook)
- Receive initial prompt 3 seconds after launch so they begin work immediately

## Monitoring Workers

```bash
# List running sessions (default) or all sessions
jeff crew list
jeff crew list --all

# Detailed status (checkpoint, inbox, pane output)
jeff crew status <gig-id>

# Recent gig events across all workers
jeff crew events --since 10m

# Raw terminal output from a worker's pane
jeff crew capture <gig-id> --lines 30

# TUI dashboard (interactive, live-updating)
jeff dashboard
```

## Messaging Workers

Four message types, from lightest to heaviest context impact:

### Nudge (default) — low context pollution
Delivered to worker's tmux pane AND stored in DB. Worker's PostToolUse hook surfaces it at the next tool call. One-way instruction.

```bash
jeff crew send <gig-id> "add error handling for expired tokens" --type nudge
jeff crew send <gig-id> "run integration tests before shipping" --type nudge
```

### Status — sidechain, no context pollution
Sends `/btw <question>` to the worker. Agent answers in a sidechain without polluting main context.

```bash
jeff crew send <gig-id> "what are you currently working on?" --type status
jeff crew send <gig-id> "are the tests passing?" --type status
```

### Normal — full conversation turn
Types directly into the agent's input. Full context impact.

```bash
jeff crew send <gig-id> "the API spec changed, new endpoint: POST /v2/auth/refresh" --type normal
```

### Divert — redirects the agent (heavy)
Interrupts the agent (C-c), then sends a new message. Use sparingly.

```bash
jeff crew send <gig-id> "stop — priority changed, focus on payments bug" --type divert
```

## Worker → Orchestrator Signals

```bash
# Worker asks orchestrator a question
jeff crew ask "should I use JWT or session tokens for the auth flow?"

# Orchestrator acknowledges
jeff crew ack <msg-id>
jeff crew ack <msg-id> "use JWT, it's already in the codebase"
```

**Automatic signals** (no manual intervention needed):
- **Heartbeat**: every tool call → `jeff crew touch <task-id>` (updates `last_seen`)
- **Stop signal**: when worker's Claude session ends → hook fires message to orchestrator pane
- **Completion signal**: when worker runs `jeff done` → `SignalOrchestrator()` fires

## Stopping Workers

```bash
jeff crew stop <gig-id>     # graceful stop
jeff crew stop --all         # stop all workers
```

## Cleanup

```bash
jeff crew cleanup            # reconcile tmux state vs DB
```

Run this after workers stop unexpectedly or when DB and tmux are out of sync.

## Workflow Patterns

### Solo task delegation
```bash
gig list --status open
jeff crew start gig-ab12 --persona jenko --repos backend
# monitor via signals, nudge if needed
# worker ships → orchestrator gets completion signal
```

### Parallel workers on related tasks
```bash
gig create "Implement auth refresh" --parent gig-epic1
gig create "Add auth tests" --parent gig-epic1
jeff crew start gig-epic1.1 --persona jenko --repos backend
jeff crew start gig-epic1.2 --persona jenko --repos backend
jeff crew list
```

### Review after implementation
```bash
jeff crew send gig-ab12 "ship when tests pass" --type nudge
# after PR created:
jeff crew start gig-review --persona hardy --repos backend
jeff crew send gig-review "review PR #42 on backend" --type normal
```

### Investigate and fix
```bash
jeff crew start gig-xyz --persona schmidt --repos backend
jeff crew status gig-xyz
jeff crew capture gig-xyz
jeff crew send gig-xyz "check the logs at /var/log/app.log" --type nudge
```

## Known Gotchas

- **Dots in task IDs** (e.g. `gig-45c2.2`) are sanitized to hyphens in tmux window names — automatic.
- **JEFF_HOME**: Set as tmux environment variable so all workers inherit it.
- **Hook injection**: Hooks are injected at `jeff crew start` time, not runtime. To update hooks, stop and resume.
- **jeff ship from task workspace**: Run from inside the task directory (`~/jeff/tasks/<task-id>/`).
- **Context exhaustion**: Workers at 90%+ context stall silently. Check `jeff crew capture` if no checkpoint in a while.
