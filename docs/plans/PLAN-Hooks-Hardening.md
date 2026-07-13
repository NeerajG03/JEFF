# PLAN-Hooks-Hardening: Kill Silent Hook Failures, Escape Injection Points, and Stop Script Drift

> **Rank:** 6/10 · **Leverage:** High (hooks deliver memory, crew messaging, and task context — when they fail, they fail invisibly) · **Effort:** Medium · **Prereqs:** none
>
> Ground rules: run `go build ./... && go vet ./... && go test ./...` after each step group. Hook script tests may execute `bash` and `jq` — both are available in CI (ubuntu) and dev machines; guard with `exec.LookPath` + `t.Skip` for portability.

## Why (the problem)

The hooks package generates bash scripts (Claude/Gemini) and a JS plugin (OpenCode) that carry all of JEFF's context injection and crew messaging. Verified defects:

1. **`jq` is an unchecked hard dependency that fails silently.** Every claude/gemini hook script ends with `jq -n --arg ctx ... ` (`hooks/builtin.go:119-159`). With `set -euo pipefail` and no fallback, a machine without `jq` produces empty hook output — the agent silently gets no task context, no memory, no inbox. `jeff doctor` (`cmd/jeff/doctor_cmd.go:37-81`) checks tmux/git/gh/claude/gemini/terminal-notifier but **not `jq`** (and not `opencode`).
2. **Unescaped interpolation.** `TaskID`/`OrchestratorID` are concatenated raw into command position in generated scripts: `jeff crew inbox <id>` (`hooks/builtin.go:415,419`), `jeff crew touch <id>` (`:470`), `jeff crew session-id <id>` (`:538`), `gig show <id>` inside a double-quoted dynamic block (`:227-228`), and `tmux send-keys ... -l "<message with id>"` (`:555-566`). User-configured `CheckpointPatterns` are concatenated into a single-quoted `grep -qE '...'` (`:304-323`) — one apostrophe in a pattern corrupts the script. The memory hooks already do this right via `shellQuote` (`hooks/sessionstart_memory.go:110-112`); the rest don't.
3. **Generated scripts drift forever.** Task hooks are written once at pickup (`cmd/jeff/crew_cmd.go:1232-1243`). `jeff work` and `jeff crew resume` never re-sync, and there is no version marker in the artifacts — a workspace created by an old binary keeps stale scripts indefinitely (`hooks/sessionstart_memory.go:5-6` even documents the workaround for exactly one of the 16 hooks).
4. **`settings.json` handling is brittle.** A single JSONC comment in the user's `.claude/settings.json` makes every install/uninstall fail (`hooks/claude.go:62-76` strict `json.Unmarshal`), and `blockContainsScript` matches by substring (`hooks/claude.go:184`) so a hook whose filename is a suffix of another would cross-match on uninstall.
5. **~13 of 16 hooks register byte-identical claude and gemini closures** (e.g. `hooks/builtin.go:36-44`, `57-67`, `223-242`; all three memory hooks) — pure duplication that doubles the maintenance surface.

## What (the goal)

- Hook wrappers degrade loudly-but-safely without `jq`; doctor checks `jq` and `opencode`.
- Every interpolated value is shell-quoted; adversarial IDs/patterns produce valid scripts.
- Scripts carry a version stamp; `jeff work`/`crew resume` re-sync stale task hooks automatically; `jeff config hooks sync --tasks` exists for bulk refresh.
- `settings.json` failures are actionable; script matching is exact.
- One registration helper eliminates the claude/gemini copy-paste.

## Files to touch

| File | Change |
|---|---|
| `hooks/builtin.go` | `bashBoth()` helper; escape all interpolations; jq guard in wrappers |
| `hooks/hook.go` | `shellQuote` moves here (exported package-privately) if not already shared |
| `hooks/sessionstart_memory.go`, `sessionend_memory.go`, `memory_propose_nudge.go` | Use the shared helper/quoting |
| `hooks/delivery_claude.go` | Version-stamp header in `installClaudeScript` |
| `hooks/opencode.go` | Version stamp in plugin header |
| `hooks/manager.go` | `Sync` rewrites scripts whose stamp ≠ current version (it already overwrites unconditionally — keep, stamp enables *detection*) |
| `hooks/claude.go` | JSONC-actionable error; exact basename match in `blockContainsScript` |
| `cmd/jeff/doctor_cmd.go` | Add `jq` (required) + `opencode` (optional) checks |
| `cmd/jeff/config_cmd.go` | `jeff config hooks sync --tasks` flag walking `tasks/` |
| `cmd/jeff/work_cmd.go`, `cmd/jeff/crew_cmd.go` (resume) | Re-run task-hook sync before launch |
| `hooks/builtin_test.go` | Execution tests under bash+jq with adversarial inputs |
| `hooks/CLAUDE.md` | Update counts, the "3 steps" claim, and the delivery matrix |

## Implementation steps

### Step 1 — shared quoting + the both-keys helper

1. Move/expose the memory hooks' `shellQuote` (single-quote wrapping with `'"'"'` escaping — see `hooks/sessionstart_memory.go:110-112`) into `hooks/hook.go` as `shellQuote(s string) string` and delete local copies.
2. Add a registration helper:

