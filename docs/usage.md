# Usage

## Task Lifecycle

### Initialize

```bash
jeff init                # first-time setup at ~/.jeff/
jeff init --here         # initialize in current directory
jeff init --update       # sync existing home (refreshes skills alias, hooks, settings)
```

### Register Codebases

```bash
jeff repo add https://github.com/org/backend.git
jeff repo add https://github.com/org/frontend.git --name frontend
jeff repo list
jeff repo remove backend
jeff repo remove backend --delete    # also delete cloned files
jeff repo sync                       # pull latest for all repos
jeff repo sync --repo backend        # pull latest for one repo
```

### Pick Up a Task

```bash
jeff pickup gig-ab12                              # minimal
jeff pickup gig-ab12 --persona jenko               # with persona
jeff pickup gig-ab12 --persona jenko --repos backend,frontend  # with worktrees
```

This claims the task in gig, creates a workspace, sets up worktrees, injects matching skills, writes CLAUDE.md, and launches the agent.

### Work and Checkpoint

```bash
jeff work gig-ab12                    # resume work (launches agent in task dir)
jeff work                             # auto-detect from cwd

jeff checkpoint --done "Implemented auth" --next "Add tests"
jeff checkpoint --done "Fixed bug" --decisions "Used retry pattern" --blockers "Waiting on API"
jeff checkpoint --task gig-ab12 --done "..."   # explicit task ID
```

### Ship

```bash
jeff ship                    # push all worktree branches, create PRs
jeff ship --repo backend     # only one repo
jeff ship --draft            # create draft PRs
jeff ship --dry-run          # show what would happen
jeff ship --title "Custom"   # override PR title
```

### Complete

```bash
jeff done gig-ab12                    # close task
jeff done gig-ab12 --reason "shipped" # with reason
jeff done                             # auto-detect from cwd
```

### Status

```bash
jeff status            # show active tasks only
jeff status --all      # include completed/stale workspaces
```

## Configuration

### Agent Tool

```bash
jeff config agent              # show current
jeff config agent claude       # set to Claude Code
jeff config agent opencode     # set to opencode
```

### IDE

```bash
jeff config ide                # show current
jeff config ide cursor         # set to Cursor
jeff config ide vscode         # VS Code
jeff config ide windsurf       # Windsurf
jeff config ide nvim           # Neovim
```

### Hooks

Four built-in hooks inject context at agent session start:

| Hook | Injects |
|------|---------|
| `gig-instructions` | gig CLI reference for the agent |
| `gig-ready-tasks` | Tasks available for pickup (`gig ready` output) |
| `jeff-instructions` | jeff CLI reference for the agent |
| `jeff-repos` | List of registered repos |

```bash
jeff config hooks list                 # show all hooks and state
jeff config hooks enable jeff-repos    # enable a hook
jeff config hooks disable gig-ready-tasks  # disable a hook
jeff config hooks sync                 # re-sync hooks to disk
```

### CLAUDE.md

```bash
jeff config reset-claude-md    # regenerate from latest template (backs up existing)
```

## Worktrees

### Base Branch

Worktrees branch from `origin/main` by default. Override per-repo in `jeff.json`:

```json
{
  "repos": {
    "backend": {
      "base_branch": "origin/develop"
    }
  }
}
```

Or per-worktree:

```bash
jeff worktree add backend gig-ab12 --base origin/staging
```

When the base references a remote (e.g. `origin/develop`), JEFF runs `git fetch` before branching.

### Branch Naming

By default, branches are named after the task ID. Provide a script that receives the full task JSON on stdin and outputs the branch name:

```json
{
  "repos": {
    "backend": {
      "branch_name": "scripts/branch.sh"
    }
  }
}
```

Example script:

```bash
#!/bin/bash
TASK=$(cat)
TYPE=$(echo "$TASK" | jq -r '.type')
ID=$(echo "$TASK" | jq -r '.id')
echo "${TYPE}/${ID}"
# outputs: feature/gig-ab12
```

The task JSON includes id, title, type, priority, labels, and custom attributes.

### Post-Setup Scripts

