# hooks/ — Agent session context injection

Injects context into Claude Code and OpenCode sessions via hooks. Two delivery mechanisms; two hook sources.

## Concepts

- **Source**: `SourceHome` (jeff init) vs `SourceTask` (jeff pickup)
- **Event**: SessionStart | PostToolUse | Stop
- **Delivery**: Claude Code → bash scripts + settings.json; OpenCode → combined JS plugin

## File roles

| File | What it does |
|------|-------------|
| `hook.go` | Hook type, Source, HookContext, EnabledForSource |
| `registry.go` | Registry — holds all hooks, BySource/All |
| `builtin.go` | All 15 built-in hook definitions + script generators |
| `claude.go` | Write bash scripts + wire settings.json |
| `opencode.go` | Generate combined OpenCode JS plugin |
| `manager.go` | Sync — install/uninstall hooks idempotently |

## Built-in hooks

Home-level (6): gig-instructions, gig-ready-tasks, jeff-repos, jeff-instructions, crew-context, orchestrator-inbox

Task-level (7): task-context, task-commands, checkpoint-nudge, inbox-check, worker-heartbeat, worker-stop, session-capture

Memory (3): memory-session-start, memory-session-end, memory-propose-nudge

## Script types

- `claudeSessionStartStatic()` — heredoc, no shell expansion. Use for literal content (backtick-safe).
- `claudeSessionStartDynamic()` — double-quoted, `$(command)` expands. Use for dynamic data.

Choosing wrong type is a correctness bug — static content with `$()` won't expand; dynamic content with backticks breaks quoting.

## Adding a hook

1. Define func in `builtin.go` returning `*Hook`
2. Add to `builtinHooks()` slice
3. Tests: `builtin_test.go` checks each hook generates non-empty content
