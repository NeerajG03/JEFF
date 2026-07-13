# PLAN-Ship-Hardening: Truthful Exit Codes, Outcome Recording, and No More Leaked Metadata

> **Rank:** 4/10 · **Leverage:** High (shipping is the money path; failures are currently invisible to automation) · **Effort:** Small-Medium · **Prereqs:** none (pairs well after PLAN-Phase1-Attrs-Resume, which defines the attr constants — if that plan is not done yet, add `AttrPRURLs` standalone as described below)
>
> Ground rules: run `go build ./... && go vet ./... && go test ./...` after each step group.

## Why (the problem)

1. **`jeff ship` always exits 0.** The per-worktree loop (`cmd/jeff/ship_cmd.go:72-122`) warns-and-continues on push failures (`:101-102`) and PR-creation failures (`:116-118`), then `return nil` (`:125`). A crew orchestrator, a script, or CI calling `jeff ship` cannot detect a partially-shipped task (repo A pushed + PR'd, repo B still local).
2. **Nothing is recorded back into gig.** After shipping, the task carries no PR URLs, no shipped-at marker — `jeff status`, the dashboard, and the future `jeff stats` can't tell shipped tasks from unshipped ones. The roadmap's approve/reject loop (Phase 5) needs this data.
3. **`.jeff-base` leaks into PRs.** `writeBaseBranch` writes `.jeff-base` into the worktree root (`workspace/worktree.go:92-109`), it is not in `.gitignore` and not in `.git/info/exclude` — an agent running `git add -A` commits JEFF-internal metadata, and `jeff ship` pushes it into the user's PR.
4. **Uncommitted work ships silently missing.** Ship never checks the worktree for dirty state; the branch pushes without the user's latest edits and nobody notices until PR review.
5. **`gh` is assumed.** `prExists`/`createPR` shell out to `gh` (`ship_cmd.go:262-290`) with no upfront availability/auth check — failures surface as one cryptic warning per repo.

## What (the goal)

- Ship exits non-zero if any worktree fails to push or PR; summary distinguishes shipped / skipped / failed.
- PR URLs and ship status recorded in gig (attr + comment) — visible to stats and the orchestrator.
- `.jeff-base` never appears in `git status` (excluded at worktree creation, and self-healed for existing worktrees at ship time).
- Dirty worktrees produce a loud warning listing uncommitted paths.
- Missing `gh` fails fast with an actionable message.

## Files to touch

| File | Change |
|---|---|
| `cmd/jeff/ship_cmd.go` | Error aggregation + exit code; dirty check; `gh` preflight; record results in gig |
| `attrs.go` | Add `AttrPRURLs = "pr_urls"` (AttrObject) + register in `EnsureAttrs` |
| `workspace/worktree.go` | Exclude `.jeff-base` via `.git/info/exclude` at creation; helper `EnsureExcluded` |
| `workspace/worktree_test.go` | Tests for the exclude helper |
| `cmd/jeff/ship_test.go` | Tests for aggregation/dirty/record logic |
| `docs/usage.md` | Document `--body`, exit-code semantics |

## Implementation steps

### Step 1 — attr for PR URLs

In `attrs.go` add `AttrPRURLs = "pr_urls"` (`gig.AttrObject`, "JSON object mapping repo name → PR URL") and register it in `EnsureAttrs`. (If PLAN-Phase1-Attrs-Resume already restructured this file, just append one entry.)

### Step 2 — `.jeff-base` exclusion

In `workspace/worktree.go`:

```go
// ensureExcluded appends pattern to the worktree's local git exclude file if absent.
// Worktrees have their own info/exclude under the worktree gitdir.
func ensureExcluded(wtDir, pattern string) error {
	out, err := gitutil.Output(wtDir, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return err
	}
	excludePath := strings.TrimSpace(string(out))
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(wtDir, excludePath)
	}
	data, _ := os.ReadFile(excludePath)
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == pattern {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(pattern + "\n")
	return err
}
```

Call `ensureExcluded(wtDir, ".jeff-base")` right after `writeBaseBranch` succeeds inside `WorktreeAdd` (~line 72). **Must use `git rev-parse --git-path info/exclude`** — for linked worktrees the gitdir lives under the main repo's `.git/worktrees/<name>/`, and `--git-path` resolves it correctly (a hardcoded `.git/info/exclude` path is wrong for worktrees; `.git` in a worktree is a *file*, not a directory).

Self-heal at ship time: in `discoverWorktrees` (`ship_cmd.go:149-197`), after resolving `target`, call `workspace.EnsureJeffBaseExcluded(target)` (export a thin wrapper) and ignore its error with a warning — existing worktrees created by older binaries get fixed on their next ship.

### Step 3 — ship result aggregation

Rework the loop in `ship_cmd.go` RunE:

1. Introduce a result struct:

```go
type shipResult struct {
	repo   string
	prURL  string
	err    error
	dirty  int // count of uncommitted paths
	pushed bool
}
```

2. Dirty check per worktree before pushing:

```go
out, err := gitutil.Output(wt.wtDir, "status", "--porcelain")
if err == nil && len(bytes.TrimSpace(out)) > 0 {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	res.dirty = len(lines)
	fmt.Fprintf(os.Stderr, "  WARNING: %d uncommitted change(s) will NOT be shipped:\n", len(lines))
	for i, l := range lines {
		if i == 5 { fmt.Fprintf(os.Stderr, "    …and %d more\n", len(lines)-5); break }
		fmt.Fprintf(os.Stderr, "    %s\n", l)
	}
}
```

3. On push failure or PR failure, set `res.err` and continue to the next worktree (current behavior, but recorded).
4. After the loop:

```go
var failed []string
for _, r := range results {
	if r.err != nil {
		failed = append(failed, fmt.Sprintf("%s: %v", r.repo, r.err))
	}
}
fmt.Fprintf(os.Stderr, "\nShipped %d, skipped %d, failed %d\n", shipped, skipped, len(failed))
if len(failed) > 0 {
	return fmt.Errorf("ship incomplete:\n  %s", strings.Join(failed, "\n  "))
}
```

`--dry-run` must keep returning nil and must skip the gig-recording step below.

### Step 4 — record results in gig

After the loop (not in dry-run), collect `map[string]string{repo: prURL}` for every worktree that has a PR URL (newly created **or** pre-existing via `prExists`) and:

```go
if len(prURLs) > 0 {
	if data, err := json.Marshal(prURLs); err == nil {
		if err := store.SetAttr(taskID, jeff.AttrPRURLs, string(data)); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: record PR URLs: %v\n", err)
		}
	}
	var lines []string
	for repo, url := range prURLs {
		lines = append(lines, fmt.Sprintf("%s: %s", repo, url))
	}
	sort.Strings(lines)
	if _, err := store.AddComment(taskID, "jeff", "Shipped:\n"+strings.Join(lines, "\n")); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: record ship comment: %v\n", err)
	}
}
```

`store.AddComment(taskID, author, content)` exists in gig v0.6.2 (verified). Call `jeff.EnsureAttrs(store)` before `SetAttr` in case ship runs before any pickup on this store defined the new attr (cheap, idempotent).

Only add the comment when at least one PR URL is **new** this run — otherwise re-running `jeff ship` on an already-shipped task spams a duplicate comment. Track `newPR bool` per result (set only in the `createPR` success branch).

### Step 5 — `gh` preflight

At the top of RunE (before the worktree loop), when not `--dry-run`:

```go
if _, err := exec.LookPath("gh"); err != nil {
	return fmt.Errorf("gh CLI not found — jeff ship needs it to create PRs.\nInstall: https://cli.github.com, then run 'gh auth login'.\n(Use --dry-run to preview without gh.)")
}
```

Do NOT preflight `gh auth status` (it's slow and needs network); per-repo failures already surface auth errors from `createPR`'s captured stderr (`ship_cmd.go:283-286`).

### Step 6 — tests

`cmd/jeff/ship_test.go` already builds worktree fixtures with real git (`ship_test.go` uses `git init` fixtures — follow its pattern):

1. **Dirty detection:** create a fixture worktree with an uncommitted file → the dirty count is reported (extract the dirty-check into a helper `countDirty(wtDir) (int, []string)` so it's unit-testable without running the whole RunE).
2. **Aggregation:** unit-test a `summarizeShip(results []shipResult) (string, error)` helper: all-ok → nil error; one failed → error mentions the repo.
3. **Exclude helper:** `workspace/worktree_test.go`: `ensureExcluded` on a plain `git init` repo appends once, is idempotent, and `git status --porcelain` no longer lists a created `.jeff-base` file. For a **linked worktree** fixture (`git worktree add`), assert the exclude lands under `.git/worktrees/<name>/info/exclude` (via `rev-parse --git-path`).
4. Keep `buildPRTitle`/`buildPRBody` tests green.

### Step 7 — docs

`docs/usage.md` ship section: document `--body` (exists but undocumented), the new exit-code behavior, and the "Shipped/skipped/failed" summary line.

## Edge cases you must handle (found during exploration)

- **Linked-worktree gitdir layout:** `.git` inside a worktree is a file (`gitdir: /path/.git/worktrees/x`) — `rev-parse --git-path info/exclude` is the only correct way to find the exclude file. Hardcoding `<wt>/.git/info/exclude` writes into a nonexistent directory or the wrong repo.
- **`hasUnpushedCommits` returns `true` on error by design** (`ship_cmd.go:247-254` — no upstream yet means everything is unpushed). Keep that; do not "fix" it to return the error.
- **Pre-existing PRs must count as shipped, not failed** (`prExists` path at `:107-112`) — re-running ship after a partial failure should converge to success (idempotent resume is ship's best property today; preserve it).
- **`--repo` filter + zero matches** already returns an error (`:192-194`) — don't regress it.
- **Base branch `origin/` stripping** (`:178-182`) happens before PR creation; the recorded PR URL comes from `gh` output, so no change needed there.
- **A task might be shipped from outside a task dir** via explicit ID; `resolveTaskID` returns `taskDir == ""` → currently errors at `:35-37`. Unchanged.
- **`.jeff-base` already committed in an old branch:** the exclude only hides *untracked* files. If a repo already has `.jeff-base` tracked, `git status` stays clean but the file is in the PR. Add a ship-time check: `git ls-files --error-unmatch .jeff-base` succeeding ⇒ print a one-line warning telling the user to `git rm --cached .jeff-base`. Do not auto-remove (that would create an unexpected commit).
- **Duplicate ship comments:** only comment when a *new* PR was created this run (Step 4).
- **`jeff.EnsureAttrs` needs the store, which is already open** in RunE — reuse it; don't open a second store handle (SQLite single-writer).

## Acceptance criteria

1. `go build ./... && go vet ./... && go test ./...` green.
2. `go test ./cmd/jeff/ -run 'TestShip|TestEnsureExcluded|TestSummarizeShip|TestCountDirty' -v` — new tests pass.
3. Grep checks:
   - `grep -n "AttrPRURLs" attrs.go cmd/jeff/ship_cmd.go` — both present.
   - `grep -n "status\", \"--porcelain" cmd/jeff/ship_cmd.go` (or the helper name) — dirty check present.
   - `grep -n "return nil" cmd/jeff/ship_cmd.go` — the unconditional final `return nil` after the loop is gone (dry-run path may keep one).
   - `grep -n "git-path" workspace/worktree.go` — exclude resolution present.
4. Behavioral: in a scratch repo fixture (see `ship_test.go` fixtures), a worktree created via `workspace.WorktreeAdd` has `.jeff-base` on disk but `git status --porcelain` output is empty.
5. Exit code: `summarizeShip` unit test proves non-nil error on any failed result.

## Out of scope

- A `jeff reject`/`jeff approve` flow (roadmap Phase 5).
- Setting `AttrOutcome` (done in PLAN-Phase1-Attrs-Resume via `jeff done`).
- Moving ship logic into a package (that's PLAN-Pickup-Rollback's pattern; ship can follow later).
- PR body templates / `.github/PULL_REQUEST_TEMPLATE.md` support (worth doing someday; not this plan).
