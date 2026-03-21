# Configuration

JEFF is configured via `jeff.yaml` in your JEFF_HOME directory.

## jeff.yaml

```yaml
agent: claude                      # agent tool: "claude" or "opencode"
ide: cursor                        # IDE: "vscode", "cursor", "windsurf", "nvim"
gig_home: ""                       # override gig home (empty = default ~/.gig/)
repos:
  backend:
    url: https://github.com/org/backend.git
    base_branch: origin/main       # base branch for PRs (default: origin/main)
    branch_name: scripts/branch.sh # script to generate branch names (optional)
    post_setup: scripts/setup.sh   # script run after worktree creation (optional)
  frontend:
    url: https://github.com/org/frontend.git
    base_branch: origin/develop
hooks:
  gig-instructions: true           # inject gig CLI reference at session start
  gig-ready-tasks: true            # inject `gig ready` output at session start
  jeff-instructions: true          # inject jeff CLI reference at session start
  jeff-repos: true                 # inject repo list at session start
```

## Agent Tool

The agent tool launched by `jeff pickup` and `jeff work`.

```bash
jeff config agent claude     # Claude Code (default)
jeff config agent opencode   # opencode
```

## IDE

Used by `jeff open` to open workspaces.

```bash
jeff config ide cursor       # Cursor
jeff config ide vscode       # VS Code
jeff config ide windsurf     # Windsurf
jeff config ide nvim         # Neovim
```

## Repos

### Registration

```bash
jeff repo add https://github.com/org/backend.git
jeff repo list
jeff repo remove backend
```

### Base Branch

The branch that worktrees are created from and PRs target. Defaults to `origin/main`. When the base references a remote (e.g. `origin/develop`), JEFF runs `git fetch` before creating the worktree.

Set per-repo in `jeff.yaml`:

```yaml
repos:
  backend:
    base_branch: origin/develop
```

Or override per-worktree:

```bash
jeff worktree add backend gig-ab12 --base origin/staging
```

### Branch Naming

By default, worktree branches are named after the task ID (e.g. `gig-ab12`). You can provide a script that receives the full task JSON (including custom attributes) on stdin and outputs the branch name on stdout.

```yaml
repos:
  backend:
    branch_name: scripts/branch-name.sh
```

The script receives JSON like:

```json
{
  "id": "gig-ab12",
  "title": "Add auth flow",
  "type": "feature",
  "priority": 1,
  "attrs": {
    "branch_prefix": "neeraj"
  }
}
```

Example script (`scripts/branch-name.sh`):

```bash
#!/bin/bash
TASK=$(cat)
PREFIX=$(echo "$TASK" | jq -r '.attrs.branch_prefix // empty')
if [ -z "$PREFIX" ]; then
  PREFIX=$(echo "$TASK" | jq -r '.type // "task"')
fi
ID=$(echo "$TASK" | jq -r '.id')
echo "${PREFIX}/${ID}"
```

This produces branches like `neeraj/gig-ab12` or `feature/gig-ab12`.

Any executable works (bash, python, node) — just make sure it has a shebang and is `chmod +x`.

### Post-Setup Script

Runs after a worktree is created. Receives the source repo dir and worktree dir as arguments.

```bash
jeff repo post-setup backend scripts/setup-backend.sh
```

```bash
#!/bin/bash
# scripts/setup-backend.sh
SRC_DIR=$1
DEST_DIR=$2
cp "$SRC_DIR/.env.example" "$DEST_DIR/.env"
cd "$DEST_DIR" && npm install
```

## Hooks

Hooks inject context into agent sessions at startup. They run as SessionStart hooks in Claude Code (bash scripts outputting JSON) or as OpenCode plugins (JS).

### Built-in Hooks

| Hook | What it injects |
|------|----------------|
| `gig-instructions` | gig CLI reference for the agent |
| `gig-ready-tasks` | Output of `gig ready` (tasks available to pick up) |
| `jeff-instructions` | jeff CLI reference for the agent |
| `jeff-repos` | List of registered repos |

All hooks are enabled by default. Disable individually:

```bash
jeff config hooks disable gig-ready-tasks
jeff config hooks enable gig-ready-tasks
jeff config hooks list               # show all hooks and their state
jeff config hooks sync               # re-install hooks (after updates)
```

Or edit `jeff.yaml` directly:

```yaml
hooks:
  gig-ready-tasks: false    # disabled
  # omitted hooks default to enabled
```

### How Hooks Work

For Claude Code, each hook becomes a bash script in `JEFF_HOME/hooks/` wired into `JEFF_HOME/.claude/settings.json`. For OpenCode, all hooks are combined into a single plugin at `JEFF_HOME/.opencode/plugins/jeff-hooks.js`.

## CLAUDE.md

JEFF generates two types of CLAUDE.md:

**Base** (`JEFF_HOME/CLAUDE.md`) — identity and home layout. Written by `jeff init`, can be reset:

```bash
jeff config reset-claude-md          # overwrites with latest template (backs up first)
```

**Task** (`tasks/<id>/CLAUDE.md`) — task context, persona, and workspace layout. Written by `jeff pickup`, auto-refreshed when worktrees are added.

## JEFF_HOME Resolution

Resolution order:

1. `JEFF_HOME` environment variable
2. `~/.config/jeff/home` pointer file
3. `~/.jeff/` default

Use `--here` with `jeff init` to set the current directory as JEFF_HOME.
