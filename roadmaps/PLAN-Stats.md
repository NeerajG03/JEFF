# PLAN-Stats: `jeff stats` — Observability Over Gig Events and Attributes (Roadmap Phase 3)

> **Rank:** 9/10 · **Leverage:** Medium-high (the feedback loop the whole memory/persona system is supposed to be measured by) · **Effort:** Medium · **Prereqs:** **PLAN-Phase1-Attrs-Resume must land first** (stats reads `persona`, `skills_loaded`, `memory_loaded`, `outcome`, `pr_urls` attrs; without them most columns are empty)
>
> Ground rules: run `go build ./... && go vet ./... && go test ./...` after each step group.

## Why (the problem)

The roadmap's Phase 3 (`docs/roadmap.md:359-431`) designs `jeff stats` as the observability layer: which personas ship first-try, which repos accumulate rejections, whether memory actually reduces failure. None of it exists — there is no `stats` command, and until PLAN-Phase1-Attrs-Resume no data to query. The payoff is the roadmap's own list (`docs/roadmap.md:421-431`): the stats output tells you which repo needs learnings, which persona's memory needs curation, and whether the memory system is working *at all*. Without it, JEFF's self-improvement story is unfalsifiable.

## What (the goal)

A `stats` package + `jeff stats` command computing, at query time, from gig only (no new storage):

```
$ jeff stats --since 30d
JEFF Stats (last 30 days)
──────────────────────────────────────
Tasks closed        12   (10 done, 1 abandoned, 1 cancelled)
Avg time to close   4.2h (claim → close, 11 tasks with both events)
Checkpoints/task    3.1

By persona:
  jenko     9 tasks   4.0h avg
  hardy     2 tasks   1.2h avg
  (none)    1 task    0.4h avg

By repo:
  backend   7 tasks
  frontend  5 tasks

Memory:
  tasks with persona/repo memory loaded   8/12
  tasks with skills injected              6/12
```

Flags: `--since <7d|30d|90d|Nd>` (default 30d), `--persona <name>`, `--repo <name>`, `--outcome <value>`, `--json`.

## Files to touch

| File | Change |
|---|---|
| `stats/stats.go` | **New** — data collection + aggregation |
| `stats/stats_test.go` | **New** — against a temp gig store |
| `cmd/jeff/stats_cmd.go` | **New** — flags, rendering, `--json` |
| `cmd/jeff/main.go` | Register `statsCmd()` |
| `docs/usage.md`, `README.md` command table | Document it |

## Implementation steps

### Step 1 — the stats package

`stats/stats.go`:

```go
package stats

type Options struct {
	Since   time.Time
	Persona string // filter; "" = all
	Repo    string // filter; "" = all
	Outcome string // filter; "" = all
}

type TaskStat struct {
	ID           string        `json:"id"`
	Title        string        `json:"title"`
	Persona      string        `json:"persona,omitempty"`
	Repos        []string      `json:"repos,omitempty"`
	Outcome      string        `json:"outcome,omitempty"`
	PRs          map[string]string `json:"prs,omitempty"`
	SkillsLoaded []string      `json:"skills_loaded,omitempty"`
	MemoryLoaded []string      `json:"memory_loaded,omitempty"`
	Checkpoints  int           `json:"checkpoints"`
	ClaimedAt    *time.Time    `json:"claimed_at,omitempty"`
	ClosedAt     *time.Time    `json:"closed_at,omitempty"`
	CycleTime    time.Duration `json:"cycle_time_ns,omitempty"` // claim→close when both known
}

type Report struct {
	Since       time.Time            `json:"since"`
	Tasks       []TaskStat           `json:"tasks"`
	ByPersona   map[string]Aggregate `json:"by_persona"`
	ByRepo      map[string]Aggregate `json:"by_repo"`
	ByOutcome   map[string]int       `json:"by_outcome"`
	MemoryUse   MemoryUse            `json:"memory_use"`
}

type Aggregate struct {
	Tasks        int           `json:"tasks"`
	AvgCycleTime time.Duration `json:"avg_cycle_time_ns,omitempty"`
	CycleSamples int           `json:"cycle_samples"`
}

type MemoryUse struct {
	WithMemory int `json:"with_memory"`
	WithSkills int `json:"with_skills"`
	Total      int `json:"total"`
}

func Collect(store *gig.Store, opts Options) (*Report, error)
```

Collection algorithm (verified against gig v0.6.2 API):

1. **Candidate tasks:** terminal tasks in the window. gig has no closed-after filter in `ListParams`, so: `store.List(gig.ListParams{Status: &closed})` and `store.List(gig.ListParams{Status: &cancelled})` (two calls; `Status *Status` — see `gig.go ListParams`), then keep tasks with `ClosedAt != nil && ClosedAt.After(opts.Since)`. (`Task.ClosedAt *time.Time` exists.)
2. **Per task:**
   - Attrs via `store.Attrs(taskID)` (single call returns all) → map by key; parse `repos`, `skills_loaded`, `memory_loaded` as JSON `[]string`; `persona`, `outcome` as strings; `pr_urls` as JSON `map[string]string`. Tolerate every missing/malformed attr (skip field).
   - Checkpoints: `len(store.ListCheckpoints(taskID))`.
   - Claim time: `store.Events(taskID)` → the **first** event with `EventType == gig.EventStatusChanged` and `NewValue == string(gig.StatusInProgress)` → its `Timestamp` is `ClaimedAt` (Event fields: `ID, TaskID, EventType, Actor, Field, OldValue, NewValue, Timestamp` — read the struct in the gig module cache before coding; use the actual field names).
   - `CycleTime = ClosedAt - ClaimedAt` when both present and positive.
