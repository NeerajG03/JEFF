# JEFF Provider Surface Map

Use this map when extending JEFF to support a new agent CLI. Paths can move;
symbols and contracts matter more than line numbers.

## End-To-End Flow

```text
jeff.json agent/model
  -> Config validation and schema
  -> AgentProvider registry
  -> pickup creates task + worktrees
  -> context aliases + native skills + task hooks
  -> foreground exec OR crew tmux command
  -> provider TUI
  -> lifecycle hooks update memory/session/crew state
  -> stored session ID feeds crew resume
```

## Core Provider

| Surface | Main files | Contract |
|---|---|---|
| Identity and inference | `jeff.go`, `agent.go` | unique name, registry, non-overlapping model ownership |
| Provider implementation | `agent_claude.go`, `agent_gemini.go`, `agent_opencode.go` | command, args, capabilities, home files, timing |
| Configuration | `config.go`, `schemas/jeff-config.json`, `schema_sync_test.go` | valid persisted agent and editor schema parity |
| Model aliases | `opencode_models.go`, provider tests | add only if the provider needs user-defined mapping |

`AgentProvider` currently groups command behavior, config paths, skill paths, hook
delivery, model routing, doctor checks, and tmux timing. If a provider has separate
roots for config, hooks, and skills (such as Codex using `.codex/` for hooks but
`.agents/` for skills), do not lie through `ConfigDir()`. Extend the contract with
explicit paths and update `agent_fake_test.go` plus all providers.

## Launch And Resume

| Path | Main files | What to prove |
|---|---|---|
| Foreground pickup/work | `cmd/jeff/launch.go`, `pickup_cmd.go`, `work_cmd.go` | binary lookup, env, safe mode, cwd, exit behavior |
| Crew command build | `cmd/jeff/crew_cmd.go`, `crew/lifecycle.go` | safe shell construction, inline prompt choice, tmux startup |
| Orchestrator | `cmd/jeff/orchestrator_cmd.go`, `crew/lifecycle.go` | same provider rules as workers |
| Resume | `cmd/jeff/crew_cmd.go`, `cmd/jeff/crew_resume_test.go` | exact session ID and fresh safety setting |
| Message delivery | `crew/lifecycle.go`, `crew/tmux.go` | bracketed paste, delays, interrupt settle time |
| Curation | `memory/curate.go`, provider `BuildCurateArgs` | no prompt, auth, approval, or repo-trust interaction |

`SupportsInlinePrompt` is a process-lifetime claim. Test it in the TUI. A CLI can
accept `PROMPT` yet exit after one turn.

## Workspace Content

| Surface | Main files | What to prove |
|---|---|---|
| Task instructions | `task/claudemd.go` | provider reads primary file from task cwd |
| Home aliases | `cmd/jeff/init_cmd.go`, `embed/embed.go` | aliases point at the canonical instructions safely |
| Skill injection | `task/pickup.go`, `skill/inject.go`, `cmd/jeff/skill_cmd.go` | links land at the native provider path |
| Custom commands | provider methods and skill command paths | empty capability is handled if unsupported |
| Home setup | `cmd/jeff/init_cmd.go`, provider `EnsureHomeDirs` and `WriteHomeDefaults` | idempotent and no user-file overwrite |

JEFF writes the canonical task context before launch. Preserve that order. Hooks
refresh context, but they must not be the only way a new worker learns its task.

## Hooks

| Surface | Main files | What to prove |
|---|---|---|
| Delivery registry | `hooks/delivery.go`, `hooks/manager.go` | install, uninstall, sync, list, ownership, event mapping |
| Provider delivery | `hooks/delivery_claude.go`, `delivery_gemini.go`, `delivery_opencode.go` | native config format and isolated artifacts |
| Hook definitions | `hooks/builtin.go`, memory hook files | every supported hook has a provider generator |
| Re-sync | `cmd/jeff/hook_sync.go`, `hooks/hook.go` | stale detection sees the provider's files |
| Contract tests | `hooks/*_test.go` | payload output names match mapped event names |

Do not infer installed hooks by scanning another delivery's directory. Uninstall
only an artifact that JEFF can prove it owns. This avoids deleting user hooks and
prevents one provider sync from changing another provider.

## Setup, Docs, And Tests

Check these after core behavior works:

- `cmd/jeff/config_cmd.go` and completion/help strings
- `cmd/jeff/doctor_cmd.go` and install/auth hints
- `docs/agent-setup.md`, `docs/config.md`, `docs/usage.md`, and `README.md`
- `agent_test.go`, `config_test.go`, `schema_sync_test.go`
- `cmd/jeff/launch_test.go`, crew start/resume tests, hook delivery tests
- init/update tests for directories, defaults, aliases, and idempotence

## Security Checklist

Answer these in the design note before implementation:

1. Does unattended mode disable approvals, sandboxing, or both?
2. Does `--safe` restore every native protection JEFF disabled?
3. Can project-local config or hooks run before the project is trusted?
4. Can hook output leak secrets into model context or transcripts?
5. Does headless mode need write access outside the task worktree?
6. Where are auth tokens stored, and does JEFF ever copy or modify them?
7. Does any generated config overwrite a user-owned file?
