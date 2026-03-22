// Package jeff provides an agent workspace management system built on gig.
//
// JEFF manages the filesystem layout, persona templates, and workflow
// orchestration for AI agents working on gig tasks.
//
// Layout:
//
//	jeff.go          # Core types: Config, JeffHome resolution
//	config.go        # LoadConfig, SaveConfig, DefaultJeffHome, jeff.json parsing
//	persona/         # Embedded persona templates
//	  personas.go    # //go:embed + GetPersona, ListPersonas
//	workspace/       # Task workspace and worktree management
//	  workspace.go   # Create, Open, Remove task workspaces
//	  worktree.go    # Git worktree add/rm, symlink into task dir
//	cmd/jeff/        # CLI (cobra) — thin wrapper over SDK
//	  main.go        # Root command, config lifecycle, bare `jeff` command
package jeff
