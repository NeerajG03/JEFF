# Roadmap

JEFF's evolution from workspace manager to autonomous agent system. Each phase is independently valuable — later phases build on earlier ones but don't gate their usefulness.

## Vision

Today: user runs one agent, manually manages context, starts fresh every session, learns nothing across tasks.

Future: personas accumulate knowledge, teams self-organize, quality gates catch mistakes the agent has seen before, and the user's job shifts from doing to approving.

The bottleneck is the user. JEFF removes that bottleneck incrementally — more context per session, then memory across sessions, then agents managing agents, then self-improving agents.

## Architecture: JEFF + Claude Code Agent Teams

JEFF does not build its own orchestration layer. Claude Code Agent Teams handles multi-agent coordination (spawning, messaging, task claiming, delegate mode). JEFF provides the infrastructure that makes Agent Teams effective for sustained work:

```
┌────────────────────────────────────────────────────────────┐
│                    Claude Code Agent Teams                  │
│  spawning · messaging · delegation · task claiming · tmux  │
└──────────────────────────┬─────────────────────────────────┘
                           │ uses
┌──────────────────────────▼─────────────────────────────────┐
│                          JEFF                               │
│  gig task state · worktrees · persona memory · skills      │
│  repo learnings · shipping pipeline · checkpoints          │
└──────────────────────────┬─────────────────────────────────┘
                           │ backed by
┌──────────────────────────▼─────────────────────────────────┐
│                          gig                                │
│  tasks · events · checkpoints · dependencies · attributes  │
└────────────────────────────────────────────────────────────┘
```

Agent Teams is the runtime. JEFF is the context layer. Gig is the state layer.

## Levels of Autonomy

Levels coexist. A user at Level 3 still drops into Level 1 for quick solo tasks. The system supports all levels simultaneously.

### Level 1: Single Agent, Rich Context

User runs one agent per task. JEFF makes that agent excellent by front-loading context.

**Current state:** Pickup creates workspace, branches worktree, injects skills, writes task CLAUDE.md, launches agent. Checkpoints save progress to gig. Ship creates PRs.

**What's missing:** No persona memory (agent starts fresh every session). No repo learnings (agent rediscovers the same patterns). No checkpoint injection on resume (context lost between sessions). Captain template references `jeff plan` and `jeff delegate` which don't exist.

**What pickup looks like after this phase:**

```bash
jeff pickup gig-42 --persona jock --repos backend
```

1. Gig task claimed, workspace created, worktree branched
2. `PERSONA.md` loaded (static role template)
3. `personas/jock/memory/MEMORY.md` loaded (accumulated persona knowledge)
4. `learnings/backend/INDEX.md` loaded (accumulated repo knowledge)
5. Latest checkpoint injected if resuming
6. Matching skills symlinked
7. Gig attrs set: `persona`, `skills_loaded`, `memory_loaded`
8. Agent launched in task directory

### Level 2: Agent Teams Integration

User says "this needs a team." JEFF sets up infrastructure, Agent Teams handles orchestration.

```bash
jeff pickup gig-42 --team --repos backend frontend
```

1. Everything from Level 1, plus:
2. Subtask creation in gig (from user-approved decomposition)
3. Task CLAUDE.md includes Agent Teams instructions
4. Team lead runs in delegate mode (captain behavior without needing a captain template)
5. Each teammate gets persona-appropriate memory + skills via workspace CLAUDE.md
6. `TeammateIdle` hook triggers checkpoint writes to gig
7. `TaskCompleted` hook runs quality gates
8. On close, `jeff learn` curates memories from all teammates' work

**Key insight:** Captain persona maps to Agent Teams' delegate mode on the team lead. The captain doesn't need to be a separate agent spawned by JEFF — it's how the team lead behaves when `--team` is passed.

### Level 3: Daemon + Auto-Routing

A `jeff daemon` process watches gig for ready tasks and auto-sets up workspaces + teams.

```bash
jeff daemon start
```

