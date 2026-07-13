# PLAN-Quality-Gates: Sync Schemas/Docs With Reality and Make CI Catch This Class of Drift Forever

> **Rank:** 10/10 · **Leverage:** Medium (each fix is small, but the class of bug — docs/schema lying to users — currently has no guard at all) · **Effort:** Small-Medium · **Prereqs:** none (if PLAN-Permission-Safety landed, include its `skip_permissions` key in the schema step)
>
> Ground rules: run `go build ./... && go vet ./... && go test ./...` after each step group.

## Why (the problem)

Verified drift between what JEFF ships and what its schemas/docs/CI claim:

1. **`schemas/jeff-config.json` rejects valid configs.** The `agent` enum is `["claude","opencode"]` — `gemini` is a fully registered first-class backend (`jeff.go:14`, `agent_gemini.go`). The `ide` enum omits `zed` (`jeff.go:95`). The persisted `memory` config object (`config.go:38`) isn't in the schema at all. Editors mark correct configs as errors.
2. **`schemas/personas.json` fails against JEFF's own output.** `SeedDefaults` writes an `agent` field into every entry (`persona/registry.go:319-327`), but the schema has no `agent` property AND sets `additionalProperties: false` (`schemas/personas.json:34`) — jeff-generated `personas.json` is schema-invalid.
3. **The post-setup script contract is documented wrong.** `docs/config.md:139-154` (and the schema description) say the script receives `src_dir`/`dest_dir` as `$1`/`$2`; the implementation passes a JSON object (`src_dir`, `dest_dir`, `repo`, `branch`) on **stdin** with no args (`workspace/worktree.go:121-132`). Every user script written from the docs is broken.
4. **Phantom and hidden personas.** `doug` is documented as a persona (root `CLAUDE.md:118`, `memory/CLAUDE.md:61`, `skill/templates/crew-orchestrator/SKILL.md:44`, `skill/templates/curation/SKILL.md:34`) but has **no template** — `--persona doug` yields nothing. `marlowe` ships (`persona/templates/marlowe.md`) but is absent from README/CLAUDE.md/help strings (`cmd/jeff/pickup_cmd.go:54` hardcodes 5 names). Model tables contradict code: README says dickson=sonnet; code says opus (`persona/persona.go:104-112`; `persona/CLAUDE.md` also wrong for dickson+schmidt).
5. **Hook-count drift:** root `CLAUDE.md` says 13 built-in hooks; the registry has 16 (`hooks/builtin.go:4-26`, pinned by `hooks/registry_test.go:84`).
6. **CI can't catch any of this.** `.github/workflows/test.yml` is PR-only (pushes to main run nothing), single-OS (ubuntu, though tmux/macOS is a primary target), no `-race` (crew is concurrency-heavy), no linter, no coverage.

## What (the goal)

- Schemas accept exactly what the code writes; a Go test enforces schema↔code sync structurally.
- Docs match shipped behavior (post-setup contract, personas, models, hook count, undocumented commands).
- CI: push+PR triggers, `-race`, macOS leg, `golangci-lint`, coverage artifact.
- Consistency tests that make the persona/doc drift impossible to reintroduce silently.

## Files to touch

| File | Change |
|---|---|
| `schemas/jeff-config.json` | Add `gemini`, `zed`, `memory` object (+ `skip_permissions` if landed) |
| `schemas/personas.json` | Add `agent` property |
| `schema_sync_test.go` | **New root test** — enums derived from code == schema enums |
| `docs/config.md` | Post-setup stdin-JSON contract; agent list; zed |
| `README.md` | Personas table (add marlowe, drop doug, fix models); command table additions (`doctor`, `notify`, `project`, `stats` if landed); Requirements mention gemini |
| `CLAUDE.md` (root) | Personas table (doug → decide, see Step 4), hook count 16 |
| `memory/CLAUDE.md`, `persona/CLAUDE.md` | doug/marlowe/model fixes |
| `skill/templates/crew-orchestrator/SKILL.md`, `skill/templates/curation/SKILL.md` | doug reference fix |
| `cmd/jeff/pickup_cmd.go:54`, `cmd/jeff/skill_cmd.go:236` | Persona flag help derived from `persona.Names()` |
| `persona/persona_test.go` | Names↔defaults consistency test |
| `.github/workflows/test.yml` | Hardened matrix |
| `.golangci.yml` | **New** — minimal linter config |
| `docs/skill_mgmt.md` | Delete (byte-identical duplicate of `skill/skill_mgmt.md`); point references at `jeff skill doc` |
| `skill/CLAUDE.md` | Fix nonexistent `InstallEmbeddedSkills()` reference → `SeedDefaults`/`SeedCuration` |

## Implementation steps

### Step 1 — fix the schemas

`schemas/jeff-config.json`:
- `agent.enum` → `["claude","opencode","gemini"]`.
- `ide.enum` → `["vscode","cursor","windsurf","nvim","zed"]`.
- Add:

