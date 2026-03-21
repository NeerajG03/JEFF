# JEFF

JEFF is an agent workspace manager. `gig` is the source of truth for all task state. `jeff` manages worktrees, task workspaces, and agent setup.

## Home

```
{{.Home}}
├── jeff.yaml    — config
├── repos/       — registered codebases
├── tasks/       — active task workspaces
├── worktrees/   — git worktrees (symlinked into task dirs)
└── exports/     — generated artifacts
```