1. Daemon uses `store.On(StatusChanged, ...)` to react to gig events
2. Routing rules in `jeff.json` determine: solo or team? which persona(s)? which repos?
3. Daemon runs `jeff pickup` programmatically
4. For teams: spawns Agent Teams session in tmux
5. For solo: spawns interactive Claude session in tmux pane
6. User monitors via `jeff dashboard` (TUI over `jeff stats`)
7. Quality gates block shipping until human approves
8. `jeff approve gig-42` triggers `jeff ship`

**The user can always take the wheel.** tmux means every agent session is visible and interactive. Click into any pane, type, redirect. `jeff daemon pause` stops auto-pickup. `jeff reject gig-42 --reason "wrong approach"` sends work back.

**Gig event system makes this possible.** No polling. `store.On(eventType, callback)` fires on every mutation. `store.EventsSince(time)` queries history. The daemon is an event loop over gig's store, not a cron job.

---

## Phase 1: Persona Memory + Repo Learnings

### Persona Memory

Each persona accumulates knowledge across sessions. Memory is scoped to the persona role, not to any specific repo.

**Examples of persona-scoped knowledge:**
- Jock: "This user prefers early returns over nested ifs"
- Jock: "Always run `go vet` before committing Go code"
- Scout: "Check for N+1 queries in any ORM code"
- Scout: "This user wants inline comments, not PR comment threads"
- Nerd: "Architecture decisions are in docs/adr/ in most repos"

**Directory structure:**

```
JEFF_HOME/
└── personas/
    ├── jock/
    │   ├── PERSONA.md                  # static template (embedded, resetable)
    │   └── memory/
    │       ├── MEMORY.md               # index file, always loaded on pickup
    │       ├── code-style.md           # topic: user's code preferences
    │       ├── debugging-patterns.md   # topic: patterns that solved past bugs
    │       └── mistakes.md             # topic: things to avoid (from PR rejections)
    ├── scout/
    │   └── memory/
    │       ├── MEMORY.md
    │       └── review-checklist.md     # topic: learned review patterns
    ├── nerd/
    │   └── memory/
    │       ├── MEMORY.md
    │       └── research-sources.md     # topic: where to find answers
    └── captain/
        └── memory/
            ├── MEMORY.md
            └── decomposition.md        # topic: how to break down task types
```

**MEMORY.md format (index file):**

```markdown
# Jock Memory

Last updated: 2026-03-20

## Code Style
User prefers early returns, minimal nesting, table-driven tests in Go.
→ details: code-style.md

## Debugging
When seeing timeout errors, check context cancellation first.
→ details: debugging-patterns.md

## Mistakes to Avoid
Never mock the database in integration tests — use t.TempDir() with SQLite.
→ details: mistakes.md
```

**Constraints:**
- MEMORY.md stays under 200 lines (always fully loaded into context)
- Each entry in MEMORY.md is a 1-2 line summary with a pointer to the detail file
- Detail files are available on demand (agent reads when the topic is relevant)
- Modeled after Claude Code's auto-memory: short descriptions the agent can scan, with deeper reads when needed

**Loading on pickup:**

During `jeff pickup`, the persona's `MEMORY.md` is read and included in the task's CLAUDE.md under a `## Persona Memory` section. The detail file paths are listed so the agent knows where to look for more depth.

### Repo Learnings

Repo-scoped knowledge that applies regardless of persona. Any persona working on `backend` gets `backend`'s learnings.

**Examples of repo-scoped knowledge:**
- "Run `make migrate` before tests"
- "The `internal/auth` package has no tests — add them for any changes there"
- "CI requires `go generate ./...` to be clean"
- "PR template is in `.github/PULL_REQUEST_TEMPLATE.md` — always fill it out"

**Directory structure:**

```
JEFF_HOME/
└── learnings/
    ├── backend/
    │   ├── INDEX.md              # always loaded when repo is in task
    │   ├── testing.md            # topic: test setup and quirks
    │   └── ci.md                 # topic: CI pipeline specifics
    └── frontend/
        ├── INDEX.md
        └── tooling.md            # topic: build tools and configs
```

