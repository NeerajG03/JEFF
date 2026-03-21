# JEFF

Agent workspace manager — Go CLI + SDK built on [gig](https://github.com/NeerajG03/gig).

## Layout

```
JEFF/
├── jeff.go             # Core types: AgentTool, IDE enums
├── config.go           # LoadConfig, SaveConfig, ResolveHome, RepoConfig
├── repo.go             # AddRepo, RemoveRepo, ListRepos, SetPostSetup
├── attrs.go            # EnsureAttrs — define JEFF custom attributes in gig
├── embed/              # Embedded assets (CLAUDE.md template, claude-settings.json)
├── hooks/              # Hook system: registry, builtin hooks, claude/opencode delivery
├── persona/            # Embedded persona templates (captain, nerd, jock, scout)
├── workspace/          # Task workspace + worktree management + branch naming
├── cmd/jeff/           # CLI (cobra) — thin wrapper over SDK
├── docs/               # Guides (testing, adding commands, config reference)
└── *_test.go           # Unit tests alongside source
```

## Build & Test

```bash
go build -o jeff ./cmd/jeff/
go test ./...
go vet ./...
```

## Conventions

- **SDK-first**: All logic in root package + sub-packages. CLI is a thin cobra wrapper.
- **gig is the brain**: All task state in gig via SDK (`import "github.com/NeerajG03/gig"`). Never shell out to gig CLI.
- **No database**: JEFF uses filesystem + gig. jeff.yaml for config, dirs for workspaces.
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
├── jeff.yaml           # config: agent, IDE, repos, hooks
├── .claude/            # Claude Code settings + hooks
├── .opencode/          # opencode settings
├── hooks/              # hook scripts (managed by jeff)
├── scripts/            # user scripts (branch naming, post-setup, etc.)
├── repos/              # registered codebases (git clones)
├── tasks/              # active task workspaces (ephemeral)
├── worktrees/          # centralized git worktrees (symlinked into tasks)
└── exports/            # generated artifacts
```

## Config (jeff.yaml)

```yaml
agent: claude                      # "claude" or "opencode"
ide: cursor                        # "vscode", "cursor", "windsurf", "nvim"
gig_home: ""                       # override gig home (empty = default)
repos:
  backend:
    url: https://github.com/org/backend.git
    base_branch: origin/develop    # base for worktrees + PRs (default: origin/main)
    branch_name: scripts/branch.sh # custom branch naming (receives task JSON on stdin)
    post_setup: scripts/setup.sh   # runs after worktree creation
hooks:
  gig-ready-tasks: false           # nil map = all enabled, set false to disable
```

See `docs/config.md` for full reference.

## Hooks

Located in `hooks/` package. Four built-in hooks inject context at agent session start:

| Hook | Content |
|------|---------|
| `gig-instructions` | gig CLI reference (agent-usable commands only) |
| `gig-ready-tasks` | `gig ready` output (dynamic) |
| `jeff-instructions` | jeff CLI reference (agent-usable commands only) |
| `jeff-repos` | Registered repo list (dynamic) |

Delivery: Claude Code gets bash scripts in `hooks/` + settings.json wiring. OpenCode gets a combined JS plugin.

Key files: `hooks/hook.go` (types), `hooks/registry.go` (collection), `hooks/builtin.go` (definitions + content), `hooks/claude.go` (Claude delivery), `hooks/opencode.go` (OpenCode delivery), `hooks/manager.go` (orchestrator).

## Worktrees

`workspace/worktree.go` manages git worktrees. Key behavior:
- **Base branch**: `RepoConfig.BaseBranch` (default `origin/main`). Fetches remote before branching.
- **Branch naming**: `RepoConfig.BranchName` script receives task JSON (with attrs) on stdin. Default: task ID.
- **`.jeff-base`**: Written in each worktree, records base branch for `jeff ship`.

## What NOT to Do

- Don't store task state in JEFF — use gig SDK.
- Don't shell out to `gig` CLI — use `import "github.com/NeerajG03/gig"`.
- Don't hardcode JEFF_HOME paths — use `ResolveHome()`.
- Don't re-tag released versions — Go module proxy caches checksums.

## Personas

Embedded in binary via `persona/templates/`. Used via `--persona` flag on pickup.

| Persona | Role |
|---------|------|
| **captain** | Orchestrator — plans, delegates, reviews. Does NOT write code. |
| **nerd** | Researcher — explores, documents, recommends. Does NOT change code. |
| **jock** | Implementer — writes code, runs tests, ships. Default persona. |
| **scout** | Reviewer — reviews diffs, checks quality, flags issues. |

## Further Reading

- `docs/config.md` — full configuration reference (repos, hooks, branch naming, IDE)
- `docs/testing.md` — test infrastructure, existing test files
- `docs/adding-commands.md` — step-by-step guide for new CLI commands