```json
"memory": {
  "type": "object",
  "description": "Memory subsystem settings.",
  "properties": {
    "disabled": {"type": "boolean", "description": "Disable memory injection, capture, and proposals."}
  },
  "additionalProperties": false
}
```

- If PLAN-Permission-Safety landed: add `skip_permissions` boolean.
- Fix the `post_setup` property description to the stdin-JSON contract (see Step 3 wording).

`schemas/personas.json`: add under entry `properties`:

```json
"agent": {"type": "string", "description": "Agent tool this persona runs on (claude, opencode, gemini)."}
```

(keep `additionalProperties: false` — it now passes).

### Step 2 — the schema-sync test (the actual guard)

New root-level `schema_sync_test.go`:

```go
package jeff

// Parses schemas/jeff-config.json and asserts its enums equal the code's
// ValidNames(), so schema drift fails CI instead of failing users' editors.
func TestConfigSchemaMatchesCode(t *testing.T) {
	data, err := os.ReadFile("schemas/jeff-config.json")
	// decode into map[string]any; walk properties.agent.enum, properties.ide.enum
	// compare as string sets against AgentTool("").ValidNames() and IDE("").ValidNames()
	// also assert properties contains every top-level json tag of Config that is persisted:
	// reflect over Config struct fields, take json tags (skip "-" and "$schema"),
	// assert each is a key under properties.
}
```

