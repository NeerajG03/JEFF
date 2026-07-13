# PLAN-Phase1-Attrs-Resume: Complete Roadmap Phase 1 — Gig Attributes + Context-Rich Resume

> **Rank:** 1/10 · **Leverage:** Very high · **Effort:** Small-Medium · **Prereqs:** none
>
> Ground rules for the executing agent: work only inside this repo. After each numbered step group, run `go build ./... && go vet ./... && go test ./...` — all three must stay green before moving on. Do not rename existing exported functions unless the step says so.

## Why (the problem)

`docs/roadmap.md` Phase 1 ("Persona Memory + Repo Learnings") is ~70% shipped. Two pieces were never built, and their absence causes real bugs today:

1. **The Phase-1 gig attributes don't exist.** `attrs.go:6-9` defines only `repos` and `worktree_setup`. The roadmap (`docs/roadmap.md:243-260`) specs `persona`, `skills_loaded`, `memory_loaded`, `team_size`, `outcome`, `rejection_count`. Because the persona used at pickup is never persisted, the code *reconstructs* it by string-prefix-matching the task's `CLAUDE.md` against embedded templates (`detectPersona` in `cmd/jeff/pickup_cmd.go:238-259`). This hack:
   - silently returns `""` for any custom/registry persona (it only checks embedded templates via `persona.Get`),
   - silently returns `""` if the persona section was edited or the memory addendum shifted the file,
   - is used in two load-bearing places: `refreshTaskClaudeMD` (`cmd/jeff/pickup_cmd.go:211-224`) and `jeff done`'s auto-curation (`cmd/jeff/done_cmd.go:82`) — so a custom persona's observations get curated with an empty persona scope.
   - Without `outcome`/attrs, `jeff stats` (roadmap Phase 3) has no data layer and can never be built.

2. **`jeff work` resumes blind.** `cmd/jeff/work_cmd.go:10-30` just launches `cfg.Agent` with model `""`. It does NOT: regenerate the task CLAUDE.md, inject the latest checkpoint (roadmap "Checkpoint Injection on Resume", `docs/roadmap.md:221-236`), resolve the persona's registered agent, or resolve the persona's registered model. A jenko (opus) task resumed via `jeff work` silently runs on the global default agent with no model and no memory of prior progress.

## What (the goal)

- Define and register all six Phase-1 attributes in gig.
- Persist `persona`, `skills_loaded`, `memory_loaded`, `team_size` at pickup time.
- Read the persona from the gig attr everywhere `detectPersona` is used today (keep `detectPersona` only as a fallback for tasks picked up by older binaries).
- Set `outcome` when a task is closed.
- Make `jeff work` regenerate the task CLAUDE.md with the latest checkpoint injected, and launch the persona's registered agent + model.

## Files to touch

| File | Change |
|---|---|
| `attrs.go` | Add 6 attr constants + register them in `EnsureAttrs` |
| `attrs_test.go` | Extend to assert all 8 attrs get defined |
| `cmd/jeff/crew_cmd.go` | In `pickupTask` (~line 1093): set the new attrs after claim/setup |
| `cmd/jeff/pickup_cmd.go` | `writeTaskClaudeMD` gains a checkpoint section; `refreshTaskClaudeMD` + a new `resolveTaskPersona` helper read the attr first |
| `cmd/jeff/work_cmd.go` | Regenerate CLAUDE.md, resolve persona agent/model, launch with them |
| `cmd/jeff/done_cmd.go` | Use `resolveTaskPersona`; set `outcome` attr before closing |
| `cmd/jeff/pickup_test.go` | New tests for checkpoint section + attr-based persona resolution |
| `docs/roadmap.md` | Mark Level-1 "What's missing" items done (persona memory, repo learnings, checkpoint injection, attrs) |

## Implementation steps

### Step 1 — attrs.go: define the six attributes

Append to the const block in `attrs.go`:

```go
const (
	AttrRepos         = "repos"          // JSON array of repo names for a task
	AttrWorktreeSetup = "worktree_setup" // post-setup script path per repo

	// Phase 1 attributes (docs/roadmap.md "New Gig Attributes").
	AttrPersona        = "persona"         // string: persona used at pickup
	AttrSkillsLoaded   = "skills_loaded"   // object: JSON array of injected skill names
	AttrMemoryLoaded   = "memory_loaded"   // object: JSON array of memory scopes loaded
	AttrTeamSize       = "team_size"       // string: "1" for solo
	AttrOutcome        = "outcome"         // string: close reason ("done", "abandoned", ...)
	AttrRejectionCount = "rejection_count" // string: times a PR was sent back (not yet written)
)
```

