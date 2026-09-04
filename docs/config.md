# Configuration

JEFF is configured via `jeff.json` in your JEFF_HOME directory. Editors get autocompletion via the `$schema` field.

## jeff.json

```json
{
  "$schema": "https://raw.githubusercontent.com/NeerajG03/JEFF/main/schemas/jeff-config.json",
  "agent": "claude",
  "ide": "cursor",
  "gig_home": "",
  "repos": {
    "backend": {
      "url": "https://github.com/org/backend.git",
      "base_branch": "origin/main",
      "branch_name": "scripts/branch.sh",
      "post_setup": "scripts/setup.sh"
    },
    "frontend": {
      "url": "https://github.com/org/frontend.git",
      "base_branch": "origin/develop"
    }
  },
  "hooks": {
    "gig-instructions": true,
    "gig-ready-tasks": true,
    "jeff-instructions": true,
    "jeff-repos": true,
    "task-context": true,
    "task-commands": true,
    "checkpoint-nudge": true
  },
  "checkpoint_patterns": ["git commit", "go test.*PASS", "npm test"],
  "skip_permissions": true
}
```

> **Migration**: If you have an existing `jeff.yaml`, JEFF auto-migrates it to `jeff.json` on first run.

## Agent Tool

The agent tool launched by `jeff pickup` and `jeff work`.

```bash
jeff config agent claude     # Claude Code (default)
jeff config agent opencode   # opencode
jeff config agent gemini     # Gemini CLI
jeff config agent codex      # OpenAI Codex

You can also configure the default agent tool per persona in `personas.json` within your JEFF_HOME.
```

## IDE

Used by `jeff open` to open workspaces.

```bash
jeff config ide cursor       # Cursor
jeff config ide vscode       # VS Code
jeff config ide windsurf     # Windsurf
jeff config ide nvim         # Neovim
```

## Permissions

By default, every agent JEFF launches (`jeff pickup`, `jeff work`, `jeff crew start`) runs with its native permission prompts disabled (Claude's `--dangerously-skip-permissions`, Gemini's `--approval-mode=yolo`, OpenCode's `--auto`, Codex's `--sandbox workspace-write --dangerously-bypass-hook-trust`) — this is the current, unchanged default.

Set `skip_permissions: false` in `jeff.json` to keep permission prompts enabled instead:

```json
{
  "skip_permissions": false
}
```

Or override per invocation with `--safe`, regardless of the config value:

```bash
jeff pickup gig-42 --safe
jeff work gig-42 --safe
jeff crew start gig-42 "..." --safe
```

Precedence: `--safe` > `jeff.json` `skip_permissions` > default (`true`).

Non-interactive/piped runs (curation, `jeff memory curate`) always skip permissions regardless of this setting — a headless run cannot answer a prompt. A `jeff crew start --safe` worker pauses on the first permission prompt until someone attaches with `jeff crew attach <id>`.

## Repos

### Registration

```bash
jeff repo add https://github.com/org/backend.git
jeff repo list
jeff repo remove backend
```

### Base Branch

The branch that worktrees are created from and PRs target. Defaults to `origin/main`. When the base references a remote (e.g. `origin/develop`), JEFF runs `git fetch` before creating the worktree.

Set per-repo in `jeff.json`:

```json
{
  "repos": {
    "backend": {
      "base_branch": "origin/develop"
    }
  }
}
```

Or override per-worktree:

```bash
jeff worktree add backend gig-ab12 --base origin/staging
```

### Removal Safety

`jeff done` and `jeff worktree rm` refuse to remove a worktree that has uncommitted changes (`git status --porcelain` is non-empty) — they print the dirty paths and abort instead of silently discarding work. Pass `--force` to discard anyway:

```bash
jeff done gig-ab12 --force
jeff worktree rm backend gig-ab12 --force
```

`jeff done` checks every repo attached to the task before removing any of them, so a dirty repo never leaves the task half-torn-down. After any removal, `git worktree prune` runs in the repo so a future `jeff worktree add` at the same path doesn't fail with "already registered".

### Branch Naming

By default, worktree branches are named after the task ID (e.g. `gig-ab12`). You can provide a script that receives the full task JSON (including custom attributes) on stdin and outputs the branch name on stdout.

