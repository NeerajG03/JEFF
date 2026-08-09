# PLAN — Selectable Orchestrator Provider & Model

## Goal

Let an orchestrator run on any registered agent — `claude`, `gemini`, or `opencode` —
chosen per-invocation, with an optional model override:

```
jeff orch start --name <name> --agent <opencode|claude|gemini> [--model <id>]
```

Example (the motivating case): a cheap free-tier opencode orchestrator:

```
jeff orch start --name cheap --agent opencode --model opencode/deepseek-v4-flash-free
```

## Why this is mostly already possible

`jeff orchestrator start` already goes through the provider abstraction:

- `orchestrator_cmd.go:219` — `jeff.GetProvider(cfg.Agent)`
- `:223` — `provider.BuildLaunchArgs(jeff.LaunchOpts{})`  ← **empty opts: no model**
- `:228` — `crew.StartOrchestratorWithLaunchCmd(..., string(cfg.Agent), name, launchCmd)`

Home hooks are synced for **every** registered agent (`config_cmd.go:224-233`), so an
opencode/gemini orchestrator at `JEFF_HOME` already gets its hook plugin. The
orchestrator-inbox snippet resolves its own id at runtime from
`${JEFF_ORCHESTRATOR_SESSION}` (`hooks/builtin.go:727-745`), and worker→orchestrator
pings are `tmux send-keys` into the pane (agent-agnostic). **The runtime supports a
non-claude orchestrator today.** What is missing is the selection UX and record-keeping.

## Scope

### 1. `--agent` and `--model` flags on `jeff orch start`

`cmd/jeff/orchestrator_cmd.go` — `orchestratorStartCmd()`:

- Add `--agent` (string) and `--model` (string) flags.
- Resolution order (mirror `crew start`, `crew_cmd.go:102-122`):
  1. Start from `cfg.Agent`.
  2. If `--agent` given, use it (validate it is a registered agent).
  3. If `--model` given, auto-route: `if inferred := jeff.InferBackend(model); inferred != ""` override the agent; else require `jeff.IsValidModel(agent, model)` or return `jeff.UnknownModelError(model)`.
  4. `--agent` and an inferred-conflicting `--model` → error (e.g. `--agent claude --model opencode/x`).
- Build launch args **with the model**:
  ```go
  provider := jeff.GetProvider(agentTool)
  if provider == nil { return fmt.Errorf("no provider registered for agent %q", agentTool) }
  launchArgs := provider.BuildLaunchArgs(jeff.LaunchOpts{Model: model})
  ```
  (Do **not** pass `AgentName`/persona — orchestrator has no persona here, and opencode
  ignores `--agent` anyway per `agent_opencode.go:26-29`.)

### 2. Persist agent + model on the orchestrator

`crew/crew.go` — add `Agent string` and `Model string` to the `Orchestrator` struct;
add columns to the `orchestrators` CREATE TABLE + a migration; update the INSERT/scan in
`PutOrchestrator`/`GetOrchestrator`/`ListOrchestrators`.

`crew/lifecycle.go` — `StartOrchestratorWithLaunchCmd` already receives `agent`; thread
`model` through (new param or an opts struct) and set both on the `Orchestrator` record.

### 3. Surface agent/model in `list` / `info`

`orchestratorListCmd` — add `AGENT` and `MODEL` columns.
`orchestratorInfoCmd` — print the orchestrator's agent/model in its header line.

### 4. `orch` alias

`orchestratorCmd()` — `Aliases: []string{"orch"}` so `jeff orch ...` works.

## Non-goals / preserve

- Do **not** change worker (`crew start`) behavior.
- Do **not** break the legacy `StartOrchestrator(store, home, agent, name)` wrapper
  (`lifecycle.go:124`) — keep it compiling for existing SDK callers (it can pass an empty
  model).
- Persona is out of scope for orchestrators.

## Verification

1. `go build ./... && go test ./... && go vet ./...`
2. Unit: orchestrator start resolution table (agent explicit, model auto-route,
   conflict error, invalid model error) — mirror `crew_start_test.go`.
3. Migration test: an old `orchestrators` row (no agent/model cols) opens and scans clean.
4. Manual E2E for each backend:
   - `jeff orch start --name t-oc --agent opencode --model opencode/deepseek-v4-flash-free`
   - Confirm the pane launches `opencode --auto --model opencode/deepseek-v4-flash-free`.
   - `jeff orch list` shows agent=opencode, model set.
   - Start a worker under it; confirm the worker's stop-ping reaches the opencode
     orchestrator pane, and that the orchestrator's `orchestrator-inbox` hook surfaces the
     worker message (this is the one runtime path that differs by agent — verify it fires).
   - Repeat sanity check with `--agent gemini` (model optional) and default claude.

## Risk notes

- The only agent-specific orchestrator runtime path is **inbox/notification surfacing**
  via hooks. Claude uses `settings.json` PostToolUse; opencode uses the `tool.execute.after`
  plugin block; gemini uses its own hook delivery. All three are generated at home init,
  but only claude has been exercised as an orchestrator in production — the opencode and
  gemini orchestrator inbox paths are the primary thing to validate.
