# JEFF

Agent workspace manager — Go CLI + SDK built on gig.

## Layout

```
JEFF/
├── doc.go              # Package documentation with file layout guide
├── jeff.go             # Core types: AgentTool enum
├── config.go           # LoadConfig, SaveConfig, ResolveHome, jeff.yaml parsing
├── config_test.go      # Config unit tests
├── embed/              # Embedded assets
│   ├── embed.go        # //go:embed CLAUDE.md + WriteClaudeMD helper
│   └── CLAUDE.md       # Default agent instructions (shipped in binary)
├── persona/            # Embedded persona templates
│   ├── persona.go      # //go:embed + Get, Names, IsValid
│   ├── persona_test.go # Persona unit tests
│   └── templates/      # Persona markdown files (captain, nerd, jock, scout)
├── workspace/          # Task workspace management
│   ├── workspace.go    # Create, Open, Remove, List task directories
│   └── workspace_test.go
├── cmd/jeff/           # CLI (cobra) — thin wrapper over SDK
│   ├── main.go         # Root command, config lifecycle, bare `jeff` command
│   ├── init_cmd.go     # jeff init [--here]
│   └── launch.go       # Agent tool launcher (claude, opencode)
└── *_test.go           # Unit tests alongside source
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
- **Personas are embedded**: Shipped in the binary via go:embed. Used via `--persona` flag.
- **CLAUDE.md is editable**: Default shipped in binary, written on init, user can customize, resettable.
- **Agent tool is pluggable**: jeff.yaml `agent` field. Currently: claude, opencode.
- **Global pointer**: `~/.config/jeff/home` stores JEFF_HOME path so `jeff` works from anywhere.

## Conventions

- Same standards as gig: fast, simple, extensible
- All public SDK functions return `(*Type, error)` or `error`
- Tests use `t.TempDir()` for isolation
- No CGO dependencies
- All gig interaction via SDK (`import "github.com/neerajg/gig"`), never shelling out to gig CLI
