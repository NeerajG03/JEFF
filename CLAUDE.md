# JEFF

Agent workspace manager — Go CLI + SDK built on [gig](https://github.com/NeerajG03/gig).

## Layout

```
JEFF/
├── jeff.go             # Core types: AgentTool, IDE enums
├── config.go           # LoadConfig, SaveConfig, ResolveHome, RepoConfig
├── repo.go             # AddRepo, RemoveRepo, ListRepos, SetPostSetup
├── attrs.go            # EnsureAttrs — define JEFF custom attributes in gig
├── crew/               # Multi-agent orchestration (SQLite + tmux)
├── embed/              # Embedded assets (CLAUDE.md template, claude-settings.json)
├── hooks/              # Hook system: registry, builtin hooks, claude/opencode delivery
├── memory/             # Persona memory + repo learnings management
├── persona/            # Embedded persona templates (dickson, eric, hardy, jenko, schmidt)
├── skill/              # Skill registry, matching, and injection (symlinks into task dirs)
├── tui/                # Bubbletea dashboard (jeff dashboard)
├── workspace/          # Task workspace + worktree management + branch naming
├── cmd/jeff/           # CLI (cobra) — thin wrapper over SDK
├── docs/               # Guides (testing, adding commands, config reference)
└── *_test.go           # Unit tests alongside source
```

Each package has its own `CLAUDE.md` with contextual detail — auto-loaded when working in that area.

## Build & Test

```bash
go build -o jeff ./cmd/jeff/
go test ./...
go vet ./...
```

## Conventions

- **SDK-first**: All logic in root package + sub-packages. CLI is a thin cobra wrapper.
- **gig is the brain**: All task state in gig via SDK (`import "github.com/NeerajG03/gig"`). Never shell out to gig CLI.
- **SQLite for crew only**: `crew/` uses SQLite (`modernc.org/sqlite`, no CGO) for session state. Everything else uses filesystem + gig.
- **Module path**: `github.com/NeerajG03/JEFF` — must match GitHub repo URL.
- All public SDK functions return `(*Type, error)` or `error`.
- Tests use `t.TempDir()` for isolation.
- No CGO dependencies.
- Task dirs are ephemeral — gig is the source of truth, not the filesystem.

## JEFF_HOME

Resolved via: `JEFF_HOME` env var → `~/.config/jeff/home` pointer → `~/.jeff/` default.

```
JEFF_HOME/
├── CLAUDE.md           # agent instructions (editable, resettable)
├── jeff.json           # config: agent, IDE, repos, hooks
├── .claude/            # Claude Code settings + hooks
├── .opencode/          # opencode settings
├── hooks/              # hook scripts (managed by jeff)
├── scripts/            # user scripts (branch naming, post-setup, etc.)
├── repos/              # registered codebases (git clones)
├── tasks/              # active task workspaces (ephemeral)
├── worktrees/          # centralized git worktrees (symlinked into tasks)
└── exports/            # generated artifacts
```

## Config (jeff.json)

```json
{
  "$schema": "https://raw.githubusercontent.com/NeerajG03/JEFF/main/schemas/jeff-config.json",
  "agent": "claude",
  "ide": "cursor",
  "gig_home": "",
  "repos": {
    "backend": {
      "url": "https://github.com/org/backend.git",
      "base_branch": "origin/develop",
      "branch_name": "scripts/branch.sh",
      "post_setup": "scripts/setup.sh"
    }
  },
  "hooks": {
    "gig-ready-tasks": false
  }
}
```

See `docs/config.md` for full reference.

## Package Guides

Each package has a `CLAUDE.md` with types, file roles, and extension patterns:

| Package | Guide | What it covers |
|---------|-------|---------------|
| `crew/` | `crew/CLAUDE.md` | SQLite store, session lifecycle, tmux ops, message types |
| `hooks/` | `hooks/CLAUDE.md` | 13 built-in hooks, static vs dynamic scripts, delivery |
| `memory/` | `memory/CLAUDE.md` | Persona memory, repo learnings, /learn command |
| `persona/` | `persona/CLAUDE.md` | Embedded templates, model defaults, adding personas |
| `skill/` | `skill/CLAUDE.md` | Registry, matching logic, injection, embedded skills |
| `tui/` | `tui/CLAUDE.md` | Bubbletea dashboard, tabs, refresh, styles |
| `workspace/` | `workspace/CLAUDE.md` | Task dirs, worktrees, branch naming, .jeff-base |
| `cmd/jeff/` | `cmd/jeff/CLAUDE.md` | CLI conventions, key helpers, thin wrapper pattern |

## What NOT to Do

- Don't store task state in JEFF — use gig SDK.
- Don't shell out to `gig` CLI — use `import "github.com/NeerajG03/gig"`.
- Don't hardcode JEFF_HOME paths — use `ResolveHome()`.
- Don't re-tag released versions — Go module proxy caches checksums.

## Personas

Embedded in binary via `persona/templates/`. Used via `--persona` flag on pickup.

| Persona | Role |
|---------|------|
| **dickson** | Orchestrator — plans, delegates, reviews. Does NOT write code. |
| **eric** | Researcher — explores, documents, recommends. Does NOT change code. |
| **hardy** | Reviewer — reviews diffs, checks quality, flags issues. |
| **jenko** | Implementer — writes code, runs tests, ships. Default persona. |
| **schmidt** | Debugger — investigates, traces root causes, finds the fix. |

## Further Reading

- `docs/config.md` — full configuration reference (repos, hooks, branch naming, IDE)
- `docs/testing.md` — test infrastructure, existing test files
- `docs/adding-commands.md` — step-by-step guide for new CLI commands