```go
// bashBoth returns a Scripts map registering the same bash generator for
// the claude and gemini deliveries (their script bodies are identical today;
// the gemini delivery remaps event names and timeout units at install time).
func bashBoth(fn func(HookContext) string) map[string]func(HookContext) string {
	return map[string]func(HookContext) string{"claude": fn, "gemini": fn}
}
```

3. Refactor every hook constructor whose claude/gemini closures are identical to use `bashBoth` (13 of 16 — all except any that genuinely differ; diff each pair before collapsing). Hooks that also have an `"opencode"` key keep it: `m := bashBoth(fn); m["opencode"] = js; return m` pattern — add an optional variadic or just assign after.

### Step 2 — escape every interpolation

In `hooks/builtin.go`, for each site below, route the value through `shellQuote` (command-argument position) or `jq --arg` (JSON-value position):

| Site | Fix |
|---|---|
| `buildInboxCheckScript` (~:415, :419) | `jeff crew inbox `+shellQuote(taskID) |
| `buildWorkerHeartbeatScript` (~:470) | shellQuote |
| `buildSessionCaptureScript` (~:538) | shellQuote (the `session_id` from stdin is already jq-extracted — leave it) |
| `buildWorkerStopScript` (~:555-566) | shellQuote both the tmux target and the `-l` literal message |
| `taskContextHook` dynamic block (~:227-228) | `gig show `+shellQuote(taskID) |
| `buildCheckpointNudgeScript` (~:304-323) | Build the ERE in Go, then embed via shellQuote; additionally escape regex-relevant single quotes: the pattern string itself passes through shellQuote so any quoting is safe; document that patterns are EREs |
| `buildOrchestratorInboxScript` (~:596-634) | shellQuote the orchestrator ID wherever concatenated |

Rule to apply mechanically: **any `"..." + ctx.X + "..."` inside a generated script body must become `"..." + shellQuote(ctx.X) + "..."`** unless the value is already inside a quoted heredoc.

### Step 3 — jq guard + doctor entries

1. In `claudeSessionStartStatic` and `claudeSessionStartDynamic` (`hooks/builtin.go:119-159`), prepend after the shebang/set-lines:

```bash
if ! command -v jq >/dev/null 2>&1; then
  echo '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"[jeff] jq not installed - hooks degraded. Run: jeff doctor"}}'
  exit 0
fi
```

(For PostToolUse/Stop wrapper generators, emit `{}` instead — check each wrapper's expected output shape; the Stop-decision hook `memory-propose-nudge` must emit `{}` so it never blocks without jq.)

2. `cmd/jeff/doctor_cmd.go`: add to the dependency table — `jq` required ("hook output formatting — hooks silently no-op without it"), `opencode` optional (only when it's the configured agent or has registered sessions; simplest: optional always, like `gemini` is handled today at `:75-80`).

### Step 4 — version stamps + re-sync

1. Define `const ScriptVersion = "2"` in `hooks/hook.go` (bump whenever generated content changes materially).
2. `installClaudeScript` (`hooks/delivery_claude.go:41-63`): write `# jeff-hook-version: <ScriptVersion>` as line 2 of every script. `syncOpenCode` (`hooks/opencode.go:17-43`): include the same marker in the plugin header comment.
3. Add `hooks.TaskHooksStale(dir string) bool` — reads any one generated script under `<dir>/hooks/*.sh`, returns true if the marker is missing or ≠ `ScriptVersion` (missing dir → false).
4. Wire re-sync (cheap, idempotent — `Manager.Sync` already overwrites):
   - `cmd/jeff/work_cmd.go`: before `launchAgent`, run the same task-hook sync block pickup uses (extract that block from `pickupTask`/`task.Pickup` into a helper `syncTaskHooks(cfg, taskDir, taskID, persona, repos)` so both call it — coordinate with PLAN-Pickup-Rollback which moves this code; if that plan landed, the helper lives in the `task` package).
   - `crew resume` (`cmd/jeff/crew_cmd.go:242-329`): same call before restarting the agent, using the session's stored persona/repos from the crew DB.
5. `jeff config hooks sync --tasks` (`cmd/jeff/config_cmd.go:170-178`): with the flag, after syncing home hooks, walk `<home>/tasks/*/` and re-sync each (persona/repos re-derived via the existing detection helpers). Print one line per task dir synced.

### Step 5 — settings.json robustness

In `hooks/claude.go`:
1. `readSettingsFile` (~:62-76): on `json.Unmarshal` failure, return `fmt.Errorf("parse %s: %w — the file may contain comments or trailing commas (JSONC); jeff needs plain JSON here", path, err)`.
2. `blockContainsScript` (~:172-189): compare the *basename* of the command's script path against the exact expected filename (`filepath.Base` on the token containing the name; the command format is known — it's the path jeff wrote), replacing `strings.Contains`.
3. Add a test that `name.sh` does not match `other-name.sh`-style suffix collisions (e.g. `context.sh` vs `task-context.sh`).

### Step 6 — execution tests

Extend `hooks/builtin_test.go` following the pattern of `hooks/memory_propose_nudge_test.go:67-140` (which already runs bash+jq):

1. For each generated claude script of: `task-context`, `inbox-check`, `worker-heartbeat`, `session-capture`, `checkpoint-nudge`, `gig-ready-tasks` — run under `bash` with a PATH shim so `jeff`/`gig` are stub scripts, feed representative stdin JSON, and assert stdout parses as JSON (`json.Valid`).
2. Adversarial cases: `TaskID = "gig-x'; rm -rf $HOME; echo '"` and `CheckpointPatterns = []string{"don't", `a"b`}` — scripts must still be valid bash (assert `bash -n` passes) and stdout must be valid JSON.
3. jq-absent case: run one script with a PATH lacking `jq` → exit 0 and the degraded JSON from Step 3.
4. Guard all with `if _, err := exec.LookPath("bash"); err != nil { t.Skip }` (same for jq where needed).

### Step 7 — docs

`hooks/CLAUDE.md`: correct hook count (16), document `bashBoth`, the version stamp, re-sync paths, and update the "adding a hook" steps to the true list (constructor, `builtinHooks()` registration, `registry_test.go` expected-names slice, `HookContext` fields if new, gemini event map if new event). Fix root `CLAUDE.md`'s "13 built-in hooks" mention if present (it's in `hooks/` row of the package table).

