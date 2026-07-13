# PLAN-Gig-Upgrades: Harden gig for the Multi-Client / Worker Model

> **Repo:** `github.com/NeerajG03/gig` (NOT this repo — file references below are gig paths, audited at v0.6.2) · **Leverage:** Foundation for EPIC-Jeff-Anywhere, and fixes real single-machine races JEFF hits today · **Effort:** Small-Medium · **Prereqs:** none. Ship as gig v0.7.0, then bump JEFF's `go.mod`.
>
> Context: gig is "the brain" — all JEFF task state lives here. The worker model (hub arbitrating claims, workers streaming events) leans on exactly the parts audited below. gig is in good shape structurally (a real `schema_migrations` framework with per-migration transactions, sensible indexes including `idx_attrs_key_value`, a dependency-aware `Ready` query) — the gaps are concurrency and event-stream fidelity.

## The findings (verified in gig v0.6.2 source)

1. **Per-connection PRAGMAs on a pooled handle** (`store.go:88-110`). `PRAGMA busy_timeout=5000` / `foreign_keys=ON` are executed via `db.Exec` on `database/sql`'s pool — they bind to whichever connection ran them. Every other pooled connection has `busy_timeout=0` (instant `SQLITE_BUSY` failures under contention) and `foreign_keys=OFF`. This is the same footgun found in JEFF's crew store. gig's own comment says the timeout exists for "multi-agent" use — the scenario where it silently doesn't apply.
2. **`Claim` is get-then-update, not compare-and-swap** (`task.go:458-495`): `Get`, then unconditional `UPDATE tasks SET assignee=?, status=?`. Two concurrent claimers both succeed; last writer wins silently. JEFF's crew can hit this *today* (two `jeff crew start` on the same task); the hub model makes it the central correctness question.
3. **Events can be silently lost** (`store.go:172-180`): `recordEvent` does `_, _ = s.db.Exec(...)` — a `SQLITE_BUSY` (see #1) drops the audit record with no trace, and the mutation + its event are not in one transaction (crash between them = mutation without audit). For the worker model, events are the progress stream — they must be durable.
4. **ID collision space is tiny and collisions aren't handled** (`store.go:21-35,142-145`, `task.go:31-41`): default IDs are `gig-` + 4 hex chars = 65,536 values; by the birthday bound you reach ~50% collision probability around ~300 tasks. On collision, `Create`'s INSERT fails with a raw `UNIQUE constraint failed: tasks.id` — no retry, confusing error. Separately, **subtask ladder IDs race**: `SELECT COUNT(*) WHERE parent_id` then insert `parent.N` (`task.go:33-39`) — two concurrent child creations mint the same ID.
5. **No reliable event cursor** (`event.go:8-35`): `EventsSince(time)` filters `timestamp > ?` with RFC3339 **second** precision (`util.go:8`). A consumer polling with a time cursor misses or duplicates events sharing the boundary second; same-second ordering is unspecified (`ORDER BY timestamp` only). The events table has an integer `id` primary key — the natural cursor — but no API exposes it.
6. *(minor)* `emit` runs listeners synchronously under an RLock (`store.go:148-156`) — a slow `On` callback blocks the writing goroutine. Acceptable, but worth a doc note and an obvious place for a buffered option later.

## What to change (in priority order)

### 1. Fix connection settings (one function)

`Open` (`store.go:76-125`):

```go
dsn := "file:" + dbPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
db, err := sql.Open("sqlite", dsn)
...
db.SetMaxOpenConns(1)
```

Delete the three `db.Exec("PRAGMA ...")` blocks. modernc.org/sqlite applies `_pragma` query params to every new connection; `SetMaxOpenConns(1)` additionally serializes Go-side access (correct for SQLite's single-writer reality and makes multi-statement sequences race-free within a process). Keep `migrate.Run` as-is.

- **Test:** open one store, hammer it from 8 goroutines × 50 mixed Create/UpdateStatus — zero `database is locked` errors. Add a second-process test if the suite has one already (two `Open`s on one path, interleaved writes).
- **Edge case:** `file:` DSN treats `?`/`#` in paths as delimiters — note the limitation in `Open`'s doc comment rather than escaping (escaping breaks `/`).

### 2. Compare-and-swap `Claim` + typed error

`task.go:458-495`:

```go
var ErrAlreadyClaimed = errors.New("task already claimed")

res, err := s.db.Exec(
    `UPDATE tasks SET assignee = ?, status = ?, updated_at = ?
     WHERE id = ? AND status NOT IN (?, ?, ?)`,
    assignee, string(StatusInProgress), now.Format(timeFormat),
    id, string(StatusInProgress), string(StatusClosed), string(StatusCancelled),
)
n, _ := res.RowsAffected()
if n == 0 {
    // Distinguish missing vs claimed for a good error.
    t, gerr := s.Get(id)
    if gerr != nil { return nil, gerr }
    return nil, fmt.Errorf("%w: %s by %q (status %s)", ErrAlreadyClaimed, id, t.Assignee, t.Status)
}
```

Keep the pre-`Get` for event old-values (or fetch once before the UPDATE and accept the tiny window — with `SetMaxOpenConns(1)` in-process it's exact; cross-process the CAS itself is the guarantee). Record the same events as today only when the CAS succeeded.

- **Semantics decision to encode:** claiming an `in_progress` task now **fails** (previously it silently re-assigned). Re-claiming by the *same* assignee should stay allowed for idempotent resume: add `AND NOT (status = 'in_progress' AND assignee != ?)`-style logic — concretely, allow when the existing row is `in_progress` AND `assignee = ?` (same claimer). Write both cases as tests: same-assignee re-claim OK; different-assignee claim → `ErrAlreadyClaimed`.
- **Blocked/deferred:** today's code claims them (only terminal statuses are rejected, `task.go:463-465`) — preserve that; the CAS `NOT IN` list is exactly `in_progress` (with the same-assignee carve-out) + terminal states.
- Callers in JEFF: `pickupTask` treats claim failure as fatal — with the typed error it can print "already claimed by X" cleanly; PLAN-Pickup-Rollback's resume path checks the same-assignee case.

### 3. Durable, transactional events

- Change `recordEvent` to return an error, and stop discarding it at call sites: log-and-continue is acceptable for the emit path, but the INSERT error must at least surface via a package-level `slog`/stderr warning (silent loss is the bug).
- Add `recordEventTx(tx *sql.Tx, ...)` and move the write-then-event pairs into transactions for the mutators where audit integrity matters most: `Create`, `UpdateStatus`, `CloseTask`, `CancelTask`, `Claim`, `Update`. Pattern per mutator: `tx begin → mutation → recordEventTx → commit → s.emit(...)` (emit after commit, never inside the tx).
- **Test:** a mutation whose event insert is forced to fail (temporarily drop the events table inside the test) rolls back the mutation.

### 4. ID generation: collision retry + bigger default + atomic ladders

- `Create` (`task.go:10-80`): on `UNIQUE constraint` error for a root task, regenerate the ID and retry (max 5, then error "id space exhausted — raise WithHashLength"). Detect via `strings.Contains(err.Error(), "UNIQUE constraint failed: tasks.id")` (modernc has no typed error; add a comment).
- Bump `defaultHashLength` 4 → 6 (16.7M space; `gig-a3f8c1`). **Compat:** existing IDs stay valid — length only affects new IDs. Note in changelog; JEFF's `workspace.ExtractTaskID` regex (`^(gig-[a-z0-9]+...)`) already accepts variable length (verified: `workspace/workspace.go:119-129` uses `+`, not `{4}`).
- Subtask ladder (`task.go:31-39`): wrap COUNT+INSERT in the same transaction as #3's `Create` tx, and on UNIQUE failure retry with count+2 (bounded loop) — the COUNT can still under-read committed-but-unseen rows without a tx; inside one tx with `SetMaxOpenConns(1)` per process plus busy_timeout across processes, a retry loop converges.

### 5. Event cursor API

`event.go`:

```go
// EventsAfterID returns up to limit events with id > afterID, ordered by id.
// The id is a monotonically increasing integer suitable as a resume cursor.
func (s *Store) EventsAfterID(afterID int64, limit int) ([]*Event, error)
```

- Ensure `Event.ID` is the integer PK (it is selected today — verify the struct field type; if it's a string, add `RowID int64` instead of breaking the field).
- Change `Events`/`EventsSince` ordering to `ORDER BY timestamp ASC, id ASC` so same-second batches are deterministic.
- Leave `timeFormat` alone (mixed-precision strings would break lexicographic ordering of existing rows); the ID cursor makes timestamp precision moot for consumers.
- JEFF hub usage: persist `last_event_id` per consumer; poll `EventsAfterID` as the sweep fallback alongside in-process `On` callbacks.

### 6. Docs

- `On`/`emit` doc comments: "listeners run synchronously on the writing goroutine; keep them fast — in-process only (other processes writing to the same DB do not trigger them)". This sentence prevents the exact misunderstanding JEFF's roadmap Phase 5 sketch has.
- CHANGELOG entries for: CAS claim semantics change (`ErrAlreadyClaimed`), default ID length, new cursor API.

## Acceptance criteria (gig repo)

1. `go build ./... && go vet ./... && go test ./... -race` green (add `-race` to gig's CI while there — same gap as JEFF's).
2. New tests: concurrent-claim (two goroutines, one `ErrAlreadyClaimed`), same-assignee re-claim OK, 8×50 concurrent-write hammer with zero busy errors, Create collision-retry (seed by pre-inserting the would-be ID with a stubbed rand — or temporarily `WithHashLength(3)` and brute-force), ladder-race test, `EventsAfterID` pagination + ordering.
3. `grep -n "PRAGMA" store.go` → only the DSN string remains.
4. `grep -n "_, _ = s.db.Exec" store.go` → gone.
5. Tag `v0.7.0`. Then in JEFF: `go get github.com/NeerajG03/gig@v0.7.0`, run JEFF's suite, and update `pickupTask`/`task.Pickup` to branch on `errors.Is(err, gig.ErrAlreadyClaimed)`.

## What NOT to do

- No server/network layer in gig — gig stays an embedded library; the hub (JEFF side) owns transport. Resist "gig-server" scope creep.
- No lease/worker concepts in gig — leases are JEFF-hub domain (they reference workers, which gig knows nothing about).
- No timestamp format migration.
- Do not change `Ready`'s semantics (its dependency/parent logic is used by completions and the TUI); the worker model consumes it as-is.