3. **Filters:** apply `Persona`/`Repo` (membership in the repos slice)/`Outcome` after collection.
4. **Aggregates:** group into `ByPersona` (empty persona groups under `"(none)"`), `ByRepo` (a task with 2 repos counts in both), `ByOutcome`; `MemoryUse` counts non-empty `MemoryLoaded`/`SkillsLoaded`.

### Step 2 — the command

`cmd/jeff/stats_cmd.go`:

- Parse `--since` with a tiny helper: `^(\d+)d$` → days (also accept `7d/30d/90d` naturally); reject other units with a clear error (`--since accepts Nd, e.g. 14d`).
- `openGigStore()` (existing helper), `defer store.Close()`.
- `--json`: `json.MarshalIndent(report)` to stdout, nothing else on stdout.
- Human rendering to stdout following the layout in "What" above:
  - durations: format as `0.4h` under 10h, `1.2d` above (helper `fmtDur`).
  - sort persona/repo groups by task count desc, then name.
  - omit the Memory block entirely when `MemoryUse.Total == 0` — avoids implying measurement where no attrs exist (pre-Phase-1 stores).
- Register in `main.go`'s command list.

### Step 3 — tests

`stats/stats_test.go` with a temp gig store (copy the open/fixture pattern from existing tests that use `gig` — grep `gig.Open(` in `cmd/jeff/*_test.go`):

1. Fixture: 3 tasks — A: claimed then closed with persona=jenko, repos=["backend"], 2 checkpoints, outcome=done; B: closed, no attrs at all (legacy task); C: cancelled with persona=hardy.
   - Drive real transitions so events exist: `store.Claim(id, "jeff")` then `store.CloseTask(id, "done", "jeff")`; set attrs via `store.SetAttr` after `jeff.EnsureAttrs(store)`.
2. Assertions: `Collect` returns 3 tasks; A has positive CycleTime and Checkpoints=2; B groups under `"(none)"` with zero-value fields (no error!); ByOutcome{"done":?,"cancelled":1}; persona filter jenko → 1 task; since-window excludes a task closed before the window (backdate by... `ClosedAt` is set by gig — you cannot backdate through the API; instead set `opts.Since` to the future for the exclusion case, or to `time.Now().Add(-time.Minute)` for inclusion — design the test around controllable Since, not fake clocks).
3. `--since` parser table test (`30d` ok, `2w` rejected, `0d` ok-but-empty).
4. JSON shape: `json.Unmarshal` the `--json` output back into `Report` (round-trip).

### Step 4 — docs

- `README.md` command table: `jeff stats [--since 30d] [--persona] [--repo] [--outcome] [--json]`.
- `docs/usage.md`: a short section with example output.
- `docs/roadmap.md`: mark Phase 3 as shipped-in-part (metrics that exist), noting first-ship-rate/rejections await a reject flow.

## Edge cases you must handle (found during exploration)

- **Most historical tasks have NO attrs** (they predate PLAN-Phase1-Attrs-Resume). Every attr read must tolerate absence — the `"(none)"` persona bucket and omitted fields are the norm on day one, not the exception.
- **gig `GetAttr` errors on missing attrs in some paths; `Attrs(taskID)` returns only set ones** — prefer the single `Attrs` call (one query, no per-key error dance).
- **`EventsSince` is store-global, `Events(taskID)` is per-task** — use per-task `Events` (N+1 queries but N is tens; correctness first). Do NOT scan `EventsSince(opts.Since)` for claims: a task claimed before the window but closed inside it would lose its ClaimedAt.
- **Claim events for re-opened/re-claimed tasks:** take the FIRST in_progress transition for ClaimedAt (matches "time to ship" intent); if `ClosedAt < ClaimedAt` (reopen weirdness), drop the CycleTime sample (`CycleSamples` exists so averages stay honest).
- **`store.List` with `Status` pointer:** take addresses of local `s := gig.StatusClosed` — you cannot take the address of a constant inline.
- **Tasks closed via `gig` CLI directly** (outside jeff) appear with no attrs — same as legacy; fine.
- **Two repos on one task double-counts in ByRepo** — intended (document in the field comment), but `Tasks` total is the unique count.
- **Duration formatting under test:** don't assert exact float strings in the human output; test the numbers in `Report` and keep rendering assertions loose (contains persona name).
- **No new deps** — table rendering is `fmt.Fprintf` padding, not a table library.
- **Do not store anything.** The roadmap is explicit (`docs/roadmap.md:361-368`): computed at query time; no new tables, no caches.

## Acceptance criteria

1. `go build ./... && go vet ./... && go test ./...` green.
2. `go test ./stats/ -v` — fixture scenarios above pass.
3. `jeff stats --json | jq .` on a real (or fixture) store emits valid JSON with `tasks`, `by_persona`, `by_outcome`.
4. `jeff stats --since 7d --persona jenko` filters (asserted via unit test on `Collect`).
5. `grep -n "statsCmd()" cmd/jeff/main.go` — registered.
6. Command table in README updated (`grep -n "jeff stats" README.md`).

## Out of scope

- First-ship rate / rejection metrics (need a reject flow writing `rejection_count`; the field exists after PLAN-Phase1-Attrs-Resume, unwritten).
- Memory-effectiveness correlation (`rejections WITH memory vs WITHOUT`) until rejections exist.
- Trend detection, CSV export (`jeff stats export`), and the TUI stats tab.
- Any daemon/event-subscription machinery (`store.On`) — stats is pull-only.
