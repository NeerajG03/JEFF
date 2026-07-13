# PLAN-Agent-Providers: Collapse the Stringly-Typed Agent Dispatch into the Provider Registry

> **Rank:** 8/10 · **Leverage:** Strategic (turns "add an agent = ~15-file edit" into "~2-file edit"; fixes real opencode/gemini gaps today) · **Effort:** Medium-Large · **Prereqs:** best done AFTER PLAN-Hooks-Hardening (its `bashBoth` helper reshapes the biggest string-key surface) and PLAN-Permission-Safety (touches the same provider methods)
>
> Ground rules: run `go build ./... && go vet ./... && go test ./...` after each step group. This is a refactor — behavior must not change except where a gap is explicitly closed.

## Why (the problem)

`AgentProvider` (`agent.go:17-63`) is a clean seam, and launching honors it. But a parallel, stringly-typed dispatch system decides everything else, spread across ~15 files. Verified consequences today:

1. **`crew/crew.go:84-100` (`buildAgentCmd`)** — the legacy fallback for sessions with no stored command hardcodes `claude --dangerously-skip-permissions` and defaults empty agent to claude. **Wrong for gemini and opencode sessions** (neither accepts that flag) — a resumed legacy gemini worker gets a broken command line.
2. **`crew/lifecycle.go:455-470`** — send/interrupt timing branches on `agent == "gemini"` string literals.
3. **`memory/suppress.go:23-52`** and **`memory/inject.go:40-49`** — `switch agentKind` string switches for native-memory suppression, context filename, and addendum template. An unknown agent silently gets claude behavior.
4. **`cmd/jeff/doctor_cmd.go:67-80`** — dependency list hardcodes claude + gemini; **opencode is missing entirely** even when it's the configured agent.
5. **`jeff.go:21-61`** — `InferBackend`/`IsValidModel`/`ValidModelsForBackend` know only claude + gemini model families in hand-written switches.
6. **`persona/persona.go:103-128`** — `DefaultAgent` hardcodes `"claude"` per persona (fine as data, but the string should be validated against the registry).
7. **`cmd/jeff/memory/session_start_cmd.go:38` / `session_end_cmd.go:40`** — `--agent` flag help says `claude | gemini`, defaults `"claude"`, no validation against the registry.

The provider registry and the string maps are two sources of truth that already disagree (opencode gaps above).

## What (the goal)

Extend `AgentProvider` so every per-agent decision lives on the provider, delete the scattered switches, and make `RegisteredAgents()` the single enumeration everywhere (doctor, help text, validation).

## Files to touch

| File | Change |
|---|---|
| `agent.go` | Interface additions (below) |
| `agent_claude.go`, `agent_gemini.go`, `agent_opencode.go` | Implement them |
| `jeff.go` | `InferBackend`/`IsValidModel`/`ValidModelsForBackend` iterate providers |
| `crew/crew.go` | `buildAgentCmd` uses the provider |
| `crew/lifecycle.go` | Timing from provider |
| `memory/suppress.go`, `memory/inject.go` | Dispatch via provider lookups — **see import-cycle note first** |
| `cmd/jeff/doctor_cmd.go` | Deps from `RegisteredAgents()` |
| `cmd/jeff/memory/session_start_cmd.go`, `session_end_cmd.go` | Help/validation from registry |
| `agent_test.go`, `crew/*_test.go`, `memory/*_test.go` | Updated tests |

## Implementation steps

### Step 0 — import-cycle survey (do this first, it shapes everything)

The `memory` package must not import the root `jeff` package if any root file imports `memory` — run `grep -rn '"github.com/NeerajG03/JEFF/memory"' *.go` (root only). As of writing, the root package does **not** import `memory`, so `memory → jeff` is legal. Verify the same for `crew → jeff`: `grep -rn '"github.com/NeerajG03/JEFF"' crew/*.go` — `crew` currently does NOT import the root package (it stores agent as a string). Decide per package:

- **`crew`**: adding a root import is acceptable IF root imports nothing from `crew` (`grep -rn '"github.com/NeerajG03/JEFF/crew"' *.go` — root does not). Proceed.
- **`memory`**: same check; if a cycle would appear at any point, instead define narrow function variables injected from `cmd/jeff` (e.g. `memory.ContextFileNameFn`) — but prefer the direct import while it's legal.

Document the resulting allowed-import graph in root `CLAUDE.md` (one line: `crew, memory may import the root package; the root package must never import them`).

### Step 1 — interface additions

Add to `AgentProvider` (`agent.go`), with doc comments:

```go
// ContextFileName returns the primary context filename (e.g. "CLAUDE.md", "GEMINI.md").
ContextFileName() string

// MemorySuppressEnv returns env assignments that disable the agent's native
// memory, or nil when not applicable.
MemorySuppressEnv() map[string]string

// SendTiming returns tmux delivery tuning for this agent's TUI.
SendTiming() SendTiming

// OwnsModel reports whether the model alias/id belongs to this agent's family.
OwnsModel(model string) bool

// ModelExamples returns human-readable model names for error messages.
ModelExamples() []string

// DoctorDeps returns binaries this agent needs, checked by jeff doctor.
DoctorDeps() []DoctorDep
```

