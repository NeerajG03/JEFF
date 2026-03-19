# JEFF

Agent workspace manager — Go CLI + SDK built on [gig](https://github.com/NeerajG03/gig).

## Layout

```
JEFF/
├── doc.go              # Package documentation with file layout guide
├── jeff.go             # Core types: AgentTool enum
├── config.go           # LoadConfig, SaveConfig, ResolveHome, RepoConfig, jeff.yaml parsing
├── repo.go             # AddRepo (git clone), RemoveRepo, ListRepos, SetPostSetup
├── attrs.go            # EnsureAttrs — define JEFF's custom attributes in gig (repos, worktree_setup)
├── embed/              # Embedded assets shipped in binary
│   ├── embed.go        # //go:embed for CLAUDE.md + claude-settings.json
│   ├── CLAUDE.md       # Default agent instructions (editable by user, resettable)
│   └── claude-settings.json  # Default .claude/settings.json
├── persona/            # Embedded persona templates
│   ├── persona.go      # //go:embed + Get, Names, IsValid
│   ├── persona_test.go
│   └── templates/      # captain.md, nerd.md, jock.md, scout.md
├── workspace/          # Task workspace and worktree management
│   ├── workspace.go    # Create, Open, Remove, List task directories (slug generation)
│   ├── worktree.go     # WorktreeAdd (with post-setup), WorktreeRemove, WorktreeList, symlinkIntoTask
│   ├── workspace_test.go
│   └── worktree_test.go
├── cmd/jeff/           # CLI (cobra) — thin wrapper over SDK
│   ├── main.go         # Root command, config lifecycle, bare `jeff` = command center
│   ├── init_cmd.go     # jeff init [--here], double-init guard, .claude/.opencode setup
│   ├── pickup_cmd.go   # jeff pickup <gig-id> — claim, workspace, CLAUDE.md, worktrees, launch
│   ├── work_cmd.go     # jeff work <gig-id> — resume in existing task workspace
│   ├── done_cmd.go     # jeff done <gig-id> — close task, cleanup worktrees, remove workspace
│   ├── status_cmd.go   # jeff status — active tasks, dirty worktrees, latest checkpoint
│   ├── checkpoint_cmd.go # jeff checkpoint — wrap gig AddCheckpoint
│   ├── repo_cmd.go     # jeff repo add/list/remove/post-setup
│   ├── worktree_cmd.go # jeff worktree add/rm/list
│   ├── gig.go          # openGigStore helper (shared gig access from CLI)
│   ├── launch.go       # Agent tool launcher (claude --dangerously-skip-permissions, opencode)
│   └── completion.go   # Shell completions (bash/zsh/fish)
├── *_test.go           # Unit tests alongside source (config_test, repo_test, attrs_test)
├── .github/workflows/
│   ├── test.yml        # CI: build + vet + tests on PRs
│   └── release.yml     # Release: tag → tests → GitHub release → homebrew tap
├── README.md           # Install, quick start, commands, personas, layout
└── LICENSE             # MIT
```

## Build & Test

```bash
go build -o jeff ./cmd/jeff/        # Build binary
go test ./...                        # All tests
go vet ./...                         # Static analysis
```

## Key Design Decisions

- **SDK-first**: All logic in root package + sub-packages. CLI is a thin cobra wrapper.
- **gig is the brain**: All task state lives in gig (imported as Go SDK). JEFF never stores task data — only workspace layout and config.
- **No database**: JEFF uses filesystem + gig. jeff.yaml for config, dirs for workspaces.
- **All gig interaction via SDK**: `import "github.com/NeerajG03/gig"` — never shell out to gig CLI.
- **Personas are embedded**: Shipped in the binary via `go:embed`. Used via `--persona` flag on pickup. Stamped into task dir's CLAUDE.md.
- **CLAUDE.md is editable**: Default shipped in binary, written on init, user can customize. Task dirs get a generated CLAUDE.md (persona + task context + jeff/gig reference).
- **Agent tool is pluggable**: jeff.yaml `agent` field. Currently: `claude` (default), `opencode`. Claude launched with `--dangerously-skip-permissions`.
- **Global pointer**: `~/.config/jeff/home` stores JEFF_HOME path so `jeff` works from anywhere.
- **`jeff init`**: `~/.jeff/` (hidden, default) or `./jeff/` (visible, `--here`). Double-init guarded.
- **Workspace layout**: `tasks/<gig-id>-slug/` for task dirs, `worktrees/<repo>/<branch>/` for git worktrees, symlinked into task dirs.
- **Post-setup scripts**: Per-repo in jeff.yaml (`post_setup` field). Run after worktree creation with `src_dir` and `dest_dir` args. For env setup (copy .env, poetry install, etc).
- **Custom attrs in gig**: JEFF defines `repos` (object — JSON array) and `worktree_setup` (string) via `EnsureAttrs()`. Idempotent.
- **Agent settings**: `.claude/` and `.opencode/` dirs created in JEFF_HOME with default settings.json files.

## JEFF_HOME Structure

```
/Volumes/Casesensitive/jeff/        # or ~/.jeff/
├── CLAUDE.md                       # agent instructions (editable)
├── jeff.yaml                       # config: agent tool, repos, post-setup scripts
├── .claude/                        # Claude Code settings
│   ├── settings.json               # schema + hooks (future)
│   └── settings.local.json         # user overrides
├── .opencode/                      # opencode settings
│   ├── settings.json
│   └── settings.local.json
├── repos/                          # registered codebases (git clones)
│   ├── gig/
│   └── jeff/
├── tasks/                          # active task workspaces (ephemeral)
│   └── gig-ab12-refactor-auth/
│       ├── CLAUDE.md               # persona + task context
│       └── backend -> ../../worktrees/backend/gig-ab12
├── worktrees/                      # centralized git worktrees
│   └── backend/
│       └── gig-ab12/
└── exports/                        # generated artifacts (scripts, reports, data)
```

## Conventions

- Same standards as gig: fast, simple, extensible
- All public SDK functions return `(*Type, error)` or `error`
- Module path is `github.com/NeerajG03/JEFF` — must match the GitHub repo URL
- Tests use `t.TempDir()` for isolation
- No CGO dependencies
- Task dirs are ephemeral/disposable — gig is the source of truth, not the filesystem
- `jeff done` cleans up worktrees + task dir. Data lives in gig (tasks, checkpoints, comments)

## What NOT to Do

- Don't store task state in JEFF — use gig SDK for all task data
- Don't shell out to `gig` CLI — always use `import "github.com/NeerajG03/gig"`
- Don't hardcode JEFF_HOME paths — use `ResolveHome()` which checks env var → pointer → default
- Don't re-tag released versions — Go module proxy caches checksums. Always bump version.

## How to Add a New CLI Command

1. **SDK first**: Add the business logic in the root package or sub-package.
2. **Wire the command**: Add a `func fooCmd() *cobra.Command` in `cmd/jeff/`.
3. **Register it**: Add `fooCmd()` to `rootCmd.AddCommand(...)` in `cmd/jeff/main.go`.
4. **Use `openGigStore()`**: For any command that needs gig access, use the shared helper in `cmd/jeff/gig.go`.
5. **Config access**: Use the package-level `cfg *jeff.Config` variable (loaded in `PersistentPreRunE`).

## Existing Test Files

| File | What it tests |
|------|--------------|
| `config_test.go` | Config defaults, save/load round-trip, invalid agent fallback, ResolveHome, WriteHomePointer, AgentTool |
| `repo_test.go` | repoNameFromURL, ListRepos, AddRepo duplicate, RemoveRepo not registered |
| `attrs_test.go` | EnsureAttrs defines repos + worktree_setup in gig, idempotency |
| `persona/persona_test.go` | Names, Get, IsValid, all 4 personas present |
| `workspace/workspace_test.go` | Create, Open, Remove, List, makeSlug, extractTaskID |
| `workspace/worktree_test.go` | symlinkIntoTask, idempotent symlink, WorktreeList empty, WorktreeAdd missing repo |
