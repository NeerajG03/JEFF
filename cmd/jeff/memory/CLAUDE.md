# cmd/jeff/memory/ — `jeff memory` cobra subtree

One file per subcommand. `memory_cmd.go`'s `init()` registers them on the root `Cmd`.

| File | Command |
|------|---------|
| `memory_cmd.go` | root (`jeff memory`) — registers all subcommands |
| `propose_cmd.go` | `jeff memory propose` — workers write proposals |
| `add_cmd.go` | `jeff memory add` — curator writes canonical (gated by `JEFF_MEMORY_CAN_ADD`) |
| `curate_cmd.go` | `jeff memory curate` — run marlowe |
| `list_cmd.go` | `jeff memory list` |
| `show_cmd.go` | `jeff memory show` |
| `disable_cmd.go` | `jeff memory disable` (toggle disabled flag) |
| `session_start_cmd.go` | `jeff memory session-start` (hook infrastructure) |
| `session_end_cmd.go` | `jeff memory session-end` (hook infrastructure) |

## Conventions

- Package name: `memory`. Imported in `cmd/jeff/main.go` as `memorycmd "github.com/NeerajG03/JEFF/cmd/jeff/memory"` to avoid clashing with `github.com/NeerajG03/JEFF/memory`.
- The root `Cmd` is exported; everything else is package-private.

## See also

- `../../memory/CLAUDE.md` — package-level design notes
- `docs/usage.md` — Memory section documents the restored commands
