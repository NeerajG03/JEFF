# PLAN-Crew-Reliability: Fix the Crew Store's SQLite Footguns and Message Double-Delivery

> **Rank:** 2/10 · **Leverage:** Very high (silent data corruption + duplicated agent input) · **Effort:** Medium · **Prereqs:** none
>
> Ground rules: run `go build ./... && go vet ./... && go test ./...` after each step group. The crew tests use a fake tmux (`crew/tmux_test.go: withFakeTmux`) — no real tmux needed.

## Why (the problem)

Four independently-verified defects in `crew/`:

1. **The SQLite pragmas don't reliably apply.** `crew.Open` (`crew/crew.go:106-130`) runs `PRAGMA busy_timeout=5000` and `PRAGMA foreign_keys=ON` via `db.Exec` on a pooled `database/sql` handle. Those pragmas are **per-connection**; they bind to whichever pooled connection executed them. Every other connection the pool opens later has `busy_timeout=0` (fails instantly with `SQLITE_BUSY` under contention) and `foreign_keys=OFF`. Crew is *multi-process by design* (every `jeff crew …` CLI call, every hook shell-out, the TUI's 2s refresh) so contention is the normal case — and the writes that lose the race are silently discarded because `Refresh` ignores errors (`crew/lifecycle.go:380,386,388,399`).
2. **Every crew message is delivered twice.** `Send` (`crew/lifecycle.go:473-536`) stores **all four** message types as unacked `to_worker` rows AND types the content into the worker's tmux pane. The worker's `inbox-check` PostToolUse hook then fetches pending messages **without filtering by type** (`PendingMessages`, `crew/crew.go:487-498`) and injects them again. The comments claiming otherwise (`crew/crew.go:64-66`, `hooks/builtin.go:378`) are false. Result: a `divert` interrupt is followed by the same text re-injected as context; `status` gets `/btw <content>` typed plus raw `<content>` injected.
3. **Cross-worker paste-buffer race.** `SendCommandViaBuffer` (`crew/tmux.go:99-111`) always uses the fixed tmux buffer name `jeff-send`. Two concurrent sends to different workers can paste each other's payloads.
4. **Schema migration errors are swallowed.** `migrate` discards all `ALTER TABLE` errors (`crew/crew.go:200`: `_, _ = db.Exec(stmt)`) to skip "duplicate column", which also masks disk-full/locked-DB failures; there is no schema versioning.

## What (the goal)

- Pragmas guaranteed on every connection (DSN `_pragma` parameters + `SetMaxOpenConns(1)`).
- `PRAGMA user_version`-based migrations with real error handling.
- Multi-statement operations (`RemoveSession`, `AppendSessionID`) inside transactions.
- One delivery channel per message type: `nudge` → hook-injected only; `status`/`normal`/`divert` → tmux-typed only (stored pre-acked for audit).
- Per-process paste buffer names, deleted after paste.
- `Refresh` surfaces write errors and stops spawning one `tmux` subprocess per worker.

## Files to touch

| File | Change |
|---|---|
| `crew/crew.go` | `Open` DSN + pool limits; versioned `migrate`; `Send`-side ack semantics; transactional `RemoveSession`/`AppendSessionID`; fix false `MessageType` comments |
| `crew/lifecycle.go` | `Send` stores non-nudge messages pre-acked; `Refresh` batches tmux lookups + returns/logs errors |
| `crew/tmux.go` | Unique buffer name; `paste-buffer -d`; platform-aware install hint |
| `crew/crew_test.go`, `crew/lifecycle_test.go` | Update + new tests |
| `crew/cleanup_test.go` | **New** — first tests for `Cleanup`/`IsClean` |
| `hooks/builtin.go` | Fix the false comment on `inbox-check` (~line 378) |
| `crew/CLAUDE.md` | Document the delivery matrix + schema versioning |

## Implementation steps

### Step 1 — connection settings that actually stick

In `crew.Open` (`crew/crew.go`):

```go
dsn := "file:" + dbPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
db, err := sql.Open("sqlite", dsn)
if err != nil { ... }
db.SetMaxOpenConns(1) // single writer; crew load is CLI-scale, serialization is correct here
```