In `EnsureAttrs`, extend the slice with:

```go
{AttrPersona, gig.AttrString, "Persona used to work this task"},
{AttrSkillsLoaded, gig.AttrObject, "JSON array of skill names injected at pickup"},
{AttrMemoryLoaded, gig.AttrObject, "JSON array of memory scopes loaded at pickup"},
{AttrTeamSize, gig.AttrString, "Number of agents on this task (1 = solo)"},
{AttrOutcome, gig.AttrString, "Task outcome recorded at close"},
{AttrRejectionCount, gig.AttrString, "How many times the task's PR was rejected"},
```

gig v0.6.2 exposes exactly these APIs (verified): `DefineAttr(key, AttrType, desc)`, `GetAttrDef(key)`, `SetAttr(taskID, key, value string)`, `GetAttr(taskID, key)`. `EnsureAttrs` is already idempotent via the `GetAttrDef` check — the new entries inherit that.

Update `attrs_test.go` to assert all 8 keys exist after `EnsureAttrs` and that calling it twice is still idempotent.

### Step 2 — pickupTask: persist the attrs

In `cmd/jeff/crew_cmd.go`, function `pickupTask` (starts ~line 1093):

1. Right after the existing `store.SetAttr(taskID, jeff.AttrRepos, ...)` call (~line 1130), add:

```go
if personaName != "" {
	if err := store.SetAttr(taskID, jeff.AttrPersona, personaName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: set persona attr: %v\n", err)
	}
}
if err := store.SetAttr(taskID, jeff.AttrTeamSize, "1"); err != nil {
	fmt.Fprintf(os.Stderr, "Warning: set team_size attr: %v\n", err)
}
```

2. `memory_loaded`: build a `[]string` of scopes as they are injected. Persona memory is loaded in `writeTaskClaudeMD` — the simplest non-invasive approach is to compute the same information in `pickupTask` after `writeTaskClaudeMD` succeeds:

```go
var memScopes []string
if personaName != "" {
	if content, _ := memory.LoadPersonaMemory(cfg.Home, personaName); content != "" {
		memScopes = append(memScopes, "persona:"+personaName)
	}
}
for _, r := range repoNames {
	if content, _ := memory.LoadRepoLearnings(cfg.Home, r); content != "" {
		memScopes = append(memScopes, "repo:"+r)
	}
}
if len(memScopes) > 0 {
	if data, err := json.Marshal(memScopes); err == nil {
		if err := store.SetAttr(taskID, jeff.AttrMemoryLoaded, string(data)); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: set memory_loaded attr: %v\n", err)
		}
	}
}
```

(`memory` and `encoding/json` are already imported in `crew_cmd.go` — verify; add if missing.)

3. `skills_loaded`: the skill-injection loop near the end of `pickupTask` (~lines 1252-1275) calls `skill.InjectMatchingTo(...)`, which **already returns `([]string, error)`** — the injected names (`skill/inject.go:131`). Collect the union across agent providers into a `map[string]bool`, then after the loop:

```go
if len(injectedSet) > 0 {
	names := make([]string, 0, len(injectedSet))
	for n := range injectedSet {
		names = append(names, n)
	}
	sort.Strings(names)
	if data, err := json.Marshal(names); err == nil {
		if err := store.SetAttr(taskID, jeff.AttrSkillsLoaded, string(data)); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: set skills_loaded attr: %v\n", err)
		}
	}
}
```

All attr writes are best-effort warnings — pickup must not fail because stats metadata couldn't be written.

### Step 3 — a single persona resolver

In `cmd/jeff/pickup_cmd.go`, add:

```go
// resolveTaskPersona returns the persona for a task: the gig attr written at
// pickup, falling back to CLAUDE.md prefix detection for workspaces created
// by older binaries.
func resolveTaskPersona(store *gig.Store, taskID, taskDir string) string {
	if attr, err := store.GetAttr(taskID, jeff.AttrPersona); err == nil && attr != nil && attr.Value != "" {
		return attr.Value
	}
	return detectPersona(taskDir)
}
```

Replace the two call sites:
- `refreshTaskClaudeMD` (`cmd/jeff/pickup_cmd.go:218`): `personaName := resolveTaskPersona(store, taskID, taskDir)`.
- `done_cmd.go:82`: `personaName := resolveTaskPersona(store, taskID, td.Path)` (the `store` is already open there; note the call currently happens inside `if tdErr == nil` — keep that guard).