Implementation notes for the executor:
- `AgentTool("").ValidNames()` derives from the provider registry (`jeff.go:69-76`) — importing the root package in its own test triggers the provider `init()`s automatically.
- The reflection walk over `Config` catches the *next* forgotten field (like `memory` was); strip `,omitempty` from tags.
- Keep it in the root package (schemas dir is adjacent; use a relative path and `t.Skip` if the file is missing so `go test ./...` from odd working dirs doesn't false-fail — actually Go tests always run in the package dir, so plain relative path is correct).

### Step 3 — post-setup contract docs

`docs/config.md` post-setup section: replace the `$1`/`$2` description with the truth (transcribe the real behavior from `workspace/worktree.go:121-132`):

> The script runs from the worktree directory and receives a JSON object on **stdin**:
> `{"src_dir": "<repo clone>", "dest_dir": "<worktree>", "repo": "<name>", "branch": "<branch>"}`
> Example:
> ```bash
> #!/usr/bin/env bash
> payload=$(cat)
> dest=$(echo "$payload" | jq -r .dest_dir)
> cp "$(echo "$payload" | jq -r .src_dir)/.env" "$dest/.env"
> ```

Mirror the same sentence into the schema's `post_setup` description.

### Step 4 — persona reconciliation

Decision rule: **code is truth.**
1. Remove `doug` rows from root `CLAUDE.md`, `memory/CLAUDE.md`, and the two SKILL.md tables (`crew-orchestrator/SKILL.md:44`, `curation/SKILL.md:34`). If the maintainer wants doug (tester persona), that's a separate feature: note it as a one-line TODO in `docs/roadmap.md` Open Questions instead.
2. Add `marlowe` to README's persona table and root `CLAUDE.md`'s (role: "Memory curator — curates proposals into canonical memory. Used by `jeff memory curate`.", model sonnet).
3. Fix model columns everywhere to match `persona/persona.go:104-112`: jenko/schmidt/dickson=opus, eric/hardy/marlowe=sonnet (README:174-180, `persona/CLAUDE.md` table).
4. Replace the hardcoded help strings: `cmd/jeff/pickup_cmd.go:54` and `cmd/jeff/skill_cmd.go:236` → `"Persona template to use ("+strings.Join(persona.Names(), ", ")+")"`. (`persona.Names()` reads the embedded FS — cheap, deterministic.)
5. Consistency test in `persona/persona_test.go` (external package `persona_test` if importing more): for every `Names()` entry, `DefaultModel(name) != ""`, `DefaultAgent(name) != ""`, `Get(name)` succeeds; and marlowe is present in `Names()`. This makes phantom/hidden personas impossible.

### Step 5 — hook count + undocumented features

1. Root `CLAUDE.md` hooks row: "16 built-in hooks" (source of truth: `hooks/registry_test.go:84`).
2. README command table: add `jeff doctor`, `jeff notify` (macOS), `jeff project init|open|list`, `jeff repo describe`, and `jeff crew attach`. Add `gemini` to Requirements ("Claude Code, opencode, or Gemini CLI").
3. `docs/usage.md`: ship `--body`; `jeff init` creates `projects/`, `.skills/`, `.personas/`, `memory/` (fix the home-layout diagram in root `CLAUDE.md` too).
4. Delete `docs/skill_mgmt.md`; grep for links to it (`grep -rn "skill_mgmt" README.md docs/ *.md`) and repoint to `jeff skill doc` / `skill/skill_mgmt.md`.
5. `skill/CLAUDE.md:25`: `InstallEmbeddedSkills()` → `SeedDefaults` (and `SeedCuration`).

### Step 6 — CI hardening

Replace `.github/workflows/test.yml` jobs with:

```yaml
name: Test
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
      - run: go build ./...
      - run: go vet ./...
      - run: go test ./... -race -count=1 -coverprofile=coverage.out
      - uses: actions/upload-artifact@v4
        with:
          name: coverage-${{ matrix.os }}
          path: coverage.out

  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
      - uses: golangci/golangci-lint-action@v7
        with:
          version: latest
```

New `.golangci.yml` (minimal, low-noise start):

```yaml
linters:
  enable:
    - govet
    - staticcheck
    - errcheck
    - unused
    - ineffassign
issues:
  exclude-dirs:
    - embed
```

**Before committing:** run the linter locally (`go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run ./...` or install the binary). The codebase has known `errcheck` debt (e.g. `os.WriteFile` in `writeIfNotExists`, `agent_claude.go:71`; unchecked `MkdirAll` in `init_cmd.go:228`). Triage: fix the trivial ones in this plan; for the rest add targeted `//nolint:errcheck // best-effort` comments or per-path excludes in `.golangci.yml` — the gate must land green, not aspirational.

### Step 7 — verify the drift is dead

Run the new tests + grep sweep; every item in "Why" maps to an acceptance check below.

## Edge cases you must handle (found during exploration)

- **`-race` on macOS runners is slow** — the crew tests already take ~10s; keep `-count=1` and don't add `-v` (log volume). If macos flakes on tmux-adjacent tests, remember the crew tests use a fake tmux (`crew/tmux_test.go:37-59`) — they don't need real tmux; any failure is real.
- **`golangci-lint` version pinning:** the action's `version: latest` can introduce new linters over time; if the maintainer prefers stability, pin (e.g. `v2.1`). Note it in the workflow comment.
- **The reflection schema test must skip unexported/non-persisted fields** — `Home` has tag `"-"` (`config.go:39`); `$schema` maps to `Schema` — handle both.
- **`persona.Names()` order** comes from embedded FS listing (alphabetical) — the help string will read `dickson, eric, hardy, jenko, marlowe, schmidt`; tests asserting the old 5-name literal (grep `pickup_test.go` for the flag help) must be updated.
- **Deleting `docs/skill_mgmt.md`:** `skill/skill_mgmt.md` is the `//go:embed` source (`skill/skill.go:15-16`) — do NOT touch that one.
- **README's brew instructions reference a tap that has never been cut** (zero git tags exist; the release workflow creates the formula on first tag). Add "(first release pending)" or leave — flag it to the maintainer in the PR/commit message rather than guessing.
- **push-trigger on main:** the workflow previously ran only on PRs; adding `push: branches: [main]` means merges re-run tests — intended, cheap.
- **`hooks/registry_test.go:84` is the hook-count source of truth** — don't write a second count test; docs just need the right number.
- **Root `CLAUDE.md` is agent-facing context** (it loads into every session) — keep edits tight; don't bloat it.

## Acceptance criteria

1. `go build ./... && go vet ./... && go test ./...` green (includes the two new consistency tests).
2. `go test ./ -run TestConfigSchemaMatchesCode -v` — passes; then temporarily remove `"gemini"` from the schema and confirm it FAILS (restore it).
3. `go test ./persona/ -run TestPersonaConsistency -v` — passes; mentions marlowe.
4. Schema validation smoke: `python3 -c "import json;json.load(open('schemas/jeff-config.json'));json.load(open('schemas/personas.json'))"` (syntactic sanity) — or a jq equivalent.
5. Grep sweep — all must return clean:
   - `grep -rn "doug" README.md CLAUDE.md memory/CLAUDE.md skill/templates/` → 0 hits.
   - `grep -n "marlowe" README.md` ≥ 1.
   - `grep -n "13 built-in" CLAUDE.md hooks/CLAUDE.md` → 0.
   - `grep -rn '\$1.*src_dir\|src_dir.*\$1' docs/config.md schemas/` → 0.
   - `grep -n "dickson, eric, hardy, jenko, schmidt" cmd/jeff/*.go` → 0 (now derived).
   - `test ! -f docs/skill_mgmt.md`.
6. `golangci-lint run ./...` exits 0 locally.
7. Workflow YAML parses: `python3 -c "import yaml;yaml.safe_load(open('.github/workflows/test.yml'))"` (or push and watch the run).

## Out of scope

- Cutting the first release/tag and validating the brew formula end-to-end (maintainer action; flag it).
- goreleaser/prebuilt binaries in release.yml.
- Rewriting `docs/adding-commands.md`'s full command inventory (worth a later docs pass; this plan fixes the *wrong* statements, not all the *missing* ones — except the README command-table additions listed).
- Windows CI (unsupported platform — tmux/symlinks).
