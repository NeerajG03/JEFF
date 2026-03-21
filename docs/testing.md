# Testing

## Running Tests

```bash
go test ./...           # All tests
go test -v ./...        # Verbose
go test -run TestSlug   # Specific test by name
go test -count=1 ./...  # Disable cache
```

## Test Helpers

- **`tempHome(t)`** (`config_test.go`): Returns a temp dir path for JEFF_HOME.
- **`tempJeffHome(t)`** (`workspace/workspace_test.go`): Creates a temp JEFF_HOME with `tasks/` dir.
- **`tempGigStore(t)`** (`attrs_test.go`): Opens a temp gig store for testing SDK interactions.

## How to Write Tests

1. **One test file per source file**: `config.go` → `config_test.go`, etc.
2. **Use temp dirs**: `t.TempDir()` for isolation. Never touch real JEFF_HOME or gig DB.
3. **Test SDK, not CLI**: Exercise SDK methods directly. CLI is thin.
4. **Don't test gig internals**: Trust gig SDK. Test JEFF's usage of gig (attrs defined, checkpoints created).

## Existing Test Files

| File | What it tests |
|------|--------------|
| `config_test.go` | Defaults, save/load round-trip, invalid agent fallback, ResolveHome, WriteHomePointer, AgentTool |
| `repo_test.go` | repoNameFromURL, ListRepos, AddRepo duplicate, RemoveRepo not registered |
| `attrs_test.go` | EnsureAttrs defines repos + worktree_setup, idempotency |
| `persona/persona_test.go` | Names, Get, IsValid, all 4 personas present |
| `workspace/workspace_test.go` | Create, Open, Remove, List, makeSlug, ExtractTaskID |
| `workspace/worktree_test.go` | symlinkIntoTask, idempotent symlink, WorktreeList, WorktreeAdd missing repo, ReadBaseBranch, ResolveBranchName |
| `hooks/registry_test.go` | Register, Get, BySource, duplicate panic, DefaultRegistry |
| `hooks/claude_test.go` | readSettings/writeSettings, add/remove entries, idempotent install |
| `hooks/opencode_test.go` | Generate combined plugin, empty = no file |
| `hooks/manager_test.go` | Sync installs/removes, idempotent, Installed |
| `hooks/builtin_test.go` | Each hook generates non-empty script content |
| `cmd/jeff/pickup_test.go` | writeTaskClaudeMD (persona, no persona, worktrees, workspace layout), detectPersona, listWorktreeSymlinks, buildTaskJSON, resolveRepoBranch (E2E with gig store + branch script) |
