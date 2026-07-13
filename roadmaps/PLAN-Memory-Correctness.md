# PLAN-Memory-Correctness: Make Shipped Memory Features Actually Work

> **Rank:** 3/10 · **Leverage:** Very high (memory is JEFF's differentiator; several shipped behaviors are no-ops or lose data) · **Effort:** Medium · **Prereqs:** none
>
> Ground rules: run `go build ./... && go vet ./... && go test ./...` after each step group. All memory tests use `t.TempDir()` — never a real `JEFF_HOME`.

## Why (the problem)

Six verified defects in the memory subsystem, in descending severity:

1. **`jeff memory disable` is a no-op.** It writes `cfg.Memory.Disabled = true` (`cmd/jeff/memory/disable_cmd.go:70-74`) and its help text promises workers get no addendum and hooks are skipped — but **nothing outside `disable_cmd.go` reads `.Disabled`**, and `JEFF_MEMORY_DISABLE` is referenced only in help text. Injection (`hooks/sessionstart_memory.go:26`), session-end capture (`hooks/sessionend_memory.go:26`), legacy pickup injection (`cmd/jeff/pickup_cmd.go:97-124`), and `propose` all run regardless.
2. **A failed curate run destroys its inputs.** `Curate` (`memory/curate.go:141-165`) captures the agent-runner error into the report but **archives the queue entries and proposals anyway** — un-curated observations are swept into `archive/` and never processed.
3. **One corrupt file breaks all reads.** `ListEntries` aborts the whole walk on the first parse error (`memory/store.go:62-65`), so a single malformed `.md` breaks `jeff memory list/show/status/diff` — while injection silently drops that whole scope (`memory/inject.go:80,89,97`), so the agent loses its memory with no visible error.
4. **Silent history loss on name collision.** `WriteCanonical` uses `os.Create` truncation (`memory/store.go:197-204`); `jeff memory add` twice with the same name+scope+bucket silently clobbers the first entry — no supersede chain, no error.
5. **`jeff memory status` reports "Last curate: never" forever.** It reads `memory/.last-curated` (`cmd/jeff/memory/status_cmd.go:134-146`), which **no production code writes** (only the unit test writes it).
6. **Repeated Stop hooks duplicate queue entries.** `RunSessionEnd` appends a new nanosecond-keyed queue file per invocation (`memory/queue.go:40`), so one session can enqueue the same transcript several times, and marlowe re-processes duplicates.

Plus one scalability gap: nothing bounds the injected index (`memory/inject.go:75-110`) or reclaims `transcripts/`, `queue/sessions/*-start.log`, or `archive/` — disk and context grow forever.

## What (the goal)

- A single `memory.Disabled(jeffHome)` gate enforced at every entry point.
- Curate keeps inputs when the agent run fails; writes `.last-curated` on success.
- Corrupt entries are skipped with a warning, never fatal.
- Duplicate canonical writes are refused with a supersede hint.
- Queue entries are deduped per (task, transcript).
- Injected index capped; basic retention sweep for transcripts and start-logs.

## Files to touch

| File | Change |
|---|---|
| `memory/types.go` (or new `memory/disable.go`) | `Disabled(jeffHome string) bool` helper |
| `hooks/sessionstart_memory.go` | Gate `RunSessionStart` |
| `hooks/sessionend_memory.go` | Gate `RunSessionEnd` |
| `cmd/jeff/pickup_cmd.go` | Gate legacy memory injection in `writeTaskClaudeMD` |
| `cmd/jeff/memory/propose_cmd.go` | Gate propose |
| `memory/curate.go` | No-archive-on-error; write `.last-curated`; retention sweep |
| `memory/store.go` | Skip corrupt entries with warning; refuse duplicate `WriteCanonical` |
| `memory/queue.go` | Dedupe key |
| `hooks/sessionend_memory.go` | Pass dedupe key through |
| `memory/inject.go` | Cap index entries per scope |
| `cmd/jeff/memory/add_cmd.go` | Map duplicate error to a supersede hint |
| tests | New/updated across `memory/*_test.go`, `cmd/jeff/memory/*_test.go` |
| `memory/CLAUDE.md`, `cmd/jeff/memory/disable_cmd.go` help | Reflect reality |

## Implementation steps

### Step 1 — the disable gate

Add to `memory` package (new file `memory/disable.go`):

```go
// Disabled reports whether the memory subsystem is switched off, either via
// jeff.json ("memory": {"disabled": true}) or the JEFF_MEMORY_DISABLE env var.
func Disabled(jeffHome string) bool {
	if v := os.Getenv("JEFF_MEMORY_DISABLE"); v == "1" || strings.EqualFold(v, "true") {
		return true
	}
	// Read jeff.json directly to avoid an import cycle with the root package.
	data, err := os.ReadFile(filepath.Join(jeffHome, "jeff.json"))
	if err != nil {
		return false
	}
	var cfg struct {
		Memory struct {
			Disabled bool `json:"disabled"`
		} `json:"memory"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return false
	}
	return cfg.Memory.Disabled
}
```

**Important:** `memory` must NOT import the root `jeff` package (the root package is imported by `cmd/jeff` which imports `memory`; today there is no `memory→jeff` edge — keep it that way with the local struct decode above).

Enforce at the four entry points; each gets an early return:

- `hooks.RunSessionStart` (`hooks/sessionstart_memory.go:26`): `if memory.Disabled(jeffHome) { return nil }` (the `hooks` package already imports `memory`? — verify; it calls `memory.ApplyToTask`, so yes).
- `hooks.RunSessionEnd` (`hooks/sessionend_memory.go:26`): same guard.
- `cmd/jeff/pickup_cmd.go` `writeTaskClaudeMD`: wrap the "Persona memory" and "Repo learnings" blocks (~lines 97-124) in `if !memory.Disabled(jeffHome) { ... }`. Also skip the scratchpad guide (`writeScratchpadGuide`) when disabled.
- `cmd/jeff/memory/propose_cmd.go` RunE: return a clear error: `memory is disabled (jeff memory disable) — re-enable before proposing`.

Update the help text in `disable_cmd.go` only if it over-promises something still not enforced after this step (the orchestrator-hook claim: the memory hooks it refers to are `RunSessionStart`/`RunSessionEnd`, both now gated — the text becomes true).

### Step 2 — curate: fail safe, then stamp success

In `memory.Curate` (`memory/curate.go:98-169`):

1. If the agent runner returns an error, **return** the error after recording it — do **not** run the archive loop (`:154-165`). Queue entries and proposals stay in place for a retry.
2. On a fully successful pass (runner OK, report parsed), after archiving write the stamp:

```go
stamp := filepath.Join(MemoryRoot(home), ".last-curated")
_ = os.WriteFile(stamp, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
```

3. Fix `cmd/jeff/memory/status_cmd_test.go` so it asserts the stamp is written by `Curate` (use the existing mock runner from `memory/curate_test.go`) instead of hand-writing the file.

### Step 3 — corrupt-file resilience

In `memory/store.go`:

- `ListEntries` (`:39-90`) and `ListScope` (`:117-154`): on a `ReadEntry` parse failure, **skip** the file, emit `fmt.Fprintf(os.Stderr, "warning: skipping unreadable memory entry %s: %v\n", path, err)`, and continue the walk. Only I/O errors on the walk itself remain fatal.
- Rationale for stderr-warn over API change: the only consumers are the CLI and injection; keeping the `([]Entry, error)` signature avoids touching every caller.
- Add a test: a scope with one valid and one corrupt (`no frontmatter fence`) file → `ListEntries` returns the valid entry, no error; `buildMemoryIndex` still renders that scope.

### Step 4 — refuse silent overwrites

In `WriteCanonical` (`memory/store.go:167-222`), before creating the file:

```go
if _, err := os.Stat(path); err == nil {
	return fmt.Errorf("memory entry %q already exists in %s/%s — use 'jeff memory add --supersede %s' to replace it", fm.Name, scope, bucket, fm.Name)
}
```

- Exception: the `core` bucket is a single well-known file that is *meant* to be rewritten — allow overwrite when `bucket == BucketCore` (check the actual constant name in `memory/types.go:62-70`).
- Check how `add_cmd.go` implements `--supersede` (it calls `Supersede`, which writes the new entry itself — confirm `Supersede`'s new-entry path doesn't trip the new guard; if it routes through `WriteCanonical`, thread an `allowExisting bool` parameter internally, keeping the exported behavior: plain add = refuse, supersede = allowed).
- Test: add twice → second returns the hint error; supersede path still works (existing `store_write_test.go` covers supersede — keep it green).

### Step 5 — queue dedupe

`RunSessionEnd` (`hooks/sessionend_memory.go:26`) → `WriteQueueEntry` (`memory/queue.go:29-52`):

- Change the filename key from `<task>-<unixnano>.json` to `<task>-<transcriptBase>.json` where `transcriptBase` is the transcript filename without extension (the transcript path is already a parameter). Same task + same transcript ⇒ same file ⇒ second Stop event **overwrites** (idempotent) instead of appending a duplicate.
- If `transcriptPath` is empty, fall back to the current unixnano key.
- Test: two `RunSessionEnd` calls with the same transcript → exactly one queue file; different transcripts → two files.

### Step 6 — cap the injected index

In `buildMemoryIndex` (`memory/inject.go:75-110`):

- Cap each scope's bullets at `maxIndexEntriesPerScope = 30`. Order before truncation: by frontmatter `importance` descending (canonical schema field — see `memory/frontmatter.go:27-55`), then by `valid_from` descending. If more entries exist, append `- …and N more — run 'jeff memory list --scope <scope>'`.
- Test: 35 accepted entries in one scope → 30 bullets + the overflow line.

### Step 7 — retention sweep (bounded, conservative)

At the end of a **successful** `Curate` pass, add `sweepRetention(home)`:

- Delete files under `transcripts/` older than 28 days (mtime).
- Delete `queue/sessions/*-start.log` older than 7 days (they are informational only — written by `hooks/sessionstart_memory.go:40-58`, read by nothing).
- Do **not** touch `archive/` (audit trail) or canonical memory.
- Log a one-line summary: `retention: removed N transcripts, M start-logs`.
- Test with back-dated files via `os.Chtimes`.

### Step 8 — dead-flag cleanup

`CurateOptions.Auto` (`memory/curate.go:60`) is plumbed but never read. Pick ONE:
- (preferred) Remove the field, the `--auto` flag in `cmd/jeff/memory/curate_cmd.go:63`, and the stale instruction in `embed/slash-commands/curate.md:8`; or
- implement it. Removal is preferred — implementing auto-conflict-resolution is out of scope.

### Step 9 — docs

Update `memory/CLAUDE.md`: corrupt-file behavior, dedupe key, cap, retention, disable gate. Update `cmd/jeff/memory/disable_cmd.go` long help to describe exactly what is now gated.

## Edge cases you must handle (found during exploration)

- **Import cycle risk:** `memory` must not import the root `jeff` package (root ← cmd/jeff → memory). The disable helper reads `jeff.json` with a local anonymous struct — do not "clean it up" by importing `jeff.Config`.
- **Legacy + v1 double injection is intentional-for-now.** Pickup injects legacy full-text memory (`pickup_cmd.go:97-124`) AND `RunSessionStart` injects the v1 index. This plan only gates both behind `Disabled` — do not remove the legacy path here (that's the dual-API migration, tracked separately in `memory/CLAUDE.md`).
- **`core.md` is a legitimate overwrite target** — the duplicate-guard exception matters or curation of core memory breaks.
- **`Supersede` must keep working** — it writes a new entry file for the successor; make sure the new existence guard doesn't reject the supersede flow (Step 4 note).
- **The addendum RMW race:** `applyAddendum` (`memory/inject.go:122-154`) read-modify-writes CLAUDE.md and can race the bash `memory-session-start` hook on resume. Full locking is out of scope, but keep writes atomic-ish: write to `CLAUDE.md.tmp` + `os.Rename` in `applyAddendum` while you're in the file (rename within the same dir is atomic on POSIX).
- **`jq`-generated queue JSON:** `listQueueItems` (`memory/curate.go:245-272`) aborts on one malformed queue JSON — apply the same skip-with-warning treatment as Step 3 (cheap, two lines, prevents one bad file from blocking all curation).
- **Windows:** `os.Rename` over an existing file fails on Windows; the repo targets macOS/Linux (tmux-based), so a comment suffices.
- **Don't break `TestMemoryStatus*`:** status tests currently hand-write `.last-curated`; update them per Step 2.3 rather than deleting assertions.

## Acceptance criteria

1. `go build ./... && go vet ./... && go test ./...` green.
2. Disable gate:
   - `go test ./memory/ -run TestDisabled -v` — env var and jeff.json variants.
   - New hook tests: `RunSessionStart`/`RunSessionEnd` with `JEFF_MEMORY_DISABLE=1` (use `t.Setenv`) do nothing (no addendum written, no queue entry).
   - `grep -rn "memory.Disabled(" hooks/ cmd/jeff/ | wc -l` ≥ 4.
3. Curate: mock runner error → queue+proposals still present, error returned; mock success → archived + `.last-curated` exists with RFC3339 content.
4. Corruption: corrupt-entry test from Step 3 passes; `jeff memory list` (via its cmd test) succeeds against a store with one bad file.
5. Duplicate add: second `WriteCanonical` errors mentioning `--supersede`; supersede test still green.
6. Queue: same-transcript dedupe test green.
7. Index cap: 35-entry test renders 30 + overflow line.
8. `grep -rn "Auto" memory/curate.go cmd/jeff/memory/curate_cmd.go` — flag gone (or implemented, if you chose that).

## Out of scope

- Retiring the legacy `/learn` API (the dual-API migration `gig-1d33.6`).
- Cross-process file locking for the whole store (only the tmp+rename hardening above).
- Semantic search / relevance-ranked retrieval.
- The `core`-slug show/diff ambiguity (cosmetic; document it in `memory/CLAUDE.md` if touched).
