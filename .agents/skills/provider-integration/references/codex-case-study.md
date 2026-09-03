# Codex Case Study

This is a worked discovery result, not a frozen Codex specification. It was
checked against the official OpenAI Codex docs on 2026-09-03. Re-run `codex
--help` and read the current docs before implementation.

## Verified CLI Shape

```text
Interactive fresh:   codex [GLOBAL_FLAGS] [PROMPT]
Interactive resume:  codex [GLOBAL_FLAGS] resume <SESSION_ID>
Latest resume:       codex [GLOBAL_FLAGS] resume --last
Headless fresh:      codex exec [FLAGS] <PROMPT>
Headless resume:     codex exec resume <SESSION_ID> [PROMPT]
```

Relevant current flags:

- `--model`, `-m`: select a model such as `gpt-5.6-terra`.
- `--ask-for-approval`, `-a`: `untrusted`, `on-request`, or `never`.
- `--sandbox`, `-s`: `read-only`, `workspace-write`, or `danger-full-access`.
- `--dangerously-bypass-approvals-and-sandbox`, `--yolo`: disable both controls.
  *Use with extreme caution*: only when running inside an isolated VM/container.
  For host machines, prefer `--ask-for-approval on-request --sandbox workspace-write`.
- `--dangerously-bypass-hook-trust`: run reviewed JEFF hooks for automation.
- `--disable memories`: turn off Codex native memory so JEFF-managed memory applies.
- `codex exec --skip-git-repo-check`: allow headless work outside a Git repo.

Do not use a deprecated convenience flag when explicit current flags exist. And
do not silently treat approval bypass and sandbox bypass as the same user choice
without recording that security decision.

## Proposed JEFF Mapping

### Default Worker (Unattended / Low Friction)
```text
codex --sandbox workspace-write \
      --ask-for-approval on-request \
      --dangerously-bypass-hook-trust \
      --disable memories \
      [-m MODEL] \
      [PROMPT]
```
If configured for complete permission bypass (matching `skip_permissions: true`),
note that `--dangerously-bypass-approvals-and-sandbox` disables all sandboxing.
Document this clearly for the user.

### Safe Worker (`--safe` flag)
```text
codex --sandbox workspace-write \
      --ask-for-approval on-request \
      --disable memories \
      [-m MODEL] \
      [PROMPT]
```
Explicitly pass `--ask-for-approval on-request` and `--sandbox workspace-write`
so that an unsafe default in a user's `~/.codex/config.toml` does not leak into
a `--safe` run.

### Resume
```text
codex [worker flags] [-m MODEL] resume SESSION_ID
```

### Headless Curation
```text
codex exec --sandbox workspace-write \
           --ask-for-approval never \
           --skip-git-repo-check \
           --disable memories \
           PROMPT
```

Confirm argument ordering with the installed version. Global flags and subcommand
flags can have different parser rules.

## Context, Skills, And Hooks

Codex reads `AGENTS.md`. Native skills live under `.agents/skills`, while project
hooks live under `.codex/hooks.json`.

Notice the file layout split:
- **Skills**: `.agents/skills/<skill-name>/SKILL.md` (this is Codex's native skill path!)
- **Hooks**: `.codex/hooks.json` and `.codex/hooks/<name>.sh`
- **Context**: `AGENTS.md` (symlinked from `CLAUDE.md`)

This exposes a JEFF abstraction leak:
`ConfigDir()` plus `SkillsSubdir()` assumes one common root (`.claude/skills`, `.gemini/skills`).
Codex uses `.agents/skills` for skills and `.codex/` for hooks. When integrating Codex
into the JEFF Go codebase, either:
1. Extend `AgentProvider` to have an explicit `SkillsDir()` method instead of deriving from `ConfigDir()`.
2. Or use `ConfigDir() = ".agents"` for skills and write `.codex/` explicitly in `EnsureHomeDirs` / hook delivery.

Option 1 is cleaner architectural design.

Codex hook events overlap JEFF's canonical events:
- `SessionStart`
- `PostToolUse`
- `PreCompact` and `PostCompact`
- `Stop` and `SessionEnd`
- `Interrupt`

Every command hook receives JSON on stdin, including `session_id`,
`transcript_path`, `cwd`, `hook_event_name`, and `model`. `SessionStart` can return
model context using `hookSpecificOutput.additionalContext` with the matching event
name. Validate the exact output contract against a live hook before reusing Claude
script bodies.

Project-local hooks load only after project trust, and changed hook definitions
need review. `--dangerously-bypass-hook-trust` handles hook-definition review for
vetted automation, but do not assume it also grants project trust. JEFF's initial
task context, memory, and skill setup must work before any hook runs.

Use provider-owned files:
```text
task/
├── AGENTS.md -> CLAUDE.md
├── .agents/skills/
└── .codex/
    ├── hooks.json
    └── hooks/
```

Do not share `task/hooks/*.sh` discovery with another delivery. Current Claude
and Gemini delivery code scans that common directory, so sharing it can make one
provider uninstall or keep stale artifacts from another provider.

## Codex-Specific Checks

1. Verify login with `codex login` and doctor behavior without exposing credentials.
2. Verify an inline prompt keeps the TUI alive after one turn.
3. Capture a real `session_id`, stop the worker, and resume that exact ID.
4. Test `resume --last` only as an explicit fallback; confirm its cwd scoping.
5. Verify bracketed paste, direct message delivery, Ctrl-C, and startup timing in tmux.
6. Run a JEFF `SessionStart` hook and confirm its context reaches the model.
7. Run curation outside a Git repo and confirm it cannot pause for approval.
8. Test both project-trusted and new/untrusted task directories.

## Official References

- `https://developers.openai.com/codex/developer-commands`
- `https://developers.openai.com/codex/config-file/config-reference`
- `https://developers.openai.com/codex/hooks`
- `https://developers.openai.com/codex/agent-configuration/agents-md`
- `https://developers.openai.com/codex/build-skills`
