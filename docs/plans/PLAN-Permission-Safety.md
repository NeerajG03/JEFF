# PLAN-Permission-Safety: Configurable Agent Permission Modes + Stop Destroying Uncommitted Work

> **Rank:** 7/10 · **Leverage:** High trust-per-line-changed (small diff, controls the blast radius the roadmap keeps promising) · **Effort:** Small · **Prereqs:** none
>
> Ground rules: run `go build ./... && go vet ./... && go test ./...` after each step group.

## Why (the problem)

1. **Every agent JEFF launches runs with permissions disabled, unconditionally.** The Claude provider hardcodes `--dangerously-skip-permissions` (`agent_claude.go:29,43`) and the Gemini provider hardcodes `--approval-mode=yolo` (`agent_gemini.go:39,52`) into both interactive and curate launches. There is no config to turn this off — a user who wants Claude Code's permission prompts (e.g. on a repo with production credentials in env) cannot have them without abandoning JEFF. The roadmap's trust story ("The user controls the blast radius", `docs/roadmap.md:100,603`) has no mechanism behind it.
2. **`jeff done` silently destroys uncommitted work.** `worktreeRemoveDir` (`workspace/worktree.go:148-173`) tries `git worktree remove` and, when that fails — which is exactly what git does for a *dirty* worktree — falls back to `os.RemoveAll(wtDir)` with no warning and no confirmation. An agent (or user) running `jeff done` on a task with unshipped edits loses them irrecoverably.
3. **Removed worktrees leave dangling git metadata.** After the `os.RemoveAll` fallback, `.git/worktrees/<name>` records linger (no `git worktree prune` anywhere in the repo), which can make the next `WorktreeAdd` at the same path fail with "already registered".

## What (the goal)

- A `permissions` config knob (default: current behavior) plus a per-invocation `--safe` flag that launches agents **with** their native permission prompts.
- Dirty worktrees are never deleted silently: `jeff done` refuses with a listing unless `--force`.
- `git worktree prune` runs after any worktree removal.

## Files to touch

| File | Change |
|---|---|
| `agent.go` | `LaunchOpts.SkipPermissions bool` |
| `agent_claude.go` | Gate `--dangerously-skip-permissions` on the opt |
| `agent_gemini.go` | Gate `--approval-mode=yolo` on the opt |
| `agent_opencode.go` | Inspect `BuildLaunchArgs` — gate any equivalent flag the same way |
| `config.go` | `Config.SkipPermissions *bool` (`json:"skip_permissions,omitempty"`) |
| `cmd/jeff/launch.go` | Resolve effective mode; pass through |
| `cmd/jeff/pickup_cmd.go`, `cmd/jeff/work_cmd.go` | `--safe` flag |
| `cmd/jeff/crew_cmd.go` | `--safe` on `crew start`; thread into `buildAgentCmd` path (`crew/crew.go:84-100` / `crew/lifecycle.go`) |
| `crew/crew.go` / `crew/lifecycle.go` | Persist per-session skip-permissions? **No** — see edge cases; resolve at launch time only |
| `workspace/worktree.go` | Dirty detection + `ErrWorktreeDirty`; `git worktree prune` after removal |
| `cmd/jeff/done_cmd.go` | `--force` flag; refuse-and-list on dirty |
| `cmd/jeff/worktree_cmd.go` | `worktree rm` gains `--force`, same semantics |
| `schemas/jeff-config.json` | Add `skip_permissions` boolean |
| `docs/config.md`, `README.md` | Document the knob and `--safe` |
| tests | `agent_test.go`, `workspace/worktree_test.go`, `config_test.go` |

## Implementation steps

### Step 1 — plumb the option through providers

1. `agent.go`: add `SkipPermissions bool` to `LaunchOpts` (`agent.go:9-13`).
2. `agent_claude.go` `BuildLaunchArgs`:

```go
args := []string{}
if opts.SkipPermissions {
	args = append(args, "--dangerously-skip-permissions")
}
```

Same for `BuildCurateArgs` — give it the flag too. **Signature change:** `BuildCurateArgs(prompt string)` has no opts; extend the interface method to `BuildCurateArgs(prompt string, opts LaunchOpts) []string` and update all three providers plus call sites (`cmd/jeff/done_cmd.go:94`, `cmd/jeff/memory/curate_cmd.go` — grep `BuildCurateArgs`). For curate (non-interactive, piped) the *default* must remain skip=true regardless of config — a headless curator cannot answer prompts; hardcode `SkipPermissions: true` at curate call sites and say so in a comment.

