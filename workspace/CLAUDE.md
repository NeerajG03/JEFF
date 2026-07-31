# workspace/ — Task directories and git worktrees

Creates task workspaces under `$JEFF_HOME/tasks/` and git worktrees under `$JEFF_HOME/worktrees/<repo>/<branch>/`, symlinked into task dirs.

## File roles

| File | What it does |
|------|-------------|
| `workspace.go` | TaskDir struct, Create/Open/Remove/List, makeSlug |
| `worktree.go` | WorktreeAdd/Remove/List, symlinkIntoTask, ReadBaseBranch |
| `branchname.go` | RunBranchNameScript — sends task JSON on stdin |

## Key behaviors

- Slug format: `gig-<id>-<title-slug>` (e.g., `gig-ab12-refactor-auth`)
- Worktree location: `$JEFF_HOME/worktrees/<repo>/<branch>/`
- `.jeff-base`: written in worktree, read by `jeff ship` for base branch
- Remote fetch before branching when base branch is `origin/*`
- Branch naming script receives full task JSON on stdin (including custom attrs)
- TaskDir is ephemeral — gig is the source of truth, not the filesystem

## Retirement (`retire.go`)

`jeff done` **retires** a task workspace rather than deleting it, because the dir
holds the live session's cwd *and* its hook scripts *and* the `settings.json`
naming them absolutely — deleting it broke every subsequent hook and Bash spawn in
that session (#94). Worktrees are the disk cost (~200 MB–1 GB) and are still
removed eagerly; a task dir is ~20 KB.

| Function | Use |
|---|---|
| `Retire(dir, taskID, reason)` | drop dangling repo symlinks, write `.closed` |
| `ReadClosedMarker` / `IsRetired` | the cheap offline "is this finished work?" test |
| `Unretire(dir)` | called by `Create` — a reopened task reuses its retired dir |
| `ListActive` | `List` minus retired dirs; use for anything showing current work |
| `PathContains` / `CwdInside` | symlink-resolving containment (task dirs are reached via symlinks, and macOS adds `/var`→`/private/var`) |
| `DirSize` | what `jeff cleanup` reports as reclaimed |

Collection lives in `task/gc.go` (`jeff cleanup`), not here — it needs gig and crew
state to know what is safe to remove.
