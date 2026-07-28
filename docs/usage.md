# Usage

## Task Lifecycle

### Initialize

```bash
jeff init                # first-time setup at ~/.jeff/
jeff init --here         # initialize in current directory
jeff init --update       # sync existing home (refreshes skills alias, hooks, settings)
```

Creates `repos/`, `tasks/`, `worktrees/`, `exports/`, `scripts/`, `projects/`, `.skills/`, `.personas/`, and `memory/` under the JEFF home.

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
jeff ship --body "Custom PR body"  # override PR body
```

Requires `gh` (the GitHub CLI) on `PATH`, unless `--dry-run` is set — `jeff ship`
fails fast at startup rather than surfacing a cryptic per-repo error later.

Each worktree is checked for uncommitted changes before pushing; any found are
printed as a warning (they are **not** shipped) so nothing is silently left behind.

`jeff ship` exits non-zero if any worktree fails to push or create a PR, and
prints a `Shipped X, skipped Y, failed Z` summary line. A pre-existing PR for
an already-pushed branch counts as shipped, not failed — re-running `jeff ship`
after a partial failure converges to success. `--dry-run` always exits 0 and
does not record anything back into gig.

On success, PR URLs are recorded on the task: a `pr_urls` attribute
(repo → PR URL JSON) plus a `Shipped: ...` comment the first time a PR is
created for each repo (re-running ship on an already-shipped task doesn't
spam duplicate comments).

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

### Stats

Query-time statistics and observability computed from gig task events and attributes.

```bash
jeff stats                         # show stats for the last 30 days
jeff stats --since 7d              # filter by last 7 days (also accepts Nd, e.g., 14d)
jeff stats --persona jenko         # filter by persona
jeff stats --repo backend          # filter by repo
jeff stats --outcome done          # filter by outcome
jeff stats --json                  # output raw JSON data
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

## Memory Management

JEFF manages a persistent memory store under `JEFF_HOME/memory/`. Workers propose
new memories during sessions; the marlowe curator consolidates them periodically.

**Promotion is human-triggered.** Workers write proposals via `jeff memory propose`;
nothing auto-promotes to canonical. A human (or an orchestrator) runs
`jeff memory curate` to invoke marlowe, who reviews each proposal against the
curation rubric, deduplicates, resolves conflicts, and writes enriched canonical
entries. This single-writer pattern (MINJA, arXiv:2503.03704) is a security
measure — auto-promotion enables memory injection attacks. Run `jeff memory status`
to see pending proposals, queue depth, and the last curation time.

```bash
# Propose a new memory (for workers — writes to proposals/ for later curation)
jeff memory propose --name <slug> --type <user|feedback|project|reference> \
                    --description "<summary>" --body "<details>"

# List canonical memory entries
jeff memory list [--scope persona:jenko] [--bucket semantic] [--status accepted]

# Show a full memory entry
jeff memory show <name|path>

# Curate proposals into canonical memory (marlowe only — JEFF_MEMORY_CAN_ADD=1)
jeff memory curate [--persona <p>]

# Show memory subsystem status (proposals pending, last curation, counts)
jeff memory status

# Add a canonical entry directly (curator only)
jeff memory add --name <slug> --type <t> --description "<summary>" \
                --body "<body>" --scope <scope> --bucket <bucket>

# Supersede an existing entry (keeps audit trail)
jeff memory add --supersede <old-path> --name <slug> ...

# Disable memory subsystem (advisory — skips addendum and propose)
jeff memory disable [--confirm]
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

### Establish an orchestrator identity (required once per project)

Every worker binds to an **orchestrator identity** — a stable id that scopes
`crew list`, routes stop-notifications and worker→orchestrator asks, and drives
stall detection. Create one per project (or a machine-wide default) before
starting workers:

```bash
jeff orchestrator init                  # writes .jeff/orchestrator.json in the current dir
jeff orchestrator init --name jeff-DM20 # override the human-readable name
jeff orchestrator init --global         # machine-wide default (~/.jeff/default-orchestrator.json)
jeff orchestrator init --force          # overwrite an existing identity file
```

The identity is resolved, in order, from: `$JEFF_ORCHESTRATOR_ID` → `.jeff/orchestrator.json`
in the current dir → the same file in a parent dir (walking up to `$HOME`) → the
global default. This is decoupled from tmux, so it works in Cursor, VS Code, a
plain terminal, or CI. `jeff crew start` **fails loud** if no identity is found
rather than silently stranding workers.

If you run `jeff orchestrator init` inside a tmux pane that already hosts a
`jeff orchestrator start` session, it offers to adopt that orchestrator's id so
already-running workers keep their binding.

### Start an orchestrator (optional tmux enhancement)

```bash
jeff orchestrator start --name work     # creates tmux session jeff-work
```

When the orchestrator runs in its own tmux session (or the identity records a
`tmux_pane`), workers signal it via direct pane notifications. Without a tmux
binding, workers still record state to the DB and the orchestrator picks it up
on its next `jeff crew events` poll.

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

# Message workers (stored in inbox + delivered to pane)
jeff crew send gig-ab12 "add error handling"
jeff crew send gig-ab12 "API spec changed"
jeff crew send gig-ab12 "stop, focus on payments" --interrupt  # Ctrl-C first

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
