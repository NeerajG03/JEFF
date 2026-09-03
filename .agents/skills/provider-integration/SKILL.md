---
name: provider-integration
description: Adds a new agent CLI provider to JEFF from discovery through a real end-to-end smoke test. Use when asked to integrate Codex or another agent/provider, add an AgentProvider, support a new agent CLI, or audit whether a provider integration is complete. Do not use for adding a model to an already-supported provider.
---

# JEFF Provider Integration

Integrate a provider by proving its real CLI behavior first, then mapping its
capabilities onto JEFF. Do not copy another provider and assume matching flags,
session behavior, hook payloads, or file locations.

## Decision Tree

```text
New agent CLI
  |
  +-- interactive TUI? -------- no --> stop: JEFF workers need an interactive mode
  |
  +-- exact session resume? --- no --> support fresh launch; mark resume unsupported
  |
  +-- lifecycle hooks? -------- no --> keep setup correct without hooks
  |                               yes --> add a Delivery and contract tests
  |
  +-- native skills? ---------- no --> rely on the context file and commands
  |
  +-- headless mode? ---------- no --> exclude memory curation
                                  yes --> implement BuildCurateArgs
```

## 1. Lock The CLI Contract

Read the provider's current official docs and inspect the installed binary. Record
the binary version and exact commands. Treat online examples and another JEFF
provider as hints, not proof.

Run commands equivalent to these without changing user configuration:

```bash
command -v <binary>
<binary> --version
<binary> --help
<binary> <resume-subcommand> --help
<binary> <headless-subcommand> --help
```

Build a capability matrix before editing code:

| Capability | Evidence to capture | JEFF consumer |
|---|---|---|
| Interactive launch | command, flags, whether a prompt keeps the TUI alive | pickup, work, orchestrator, crew |
| Safe and unattended modes | approval and sandbox flags separately | `--safe`, default worker launch, curation |
| Model selection | valid IDs, aliases, provider overlap | model inference and persona defaults |
| Resume | exact ID syntax, latest fallback, cwd scope | `jeff crew resume` |
| Headless run | prompt input, output, exit codes, repo requirement | memory curation |
| Context file | filename, lookup order, trust rules | task and home instructions |
| Skills and commands | exact directories and formats | skill and command injection |
| Hooks | config location, events, stdin/output schema, trust | context refresh, heartbeat, session capture |
| Auth | login flow, environment variables, credential location | doctor and setup docs |
| Terminal behavior | startup time, bracketed paste, Ctrl-C, process exit | tmux delivery and worker state |

For claims that affect process lifetime, resume, or tmux input, run a tiny manual
probe. Help text alone does not prove that a prompt stays interactive.

## 2. Trace JEFF End To End

Read `references/jeff-provider-surface.md`. Then run the bundled audit from the
JEFF repository root:

```bash
uv run .agents/skills/provider-integration/scripts/audit_provider_surfaces.py .
```

Classify each finding as:

- **Required:** provider identity, launch command, model routing, config schema,
  home setup, context path, pickup/work launch, crew launch, doctor, and tests.
- **Capability-dependent:** resume, hooks, native skills, custom commands,
  memory suppression, headless curation, and provider model aliases.
- **Documentation-only:** README, setup guide, config reference, and shell help.

Do not force unsupported features into fake implementations. Return an empty
capability where `AgentProvider` allows it. If the interface cannot express a
real provider behavior, change the shared contract and its fake provider first.

## 3. Write The Integration Design

Write a short design note before code. Include:

1. The verified command table for fresh, safe, unattended, resume, and headless runs.
2. The capability matrix with unsupported items called out.
3. The files to change, grouped by core provider, hooks, lifecycle, setup, and docs.
4. Trust and security choices, especially sandbox policies and project-local config.
5. An end-to-end test path and the installed CLI version used for it.

Keep correctness before launch independent of hooks. Project hook trust can block
the first `SessionStart`, so task context, memory, and skills must already exist
before the agent starts.