**INDEX.md format:**

```markdown
# backend learnings

Last updated: 2026-03-18

## Testing
Run `make migrate-test` before the test suite. Tests use a real SQLite DB, not mocks.
→ details: testing.md

## CI
CI runs `go vet`, `go test`, and `golangci-lint`. Generate files must be committed.
→ details: ci.md
```

**Same constraints as persona memory:** INDEX.md under 200 lines, detail files on demand.

**Loading on pickup:**

For each repo in the task's `--repos` list, the repo's `INDEX.md` is included in the task CLAUDE.md under `## Repo Learnings: <name>`.

### Checkpoint Injection on Resume

When `jeff work gig-42` resumes a task, the latest checkpoint is injected into the agent's starting context:

```markdown
## Resuming Task gig-42

### Last Checkpoint (2026-03-22 14:30)
**Done:** Implemented auth middleware, added JWT validation
**Decisions:** Used RS256 over HS256 for key rotation support
**Next:** Add refresh token endpoint, write integration tests
**Blockers:** None
**Files touched:** internal/auth/middleware.go, internal/auth/jwt.go
```

This uses `store.LatestCheckpoint(taskID)` which already exists. The injection point is the task CLAUDE.md regeneration on `jeff work`.

### New Gig Attributes

Extend `attrs.go` with attributes that enable the data layer:

```go
const (
    AttrRepos          = "repos"
    AttrWorktreeSetup  = "worktree_setup"

    // Phase 1: new attributes
    AttrPersona        = "persona"          // string: which persona worked this task
    AttrSkillsLoaded   = "skills_loaded"    // object: JSON array of skill names injected
    AttrMemoryLoaded   = "memory_loaded"    // object: JSON array of memory files loaded
    AttrTeamSize       = "team_size"        // string: "1" for solo, "3" for team
    AttrOutcome        = "outcome"          // string: "shipped" | "rejected" | "abandoned"
    AttrRejectionCount = "rejection_count"  // string: how many times PR was sent back
)
```

Set on pickup: `persona`, `skills_loaded`, `memory_loaded`, `team_size`.
Set on completion: `outcome`, `rejection_count`.

These cost nothing to store (gig attributes are key-value pairs in SQLite) but make every task queryable for stats.

### Captain Template Fix

Current captain template references `jeff plan` and `jeff delegate` which don't exist. Two options:

**Option A:** Remove captain as a standalone persona. Its role is fulfilled by Agent Teams' delegate mode on the team lead. Captain-specific memory still exists for decomposition patterns.

**Option B:** Rewrite captain template to use actual commands. Captain uses `gig` to create subtasks, uses `jeff checkpoint` to record plans, and the user manually delegates by running `jeff pickup` on subtasks.

Decision: defer until Phase 2 (Agent Teams integration) clarifies the captain's role.

---

## Phase 2: Learning Loop

### `jeff learn` — Post-Task Curation

After a task closes, extract and curate learnings.

```bash
jeff learn gig-42                    # curate from completed task
jeff learn gig-42 --auto             # use curator agent instead of human
jeff learn                           # auto-detect from cwd
```

**What it reads:**
1. All checkpoints for the task (`store.ListCheckpoints(taskID)`)
2. Full event timeline (`store.Events(taskID)`)
3. Task attributes (persona, repos, outcome, rejection count)
4. `tasks/<id>/scratchpad.md` if it exists (raw agent observations)
5. PR feedback if available (from gig comments or linked PR)

**What it produces:**

A `tasks/<id>/learnings-draft.md` file with candidate entries:

```markdown
# Learning Candidates for gig-42

## Persona: jock
- [NEW] When adding middleware in backend, always register it in cmd/server/main.go router setup — agent spent 20 min debugging why middleware wasn't running
- [UPDATE mistakes.md] JWT validation: always check `exp` claim timezone — it's UTC in this codebase

## Repo: backend
- [NEW] The auth package uses a custom `Claims` struct, not `jwt.StandardClaims`
- [UPDATE testing.md] Auth integration tests need `TEST_JWT_KEY` env var set
```