Do **not** delete `detectPersona` — it is the documented fallback and has tests.

### Step 4 — checkpoint injection into the task CLAUDE.md

In `cmd/jeff/pickup_cmd.go`:

1. Change `writeTaskClaudeMD` signature to accept the store:

```go
func writeTaskClaudeMD(taskDir, jeffHome string, store *gig.Store, task *gig.Task, personaName string, repos []string) error
```

2. After the "Task context" block (after the `sb.WriteString("\n")` that ends the task fields, ~line 95), insert:

```go
// Latest checkpoint — so resumed sessions start with prior progress.
if store != nil {
	if cp, err := store.LatestCheckpoint(task.ID); err == nil && cp != nil {
		sb.WriteString("## Resuming: Last Checkpoint\n\n")
		sb.WriteString(fmt.Sprintf("_Recorded %s_\n\n", cp.CreatedAt.Format("2006-01-02 15:04")))
		if cp.Done != "" {
			sb.WriteString("- **Done:** " + cp.Done + "\n")
		}
		if cp.Decisions != "" {
			sb.WriteString("- **Decisions:** " + cp.Decisions + "\n")
		}
		if cp.Next != "" {
			sb.WriteString("- **Next:** " + cp.Next + "\n")
		}
		if cp.Blockers != "" {
			sb.WriteString("- **Blockers:** " + cp.Blockers + "\n")
		}
		if len(cp.Files) > 0 {
			sb.WriteString("- **Files touched:** " + strings.Join(cp.Files, ", ") + "\n")
		}
		sb.WriteString("\n")
	}
}
```

The gig `Checkpoint` struct fields are `ID, TaskID, Author, Done, Decisions, Next, Blockers, Files []string, CreatedAt` (verified in gig v0.6.2 `checkpoint.go`). `LatestCheckpoint` returns `(nil, nil)`-style absence — guard both `err` and `cp`.

3. Fix the two callers:
   - `pickupTask` (`cmd/jeff/crew_cmd.go:~1183`): pass the already-open `store`.
   - `refreshTaskClaudeMD` (`cmd/jeff/pickup_cmd.go:223`): pass its `store` argument.
   - `pickup_test.go` callers: pass `nil` for store where no store exists (the `store != nil` guard makes this safe) — or construct a temp gig store where the test asserts checkpoint rendering.

### Step 5 — make `jeff work` resume with context

Rewrite the `RunE` of `workCmd` (`cmd/jeff/work_cmd.go`) to:

```go
taskID, taskDir, err := resolveTaskID(args)
if err != nil {
	return err
}
if taskDir == "" {
	return fmt.Errorf("no workspace found for %s", taskID)
}

store, err := openGigStore()
if err != nil {
	return err
}
defer store.Close()

// Regenerate CLAUDE.md (injects latest checkpoint + current memory/worktrees).
if err := refreshTaskClaudeMD(taskDir, store, taskID); err != nil {
	fmt.Fprintf(os.Stderr, "Warning: refresh task context: %v\n", err)
}

// Resolve persona → agent + model, mirroring pickup.
personaName := resolveTaskPersona(store, taskID, taskDir)
agentTool := cfg.Agent
if personaName != "" {
	if pa := persona.RegisteredAgent(cfg.Home, personaName); pa != "" {
		agentTool = jeff.AgentTool(pa)
	}
}
model := persona.RegisteredModel(cfg.Home, personaName)

fmt.Fprintf(os.Stderr, "Resuming %s in %s...\n", taskID, taskDir)
return launchAgent(taskDir, agentTool, model)
```

Add the imports (`jeff`, `persona`). `persona.RegisteredAgent(jeffHome, name)` and `persona.RegisteredModel(jeffHome, name)` exist in `persona/registry.go:271-286`; both return `""` when unknown, which preserves today's behavior for persona-less tasks.

### Step 6 — record outcome at close

In `cmd/jeff/done_cmd.go`, immediately **before** the `store.CloseTask(taskID, reason, "jeff")` call (~line 113):

```go
if err := store.SetAttr(taskID, jeff.AttrOutcome, reason); err != nil {
	fmt.Fprintf(os.Stderr, "Warning: set outcome attr: %v\n", err)
}
```

The default `--reason` is `"done"`, so the default outcome is `"done"`. Do not invent a shipped/rejected taxonomy here — `PLAN-Ship-Hardening` records PR data, and a future reject flow can overwrite this attr.