## 4. Implement In Layers

### Core Provider

1. Add the `AgentTool` constant in `jeff.go`.
2. Add `agent_<name>.go` and register an `AgentProvider` in `init()`.
3. Build argument slices without shell quoting. Shell quoting belongs where JEFF
   turns the command into a tmux command string.
4. Reject models owned by another provider. Avoid broad model matching that makes
   `InferBackend` depend on provider registration order.
5. Create only provider-owned directories and defaults. Never overwrite user files.

### Workspace And Setup

1. Add the provider to `schemas/jeff-config.json` and schema-sync tests.
2. Verify the primary context filename and every alias in home and task workspaces.
3. Map the real native skill directory (`.agents/skills`, `.claude/skills`, etc.).
   Do not assume config and skills share a single root directory.
4. Add doctor detection and a clear install/auth hint.
5. Update setup prompts, completion text, README tables, and config docs.

### Hooks And Lifecycle

1. Add a `hooks.Delivery` only when the CLI has a supported hook system.
2. Use a provider-owned artifact directory (`.codex/hooks`, `.claude/settings.json`).
   Do not make two deliveries discover or delete each other's generated scripts.
3. Map every event name, matcher, timeout unit, input field, and output field.
4. Add the provider script key to each compatible built-in hook generator.
5. Export provider identity when a shared hook body needs provider-specific behavior.
6. Verify session ID capture before claiming resume works.
7. Tune prompt paste and Ctrl-C timing with a real tmux session.

## 5. Verify By Contract

Run all static checks:

```bash
gofmt -w <changed-go-files>
go test ./...
go vet ./...
go build -o /tmp/jeff-dev ./cmd/jeff/
uv run .agents/skills/provider-integration/scripts/audit_provider_surfaces.py . --provider <name>
```

Add tests for:

- provider registration and config-schema parity
- fresh, safe, model, prompt, resume, and headless argument vectors
- foreign-model rejection and inference ambiguity
- idempotent home defaults that preserve user files
- context aliases, skill injection, and unsupported capabilities
- hook install, update, disable, orphan cleanup, event mapping, and ownership
- pickup, orchestrator, worker start, message send, stop, and resume command building

Then use the throwaway JEFF binary for a real smoke test. Never replace the live
`jeff` binary.

```text
gig task -> /tmp/jeff-dev pickup -> task/worktree/context/skills/hooks
         -> provider TUI -> tool call -> hook heartbeat/checkpoint
         -> crew message -> stop -> captured session ID -> crew resume
         -> checkpoint -> ship --dry-run -> done
```

Test both default unattended mode and `--safe`. Confirm that the provider remains
interactive after the initial prompt and that a resumed worker continues the same
session rather than starting a new one.

## Failure Recovery

| Symptom | Likely cause | Action |
|---|---|---|
| Provider compiles but cannot be selected | missing enum/schema/config surface | run the audit and schema-sync test |
| Worker exits after its first prompt | prompt flag starts headless mode | set `SupportsInlinePrompt` false and tune tmux paste |
| Persona default selects the wrong CLI | model ownership overlaps another provider | narrow `OwnsModel`; add pairwise tests |
| Resume starts a new chat | wrong ID, arg order, or hook capture field | inspect the real session payload and resume help |
| Hooks work only after manual approval | project or hook trust gate | keep pre-launch setup complete; document or safely bypass trust |
| Disabling one provider removes another provider's hook | shared artifact discovery | isolate generated files and prove ownership before deletion |
| Headless curation hangs | it can still ask for auth, approval, or repo trust | add preflight checks and truly noninteractive flags |
| Skills are missing | native path differs from `ConfigDir/SkillsSubdir` | change the provider contract instead of adding ad hoc aliases |

## Codex Example

Read `references/codex-case-study.md` when integrating Codex. Re-check every flag
against the installed Codex version because the CLI changes independently of JEFF.
