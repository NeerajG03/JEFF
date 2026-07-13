# PLAN-Pickup-Rollback: Make Pickup Atomic (Un-claim on Failure) and Move It Out of the CLI Layer

> **Rank:** 5/10 · **Leverage:** High (a half-failed pickup wedges the task today; the 185-line orchestration is untestable where it lives) · **Effort:** Medium-Large · **Prereqs:** none (merge before or after other plans; coordinate with PLAN-Phase1-Attrs-Resume if both touch `pickupTask` — do that plan first, it's smaller)
>
> Ground rules: run `go build ./... && go vet ./... && go test ./...` after each step group.

## Why (the problem)

1. **No rollback, no resume.** `pickupTask` (`cmd/jeff/crew_cmd.go:1093-1278`) claims the task in gig (step: `store.Claim`, ~line 1111) and then performs ~10 more steps, several of which hard-fail (`workspace.Create` ~1120, `SetAttr` ~1130, `writeTaskClaudeMD` ~1184). If any of those fail, the task is left `in_progress`/assigned with a missing or half-built workspace. Re-running `jeff pickup` calls `Claim` again, which errors on an already-claimed task — the user is wedged between "claimed" and "no workspace" with no un-claim command.
2. **The core workflow lives in the CLI package.** `cmd/jeff/CLAUDE.md:27` says "No business logic in cmd/ — put it in a package with tests," yet the entire pickup orchestration sits in `crew_cmd.go` (a 1405-line file), and the teardown mirror lives inline in `done_cmd.go:23-128`. Neither has an end-to-end test.
3. **Worktree-symlink discovery is quadruplicated.** `listWorktreeSymlinks` (`cmd/jeff/pickup_cmd.go:292-314`), `discoverWorktrees` (`cmd/jeff/ship_cmd.go:149-197`), `discoverWorktreeStatus` (`cmd/jeff/status_cmd.go:170-195`), plus an inline variant in `done_cmd.go:56-60` — four copies of the same `ReadDir → IsSymlink → Readlink` scan.

## What (the goal)

- A new `task` package owning `task.Pickup(...)` and `task.Teardown(...)` with tests.
- Pickup is **idempotent and self-healing**: re-running after a partial failure resumes cleanly; a hard failure after claim un-claims the task and removes the partial workspace.
- One `workspace.ListTaskWorktrees(taskDir)` replaces all four scanners.
- `cmd/jeff` shrinks back toward flag-parsing + printing.

## Files to touch

| File | Change |
|---|---|
| `task/pickup.go` | **New** — `Pickup(store, cfg, opts)` moved from `crew_cmd.go:1093-1278` |
| `task/teardown.go` | **New** — teardown orchestration moved from `done_cmd.go` |
| `task/pickup_test.go`, `task/teardown_test.go` | **New** — integration tests with temp gig store + local git fixture |
| `workspace/worktree.go` | **New** `ListTaskWorktrees(taskDir) ([]TaskWorktree, error)` |
| `workspace/worktree_test.go` | Tests for it |
| `cmd/jeff/crew_cmd.go` | Delete `pickupTask`; call `task.Pickup`; keep only tmux/CLI glue |
| `cmd/jeff/pickup_cmd.go` | Call `task.Pickup`; move `writeTaskClaudeMD` + helpers into `task/claudemd.go` (exported as needed) |
| `cmd/jeff/done_cmd.go` | Call `task.Teardown` |
| `cmd/jeff/ship_cmd.go`, `cmd/jeff/status_cmd.go` | Use `workspace.ListTaskWorktrees` |
| `cmd/jeff/pickup_test.go` | Move relevant tests to `task/`, keep CLI-glue tests |

## Implementation steps

### Step 0 — pre-flight: map the dependencies

`pickupTask` currently uses: `gig`, root `jeff` (attrs/config/providers), `workspace`, `memory`, `persona`, `hooks`, `skill`, `embed`, `internal/gitutil`. A new `task` package may import all of these. **Verify there is no reverse edge** before starting: `grep -rn '"github.com/NeerajG03/JEFF/task"' --include='*.go' .` must be empty in all of those packages after creation (nothing but `cmd/jeff` may import `task`).

### Step 1 — `workspace.ListTaskWorktrees`

In `workspace/worktree.go`:

```go
// TaskWorktree describes a repo worktree symlinked into a task directory.
type TaskWorktree struct {
	Repo   string // symlink name in the task dir
	Path   string // resolved worktree path
	Branch string // git branch (from `git rev-parse --abbrev-ref HEAD`); "" if undetectable
	Base   string // contents of .jeff-base; "" if absent
}

func ListTaskWorktrees(taskDir string) ([]TaskWorktree, error)
```

Behavior: `os.ReadDir(taskDir)`; for each entry where `gitutil.IsSymlink`, `os.Readlink`; resolve `Branch` via git (tolerate failure → `""`), `Base` via `ReadBaseBranch`. Skip unreadable links silently (matches all four current implementations).

Port the four call sites. Each keeps its own filtering/presentation:
- `pickup_cmd.go` `listWorktreeSymlinks` → delete; `writeWorkspaceLayout`/`detectRepos` consume the new type (`Branch` from the symlink target's basename is now replaced by the *real* branch — this **fixes** the slash-in-branch-name display bug noted in `ship_cmd.go:237`'s comment).
- `ship_cmd.go` `discoverWorktrees` → thin adapter mapping `TaskWorktree` → `shipWorktree` (keep the `origin/` stripping + repo filter where they are).
- `status_cmd.go` `discoverWorktreeStatus` → adapter adding its `isGitDirty` probe.
- `done_cmd.go` inline readlink → use the resolved `Path`.

### Step 2 — create `task.Pickup`

Move `pickupTask` verbatim into `task/pickup.go` first (mechanical move, compile, test), exposing:

```go
type PickupOpts struct {
	TaskID        string
	Persona       string
	Repos         []string
	ReposReadonly []string
	Prompt        string   // used by crew start
	AgentOverride jeff.AgentTool
}

type PickupResult struct {
	TaskDir string
	Claimed bool // whether this call performed the claim (vs resumed)
}

func Pickup(store *gig.Store, cfg *jeff.Config, opts PickupOpts) (*PickupResult, error)
```

The current function reads the package-global `cfg` from `cmd/jeff/main.go` — pass it explicitly. Same for anything reading `cfg.Home`.

`writeTaskClaudeMD`, `writeScratchpadGuide`, `writeWorkspaceLayout`, `appendReadonlyNote`, `detectPersona`, `detectRepos`, `resolveRepoBranch`, `buildTaskJSON` move to `task/claudemd.go` (they are pickup's private helpers; `refreshTaskClaudeMD` becomes `task.RefreshClaudeMD(store, cfg, taskID, taskDir)` since `worktree_cmd.go` calls it too). Move their tests from `cmd/jeff/pickup_test.go` into `task/` (mostly package-rename mechanical work).

### Step 3 — idempotent resume

At the top of `Pickup`, before claiming:

```go
t, err := store.Get(opts.TaskID)
if err != nil { return nil, fmt.Errorf("get task: %w", err) }

resuming := false
if t.Status == gig.StatusInProgress {
	if td, err := workspace.Open(cfg.Home, opts.TaskID); err == nil {
		// Task already claimed AND workspace exists → resume: skip Claim,
		// re-run the idempotent setup steps against td.Path.
		resuming = true
		_ = td
	} else {
		return nil, fmt.Errorf(
			"task %s is in_progress (assignee %q) but has no workspace — a previous pickup failed.\n"+
			"Run 'jeff pickup %s' again after releasing it, or it will be auto-released now.",
			opts.TaskID, t.Assignee, opts.TaskID)
		// see Step 4: instead of erroring, fall through to re-claim after release —
		// choose the auto-release path, keep this comment block out of final code.
	}
}
```

Final semantics (implement exactly this):
- `in_progress` + workspace exists → **resume**: skip `Claim`, run everything after it (all later steps are already idempotent: `workspace.Create` returns the existing dir? — **verify**: `workspace.Create` (`workspace/workspace.go:21-34`) may error on existing dir; if so call `workspace.Open` in the resume path instead).
- `in_progress` + **no** workspace → **self-heal**: log a notice, proceed directly to workspace creation *without* re-claiming (the claim already holds). `Claimed=false`.
- any terminal status (`closed`/`cancelled`) → error out clearly (`task %s is %s — reopen it first (gig reopen)`).
- `open`/`blocked`/`deferred` → claim as today.

### Step 4 — rollback on hard failure

Wrap the post-claim hard-fail steps. Track `claimedHere := true` when this invocation performed the `Claim`. On any subsequent hard error:

```go
func (rollback) { // only when claimedHere
	if err := store.UpdateStatus(opts.TaskID, gig.StatusOpen, "jeff"); err != nil { warn }
	empty := ""
	if _, err := store.Update(opts.TaskID, gig.UpdateParams{Assignee: &empty}, "jeff"); err != nil { warn }
	if rmErr := workspace.Remove(cfg.Home, opts.TaskID); rmErr != nil { warn }
}
```

- gig v0.6.2 has `UpdateStatus(id, status, actor)` and `Update(id, UpdateParams{Assignee *string}, actor)` (verified — `task.go:212`, `gig.go:264`); there is no `Unclaim`, so this pair is the un-claim.
- Implement with a named `rollbackPickup(store, cfg, taskID)` function called from each hard-fail return path (or a deferred closure guarded by a `success bool`) — prefer the deferred-guard pattern:

```go
success := false
defer func() {
	if !success && claimedHere {
		rollbackPickup(store, cfg.Home, opts.TaskID)
	}
}()
...
success = true
return &PickupResult{...}, nil
```

- Do **not** remove created *worktrees* on rollback — they live under `worktrees/<repo>/<branch>`, are expensive to recreate, and are reused by the next attempt (idempotent `WorktreeAdd` short-circuits on the existing dir).

### Step 5 — `task.Teardown`

Move the body of `done_cmd.go` RunE (steps 0-5: orchestrator signal, worktree removal, auto-curate, workspace removal, close, crew cleanup) into:

```go
type TeardownOpts struct {
	TaskID string
	Reason string
}

func Teardown(store *gig.Store, cfg *jeff.Config, opts TeardownOpts) error
```

Keep the exact best-effort semantics (warn-and-continue for everything except `CloseTask`). `done_cmd.go` becomes ~30 lines. (If PLAN-Phase1-Attrs-Resume landed, the outcome-attr write and `resolveTaskPersona` move along with it.)

### Step 6 — CLI rewiring

- `pickup_cmd.go` RunE: `res, err := task.Pickup(store, cfg, task.PickupOpts{...})` — note it must now open the gig store itself (previously `pickupTask` did); reuse `openGigStore()` and pass the handle in, closing it before `launchAgent` blocks on the interactive session.
- `crew_cmd.go` crew-start path: same call; the JSON it prints (`crew_cmd.go:197`) sources from `PickupResult`.
- Delete `pickupTask` from `crew_cmd.go`.

### Step 7 — tests

`task/pickup_test.go` (integration-style, no network):

1. Fixture: `t.TempDir()` JEFF_HOME; local bare repo + clone under `repos/<name>` (copy the git fixture pattern from `cmd/jeff/ship_test.go`); temp gig store (`gig.Open` pattern from existing tests) with one open task.
2. **Happy path:** `Pickup` → task in_progress, workspace exists, CLAUDE.md exists, worktree symlink resolves, `Claimed=true`.
3. **Idempotent resume:** call `Pickup` again → no error, `Claimed=false`, workspace intact.
4. **Rollback:** force a hard failure after claim (easiest deterministic lever: pre-create a *file* — not dir — at the workspace target path so `workspace.Create` fails) → task back to `open`, assignee empty, no workspace dir.
5. **Self-heal:** claim the task manually via `store.Claim`, no workspace → `Pickup` succeeds without double-claim error.
6. **Terminal status:** closed task → error mentions `reopen`.
7. `task/teardown_test.go`: happy path closes the task and removes the workspace; missing workspace still closes the task (today's behavior — `resolveTaskID` tolerates it).

## Edge cases you must handle (found during exploration)

- **`workspace.Create` vs existing dir:** check its real behavior before writing the resume path (`workspace/workspace.go:21-34`). If it errors on an existing directory, the resume path must use `workspace.Open`; if it silently reuses, document that in the function comment.
- **`store.Claim` auto-progresses parents** (gig `ClaimResult.ParentProgressed`) — on rollback, do **not** try to un-progress the parent: another sibling may legitimately be in progress. Note this in a comment; it is accepted residue.
- **`crew start` calls pickup with `--repos-readonly` and a prompt** — those flow through `PickupOpts`; keep the readonly-note append (`appendReadonlyNote`) inside `task.Pickup`, not in the CLI.
- **The gig store cannot be open twice concurrently by the same process safely** (SQLite) — pass the one open handle into `Pickup`; don't open a second inside.
- **`launchAgent` blocks for the whole interactive session** (`pickup_cmd.go:47-50`); close the gig store **before** launching, or the DB stays locked for hours. Today `pickupTask` opens and closes its own store before launch — preserve that ordering (open → Pickup → Close → launch).
- **Best-effort vs hard-fail step classification must not change** in the mechanical move: hard-fail = store open, EnsureAttrs, Get, Claim, workspace.Create, SetAttr(repos), writeTaskClaudeMD; everything else warns and continues (`crew_cmd.go:1136-1275`). Rollback triggers only on the hard-fail ones after claim.
- **`detectPersona` string-matching moves along unchanged** — its replacement is PLAN-Phase1-Attrs-Resume's `resolveTaskPersona`; don't entangle the two changes.
- **Import cycle guard:** nothing under `workspace/`, `memory/`, `hooks/`, `skill/`, `persona/` may import `task`. Only `cmd/jeff` does.
- **`cmd/jeff/pickup_test.go` has ~800 lines of tests for the helpers you're moving** — move the tests with the code; do not leave dead copies in both packages (duplicate test names across packages are legal but confusing; delete the originals).

## Acceptance criteria

1. `go build ./... && go vet ./... && go test ./...` green.
2. `go test ./task/ -v` — the six scenarios from Step 7 pass.
3. `wc -l cmd/jeff/crew_cmd.go` — at least 150 lines smaller than before (pickupTask moved out).
4. `grep -c "func pickupTask" cmd/jeff/crew_cmd.go` → 0; `grep -c "task.Pickup(" cmd/jeff/*.go` ≥ 2 (pickup + crew start).
5. `grep -rn "ReadDir" cmd/jeff/ship_cmd.go cmd/jeff/status_cmd.go cmd/jeff/pickup_cmd.go` — no per-file symlink scans remain (all via `workspace.ListTaskWorktrees`).
6. Manual wedge-recovery scenario (as a Go test, Step 7.4/7.5): a task stuck `in_progress` without a workspace is recoverable by plain `jeff pickup <id>` with no gig surgery.

## Out of scope

- Splitting the rest of `crew_cmd.go` into multiple files (nice-to-have; do not block on it).
- Moving ship logic into a package (see PLAN-Ship-Hardening; it stays in `cmd/jeff` for now).
- A `jeff abort`/`jeff release` command (self-heal in pickup covers the wedge; an explicit command can come later).
- Any behavior change to hooks/skills/memory steps inside pickup beyond relocation.
