# JEFF

Agent workspace manager — Go CLI + SDK built on [gig](https://github.com/NeerajG03/gig).

## Layout

```
JEFF/
├── jeff.go             # Core types: AgentTool enum
├── config.go           # LoadConfig, SaveConfig, ResolveHome, RepoConfig
├── repo.go             # AddRepo, RemoveRepo, ListRepos, SetPostSetup
├── attrs.go            # EnsureAttrs — define JEFF custom attributes in gig
├── embed/              # Embedded assets (CLAUDE.md, claude-settings.json)
├── persona/            # Embedded persona templates (captain, nerd, jock, scout)
├── workspace/          # Task workspace + worktree management
├── cmd/jeff/           # CLI (cobra) — thin wrapper over SDK
├── docs/               # Detailed guides (testing, adding commands)
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
├── jeff.yaml           # config: agent tool, repos, post-setup scripts
├── .claude/            # Claude Code settings + hooks
├── .opencode/          # opencode settings
├── repos/              # registered codebases (git clones)
├── tasks/              # active task workspaces (ephemeral)
├── worktrees/          # centralized git worktrees (symlinked into tasks)
└── exports/            # generated artifacts (scripts, reports, data)
```

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

- `docs/testing.md` — test infrastructure, existing test files
- `docs/adding-commands.md` — step-by-step guide for new CLI commands
