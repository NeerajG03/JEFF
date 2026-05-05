# cmd/jeff/memory/ — `jeff memory` cobra subtree

One file per subcommand. Each file declares a lowercase `*Cmd` var; `memory_cmd.go`'s `init()` registers them on the root `Cmd`. Worker letters (B/C/D) own implementations — FND ships only stubs that return `errors.New("not yet implemented: Worker <X> will fill this in")`.

| File | Worker | Command |
|------|--------|---------|
| `memory_cmd.go` | FND | root (`jeff memory`) — registers all subcommands |
| `propose_cmd.go` | B | `jeff memory propose` — workers write proposals |
| `add_cmd.go` | B | `jeff memory add` — curator writes canonical (gated by `JEFF_MEMORY_CAN_ADD`) |
| `curate_cmd.go` | C | `jeff memory curate` — run marlowe |
| `list_cmd.go` | D | `jeff memory list` |
| `show_cmd.go` | D | `jeff memory show` |
| `status_cmd.go` | D | `jeff memory status` |
| `diff_cmd.go` | D | `jeff memory diff` |
| `disable_cmd.go` | D | `jeff memory disable` (soft-invalidate) |
| `doc_cmd.go` | D | `jeff memory doc` |

## Conventions

- Package name: `memory`. Imported in `cmd/jeff/main.go` as `memorycmd "github.com/NeerajG03/JEFF/cmd/jeff/memory"` to avoid clashing with `github.com/NeerajG03/JEFF/memory`.
- The root `Cmd` is exported; everything else is package-private.
- Stubs must keep `Use:` and `Short:` accurate so `jeff memory --help` is informative even before workers fill in `RunE`.
- When you (B/C/D) implement a stub, also drop the `// stub — Worker X fills in` header and update this table.

## See also

- `../../memory/CLAUDE.md` — package-level design notes
- `exports/memory-research/specs/{B-capture,C-curate,D-introspect}.md` — worker specs