### Step 7 — tests

1. `attrs_test.go`: all 8 defined; idempotent on second call.
2. New test in `cmd/jeff/pickup_test.go`: `writeTaskClaudeMD` with a temp gig store that has a checkpoint renders the `## Resuming: Last Checkpoint` section with Done/Next lines; with no checkpoint, the section is absent; with `store == nil`, the section is absent and no panic.
3. New test: `resolveTaskPersona` prefers the attr over `detectPersona`, and falls back to `detectPersona` when the attr is missing (create a CLAUDE.md starting with an embedded persona template to exercise the fallback).
4. Use `internal/testutil` / `t.TempDir()` patterns already present in `pickup_test.go` for gig store setup (grep for `gig.Open` in existing tests and copy the fixture pattern).

### Step 8 — roadmap doc touch-up

In `docs/roadmap.md`, update the Level-1 "What's missing" paragraph (~line 47): persona memory, repo learnings, and checkpoint injection are now done; the attrs list at lines 243-260 is now implemented. One-sentence edits — do not rewrite the file.

## Edge cases you must handle (found during exploration)

- **`writeTaskClaudeMD` is called at initial pickup too.** At pickup there are no checkpoints yet, so `LatestCheckpoint` returns nothing — the guard makes the section naturally absent. Do not special-case pickup vs resume.
- **`gig.Store.GetAttr` returns an error for unset attrs** in some versions and `(nil, nil)` in others — guard **both** `err == nil` and `attr != nil` and `attr.Value != ""` as shown.
- **`detectPersona` only recognizes embedded personas** (`persona.Get`), not registry personas (`persona.GetTemplate`). That is exactly why the attr must be the primary source. Don't "improve" `detectPersona` to scan the registry — the attr supersedes it; the fallback exists only for old workspaces.
- **`crew resume` has its own resume path** (`cmd/jeff/crew_cmd.go:242-329`) that restores agent/model from the crew SQLite DB — do not touch it in this plan; it already persists agent+model per session.
- **`refreshTaskClaudeMD` re-derives repos from worktree symlinks** (`detectRepos`). Keep that — repos can change after pickup via `jeff worktree add`, so the symlinks are fresher than the attr.
- **Import cycles:** `cmd/jeff` already imports `jeff`, `persona`, `memory`, `gig` — no new module edges are created by this plan.
- **`skills_loaded` when no skills match:** don't write an empty array attr; skip (keeps `jeff stats` queries simple: attr present ⇔ skills were injected).
- **Attr writes must never fail the pickup/done flow** — warn-and-continue on every `SetAttr` error (gig store could be read-only or attrs undefined if `EnsureAttrs` failed).
- **`work` on a task with no workspace** must keep failing with the current "no workspace found" error — don't create one.

## Acceptance criteria

Run each and verify:

1. `go build ./... && go vet ./... && go test ./...` — green.
2. `go test ./ -run TestEnsureAttrs -v` — asserts 8 attrs.
3. Manual smoke (uses a throwaway `JEFF_HOME` + gig store):
   ```bash
   export JEFF_HOME=$(mktemp -d) GIG_HOME=$(mktemp -d)
   gig init 2>/dev/null || true   # if gig CLI absent, use the Go test instead
   ```
   If the gig CLI is unavailable in the environment, the Go tests from Step 7 are the acceptance vehicle — they must cover: attr persistence at pickup (call `pickupTask` is not directly testable; test `resolveTaskPersona` + `writeTaskClaudeMD` instead) and checkpoint rendering.
4. Grep checks:
   - `grep -n "AttrPersona" attrs.go cmd/jeff/crew_cmd.go cmd/jeff/pickup_cmd.go cmd/jeff/done_cmd.go` — hits in all four.
   - `grep -n "LatestCheckpoint" cmd/jeff/pickup_cmd.go` — present in `writeTaskClaudeMD`.
   - `grep -n "RegisteredModel\|RegisteredAgent" cmd/jeff/work_cmd.go` — both present.
5. `jeff work` on a checkpoint-bearing task regenerates `CLAUDE.md` containing `## Resuming: Last Checkpoint` (verify in a unit test via `refreshTaskClaudeMD` against a temp store with `AddCheckpoint`).

## Out of scope

- `rejection_count` writers (no reject flow exists yet — the attr is defined for forward-compat only).
- `jeff stats` itself (PLAN-Stats depends on this plan).
- PR URL recording on ship (PLAN-Ship-Hardening).
- Any change to crew resume or the crew SQLite schema.
