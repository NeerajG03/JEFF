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