## Edge cases you must handle (found during exploration)

- **The gemini delivery reuses claude script bodies wholesale** (`hooks/delivery_gemini.go:32-43`) — scripts emit `hookEventName: "PostToolUse"` etc. even when installed under gemini's `AfterTool` key. This plan does NOT attempt to fix the gemini protocol mismatch (unverified against the Gemini CLI); it only centralizes registration so a later fix is one place. Note this explicitly in `bashBoth`'s comment.
- **OpenCode has no PostToolUse/Stop mechanism** (`hooks/opencode.go:55` — plugin only handles `session.created`), so 9 hooks legitimately have no opencode key. Don't add opencode keys as part of the dedup.
- **`registry_test.go:84` hard-codes the 16 expected hook names** — any registration refactor must keep names identical or the test fails (good — it's the guard).
- **`Manager.Sync` intentionally overwrites scripts every run** (`hooks/manager.go:53-59`) and intentionally preserves unknown/user hooks (`manager.go:63`, `TestSyncPreservesUnknownHooks`) — the version stamp is for *detection/reporting*, not to change either behavior.
- **The heredoc terminator:** static content containing a line exactly `HEREDOC` would break `claudeSessionStartStatic` (`builtin.go:125-127`). All static content is compile-time constants today; add a defensive comment, not code.
- **`shellQuote` on the tmux `-l` literal:** `worker-stop` sends a human-readable message; quoting must wrap the whole `-l` argument, and the two-send-keys structure (message, then Enter) is a gig-4040/gig-33ab regression fix — preserve the two-call structure exactly (`hooks/builtin.go:558-566`, tests at `hooks/claude_test.go:126-192`).
- **Checkpoint patterns are user-controlled config** (`cfg.CheckpointPatterns` → `crew_cmd.go:1226`) — treat as hostile input in tests.
- **Stub `jeff`/`gig` in PATH for execution tests** must be executable files (`0o755`) in `t.TempDir()`; set `PATH` via `t.Setenv` prepending the stub dir.
- **`jeff work` re-sync needs persona/repos** — before PLAN-Phase1-Attrs-Resume lands, derive them via `detectPersona`/`detectRepos` (both exist in `cmd/jeff/pickup_cmd.go`); after it lands, prefer `resolveTaskPersona`.

## Acceptance criteria

1. `go build ./... && go vet ./... && go test ./...` green.
2. `go test ./hooks/ -run 'TestScriptExecution|TestAdversarial' -v` — new execution tests pass (and skip gracefully where bash/jq absent).
3. Grep checks:
   - `grep -c "bashBoth(" hooks/builtin.go` ≥ 10.
   - `grep -n "jeff-hook-version" hooks/delivery_claude.go hooks/opencode.go` — both stamp.
   - `grep -n '"jq"' cmd/jeff/doctor_cmd.go` — doctor checks jq.
   - `grep -n 'ctx.TaskID +' hooks/builtin.go` — zero raw concatenations remain outside `shellQuote(...)`.
4. `jeff config hooks sync --tasks` compiles and its RunE walks `tasks/` (unit-test the walk with a fixture home containing two task dirs).
5. `go test ./hooks/ -run TestBlockContainsScript -v` — suffix-collision test passes.
6. Running any generated SessionStart script on a PATH without `jq` exits 0 and emits parseable JSON (covered by Step 6.3 test).

## Out of scope

- Making gemini scripts protocol-native (needs verification against the actual Gemini CLI hook schema first).
- OpenCode PostToolUse/Stop support (blocked on opencode's plugin API).
- JSONC parsing support (actionable error only; a `hujson` dependency is a product decision).
- Unifying `hooks/claude.go` settings I/O with `memory/suppress.go`'s (worthwhile, but touches the memory plan's files — do it after both plans land).