With supporting types in `agent.go`:

```go
type SendTiming struct {
	PasteDelay      time.Duration // between paste and Enter (0 = default 100ms)
	InterruptSettle time.Duration // after C-c before typing (divert)
	UseBracketedPaste bool        // route via load-buffer/paste-buffer -p
}

type DoctorDep struct {
	Name     string // binary name
	Required bool
	Hint     string // install hint
}
```

Populate from today's literals:
- claude: `ContextFileName "CLAUDE.md"`, suppress env from `memory/suppress.go:23-28` claude case, timing {UseBracketedPaste:true (claude routes via buffer per `crew/lifecycle.go:465-470`), InterruptSettle: current claude value at `lifecycle.go:448-459`}, `OwnsModel` = existing `isClaudeModel`, deps {claude required}.
- gemini: `"GEMINI.md"`, gemini env (`GEMINI_NO_AUTO_MEMORY` — see `memory/suppress.go:74-92`), timing {UseBracketedPaste:true, PasteDelay:500ms, InterruptSettle:4s — from `lifecycle.go:440-459`}, `OwnsModel` = `isGeminiModel`, deps {gemini optional-if-configured → keep it simple: required}.
- opencode: `"CLAUDE.md"` (opencode reads CLAUDE.md? — verify what today's inject does for opencode: `memory/inject.go:47-52` defaults to CLAUDE.md for anything not gemini; mirror that), nil suppress env (or current opencode case if one exists in `suppress.go:23-52`), default timing, `OwnsModel` returns false (no model routing), deps {opencode}.

Exact current values MUST be read from the cited lines, not from this document — transcribe, don't guess.

### Step 2 — model routing via providers

Rewrite in `jeff.go`:

```go
func InferBackend(model string) AgentTool {
	if model == "" { return "" }
	for _, t := range RegisteredAgents() {
		if GetProvider(t).OwnsModel(model) { return t }
	}
	return ""
}

func IsValidModel(agent AgentTool, model string) bool {
	p := GetProvider(agent)
	return p != nil && p.OwnsModel(model)
}

func ValidModelsForBackend(agent AgentTool) []string {
	if p := GetProvider(agent); p != nil { return p.ModelExamples() }
	return nil
}
```

`UnknownModelError` iterates `RegisteredAgents()` building the per-family lines instead of hardcoding two. Keep `isClaudeModel`/`isGeminiModel` as private helpers inside their provider files. Existing tests in `agent_test.go`/root tests pin current behavior — they must pass unchanged.

### Step 3 — crew uses providers

1. `buildAgentCmd` (`crew/crew.go:84-100`): replace the hardcoded fallback with:

```go
p := jeff.GetProvider(jeff.AgentTool(agentName))
if p == nil {
	p = jeff.GetProvider(jeff.AgentClaudeCode) // legacy rows: preserve old default
}
parts := append([]string{p.Command()}, p.BuildLaunchArgs(jeff.LaunchOpts{Model: model, SkipPermissions: skip})...)
```

(where `skip` comes from PLAN-Permission-Safety's threading; if that plan hasn't landed, pass `SkipPermissions: true` to preserve behavior).

2. `crew/lifecycle.go:455-470`: replace `agent == "gemini"` branches with `timing := providerTiming(sess.Agent)` where `providerTiming` resolves via `jeff.GetProvider` with a safe default struct for unknown agents. The constants move into the provider `SendTiming()` implementations.

### Step 4 — memory uses providers

- `memory/inject.go:40-49`: `contextFilePath`/`addendumTemplate` resolve via `jeff.GetProvider(jeff.AgentTool(agentKind))` → `ContextFileName()`; template selection keys off `ContextFileName() == "GEMINI.md"` today's split (or add a provider method `MemoryAddendumTemplate() string` returning the embedded template name — simplest: keep the two templates, select by ContextFileName). Unknown agent → CLAUDE.md (today's behavior).
- `memory/suppress.go:23-52`: iterate `MemorySuppressEnv()` from the provider instead of switching on strings. The settings-file application logic stays; only the per-agent data moves.

### Step 5 — doctor + CLI help from the registry

