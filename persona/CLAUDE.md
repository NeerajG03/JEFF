# persona/ — Embedded agent persona templates

Embeds persona template markdown files in the binary (`go:embed`). On `jeff pickup --persona <name>`, the template is prepended to the task CLAUDE.md.

## Personas

| Name | Role | Default model |
|------|------|--------------|
| dickson | Orchestrator — plans, delegates. Does NOT write code. | sonnet |
| eric | Researcher — explores, documents. Does NOT change code. | sonnet |
| hardy | Reviewer — reviews diffs, flags issues. | sonnet |
| jenko | Implementer — writes code, runs tests, ships. | opus |
| schmidt | Debugger — traces root causes, investigates. | sonnet |

## File roles

| File | What it does |
|------|-------------|
| `persona.go` | `go:embed` + `Names()` / `Get(name)` / `IsValid(name)` / `DefaultModel(name)` |
| `registry.go` | Load `personas.json` from JEFF_HOME (model, memory_hint metadata) |
| `templates/` | `*.md` — one file per persona, embedded in binary |

## Adding a persona

1. Create `persona/templates/<name>.md` with role + workflow
2. Add entry to `personas.json` in JEFF_HOME for model + memory_hint
3. Add default model mapping in `DefaultModel()` if needed
4. Rebuild binary