- modernc.org/sqlite parses repeated `_pragma=name(value)` query parameters and applies them to **every** new connection — this is the canonical fix for the pooled-pragma problem.
- Keep the existing three `db.Exec("PRAGMA ...")` calls **deleted** (they're redundant now); keep the error-checked `migrate(db)` call.
- `SetMaxOpenConns(1)` also makes Go-side transactions serialize instead of deadlocking on `SQLITE_BUSY`.

### Step 2 — versioned migrations

Replace `migrate` (`crew/crew.go:142-204`) with:

```go
func migrate(db *sql.DB) error {
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	steps := []func(*sql.DB) error{
		migrateV1, // CREATE TABLE orchestrators / sessions / messages + indexes (current CREATE block verbatim)
		migrateV2, // ALTER TABLE sessions ADD COLUMN model/session_ids/agent (current ALTERs, errors checked)
	}
	for i := v; i < len(steps); i++ {
		if err := steps[i](db); err != nil {
			return fmt.Errorf("migration to v%d: %w", i+1, err)
		}
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			return fmt.Errorf("bump schema version: %w", err)
		}
	}
	return nil
}
```

- `migrateV1` contains the existing `CREATE TABLE IF NOT EXISTS` + `CREATE INDEX IF NOT EXISTS` statements — keeping `IF NOT EXISTS` makes v1 safe for **existing databases whose user_version is 0** but whose tables already exist (every current install).
- `migrateV2` runs each `ALTER TABLE` and **tolerates only** the "duplicate column name" error (match `strings.Contains(err.Error(), "duplicate column")`), returning all other errors. This upgrades existing DBs (columns exist → tolerated) and fresh DBs (columns added) while surfacing real failures.
- Also move the `model` column into `COALESCE` in `scanSession` (`crew/crew.go:279`) like `agent`/`session_ids` already are: `COALESCE(model, '')` — protects against NULLs on oddly-migrated DBs.

### Step 3 — transactions for multi-statement ops

- `RemoveSession` (`crew/crew.go:326-333`): wrap the two DELETEs in `s.db.Begin()` / `tx.Commit()` with `defer tx.Rollback()`.
- `AppendSessionID` (`crew/crew.go:253-274`): perform SELECT + UPDATE inside one transaction so concurrent SessionStart hooks can't lose an ID.

### Step 4 — one delivery channel per message type

Decision (matches user-visible docs in README which describe nudge as "low context impact" hook delivery):

| Type | tmux pane typing | inbox-hook injection |
|---|---|---|
| `nudge` | no | yes |
| `status` | yes (`/btw` sidechain) | no |
| `normal` | yes | no |
| `divert` | yes (interrupt first) | no |

Implementation in `Send` (`crew/lifecycle.go:473-536`):
1. For `nudge`: keep storing the row unacked; **remove** the pane-typing call for nudge.
2. For `status`/`normal`/`divert`: keep the pane-typing; store the row with `acked_at` set to now (audit trail without pending state). Add a store helper:

```go
// AddDeliveredMessage records a message that was already delivered via tmux.
func (s *Store) AddDeliveredMessage(m *Message) error {
	now := time.Now().UTC().Format(timeLayout)
	_, err := s.db.Exec(`INSERT INTO messages (id, task_id, direction, msg_type, content, response, created_at, acked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.TaskID, m.Direction, m.MsgType, m.Content, m.Response, now, now)
	return err
}
```

3. If the tmux typing **fails** for status/normal/divert, fall back to storing the row unacked (so the hook path picks it up) and print a notice — delivery must not be lost.
4. Fix the doc comments: `crew/crew.go:60-91` (`MessageType` docs) and `hooks/builtin.go` (~line 378) to describe this matrix.
5. Do **not** change `PendingMessages`' signature — with non-nudge rows pre-acked, its existing `acked_at IS NULL` filter now yields only nudges for `to_worker`. `PendingCount` (used by the TUI) keeps working unchanged.

### Step 5 — paste-buffer race + platform hint

In `crew/tmux.go`:
- `SendCommandViaBuffer` (~line 99): buffer name `fmt.Sprintf("jeff-send-%d", os.Getpid())`; use `paste-buffer -d -b <name>` so tmux deletes the buffer after pasting (removes both the race window and buffer leaks).
- `EnsureTmux` (~line 20): replace the hardcoded `brew install tmux` hint with a `runtime.GOOS` switch: darwin → `brew install tmux`, linux → `apt/dnf install tmux`, default → "install tmux".

### Step 6 — Refresh: batch + surface errors

Rewrite `Refresh` (`crew/lifecycle.go:372-404`):
1. List windows **once per tmux session** (copy the pattern already used by `Cleanup` at `crew/cleanup.go:40`) instead of calling `HasWindowInSession` per worker — O(sessions) subprocesses instead of O(workers).
2. Collect `TouchSession`/`UpdateStatus` errors into a joined error (`errors.Join`) and return it; callers (`crew_cmd.go:374`, `orchestrator_cmd.go:78`, `tui/tui.go:547`) print it as a warning — check each call site compiles and warns rather than aborting.

### Step 7 — tests

1. **Pragma test:** open a store, spin 8 goroutines × 25 `UpdateStatus`/`TouchSession` writes on the same DB — must complete without `database is locked` errors (with `SetMaxOpenConns(1)` + busy_timeout this is deterministic).
2. **Migration test:** create a DB with the v1 schema and `user_version=0`, call `Open` — succeeds, version becomes 2, columns exist. Corrupt case: pre-create `sessions` with a conflicting column type is out of scope; instead assert a failing migration step propagates its error (inject via a table that already has `model` as the wrong-name column is overkill — assert "duplicate column" tolerance by calling `migrateV2` twice).
3. **Delivery matrix test:** `Send` with each type against the fake tmux; assert nudge rows are pending (`PendingMessages` returns them) and no tmux `send-keys` was recorded; assert status/normal rows exist but are **not** pending, and the fake tmux log (`withFakeTmux` writes a log file) contains the paste.
4. **Fallback test:** make the fake tmux exit non-zero → `Send --type normal` stores an unacked row.
5. **`crew/cleanup_test.go` (new):** seed sessions/orchestrators in a temp store; fake tmux reports one live window; `Cleanup(cs, home, false)` marks the dead ones failed/stopped and reports the live one untouched; `--dry-run` (pass `true`) mutates nothing.
6. Keep every existing test green — several assert `Send` behavior; update them to the new matrix deliberately, not incidentally.

## Edge cases you must handle (found during exploration)

- **Existing databases:** every current install has tables but `user_version=0`. Migration v1 must be `IF NOT EXISTS`-safe and v2 must tolerate duplicate-column — otherwise `jeff crew list` bricks on upgrade.
- **DSN paths with special characters:** `file:` DSNs treat `?` and `#` as delimiters. JEFF_HOME paths with spaces are fine, but add a comment noting the limitation; do not `url.QueryEscape` the whole path (that breaks `/`).
- **`SetMaxOpenConns(1)` + long transactions:** never hold a `*sql.Rows` open while issuing another query on the same store (it deadlocks with a single connection). Audit: `ListSessions` and `scanMessages` fully drain rows before returning — keep it that way; in new code, always `rows.Close()` before the next query.
- **The nudge path relies on the hook actually firing.** An idle worker receives nudges only on its next tool use (PostToolUse). That is the documented intent ("low context impact"), but `jeff crew send --type nudge` should print `queued for delivery on next tool use` so operators aren't surprised.
- **Divert's `C-c` interrupt still types into the pane** — the pre-acked row means the hook no longer re-injects it; do not also remove the interrupt sleep constants in this plan (they're gig-906c/gig-c6dd regression fixes).
- **`worker-stop`/`SignalOrchestrator` messages are fire-and-forget pane writes with no DB row** — leave them; they are not part of the inbox system.
- **`jeff crew ask` (worker→orchestrator)** stores `to_orchestrator` rows that the orchestrator-inbox hook drains — that direction has no double-delivery (typing goes to the orchestrator pane, hook injects into the orchestrator's context; both are intentional per its design). Only change the `to_worker` direction. If you decide to keep this asymmetry, say so in the updated comments.
- **TUI `PendingCount`** counts unacked `to_worker` rows: after this change it counts only undelivered nudges — that is the correct semantic ("pending"), but update the TUI label if it says "messages".

## Acceptance criteria

1. `go build ./... && go vet ./... && go test ./...` green; `go test -race ./crew/` green.
2. `go test ./crew/ -run TestSend -v` shows the four-type delivery matrix assertions passing.
3. `go test ./crew/ -run TestCleanup -v` — new file, passing.
4. Grep checks:
   - `grep -n "_pragma=busy_timeout" crew/crew.go` — present.
   - `grep -n "SetMaxOpenConns(1)" crew/crew.go` — present.
   - `grep -n "user_version" crew/crew.go` — present.
   - `grep -rn "jeff-send\"" crew/tmux.go` — **no** fixed-name buffer remains (name is now PID-derived).
   - `grep -n "delivered via PostToolUse hook" crew/crew.go` — comment rewritten to the matrix.
5. Concurrency smoke: `go test ./crew/ -run TestConcurrentWrites -count=3` (the new test from Step 7.1) passes consistently.

## Out of scope

- Worker liveness/PID health detection and the stall daemon (kept for a future daemon plan — see IMPROVEMENTS.md honorable mentions).
- `StopAll` parallelization and the injectable-clock refactor for `Start*` functions.
- Log capture / persistent worker output.
- Any TUI changes beyond compile-compatibility with the `Refresh` signature.
