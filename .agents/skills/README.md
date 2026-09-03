# JEFF Repository Agent Skills

Skills available when developing and maintaining the JEFF codebase itself.

## Skills Router

| Touching | Load |
|---|---|
| Adding, extending, or auditing an agent CLI provider (Codex, etc.) | `provider-integration` |

## Discovery

- Canonical location: `.agents/skills/<skill-name>/SKILL.md`
- Claude Code discovers skills via `.claude/skills` symlink
- Gemini CLI discovers skills via `.gemini/skills` symlink
- OpenAI Codex discovers skills via `.agents/skills` natively