- `cmd/jeff/doctor_cmd.go:37-81`: replace the hardcoded agent entries with a loop over `jeff.RegisteredAgents()` collecting `DoctorDeps()` (dedupe by binary name; the non-agent deps — tmux, git, gh, jq, terminal-notifier — stay as-is).
- `cmd/jeff/memory/session_start_cmd.go` / `session_end_cmd.go`: `--agent` help = `strings.Join(jeff.AgentTool("").ValidNames(), " | ")`; validate the value with `jeff.AgentTool(v).IsValid()` and error with the same list.
- `cmd/jeff/config_cmd.go:78` (`agent [claude|opencode]` help drift): derive from `ValidNames()` too.
- `persona/persona.go:115-128` `DefaultAgent`: keep the data map, but add a package test asserting every default agent `IsValid()` — catches typos when agents are added/renamed. (persona → jeff import: verify no cycle — root does not import persona? `grep -rn '"github.com/NeerajG03/JEFF/persona"' *.go` — if root is clean, importing jeff from persona_test only (external test package `persona_test`) avoids any cycle risk entirely; do that.)

### Step 6 — tests

1. `agent_test.go`: table-test the new methods for all three providers (ContextFileName, OwnsModel positives/negatives, ModelExamples non-empty, DoctorDeps non-empty).
2. `TestInferBackend` (exists? extend): `sonnet→claude`, `flash→gemini`, `claude-x→claude`, `gemini-x→gemini`, `gpt-4→""`.
3. `crew`: `TestBuildAgentCmd` extended for gemini + opencode rows — asserts NO `--dangerously-skip-permissions` for them (this pins the bug fix).
4. `memory`: suppress/inject tests updated to construct via provider lookups; add unknown-agent fallback cases.
5. Doctor: assert `jq`... (from PLAN-Hooks-Hardening) plus one entry per registered agent (`--json` output contains "claude","gemini","opencode").

### Step 7 — docs

Root `CLAUDE.md`: add the "adding a new agent backend" recipe (implement `AgentProvider` in one file + hooks delivery file + done). `docs/config.md` agent section lists all registered agents.

## Edge cases you must handle (found during exploration)

- **Do not attempt to registry-drive `hooks/builtin.go`'s script maps in this plan.** The delivery keys ("claude"/"gemini"/"opencode") are *hook-delivery* identities, deliberately decoupled via `HookDeliveryKey()` — PLAN-Hooks-Hardening's `bashBoth` already collapses the duplication. Registry-driving which *hooks* an agent gets requires knowing each agent's event capabilities (opencode has SessionStart only) — that stays explicit.
- **`crew` gaining a root-package import** changes the package graph — confirm `go build ./...` has no cycle before writing code (Step 0). If a cycle exists via some transitive edge, fall back to a `crew.ProviderHooks` struct of function fields set from `cmd/jeff/main.go` `init` — but only as plan B.
- **Legacy sessions with empty `agent` column** (pre-migration rows) must keep resolving to claude — pin with a test (`crew/crew_test.go` has the fixture pattern).
- **The gemini `--resume latest` quirk** (`agent_gemini.go:43-45` ignores the session ID) and `flash` pinning (`resolveGeminiModel`) are provider-internal — don't move or "fix" them here.
- **`SendTiming` zero values:** unknown/default agents must get today's generic path (100ms paste delay, plain send-keys). Make the resolver return an explicit default struct, never zero-with-meaning.
- **`ValidModelsForBackend` error-message stability:** `UnknownModelError` output is asserted in tests (grep for it) — keep the message shape or update the tests deliberately.
- **opencode `SkillsSubdir() == ""`** semantics (unsupported) ripple through skill injection loops (`crew_cmd.go:1252-1275` skip empty) — your interface additions must not disturb that convention.
- **Do the mechanical moves one consumer at a time** (crew, then memory, then doctor), compiling+testing between each — a big-bang refactor of 15 files is where a less capable model wrecks itself.

## Acceptance criteria

1. `go build ./... && go vet ./... && go test ./...` green after EVERY step, not just the end.
2. Grep checks (the point of the plan):
   - `grep -rn '== "gemini"' crew/ memory/` → 0 hits.
   - `grep -rn '"claude"' crew/crew.go` → only the legacy-default comment/lookup remains (no hardcoded flag strings).
   - `grep -n "dangerously-skip-permissions" crew/crew.go` → 0.
   - `grep -rn 'case "claude"' memory/` → 0.
   - `grep -n "RegisteredAgents()" cmd/jeff/doctor_cmd.go` → present.
3. `go test ./crew/ -run TestBuildAgentCmd -v` — gemini/opencode rows produce provider-correct commands.
4. `jeff doctor --json` (or its unit test) lists deps for all three agents + jq.
5. Adding a hypothetical agent: write a scratch `agent_fake_test.go` registering a fake provider in a test and assert `InferBackend`/doctor-dep enumeration pick it up with zero other edits (then the test file is the living proof; keep it as `TestProviderRegistryExtensibility`).

## Out of scope

- Gemini hook-protocol correctness (needs external verification).
- OpenCode PostToolUse/Stop support.
- IDE enum registry-fication (`jeff.go:87-142`) — same pattern, different axis; cheap to do later.
- `persona.DefaultAgent` becoming config-driven (data location is fine; only validation is added).