3. `agent_gemini.go`: gate `--approval-mode=yolo` identically (interactive path only; curate keeps yolo per above).
4. `agent_opencode.go`: read its current `BuildLaunchArgs`; if it passes no permission flag, nothing to gate — leave a comment.

### Step 2 — config + resolution

1. `config.go`: add `SkipPermissions *bool` `json:"skip_permissions,omitempty"` to `Config`. Pointer so "unset" (nil → default true) is distinguishable from explicit `false`.
2. Resolution helper in `cmd/jeff` (e.g. in `launch.go`):

```go
// effectiveSkipPermissions resolves the launch permission mode:
// --safe flag > jeff.json skip_permissions > default (true, current behavior).
func effectiveSkipPermissions(cfg *jeff.Config, safeFlag bool) bool {
	if safeFlag {
		return false
	}
	if cfg.SkipPermissions != nil {
		return *cfg.SkipPermissions
	}
	return true
}
```

3. `launchAgent(dir, agent, model)` (`cmd/jeff/launch.go:13-36`) gains a `skip bool` parameter → `LaunchOpts{Model: model, SkipPermissions: skip}`. Update all callers (`pickup_cmd.go:50`, `work_cmd.go:25`, `main.go:47-52` bare-`jeff` launch, `open_cmd.go` if it launches — grep `launchAgent(`).

### Step 3 — flags

