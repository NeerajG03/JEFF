# PLAN — Unified message delivery: inbox is the one channel, tmux is the wake

## DECISION: Model B (direct-delivery + inbox-as-log)

Chosen over the original "inbox = single content channel" model. Neeraj's call,
for simplicity + cross-agent scalability.

**The pane keystroke IS the delivery. The inbox is a durable recovery log.**

```
jeff crew send gig-x "deploy the fix"
    ├─► store a LOG row (msg-a1b2) in messages     ← durable; replayed ONLY on SessionStart
    └─► tmux send-keys into the worker's pane:
          "[Orchestrator msg-a1b2]: deploy the fix" + Enter   ← the actual delivery
```

Why B (not the content-free-wake model A): delivery is agent-agnostic — typing
framed text into a TUI works on claude, gemini, opencode, and future agents with
ZERO per-agent turn-end hook wiring. Model A needed a reliable turn-end hook per
agent to drain content after a wake, and gemini has none — so A doesn't scale.

## Goal

Eliminate the `[Orchestrator msg-x]` double-delivery while making delivery work
in every worker state across all three agents (claude, gemini, opencode).

Root cause of the double today (`crew/lifecycle.go` `Send`): every send does BOTH
`store.SendMessage` AND types content into the pane, and then the `inbox-check`
hook RE-SURFACES the stored copy on the next tool use → message appears twice.
Fix: keep direct typing as the delivery, MOVE attribution framing into `send`,
and STOP the hook from re-surfacing content mid-session.

## The constraint that killed model A

The `inbox-check` hook fires ONLY on tool-completion (`PostToolUse` /
`AfterTool` / `tool.execute.after`). An idle agent runs no tools, so a hook-drained
inbox can never reach an already-idle worker without a keystroke anyway. Since a
keystroke is unavoidable, B makes the keystroke carry the content directly.

## Implementation

### 1. `Send` frames + types the content directly — crew/lifecycle.go
- Keep `store.SendMessage(msg)` — but its role is now a **durable log row**, not a
  live delivery queue.
- Delivery = type the ATTRIBUTED content into the pane. Move the
  `[Orchestrator <msg-id>]: <content>` framing (today added by the inbox-check
  hook at `crew_cmd.go:842`) INTO the `send` path, so the keystroke carries the
  attribution and the msg-id (so the worker can still `jeff crew ack <msg-id>`).
- Keep the `interrupt` path (Ctrl-C, settle, then type).
- This works whether the worker is idle (wakes + delivers) or busy (queues,
  processed at turn-end) — on all three agents, no hook-timing dependency.

### 2. Stop hooks from re-surfacing content mid-session; replay log on SessionStart
- `inbox-check` (PostToolUse) must NO LONGER re-inject message content — that is
  the double-delivery. Remove/neutralize the mid-session content re-surfacing.
- Instead, replay the durable log at **SessionStart only**: on launch/resume, a
  worker drains any log rows that were unacked (messages typed while its pane was
  dead/restarting), attributes them, and acks them. This is the crash/restart
  recovery path — the sole remaining hook-driven surfacing.
- Net: exactly one delivery in the normal case (the direct type); a second
  surfacing happens ONLY when a message was sent while the pane wasn't live, and
  only once, at next SessionStart.

### 3. Orchestrator direction mirrors this — crew/lifecycle.go
- `SignalOrchestrator` / `worker-stop`: type the framed content into the
  orchestrator pane directly (already fire-and-forget) AND write a durable
  `to_orchestrator` log row. The `orchestrator-inbox` hook stops re-surfacing
  mid-session and instead the log is replayed on the orchestrator's SessionStart —
  so a stop-ping sent while the orchestrator pane was dead is recovered on relaunch.

### 4. Fix the gemini hookEventName mismatch — hooks/builtin.go + delivery_gemini.go
`buildInboxCheckScript` hardcodes `hookEventName:"PostToolUse"` in the jq payload
(`builtin.go`), but gemini's settings key is `AfterTool` (`delivery_gemini.go`).
Make the emitted event name match the delivery target for gemini.

### 5. Debounce carries over
The `stopSignalled` debounce from #54 stays. B removes the mid-session content
re-surfacing entirely, so there is no turn-end drain to re-introduce spam.

## TESTING — this is the acceptance bar

### A. Generation/unit tests (no live agents)
- Assert `Send` writes exactly one log row AND types the framed
  `[Orchestrator <msg-id>]: <content>` string into the pane (content + attribution
  + msg-id present in the typed command).
- Assert the SessionStart hook replays unacked log rows and acks them; assert
  PostToolUse no longer re-surfaces message content (no double).
- Assert gemini's emitted hook event name matches `AfterTool`.

### B. LIVE cross-agent integration test (REQUIRED — the point of this task)
Do this in an **isolated scratch JEFF_HOME** (e.g. `JEFF_HOME=$(mktemp -d)`),
never the real one, so the real crew DB is untouched. All three CLIs are
installed (`claude`, `gemini`, `opencode`).

For EACH agent ∈ {claude, gemini, opencode}:
1. `JEFF_HOME=$SCRATCH jeff init`; register a tiny throwaway git repo.
2. Create a gig; `jeff crew start <gig> "<prompt>" --agent <agent> [--model opus for claude]`
   — this creates the tmux window and installs the hooks.
3. Let it reach idle. Then `jeff crew send <gig> "PING-<agent>-<nonce>"`.
4. **Verify via `tmux capture-pane`:**
   - `[Orchestrator msg-…]: PING-<agent>-<nonce>` appears **exactly once** — the
     direct type, NOT surfaced again by a hook;
   - sending while the worker was **idle** actually woke it (it acted on the ping);
   - no repeated `[Worker stopped]` spam.
5. **Recovery test:** stop the worker, `jeff crew send` a message while it is down,
   then `jeff crew resume` it and verify the message is replayed exactly once at
   SessionStart (the inbox-as-log recovery path).
6. Record pass/fail + captured evidence per agent. Tear down the scratch home and
   tmux windows.

If a given CLI cannot authenticate in this environment, document it clearly as
SKIPPED-with-reason — do NOT claim it passed. At minimum claude and opencode must
be exercised live.

### C. Gates
`go build ./... && go test ./... && go vet ./...` all green.

## Deliverable
A PR with the implementation, the generation tests, and a short
`test-evidence.md` (or PR description section) showing the captured pane output
proving single attributed delivery + idle-wake for each agent actually tested.
