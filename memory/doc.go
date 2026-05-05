// doc.go — content for `jeff memory doc`.
// Synthesized from exports/memory-research/specs/EPIC.md.
package memory

// Doc is the comprehensive memory-system explainer printed by `jeff memory doc`.
const Doc = `# JEFF memory system

## What it is

JEFF's memory subsystem captures durable learnings from agent work sessions and
consolidates them into a curated, versioned knowledge base. Personas (jenko,
schmidt, eric, hardy, doug, dickson) accumulate memory over time — this is what
makes a persona useful across tasks, not just within a single session.

Memory is not a sidecar feature. It is the substrate that lets JEFF personas
improve over time. Without memory, every session starts cold. With memory, a
persona carries forward what it learned in previous tasks.

## Core security principle

Only one agent (marlowe) may write to canonical memory. This is a hard invariant:

  * Workers (jenko, schmidt, etc.) propose memories via 'jeff memory propose'.
    Proposals land in JEFF_HOME/proposals/<persona>/<task>/ — not in canonical.
  * On session end, a queue entry drops into JEFF_HOME/queue/sessions/.
  * marlowe runs 'jeff memory curate' to review proposals and the queue, then
    writes consolidated, enriched entries to JEFF_HOME/memory/.

Single-writer design prevents memory-injection attacks (MINJA). Research shows a
98.2% injection success rate against self-writable memory stores. By restricting
writes to a dedicated curator session, JEFF makes injection attacks structurally
impossible during normal worker sessions.

## How it works — the full loop

1. A worker session starts. CLAUDE.md addendum tells the agent:
   "You have a memory system. Call 'jeff memory propose' when you observe a
   learning worth keeping."

2. The agent calls 'jeff memory propose --name <slug> --type <type>
   --description "<one-liner>" --body "<detail>"' when it:
   - Is corrected by the user
   - Discovers a non-obvious fact about a codebase
   - Observes a preference or convention worth remembering

3. The proposal is written to JEFF_HOME/proposals/<persona>/<task>/<slug>.md.
   This is outside the canonical store — workers cannot touch JEFF_HOME/memory/.

4. On SessionEnd (shell hook), a queue entry drops to
   JEFF_HOME/queue/sessions/<task>-<ts>.json. The hook also copies the transcript
   to JEFF_HOME/transcripts/<task>/.

5. When the user runs 'jeff memory curate', marlowe:
   a. Reads queue entries and associated proposals.
   b. Decides scope (which persona/repo/project best owns the entry).
   c. Deduplicates: if a similar entry already exists, updates rather than adds.
   d. Soft-invalidates conflicts: sets valid_to on the old entry, links it via
      superseded_by to the new one.
   e. Writes enriched canonical entries to JEFF_HOME/memory/.
   f. Archives processed proposals and queue entries.

6. On the next task pickup, the worker's CLAUDE.md is regenerated with the
   current memory pack injected — only entries relevant to that persona + repos
   are included, keeping prompt cost low.

## Memory tree layout

  JEFF_HOME/
  ├── memory/                       canonical — only marlowe writes
  │   ├── personas/<name>/
  │   │   ├── GOAL.md               persona's durable goal statement
  │   │   ├── core.md               always-in-prompt block (very short)
  │   │   ├── procedural/           rules: how to do things
  │   │   ├── semantic/             facts: what is true
  │   │   └── episodic/             per-task event log (Phase 2)
  │   ├── repos/<repo>/
  │   │   ├── core.md
  │   │   ├── procedural/
  │   │   ├── semantic/
  │   │   └── episodic/
  │   ├── projects/<key>/
  │   │   ├── core.md
  │   │   ├── procedural/
  │   │   ├── semantic/
  │   │   └── episodic/
  │   └── orchestrator/             marlowe's own memory
  │
  ├── proposals/<persona>/<task>/   workers write here via 'jeff memory propose'
  │   └── <slug>.md
  ├── queue/sessions/               SessionEnd hook drops JSON here
  │   └── <task>-<ts>.json
  ├── transcripts/<task>/           session transcript copies
  └── archive/<iso-week>/           processed proposals + queue entries

## Buckets

Each scope (persona, repo, project) is divided into four CoALA-aligned buckets:

  core        Always injected into the prompt. Very short — high-value
              distilled facts and persona identity. Keep this small.

  procedural  Rules, norms, and how-tos. "Don't mock the database in tests."
              "auth middleware lives in security/, not middleware/."
              These shape behavior across every task in this scope.

  semantic    Curated facts about the world. "The redis cluster serving
              personalization has a 200ms timeout budget."
              More volatile than procedural — facts change as systems change.

  episodic    Per-task event log (Phase 2, not yet active). Records what
              happened in a task for reflection passes.

## Schema

### Worker-facing (3 fields — mirrors Claude Code)

  ---
  name: async-error-handling
  description: Don't wrap async in try/catch — repo uses top-level boundaries
  type: feedback
  ---
  # body
  Detailed rationale, context, examples.

  Why: use user | feedback | project | reference.

### Canonical (marlowe-enriched)

  ---
  name: async-error-handling
  description: Don't wrap async in try/catch — repo uses top-level boundaries
  type: feedback
  status: accepted                    # accepted | superseded
  scope: repo:frontend                # persona:<x> | repo:<y> | project:<z>
  goal_served: jenko/ship-clean-code
  importance: 7                       # 1-10
  valid_from: 2026-04-30T00:00:00Z
  valid_to: null                      # set by soft-invalidate (never deleted)
  supersedes: [async-error-handling-v1]
  superseded_by: ""
  provenance: trusted                 # trusted | review-required
  source:
    persona: jenko
    task: gig-1234
    trigger: user-correction          # user-correction | self-noted | sessionend
  ---
  # body (same as worker-authored, marlowe may expand)

  Key invariant: entries are never deleted. Superseded entries get valid_to set.
  The full audit trail is always preserved.

## Permission model

  Persona    JEFF_MEMORY_CAN_ADD   Allowed ops
  ─────────────────────────────────────────────
  jenko      unset                 propose, list, show, status, doc
  schmidt    unset                 propose, list, show, status, doc
  eric       unset                 propose, list, show, status, doc
  hardy      unset                 propose, list, show, status, doc
  doug       unset                 propose, list, show, status, doc
  dickson    unset                 propose, list, show, status, doc
  marlowe    1                     all of the above + add, curate

'jeff memory add' checks JEFF_MEMORY_CAN_ADD and refuses if unset. This is a
belt-and-suspenders check — even if the addendum accidentally reaches a non-marlowe
session, the CLI enforces the boundary.

## Common operations

  # See all accepted memory for a persona
  jeff memory list --persona jenko

  # See all memory across all scopes (a lot — use filters)
  jeff memory list --status accepted

  # Show a specific entry
  jeff memory show async-error-handling
  jeff memory show memory/personas/jenko/procedural/rules/async-error-handling.md

  # Check system health (queue depth, pending proposals, counts)
  jeff memory status

  # See the version history of a memory entry
  jeff memory diff async-error-handling

  # Trigger marlowe to consolidate proposals → canonical
  jeff memory curate

  # Propose a memory from a worker session
  jeff memory propose --name my-learning --type feedback \
    --description "short one-liner" --body "details and why"

## Disabling memory

To disable memory in a single shell session:
  export JEFF_MEMORY_DISABLE=1

To persist the disable flag across sessions:
  jeff memory disable --confirm

This writes {"memory":{"disabled":true}} to jeff.json. Workers in subsequent
sessions will not receive the memory addendum and will not call 'jeff memory
propose'. Run 'jeff memory disable --confirm' again to toggle it back on.

Memory disable is advisory — it suppresses the addendum injection and skips
the SessionEnd hook. Canonical memory is not deleted; it can be re-enabled at
any time.

## Gotchas

  * Proposals are NOT memory. They sit in proposals/ until marlowe processes them.
    If you never run 'jeff memory curate', proposals accumulate but nothing is
    canonicalized.

  * core.md is a single file per scope, not a directory. All other buckets
    (procedural/semantic/episodic) are directories containing one file per entry.

  * 'jeff memory list' defaults to accepted entries only. Pass --status superseded
    to see historical (superseded) entries.

  * Scope routing is marlowe's job, not the worker's. Workers propose; marlowe
    decides where an entry lives (persona vs repo vs project scope).

  * The queue entry (SessionEnd hook) and the proposal file are separate. A session
    may end with no proposals (queue entry still drops) or with proposals and no
    queue entry (if the hook was disabled). marlowe processes both independently.
`
