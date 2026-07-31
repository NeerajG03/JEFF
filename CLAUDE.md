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
├── persona/            # Embedded persona templates (dickson, eric, hardy, jenko, marlowe, schmidt)
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
go build -o /tmp/jeff-dev ./cmd/jeff/   # local test build — throwaway path, invoke explicitly: /tmp/jeff-dev ...
go test ./...
go vet ./...
```

> ⚠️ **NEVER touch, repoint, or replace the live `jeff` command, and never build a local dev build into it.**
> Do NOT `go install`, `go build -o jeff` into PATH, symlink, alias, `mv`, or otherwise overwrite/shadow the
> installed `jeff` binary or change what `jeff` on PATH resolves to. The running orchestrator and **every active
> crew worker depend on the installed `jeff`** — swapping it out mid-session kills them (this has repeatedly caused
> workers to die). For local testing always build to a throwaway path (e.g. `/tmp/jeff-dev`) and invoke that path
> explicitly; leave the system `jeff` untouched. Shipping a real change goes through the normal PR + release flow,
> not by repointing the local command.

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
All of it lives in `home.go`. **Read that file's header before touching anything
home-related** — the rules below are load-bearing, and four shipped bugs came from
breaking them.

### Two operations, never conflated

| | Question | Who asks | May write the pointer? |
|---|---|---|---|
| **Selection** | "where should a home *live*?" | `jeff init`, `jeff home use` — once | **yes** |
| **Resolution** | "where *is* my home?" | every other command, every run | **never** |

- `ResolveHome()` / `ResolveHomeWithSource()` — resolution. **Read-only by contract.**
- `SelectHomeForInit(SelectHomeOpts{...})` — selection. `--home` → `--here` → env → pointer → default.
- `WriteHomePointer()` — selection path only.

`ResolveHomeWithSource` also returns a `HomeSource` (`flag`/`env`/`pointer`/`default`).
Surface it in anything user-facing — `jeff home` and `jeff doctor` both print it. Every
bug here was hard to see because the provenance was invisible.

### The three layers are three scopes, not three ways to do one thing

| Layer | Lifetime | Why it must exist |
|---|---|---|
| `$JEFF_HOME` | one process / one tmux session | `crew.Start` does `tmux set-environment JEFF_HOME` so every pane inherits one home and workers can't drift onto a different `jeff.db` |
| `~/.config/jeff/home` | per user, persistent | the install record — the only thing that knows a non-default home exists |
| `~/.jeff` | bootstrap | the very first run, before any pointer |

### Rules

1. **Never write the pointer from a read path.** It used to "self-heal" on every
   command, so one throwaway `JEFF_HOME=/tmp/x jeff status` permanently repointed the
   pointer for every future shell — the most transient layer promoting itself to the
   most durable. Pinned by `TestResolveHomeNeverWrites`.
2. **Never derive a home yourself.** No `os.UserHomeDir()` + `".jeff"`, no second
   `ResolveHome()` inside a process that already has `cfg.Home`. A duplicate resolver
   drifts from the real one (`jeff init` did, → #82; `--global` did, → #85). Inside
   `cmd/jeff`, prefer `cfg.Home`; `resolvedJeffHome()` encodes that preference.
3. **Store paths inside the home as home-relative** via `internal/homepath` — `Rel` on
   write, `Abs` on read. Absolute paths are what made the home non-relocatable (#84).
   Paths genuinely outside the home stay absolute; they aren't the home's to move.
4. **`$JEFF_HOME` is not `$HOME`.** `identity.ProjectDirName` (`.jeff`) is a
   per-directory marker like `.git`; the home is a different thing that merely
   defaults to `~/.jeff`. `identity.GlobalFilePathIn` takes the **JEFF home**.
5. **Relocating a home** is `mv` + `jeff home use <path>`. Registries travel on their
   own; `home use` repairs leftovers and regenerates the per-agent settings, whose
   hook commands must be absolute to be executable. `jeff doctor` flags stored paths
   that escape the resolved home (`home_paths_relative`).

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
├── projects/           # project workspaces (jeff project init|open|list)
├── .skills/            # user-added skills (jeff skill add)
├── .personas/          # user-added persona templates
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
| `hooks/` | `hooks/CLAUDE.md` | 16 built-in hooks, static vs dynamic scripts, delivery |
| `memory/` | `memory/CLAUDE.md` | Persona memory, repo learnings, /learn command |
| `persona/` | `persona/CLAUDE.md` | Embedded templates, model defaults, adding personas |
| `skill/` | `skill/CLAUDE.md` | Registry, matching logic, injection, embedded skills |
| `tui/` | `tui/CLAUDE.md` | Bubbletea dashboard, tabs, refresh, styles |
| `workspace/` | `workspace/CLAUDE.md` | Task dirs, worktrees, branch naming, .jeff-base |
| `cmd/jeff/` | `cmd/jeff/CLAUDE.md` | CLI conventions, key helpers, thin wrapper pattern |

## What NOT to Do

- Don't store task state in JEFF — use gig SDK.
- Don't shell out to `gig` CLI — use `import "github.com/NeerajG03/gig"`.
- Don't hardcode JEFF_HOME paths — use `ResolveHome()` (or `cfg.Home` in `cmd/jeff`).
- Don't write the home pointer outside `jeff init` / `jeff home use` — see JEFF_HOME above.
- Don't persist absolute paths inside the home — use `internal/homepath`.
- Don't pass `$HOME` where a JEFF home is wanted — they are different things.
- Don't re-tag released versions — Go module proxy caches checksums.

## Personas

Embedded in binary via `persona/templates/`. Used via `--persona` flag on pickup.

| Persona | Role |
|---------|------|
| **dickson** | Orchestrator — plans, delegates, reviews. Does NOT write code. |
| **eric** | Researcher — explores, documents, recommends. Does NOT change code. |
| **hardy** | Reviewer — reviews diffs, checks quality, flags issues. |
| **jenko** | Implementer — writes code, runs tests, ships. Default persona. |
| **marlowe** | Memory curator — curates proposals into canonical memory. Used by `jeff memory curate`. |
| **schmidt** | Debugger — investigates, traces root causes, finds the fix. |

## Further Reading

- `docs/config.md` — full configuration reference (repos, hooks, branch naming, IDE)
- `docs/testing.md` — test infrastructure, existing test files
- `docs/adding-commands.md` — step-by-step guide for new CLI commands
crew, memory may import the root package; the root package must never import them
