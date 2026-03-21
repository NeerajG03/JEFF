# JEFF

Agent workspace manager built on [gig](https://github.com/NeerajG03/gig). JEFF gives AI agents structured workspaces, personas, and lifecycle management for task-driven development.

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
cd JEFF
go build -o jeff ./cmd/jeff/
```

## Quick Start

```bash
# Initialize JEFF (creates ~/.jeff/)
jeff init

# Register a codebase
jeff repo add https://github.com/org/backend.git

# Pick up a task from gig and start working
jeff pickup gig-ab12 --persona jock --repos backend

# Resume work on an existing task
jeff work gig-ab12

# Save a structured progress checkpoint
jeff checkpoint --task gig-ab12 --done "Implemented auth flow" --next "Add tests"

# See all active tasks and workspace state
jeff status

# Push branches and create PRs
jeff ship

# Close task and clean up workspace
jeff done gig-ab12
```

## How It Works

```
┌─────────────────────────────────┐
│           JEFF (agent)          │
│  manages workspaces & personas  │
│  launches agent tools           │
│  orchestrates lifecycle         │
│                                 │
│  state lives in:  gig           │
│  code lives in:   git           │
└────────────┬────────────────────┘
             │
      ┌──────▼──────┐
      │     gig     │
      │  tasks      │
      │  checkpoints│
      │  events     │
      └─────────────┘
```

JEFF manages the filesystem, gig manages the state. The agent tool (Claude Code, opencode) does the actual work inside JEFF's managed workspaces.

## Workspace Layout

```
~/.jeff/
├── CLAUDE.md       — agent instructions (editable, resettable)
├── jeff.yaml       — config (agent, IDE, repos, hooks)
├── .claude/        — Claude Code settings (settings.json, hooks)
├── .opencode/      — opencode settings
├── hooks/          — hook scripts (managed by jeff)
├── scripts/        — user scripts (branch naming, post-setup, etc.)
├── repos/          — registered codebases (git clones)
│   └── backend/
├── tasks/          — active task workspaces
│   └── gig-ab12-refactor-auth/
│       ├── CLAUDE.md       — persona + task context + workspace layout
│       └── backend -> ../../worktrees/backend/gig-ab12
├── worktrees/      — centralized git worktrees
│   └── backend/
│       └── gig-ab12/
└── exports/        — generated artifacts
```

## Commands

| Command | Description |
|---------|-------------|
| `jeff init [--here]` | Initialize JEFF home directory |
| `jeff pickup <gig-id> [--persona] [--repos]` | Claim task, set up workspace, launch agent |
| `jeff work <gig-id>` | Resume work in existing task workspace |
| `jeff checkpoint --task <id> --done "..."` | Save structured progress snapshot |
| `jeff ship` | Push branches and create PRs for all repos |
| `jeff done <gig-id> [--reason]` | Close task and clean up workspace |
| `jeff status` | Overview of active tasks and workspaces |
| `jeff open [<gig-id>]` | Open JEFF_HOME or task workspace in IDE |
| `jeff config [agent\|ide\|hooks]` | View and update configuration |
| `jeff repo add <url>` | Register and clone a codebase |
| `jeff repo list` | List registered codebases |
| `jeff repo remove <name>` | Unregister a codebase |
| `jeff worktree add <repo> <branch> [--base]` | Create git worktree |
| `jeff worktree rm <repo> <branch>` | Remove git worktree |
| `jeff completion [bash\|zsh\|fish]` | Shell completions |

## Configuration

JEFF is configured via `jeff.yaml`. Key settings:

```yaml
agent: claude                      # "claude" or "opencode"
ide: cursor                        # "vscode", "cursor", "windsurf", "nvim"
repos:
  backend:
    url: https://github.com/org/backend.git
    base_branch: origin/develop    # base for worktrees + PRs (default: origin/main)
    branch_name: scripts/branch.sh # custom branch naming script (optional)
    post_setup: scripts/setup.sh   # runs after worktree creation (optional)
hooks:
  gig-ready-tasks: false           # disable specific hooks (all enabled by default)
```

See [docs/config.md](docs/config.md) for full configuration reference.

## Personas

JEFF ships with four embedded personas that shape agent behavior:

| Persona | Role |
|---------|------|
| **captain** | Orchestrator — plans, delegates, reviews |
| **nerd** | Researcher — explores, documents, recommends |
| **jock** | Implementer — writes code, runs tests, ships |
| **scout** | Reviewer — reviews code, checks quality |

```bash
jeff pickup gig-ab12 --persona nerd    # research mode
jeff pickup gig-ab12 --persona jock    # build mode
```

## Requirements

- [gig](https://github.com/NeerajG03/gig) — task management (installed automatically as Go dependency)
- [Claude Code](https://claude.com/product/claude-code) or [opencode](https://github.com/anomalyco/opencode) — agent tool
- Git

## License

MIT