**Curation modes:**

1. **Human review (default):** `jeff learn` produces the draft, opens it in the IDE, user edits/approves, then `jeff learn --apply` writes approved entries to memory dirs and updates indexes.

2. **Curator agent (`--auto`):** A Claude session reads the draft, applies it using the rules below, and presents a summary for quick user confirmation. The agent can:
   - Add new entries to MEMORY.md / INDEX.md
   - Create new topic files
   - Update existing topic files with new information
   - The agent cannot delete entries (only humans delete memory)

**Rules for memory curation:**
- Each MEMORY.md / INDEX.md entry is 1-2 lines max
- If a topic file grows over 50 lines, split it
- Duplicates are merged, not added
- Entries must be actionable ("do X when Y") not narrative ("we did X")
- Rejections and mistakes get priority — they prevent future failures

### Scratchpad Convention

During a task, the agent writes raw observations to `tasks/<id>/scratchpad.md`. This is unstructured — the agent dumps anything it thinks might be worth remembering:

```markdown
# Scratchpad

- The error message "context deadline exceeded" was misleading — actual issue was DB connection pool exhaustion
- User corrected me: they want `errors.Is()` not `errors.As()` for sentinel errors
- Found that `internal/cache` has a TTL bug but it's out of scope for this task
- PR review feedback: "always add table-driven tests for new public functions"
```

The scratchpad is ephemeral — it lives in the task directory and feeds into `jeff learn`. It's not memory. It's the raw material that gets refined into memory.

**Agent writes to scratchpad. Agent reads from memory. The curation step in between is the quality gate.**

### Budget Guard Hook

A SessionStart hook that warns the agent about context budget:

```bash
# hooks/budget-guard.sh
# Injected as a Claude Code hook, runs at session start
# Outputs a JSON instruction reminding the agent to checkpoint before context fills
```

The hook adds a system instruction: "When you've made significant progress but haven't checkpointed in a while, run `jeff checkpoint`. If you're approaching context limits, checkpoint your state so work can be resumed."

This isn't a hard limit — it's a behavioral nudge injected via the hook system.

---

## Phase 3: Observability (`jeff stats`)

### The Data Layer

No separate database. All data lives in gig:
- **Events table:** every mutation is an audit event with timestamp, actor, old/new values
- **Custom attributes:** persona, outcome, skills loaded, memory loaded, rejection count
- **Checkpoints:** structured progress snapshots with timestamps

`jeff stats` is a query command over this data.

### `jeff stats` Command

```bash
jeff stats                            # summary dashboard
jeff stats --persona jock             # filter by persona
jeff stats --repo backend             # filter by repo
jeff stats --since 7d                 # time window (7d, 30d, 90d)
jeff stats --outcome rejected         # filter by outcome
jeff stats --json                     # machine-readable output
```

**Summary output:**

```
JEFF Stats (last 30 days)
─────────────────────────────────────
Tasks completed    12   (8 shipped, 3 rejected→reshipped, 1 abandoned)
Avg time to ship   4.2h
Checkpoints/task   3.1

By persona:
  jock    9 tasks   3.8h avg   89% first-ship rate
  scout   2 tasks   1.2h avg   review-only
  nerd    1 task    2.0h avg   research-only

By repo:
  backend    7 tasks   2 rejections (both: missing tests)
  frontend   5 tasks   1 rejection  (style inconsistency)

Memory effectiveness:
  Tasks using persona memory    8/12 (67%)
  Tasks using repo learnings    10/12 (83%)
  Rejections WITH memory        1/10 (10%)
  Rejections WITHOUT memory     2/2  (100%)
```

### Metrics Derived From Gig Events

All computed at query time, not stored separately:

| Metric | Source |
|--------|--------|
| Tasks completed | `store.List(Status: closed)` with time filter |
| Time to ship | `EventsSince` → delta between `claimed` and `closed` events |
| First-ship rate | Tasks where `rejection_count` attr is "0" or unset |
| Checkpoints per task | `store.ListCheckpoints(taskID)` count |
| Persona distribution | `store.List()` with `AttrFilter` on `persona` attr |
| Repo distribution | `store.List()` with `AttrFilter` on `repos` attr |
| Memory effectiveness | Cross-reference `memory_loaded` attr with `outcome` attr |
| Rejection patterns | Tasks with `outcome: rejected` — group by repo + persona |

### What the Data Tells You

Without the user asking, the stats surface:

- **Which repos have the most rejections?** → Those repos need better learnings
- **Which persona has the lowest first-ship rate?** → That persona's memory needs curation
- **Are tasks with memory loaded rejected less often?** → Memory system is (or isn't) working
- **Which skills correlate with success?** → Skill injection is (or isn't) helping
- **Is checkpoint frequency dropping?** → Agents may need stronger budget guard nudges
- **Are team tasks faster than solo for the same task type?** → Informs routing rules for Level 3

---

## Phase 4: Agent Teams Integration

### `--team` Flag on Pickup

```bash
jeff pickup gig-42 --team --repos backend frontend
jeff pickup gig-42 --team --size 3    # explicit teammate count
```

**What happens:**

1. Normal pickup flow (workspace, worktree, skills, memory)
2. JEFF creates subtasks in gig if the task doesn't already have children
   - User provides decomposition, or JEFF generates one for approval
   - Each subtask gets its own gig ID, inherits parent's repos and labels
3. Task CLAUDE.md includes team setup instructions:

```markdown
## Team Configuration

This task uses Claude Code Agent Teams. You are the team lead.

### Setup
- Enable delegate mode (you coordinate, do not implement)
- Spawn teammates for each subtask listed below
- Each teammate works in the existing worktree (same branch, different files)

### Subtasks
- gig-42.1: "Implement auth middleware" → assign to implementer
- gig-42.2: "Write integration tests" → assign to implementer
- gig-42.3: "Review security implications" → assign to reviewer

### Quality Gates
- Teammates must checkpoint progress via `jeff checkpoint`
- All tests must pass before marking subtasks complete
- Ship only after all subtasks are closed
```

4. Agent Teams' shared task list maps 1:1 with gig subtasks
5. Hooks wire Agent Teams events back to gig:
   - `TeammateIdle` → writes checkpoint to the teammate's gig subtask
   - `TaskCompleted` → runs test suite, blocks completion if tests fail

### Persona Mapping in Teams

The team lead always runs in delegate mode (captain behavior). Teammates get persona instructions via their spawn prompt:

```
Spawn prompt for teammate "auth-impl":
  You are working on gig-42.1. Your role is implementer.
  [jock PERSONA.md contents]
  [jock MEMORY.md contents]
  [backend INDEX.md contents]
```

JEFF doesn't control which persona each teammate gets — it provides the context in the spawn prompt. The team lead (or user) decides the team structure.

### Post-Team Learning

When the parent task closes, `jeff learn gig-42` reads:
- All subtask checkpoints (across all teammates)
- All subtask events (who did what, in what order)
- PR feedback on the shipped branch
- Any scratchpad files from the task workspace

This produces learning candidates scoped appropriately — implementer learnings go to jock memory, review learnings go to scout memory, repo-specific learnings go to repo learnings.

---

## Phase 5: Daemon + Auto-Routing

### `jeff daemon`

A long-running Go process that reacts to gig events.

```bash
jeff daemon start                     # start in foreground
jeff daemon start --detach            # start in background
jeff daemon stop                      # stop gracefully
jeff daemon pause                     # stop auto-pickup, keep monitoring
jeff daemon resume                    # resume auto-pickup
jeff daemon status                    # show daemon state + active sessions
```

### Event Loop

```go
// Pseudocode — the daemon's core loop
store.On(gig.EventStatusChanged, func(e gig.Event) {
    if e.NewValue == "open" {
        // Task became ready — check routing rules
        task, _ := store.GetFull(e.TaskID)
        route := matchRoute(config.Routes, task)
        if route != nil {
            pickup(task, route) // spawn agent session
        }
    }
    if e.NewValue == "closed" {
        // Task completed — auto-learn if configured
        if config.AutoLearn {
            learn(e.TaskID)
        }
    }
})
```

### Routing Rules

New `jeff.json` section:

```json
{
  "routing": {
    "rules": [
      {
        "match": {"type": "bug", "labels": ["backend"]},
        "action": {"persona": "jock", "repos": ["backend"], "team": false}
      },
      {
        "match": {"type": "feature", "priority": [0, 1]},
        "action": {"persona": "jock", "repos": ["backend", "frontend"], "team": true, "team_size": 3}
      },
      {
        "match": {"type": "epic"},
        "action": {"team": true, "team_size": 5, "plan_approval": true}
      }
    ],
    "auto_learn": true,
    "auto_ship": false,
    "require_approval_to_ship": true
  }
}
```

**Routing logic:**
1. Task enters `open` (or `ready` via `gig ready` criteria)
2. Rules evaluated top-to-bottom, first match wins
3. If no match, task is skipped (user must manually pick up)
4. Matched tasks are auto-picked-up with the configured persona/team settings

### Session Management

The daemon manages agent sessions via tmux:

```
┌─────────────────────────────────────────────────────┐
│ jeff-daemon (tmux session)                          │
├──────────────┬──────────────┬───────────────────────┤
│ gig-42 jock  │ gig-43 team  │ gig-44 scout         │
│ backend      │ lead + 3     │ backend (review)      │
│ [working]    │ [working]    │ [idle - awaiting PR]  │
├──────────────┴──────────────┴───────────────────────┤
│ dashboard: 3 active, 2 queued, 12 completed today   │
└─────────────────────────────────────────────────────┘
```

Each task gets a tmux window (or pane group for teams). User clicks into any pane to interact. `jeff dashboard` shows the status bar.

### Approval and Shipping

The daemon does NOT auto-ship by default. When a task's agent signals completion:

1. Agent runs `jeff checkpoint --done "Ready for review"`
2. Daemon marks task as "awaiting approval" in the dashboard
3. User reviews via `jeff review gig-42` (opens diff + checkpoint summary)
4. User runs `jeff approve gig-42` → triggers `jeff ship` → creates PR
5. Or `jeff reject gig-42 --reason "missing edge case"` → agent resumes with feedback

This is the trust layer. The user controls the blast radius.

---

## Phase 6: Self-Improving Personas

### How Personas Learn

The full learning cycle:

```
Task execution
    │
    ▼
Agent writes to scratchpad (raw observations)
Agent CAN update existing memory entries (corrections, refinements)
Agent CANNOT create new memory files
    │
    ▼
Task closes → jeff learn
    │
    ▼
Curator reads: scratchpad + checkpoints + events + PR feedback
    │
    ▼
Produces learning candidates with scope (persona vs repo)
    │
    ▼
Human reviews (default) or curator agent reviews (--auto)
    │
    ▼
Approved entries → written to memory dirs, indexes updated
```

### Memory Patterns

Inspired by Claude Code's auto-memory. The key insight: memory that's always loaded must be scannable (short summaries). Detailed knowledge lives in topic files read on demand.

**Pattern: Summary + Pointer**

```markdown
## Testing Gotcha
Integration tests need TEST_JWT_KEY env var. Forgot twice, wasted 30 min total.
→ details: testing-env.md
```

The agent sees the summary in MEMORY.md. If the current task involves testing, it reads `testing-env.md` for the full context. If not, the 2-line summary is enough to jog awareness.

**Pattern: Anti-Pattern Registry (mistakes.md)**

```markdown
# Mistakes

## Don't mock DB in integration tests
Mocking hides schema drift bugs. Use t.TempDir() + SQLite.
Source: gig-31 rejection feedback

## Don't use errors.As for sentinel errors
User corrected: use errors.Is() for sentinel, errors.As() for type extraction.
Source: gig-35 in-session correction
```

These are high-value — they directly prevent repeat failures. The learning loop prioritizes extracting anti-patterns from rejections and user corrections.

**Pattern: Preference Tracking**

```markdown
## Code Style
- Early returns over nested ifs
- Table-driven tests for Go
- Descriptive variable names, no abbreviations
- Comments explain "why", not "what"
```

Accumulated from user corrections and PR feedback. The jock persona loads these and follows them without the user having to repeat themselves.

### Memory Budget

Each persona's MEMORY.md has a soft cap of 200 lines. When approaching the cap:

1. `jeff learn` flags that memory is near capacity
2. Curator (human or agent) consolidates: merge related entries, promote frequently-referenced entries, archive rarely-accessed ones
3. Archived entries move to an `archive/` subdirectory — still accessible but not auto-loaded

This prevents memory bloat while preserving all knowledge.

### Cross-Persona Learning

Some learnings apply across personas. Example: "This codebase uses a monorepo with pnpm workspaces" is useful for jock, scout, and nerd.

These go to **repo learnings**, not persona memory. The scoping rule:

- **Persona memory:** knowledge about HOW to do the persona's job (debugging patterns, code style, review checklists)
- **Repo learnings:** knowledge about THE CODEBASE (setup, tooling, conventions, architecture)

If in doubt, it goes to repo learnings (broader audience).

---

## Open Questions

### Captain Persona Direction

Two paths:

1. **Remove captain as a JEFF persona.** Its role is delegate mode in Agent Teams. Captain memory still exists for decomposition patterns, loaded into the team lead's context.

2. **Keep captain as a JEFF persona.** Rewrite template to use actual gig commands for subtask creation. Captain works as a planning-only agent that produces subtasks, then user picks them up separately.

Decision deferred to Phase 4 implementation.

### Memory Format Standardization

Should memory files follow a strict schema (frontmatter with tags, last-accessed dates, source task IDs)? Or free-form markdown?

Leaning toward: **light frontmatter** with `source` and `updated` fields, free-form body. Heavy schema adds friction to curation without clear benefit at this stage.

```markdown
---
source: gig-42
updated: 2026-03-20
---
Integration tests need TEST_JWT_KEY env var set. Without it, auth tests silently pass with mock responses, hiding real bugs.
```

### Gig Event Persistence

Gig events are stored in SQLite. For `jeff stats` to query across weeks/months, the gig store must persist (not be ephemeral). This is already the case — gig's DB lives at `GIG_HOME` — but worth noting: if the user deletes/recreates the gig store, historical stats are lost.

Consider: `jeff stats export --format csv` for periodic backup of computed metrics.

### Skill Auto-Creation From Learnings

Future possibility: when a repo learning becomes complex enough (e.g., a multi-step setup procedure), auto-promote it to a skill. Skills have richer structure (SKILL.md with examples, matching criteria) vs. learnings (short notes). The boundary between "learning" and "skill" may blur.

Defer until the learning system is running and we see real data on what kinds of learnings accumulate.

### Agent Teams Availability

Agent Teams is experimental and requires `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` flag. If Anthropic changes the API or removes the feature, Phase 4 needs a fallback. The fallback is Phase 3's tmux-based approach — JEFF spawns separate Claude sessions in tmux panes and coordinates via gig (no inter-agent messaging, but workable).

---

## Summary

| Phase | What | Key Deliverables |
|-------|------|-----------------|
| 1 | Persona memory + repo learnings | Memory dirs, loading on pickup, checkpoint injection, new gig attrs |
| 2 | Learning loop | `jeff learn`, scratchpad convention, curation flow, budget guard |
| 3 | Observability | `jeff stats`, metrics from gig events, trend detection |
| 4 | Agent Teams integration | `--team` flag, subtask creation, hook wiring, post-team learning |
| 5 | Daemon + auto-routing | `jeff daemon`, routing rules, tmux session management, approval flow |
| 6 | Self-improving personas | Full learning cycle, memory budget management, cross-persona sharing |