```json
{
  "repos": {
    "backend": {
      "branch_name": "scripts/branch-name.sh"
    }
  }
}
```

The script receives JSON like:

```json
{
  "id": "gig-ab12",
  "title": "Add auth flow",
  "type": "feature",
  "priority": 1,
  "repo": "backend",
  "base_branch": "origin/production",
  "attrs": {
    "branch_prefix": "neeraj"
  }
}
```

Example script (`scripts/branch-name.sh`):

```bash
#!/bin/bash
TASK=$(cat)
BASE=$(echo "$TASK" | jq -r '.base_branch // empty')
if [[ "$BASE" == *"production"* || "$BASE" == *"release"* ]]; then
  PREFIX="hotfix"
else
  PREFIX=$(echo "$TASK" | jq -r '.attrs.branch_prefix // .type // "task"')
fi
ID=$(echo "$TASK" | jq -r '.id')
echo "${PREFIX}/${ID}"
```

This produces branches like `neeraj/gig-ab12` or `feature/gig-ab12`.

Any executable works (bash, python, node) — just make sure it has a shebang and is `chmod +x`.

### Post-Setup Script

Runs after a worktree is created. The script runs from the worktree directory
and receives a JSON object on **stdin**:
`{"src_dir": "<repo clone>", "dest_dir": "<worktree>", "repo": "<name>", "branch": "<branch>"}`

```bash
jeff repo post-setup backend scripts/setup-backend.sh
```

```bash
#!/usr/bin/env bash
# scripts/setup-backend.sh
payload=$(cat)
src_dir=$(echo "$payload" | jq -r .src_dir)
dest_dir=$(echo "$payload" | jq -r .dest_dir)
cp "$src_dir/.env.example" "$dest_dir/.env"
cd "$dest_dir" && npm install
```

## Hooks

Hooks inject context into agent sessions at startup. They run as SessionStart hooks in Claude Code (bash scripts outputting JSON) or as OpenCode plugins (JS).

### Built-in Hooks

**Home hooks** — installed at JEFF_HOME level by `jeff init`:

| Hook | What it injects |
|------|----------------|
| `gig-instructions` | gig CLI reference for the agent |
| `gig-ready-tasks` | Output of `gig ready` (tasks available to pick up) |
| `jeff-instructions` | jeff CLI reference for the agent |
| `jeff-repos` | List of registered repos |

**Task hooks** — installed in task directories by `jeff pickup`:

| Hook | Event | What it does |
|------|-------|-------------|
| `task-context` | SessionStart | Injects `gig show <task-id>` — task details, status, checkpoints |
| `task-commands` | SessionStart | Task-dir commands and good practices (checkpoint, ship, done) |
| `checkpoint-nudge` | PostToolUse/Bash | Matches commands against `checkpoint_patterns`, nudges agent to checkpoint |

All hooks are enabled by default. Disable individually:

```bash
jeff config hooks disable gig-ready-tasks
jeff config hooks enable gig-ready-tasks
jeff config hooks list               # show all hooks and their state
jeff config hooks sync               # re-install hooks (after updates)
```

Or edit `jeff.json` directly:

```json
{
  "hooks": {
    "gig-ready-tasks": false
  }
}
```

### Checkpoint Patterns

The `checkpoint-nudge` hook fires after Bash tool use and checks the command against regex patterns defined in `checkpoint_patterns`. When a match is found, the agent is reminded to run `jeff checkpoint`.

```json
{
  "checkpoint_patterns": ["git commit", "go test.*PASS", "npm test", "pytest"]
}
```

Patterns use extended regex (ERE) syntax. If `checkpoint_patterns` is empty or omitted, the hook is a no-op.

JEFF unconditionally aliases `.gemini/skills` to `.claude/skills` during `jeff init --update` and `jeff pickup` to ensure skill parity across agent tools.

### How Hooks Work

Home hooks: each becomes a bash script in `JEFF_HOME/hooks/` wired into `JEFF_HOME/.claude/settings.json`. Task hooks: scripts go into `tasks/<id>/hooks/` wired into `tasks/<id>/.claude/settings.json`. For OpenCode, all hooks are combined into a single plugin.

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
