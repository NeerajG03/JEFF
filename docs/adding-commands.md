# Adding a New CLI Command

## Steps

1. **SDK first**: Add business logic in the root package or sub-package (`workspace/`, `persona/`, `hooks/`).
2. **Wire the command**: Add `func fooCmd() *cobra.Command` in `cmd/jeff/`.
3. **Register it**: Add `fooCmd()` to `rootCmd.AddCommand(...)` in `cmd/jeff/main.go`.
4. **Use `openGigStore()`**: For any command that needs gig access, use the shared helper in `cmd/jeff/gig.go`. Always `defer store.Close()`.
5. **Config access**: Use the package-level `cfg *jeff.Config` (loaded in `PersistentPreRunE`). Skip config load for commands that don't need it (like `init`) by adding to the skip list.

## Existing Command Files

| File | Commands |
|------|----------|
| `init_cmd.go` | `jeff init [--here]` |
| `pickup_cmd.go` | `jeff pickup <gig-id> [--persona] [--repos]` + task CLAUDE.md generation |
| `work_cmd.go` | `jeff work <gig-id>` |
| `done_cmd.go` | `jeff done <gig-id> [--reason]` |
| `status_cmd.go` | `jeff status` |
| `checkpoint_cmd.go` | `jeff checkpoint --task <id> --done "..."` |
| `repo_cmd.go` | `jeff repo add/list/remove/post-setup` |
| `worktree_cmd.go` | `jeff worktree add/rm/list` |
| `config_cmd.go` | `jeff config agent/ide/hooks/reset-claude-md` |
| `open_cmd.go` | `jeff open [<gig-id>]` |
| `completion.go` | `jeff completion [bash\|zsh\|fish]` |

## Helpers

- `openGigStore()` (`gig.go`) — opens gig store using default config resolution.
- `launchAgent(dir, agent)` (`launch.go`) — exec agent tool in a directory.
- `writeTaskClaudeMD(taskDir, task, persona)` (`pickup_cmd.go`) — generate task-specific CLAUDE.md with workspace layout.
- `refreshTaskClaudeMD(taskDir, store, taskID)` (`pickup_cmd.go`) — regenerate task CLAUDE.md preserving persona.
- `buildTaskJSON(store, task)` (`pickup_cmd.go`) — task JSON with custom attributes for branch naming scripts.
- `resolveRepoBranch(rc, taskJSON, default)` (`pickup_cmd.go`) — resolve branch name from config script or default.

## Checklist

- [ ] SDK function in root/sub-package (not in `cmd/`)
- [ ] Error wrapping with `fmt.Errorf("context: %w", err)`
- [ ] gig interaction via SDK, not CLI
- [ ] `defer store.Close()` after `openGigStore()`
- [ ] Test in `*_test.go` using temp dirs
