# JEFF

Agent workspace manager built on [gig](https://github.com/NeerajG03/gig). JEFF gives AI agents structured workspaces, personas, skills, and lifecycle management for task-driven development — from solo tasks to multi-agent crews.

## Install

### Homebrew (macOS/Linux)

```bash
brew install NeerajG03/tap/jeff
```

### Go

```bash
go install github.com/NeerajG03/JEFF/cmd/jeff@latest
```

### From source

```bash
git clone https://github.com/NeerajG03/JEFF.git
cd JEFF && go build -o jeff ./cmd/jeff/
```

## Quick Start

```bash
# 1. Initialize JEFF
jeff init

# 2. Register a codebase
jeff repo add https://github.com/org/backend.git

# 3. Pick up a task and start working
jeff pickup gig-ab12 --persona jenko --repos backend

# 4. Save progress
jeff checkpoint --done "Implemented auth flow" --next "Add tests"

# 5. Push and create PRs
jeff ship

# 6. Close task
jeff done gig-ab12
```

## How It Works

```
  You ──► gig create "fix auth bug"
                  │
                  ▼
  jeff pickup gig-ab12 --persona jenko --repos backend
        │
        ├── Claims task in gig
        ├── Creates task workspace
        ├── Creates git worktree (branch from origin/main)
        ├── Injects matching skills via symlinks
        ├── Writes CLAUDE.md (persona + task context)
        ├── Unconditionally aliases .gemini/skills to .claude/skills
        └── Launches agent in workspace
                  │
                  ▼
  Agent works ──► jeff checkpoint ──► jeff ship ──► jeff done
```

## Multi-Agent Crews

Run multiple AI agents in parallel, each in its own tmux window with dedicated task workspace.

```
jeff-work (tmux session)
├── tab 1: orchestrator  ← you live here, coordinating
├── tab 2: gig-abc1      ← worker 1 (jenko, implementing)
├── tab 3: gig-def2      ← worker 2 (eric, researching)
└── tab 4: gig-ghi3      ← worker 3 (hardy, reviewing)
```

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

### Dashboard

```bash
jeff dashboard                          # interactive TUI (auto-refreshes every 2s)
```

## Commands

| Command | Description |
|---------|-------------|
| **Task lifecycle** | |
| `jeff init [--here] [--update]` | Initialize or update JEFF home |
| `jeff pickup <id> [--persona] [--repos]` | Claim task, set up workspace, launch agent |
| `jeff work [id]` | Resume work in existing task workspace |
| `jeff checkpoint --done "..."` | Save structured progress snapshot |
| `jeff ship [--repo] [--draft] [--dry-run]` | Push branches and create PRs |
| `jeff done [id] [--reason]` | Close task and clean up workspace |
| `jeff status [--all]` | Overview of active tasks and workspaces |
| `jeff open [id]` | Open workspace in IDE |
| **Crew orchestration** | |
| `jeff orchestrator start [--name]` | Launch orchestrator tmux session |
| `jeff orchestrator list` | List orchestrator sessions |
| `jeff orchestrator info [id]` | Show all tasks under an orchestrator |
| `jeff orchestrator attach <id>` | Attach to orchestrator session |
| `jeff orchestrator stop <id>` | Stop orchestrator and all workers |
| `jeff crew start <id> "<prompt>" [--persona] [--repos] [--model]` | Launch worker in tmux |
| `jeff crew resume <id>` | Resume stopped worker (restores Claude session) |
| `jeff crew list [--all]` | List workers (filtered to current orchestrator by default) |
| `jeff crew status <id>` | Worker detail + checkpoint + pane output |
| `jeff crew send <id> "msg" [--interrupt]` | Message a worker (stored + delivered to pane) |
| `jeff crew ask "question"` | Worker asks orchestrator a question |
| `jeff crew ack <msg-id> ["response"]` | Acknowledge worker question |
| `jeff crew events [--since]` | Recent gig activity across workers |
| `jeff crew capture <id> [--lines]` | Raw terminal output from worker pane |
| `jeff crew stop <id> [--all]` | Stop worker(s) |
| `jeff crew cleanup` | Reconcile tmux state vs DB |
| `jeff dashboard` | Interactive TUI dashboard |
| **Resources** | |
| `jeff repo add\|list\|remove\|sync` | Manage registered codebases |
| `jeff worktree add\|rm\|list` | Manage git worktrees |
| `jeff skill doc\|list\|show\|add\|remove\|tag\|inject\|eject` | Manage agent skills |
| `jeff persona list\|show\|set-model` | Manage personas and model defaults |
| `jeff config [agent\|ide\|hooks\|reset-claude-md]` | View and update configuration |
| `jeff completion [bash\|zsh\|fish]` | Shell completions |

## Personas

Shape agent behavior with embedded personas. Each has a default model for cost optimization.

| Persona | Role | Default model | Use when |
|---------|------|--------------|----------|
| **jenko** | Implementer — writes code, ships | opus | Building features, fixing bugs |
| **schmidt** | Debugger — traces root causes | opus | Investigating issues, debugging |
| **dickson** | Orchestrator — plans, delegates | sonnet | Breaking down epics, coordinating |
| **eric** | Researcher — explores, documents | sonnet | Investigating code, researching |
| **hardy** | Reviewer — checks quality | sonnet | Code review, PR review |

```bash
jeff pickup gig-ab12 --persona jenko --repos backend
jeff crew start gig-ab12 "Fix the issue" --persona schmidt --repos backend --model opus
```

## Skills

Skills are reusable SKILL.md instructions auto-injected into task workspaces based on persona, task type, or tags.

```bash
jeff skill add ./my-skill              # register a skill
jeff skill tag my-skill --persona jenko # tag for auto-injection
jeff skill inject slack notion         # inject into JEFF home
jeff skill list                        # see all skills
```

The `crew-orchestrator` skill is embedded in the binary and auto-installed on `jeff init`.

## Agent Memory

JEFF maintains persistent memory across sessions:

- **Persona memory** (`personas/<name>/memory/`) — per-persona knowledge that carries across tasks
- **Repo learnings** (`learnings/<repo>/`) — repo-specific quirks and patterns
- **Scratchpad** (`scratchpad.md` in task dir) — raw observations during a session

Use `jeff memory propose` to capture observations, and `jeff memory curate` to consolidate them into persistent memory.

## Configuration

JEFF is configured via `jeff.json` with [JSON schema](https://raw.githubusercontent.com/NeerajG03/JEFF/main/schemas/jeff-config.json) for editor autocompletion.

```json
{
  "$schema": "https://raw.githubusercontent.com/NeerajG03/JEFF/main/schemas/jeff-config.json",
  "agent": "claude",
  "ide": "cursor",
  "repos": {
    "backend": {
      "url": "https://github.com/org/backend.git",
      "base_branch": "origin/develop",
      "branch_name": "scripts/branch.sh",
      "post_setup": "scripts/setup.sh"
    }
  },
  "hooks": {
    "gig-ready-tasks": true
  }
}
```

See [docs/config.md](docs/config.md) for full configuration reference.

## Requirements

- [gig](https://github.com/NeerajG03/gig) — task management
- [Claude Code](https://claude.com/product/claude-code) or [opencode](https://github.com/anomalyco/opencode) — agent tool
- Git, `gh` CLI (for `jeff ship`), tmux (for crew mode)

## License

MIT
