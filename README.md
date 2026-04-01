# JEFF

Agent workspace manager built on [gig](https://github.com/NeerajG03/gig). JEFF gives AI agents structured workspaces, personas, skills, and lifecycle management for task-driven development.

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
        └── Launches agent in workspace
                  │
                  ▼
  Agent works ──► jeff checkpoint ──► jeff ship ──► jeff done
```

## Commands

| Command | Description |
|---------|-------------|
| `jeff init [--here] [--update]` | Initialize or update JEFF home |
| `jeff pickup <id> [--persona] [--repos]` | Claim task, set up workspace, launch agent |
| `jeff work [id]` | Resume work in existing task workspace |
| `jeff checkpoint --done "..."` | Save structured progress snapshot |
| `jeff ship [--repo] [--draft] [--dry-run]` | Push branches and create PRs |
| `jeff done [id] [--reason]` | Close task and clean up workspace |
| `jeff status [--all]` | Overview of active tasks and workspaces |
| `jeff open [id]` | Open workspace in IDE |
| `jeff repo add\|list\|remove\|sync` | Manage registered codebases |
| `jeff worktree add\|rm\|list` | Manage git worktrees |
| `jeff skill doc\|list\|show\|add\|remove\|tag\|inject\|eject` | Manage agent skills |
| `jeff config [agent\|ide\|hooks\|reset-claude-md]` | View and update configuration |
| `jeff completion [bash\|zsh\|fish]` | Shell completions |

See [docs/usage.md](docs/usage.md) for detailed command reference.

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

## Personas

Shape agent behavior with embedded personas:

| Persona | Role | Use when |
|---------|------|----------|
| **dickson** | Orchestrator — plans, delegates, reviews | Breaking down epics, coordinating work |
| **eric** | Researcher — explores, documents, recommends | Investigating code, researching approaches |
| **jenko** | Implementer — writes code, runs tests, ships | Building features, fixing bugs |
| **hardy** | Reviewer — reviews code, checks quality | Code review, quality checks |

```bash
jeff pickup gig-ab12 --persona jenko --repos backend
```

## Skills

Skills are reusable SKILL.md instructions auto-injected into task workspaces.

```bash
jeff skill add ./my-skill              # register a skill
jeff skill tag my-skill --persona jenko # tag for auto-injection
jeff skill inject slack notion         # inject into JEFF home
jeff skill list                        # see all skills
```

Skills auto-inject on `jeff pickup` when persona, task type, or tags match. See [docs/usage.md](docs/usage.md) for details.

## Requirements

- [gig](https://github.com/NeerajG03/gig) — task management
- [Claude Code](https://claude.com/product/claude-code) or [opencode](https://github.com/anomalyco/opencode) — agent tool
- Git
- `gh` CLI (for `jeff ship`)

## License

MIT
