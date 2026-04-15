# skill/ — Skill registry and injection

Registers skills in `skills.json` and symlinks matched skills into task workspaces. Skills auto-load in agent sessions via `.claude/skills/<name>/`.

## Concepts

- **skills.json**: registry at `$JEFF_HOME/.skills/skills.json`
- **Matching**: any non-empty dimension (persona OR gig_type OR tags) → inject. All-empty = manual-only.
- **Injection**: symlink `taskDir/.claude/skills/<name>` → skill location
- **Embedded skills**: `crew-orchestrator` (installed on `jeff init`, idempotent)

## File roles

| File | What it does |
|------|-------------|
| `skill.go` | SkillEntry/Config, Load/Save/Add/Remove/SetTags/List |
| `match.go` | `Match(entry, ctx)` — dimension-or logic |
| `inject.go` | Inject/Eject — create/remove symlink |
| `templates.go` | Write embedded skill files to JEFF_HOME |
| `templates/` | Embedded skill content (`crew-orchestrator/`) |

## Adding an embedded skill

1. Create `skill/templates/<name>/` with `SKILL.md` at root
2. Add to `templates.go` `InstallEmbeddedSkills()`

## Adding a user skill

```bash
jeff skill add <path> [--name <name>]
jeff skill tag <name> --persona jenko --type feature
```

A skill must have a `SKILL.md` at its root — validated on `Add()`.