Run after worktree creation. Receives JSON on stdin with `src_dir`, `dest_dir`, `repo`, and `branch`:

```json
{
  "repos": {
    "backend": {
      "post_setup": "scripts/setup.sh"
    }
  }
}
```

```bash
#!/bin/bash
CTX=$(cat)
SRC=$(echo "$CTX" | jq -r '.src_dir')
DEST=$(echo "$CTX" | jq -r '.dest_dir')
cp "$SRC/.env.example" "$DEST/.env"
cd "$DEST" && npm install
```

## Skills

### Add and Tag

```bash
jeff skill add ./my-skill                          # copy into .skills/
jeff skill add ~/shared/deploy --external          # register without copying
jeff skill add ./review --name pr-review           # custom name

jeff skill tag pr-review --persona hardy           # auto-inject for hardy persona
jeff skill tag deploy --type chore,feature         # auto-inject for chore/feature tasks
jeff skill tag aws --tag aws,infra                 # auto-inject when task has these labels
```

### Browse

```bash
jeff skill list                        # all skills
jeff skill list --persona jenko         # filtered
jeff skill list --type bug             # filtered
jeff skill show deploy                 # details + SKILL.md preview
jeff skill doc                         # full skill management guide
```

### Inject and Eject

```bash
jeff skill inject slack notion         # inject into JEFF home (all sessions)
jeff skill eject slack                 # remove from JEFF home
jeff skill inject deploy --task gig-ab12  # inject into specific task
```

### Auto-Injection

On `jeff pickup`, skills matching any of these dimensions are auto-injected:

- **personas** — pickup persona matches
- **gig_type** — task type matches
- **tags** — any tag intersects with task labels

Empty dimensions are ignored. All-empty = manual inject only.

## Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `JEFF_PERSONA` | Active persona name | (auto-exported in crew sessions) |
| `JEFF_TASK_ID` | Active task ID | (auto-exported in crew sessions) |
| `JEFF_HOME` | JEFF workspace location | `~/.config/jeff/home` pointer, then `~/.jeff/` |
| `GIG_HOME` | gig task store location | gig default (`~/.gig/`) |

## Opening Workspaces

```bash
jeff open                  # open JEFF home in IDE
jeff open gig-ab12         # open task workspace in IDE
```

Uses the IDE configured via `jeff config ide`.

## Multi-Agent Crews

Run multiple AI agents in parallel, each in its own tmux window with dedicated task workspace.

### Start an orchestrator

```bash
jeff orchestrator start --name work     # creates tmux session jeff-work
```

### Launch workers

```bash
# Start workers on tasks (each gets its own workspace + worktrees)
jeff crew start gig-ab12 "Fix the issue" --persona jenko --repos backend
jeff crew start gig-cd34 "Fix the issue" --persona eric --repos backend,frontend
jeff crew start gig-ef56 "Fix the issue" --persona hardy --repos backend --model opus
```

### Monitor and communicate

```bash
jeff crew list                          # show workers (filtered to current orchestrator)
jeff crew status gig-ab12               # detailed worker status + pane output
jeff crew events --since 5m             # recent activity across all workers
jeff crew capture gig-ab12 --lines 30   # raw terminal output

# Message workers (4 types, lightest to heaviest)
jeff crew send gig-ab12 "add error handling" --type nudge     # low context impact
jeff crew send gig-ab12 "what are you working on?" --type status  # sidechain, no pollution
jeff crew send gig-ab12 "API spec changed" --type normal      # full conversation turn
jeff crew send gig-ab12 "stop, focus on payments" --type divert  # interrupts agent

# Workers can ask the orchestrator questions
jeff crew ask "should I use JWT or session tokens?"
jeff crew ack <msg-id> "use JWT"
```

### Manage lifecycle

```bash
jeff crew resume gig-ab12               # resume stopped worker (restores Claude session)
jeff crew stop gig-ab12                 # graceful stop
jeff crew stop --all                    # stop all workers
jeff crew cleanup                       # reconcile tmux vs DB state
jeff orchestrator info                  # show all tasks under orchestrator
jeff orchestrator stop jeff-work        # stop orchestrator + all workers
```