- `jeff pickup`, `jeff work`, `jeff crew start`: add `--safe` (`Launch the agent with its permission prompts enabled (overrides skip_permissions)`).
- Crew: the worker launch command is built in `crew/lifecycle.go` via the provider (`buildAgentCmd` legacy fallback at `crew/crew.go:84-100` — grep which path is live; the provider path at `crew_cmd.go:158-183` builds args via `BuildLaunchArgs`). Thread a `SkipPermissions` field through `crew.StartOpts` (or whatever the start-params struct is named — locate `StartWorkerForOrchestrator`'s options) so the flag reaches `BuildLaunchArgs`. Do **not** persist it in the sessions table — a resumed worker re-resolves from current config/flag (least-surprise: safety posture follows *current* config, not historical).

### Step 4 — dirty-worktree protection

In `workspace/worktree.go`:

1. Add:

```go
var ErrWorktreeDirty = errors.New("worktree has uncommitted changes")

// dirtyPaths returns up to max uncommitted paths in the worktree ("" clean).
func dirtyPaths(wtDir string, max int) []string {
	out, err := gitutil.Output(wtDir, "status", "--porcelain")
	if err != nil || len(bytes.TrimSpace(out)) == 0 {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > max {
		lines = lines[:max]
	}
	return lines
}
```

2. `worktreeRemoveDir(...)` gains a `force bool` parameter: when `!force` and `dirtyPaths` is non-empty, return `fmt.Errorf("%w:\n  %s\n(commit/ship first, or pass --force to discard)", ErrWorktreeDirty, strings.Join(paths, "\n  "))` **before** attempting removal. When forcing, proceed as today (git remove → RemoveAll fallback).
3. After successful removal (both paths), run `gitutil.Run(repoDir, "worktree", "prune")` best-effort — `repoDir` is the main clone under `<home>/repos/<name>`; it is already known to the callers (`WorktreeRemove`/`WorktreeRemoveByPath` compute it — verify at `workspace/worktree.go:136-173`).
4. Propagate `force` up through `WorktreeRemove`/`WorktreeRemoveByPath` signatures.

### Step 5 — CLI wiring for force

- `jeff done` (`cmd/jeff/done_cmd.go`): add `--force`; pass into the worktree-removal calls (~lines 63,70). On `errors.Is(err, workspace.ErrWorktreeDirty)`, print the error and **abort before closing the task** (return the error) — a dirty refuse must not half-teardown: check dirtiness for ALL repos **first**, and only then start removing. Order becomes: resolve worktrees → dirty preflight (unless `--force`) → existing steps.
- `jeff worktree rm` (`cmd/jeff/worktree_cmd.go:80`): add `--force`, same semantics.

### Step 6 — schema + docs + tests

1. `schemas/jeff-config.json`: add `"skip_permissions": {"type":"boolean","description":"Launch agents with permission prompts disabled (default true). Set false to keep native permission prompts; --safe overrides per run."}`.
2. `docs/config.md` + `README.md` Requirements/Configuration sections: document the knob, `--safe`, and the new `done --force` behavior.
3. Tests:
   - `agent_test.go`: `BuildLaunchArgs` with `SkipPermissions:false` contains neither `--dangerously-skip-permissions` nor `--approval-mode=yolo`; with true, both appear per provider (extend the existing args table tests).
   - `config_test.go`: round-trip `skip_permissions: false` through Save/Load (pointer stays non-nil false).
   - `workspace/worktree_test.go`: create worktree fixture → dirty file → remove without force errors with `ErrWorktreeDirty`; with force succeeds; after removal `git worktree list` in the repo no longer shows it (prune ran).
   - `cmd/jeff` test for `effectiveSkipPermissions` precedence (flag > config > default).

## Edge cases you must handle (found during exploration)

- **Curate must stay non-interactive.** `jeff done` auto-curation and `jeff memory curate` pipe a prompt (`-p`) — with prompts enabled they'd hang forever. Curate call sites hardcode skip=true; only *interactive* launches honor the knob.
- **Crew workers run inside tmux with nobody watching.** With `--safe`, a permission prompt stalls the worker until someone attaches. That is the point of safe mode, but `jeff crew start --safe` must print: `worker will pause on permission prompts — attach with: jeff crew attach <id>`.
- **The legacy `buildAgentCmd` fallback** (`crew/crew.go:84-100`) hardcodes `claude --dangerously-skip-permissions` for empty-agent legacy rows — gate it with the same resolved value you thread in (or route it through the provider; minimal change is fine, PLAN-Agent-Providers does the full cleanup).
- **Bare `jeff` launches the agent at JEFF_HOME** (`cmd/jeff/main.go:47-52`) — it has no flags; it uses config-only resolution. Make sure it compiles with the new `launchAgent` signature.
- **`Config.SkipPermissions` must be a pointer.** A plain bool cannot express "unset → default true"; marshaling a false pointer must survive Save/Load (hence the round-trip test).
- **Dirty preflight ordering in `done`:** check every repo before removing any (a mid-loop refuse would leave asymmetric state). The orchestrator-signal step (step 0 in done) is harmless to run before the preflight; keep order: signal → dirty preflight → removals.
- **Submodules/nested repos:** `git status --porcelain` in a worktree with dirty submodules reports them — that counts as dirty; fine, safer default.
- **`git worktree prune` runs in the main repo dir, not the worktree** (which no longer exists). Verify the repo path used is `<home>/repos/<repo>`.
- **Do not** make `--force` also imply `--safe`-anything — unrelated concerns, keep flags orthogonal.

## Acceptance criteria

1. `go build ./... && go vet ./... && go test ./...` green.
2. `go test ./ -run TestBuildLaunchArgs -v` — permission gating per provider asserted both ways.
3. `go test ./workspace/ -run TestWorktreeRemoveDirty -v` — refuse/force/prune behaviors pass.
4. Grep checks:
   - `grep -n "dangerously-skip-permissions" agent_claude.go` — appears only inside the `if opts.SkipPermissions` guard.
   - `grep -n "approval-mode=yolo" agent_gemini.go` — same.
   - `grep -rn '"--safe"' cmd/jeff/ | wc -l` ≥ 3 (pickup, work, crew start).
   - `grep -n "worktree\", \"prune" workspace/worktree.go` — present.
   - `grep -n "skip_permissions" schemas/jeff-config.json docs/config.md` — both documented.
5. Behavior: with `"skip_permissions": false` in a temp-home jeff.json, `effectiveSkipPermissions` test proves the launch args change; `--safe` overrides a `true` config.

## Out of scope

- Per-repo or per-persona permission profiles (config granularity can grow later; the knob + flag establish the mechanism).
- Claude Code `settings.json` `permissions` allowlists (orthogonal to the CLI flag).
- The full provider-abstraction cleanup of `buildAgentCmd` (PLAN-Agent-Providers).
- An interactive confirmation prompt in `jeff done` (flag-based refuse is automation-friendly; prompts block agents).
