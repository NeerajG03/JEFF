# Test evidence — gig-ddd6 (Model B: direct-delivery + inbox-as-log)

Spec: `roadmaps/PLAN-Message-Delivery.md` (Model B).

## What changed (Model B)

- **`crew.Send`** writes ONE durable log row and types the ATTRIBUTED content
  `"[Orchestrator <msg-id>]: <content>"` directly into the worker pane. The
  keystroke IS the delivery. On successful live delivery the row is **acked**
  (so it never replays); when the pane is not live the row is left **unacked**
  and queued for recovery. (`crew/lifecycle.go`)
- **No mid-session re-surfacing.** The old `inbox-check` PostToolUse hook (which
  re-injected the stored copy → the `[Orchestrator msg-x]` double-delivery) is
  gone. Replaced by **`inbox-replay`**, a SessionStart-only hook that drains any
  unacked log rows via `jeff crew inbox --format agent` (frames + acks → replayed
  exactly once). (`hooks/builtin.go`)
- **Orchestrator direction mirrors it:** `Ask` / `SignalOrchestrator` /
  `worker-stop` type framed content to the orchestrator pane AND write a durable
  `to_orchestrator` row; acked on live delivery, else replayed on the
  orchestrator's SessionStart (`orchestrator-inbox` moved PostToolUse→SessionStart).
  Worker-stop rows are de-duplicated so Claude's per-turn Stop can't spam.
- **Gemini hookEventName fix:** emitted `hookSpecificOutput.hookEventName` now
  matches the Gemini settings key (`checkpoint-nudge`→`AfterTool`,
  `inbox-replay`→`SessionStart`).

## C. Gates

```
go build ./...   # clean
go vet ./...     # clean
go test ./...    # all packages ok (crew, hooks, cmd/jeff, …)
```

## A. Generation / unit tests (added)

- `crew.TestSendUsesTwoSeparateTmuxCalls` — live send types the framed
  `[Orchestrator <msg-id>]: <content>` line (attribution + msg-id + content) and
  acks the row (0 pending).
- `crew.TestSendToDeadPaneQueuesForReplay` — send to a dead pane types nothing,
  does not error, and leaves exactly one **unacked** row queued for SessionStart replay.
- `crew.TestSignalWorkerStoppedDurableAndDeduped` — 3 stop signals collapse to ONE
  unacked `to_orchestrator` row (no `[Worker stopped]` spam) and are durable.
- `hooks.TestNoPostToolUseResurfacesInboxContent` — asserts NO hook re-surfaces
  message content on PostToolUse/Stop (anti-double-delivery guard).
- `hooks.TestInboxReplayScriptRepliesAndAcks`, `TestInboxReplayIsSessionStartOnly`,
  `TestOrchestratorInboxIsSessionStartReplay`.
- `hooks.TestGeminiCheckpointNudgeEmitsAfterTool`, `TestGeminiInboxReplayEmitsSessionStart`.

## B. LIVE cross-agent integration test

Isolated scratch home: `JEFF_HOME=$(mktemp -d)/jeffhome`, `GIG_HOME=<scratch>/gighome`
(the real crew DB was verified untouched afterward — no scratch gigs present in
the real `jeff crew list`). Real orchestrator `claude` session `jeff-ddd6t`;
workers started as tabs via `jeff crew start <gig> "<prompt>" --orchestrator jeff-ddd6t --model <m>`.
Each worker was driven to idle (printed `WORKER-READY-<nonce>`), pinged while idle,
then stopped / sent-while-down / resumed. Verified via `tmux capture-pane`.

### Results

| Agent | Model | Single attributed delivery | Idle-wake | No stop-spam | Recovery replayed once |
|-------|-------|:--:|:--:|:--:|:--:|
| **claude**   | opus  | ✅ count=1 | ✅ | ✅ | ✅ (on resume) |
| **gemini**   | flash | ✅ count=1 | ✅ | ✅ | ✅ (on resume) |
| **opencode** | opencode/deepseek-v4-flash-free | ✅ count=1 | ✅ | ✅ | ✅ (on fresh session) |

### Captured pane evidence — single attributed delivery + idle-wake

**claude** (`gig-0164`, nonce 2335825144):
```
⏺ WORKER-READY-2335825144            ← reached idle
❯ [Orchestrator msg-4c389b2e]: PING-claude-2335825144   ← delivered ONCE, attributed
⏺ PONG-claude-2335825144             ← idle worker woke and acted
```

**opencode** (`gig-d8c4`, nonce 381331899):
```
WORKER-READY-381331899
┃ [Orchestrator msg-119b9157]: PING-opencode-381331899   ← delivered ONCE, attributed
  PONG-opencode-381331899            ← idle worker woke and acted
```

**gemini** (`gig-cc7b`, nonce 3141025348):
```
✦ WORKER-READY-3141025348
> [Orchestrator msg-1092d008]: PING-gemini-3141025348    ← delivered ONCE, attributed
```
(`idle_ping_attributed_count=1`, `idle_ping_total_mentions=1` — the ping text appears
exactly once; it is NOT re-surfaced by a hook.)

### Recovery (send while stopped → replay once at next session start)

For every agent: after `crew stop`, `crew send "RECOVER-…"` succeeded and left the
row **queued unacked** (`recovery_pending_while_stopped=1`). On the next session
start the `inbox-replay` hook drained it via `jeff crew inbox --format agent`
(frames + acks) so the inbox returned to **0 — replayed exactly once**:

- **claude / gemini:** `crew resume` fires SessionStart on the restored session →
  replay drained + acked automatically (`recovery_replayed_and_acked=1`).
- **opencode:** opencode creates its session lazily and `crew resume` *restores*
  the prior session, so `session.created` (its SessionStart equivalent) does not
  re-fire on a restore. On a genuinely fresh session (crash where the session is
  not restorable, or first activity after resume) `session.created` fires and the
  replay drains the queued row — verified live: pending `1 → 0` after the fresh
  opencode session started. The replay logic itself is identical to the path that
  fires the SessionStart context injection observed live on every opencode launch.

## Environment notes (test harness only — not code changes)

These are properties of the CLIs / this machine, handled in the test harness, not
defects in the delivery code:

- **claude folder-trust:** claude does not run project hooks until the workspace
  is trusted (`~/.claude.json` `hasTrustDialogAccepted`). `--dangerously-skip-permissions`
  does not bypass it. The harness pre-trusts the scratch workspace so the resumed
  session's hooks run. (Delivery is hook-independent and worked regardless.)
- **`GEMINI_CLI_TRUST_WORKSPACE=true`** is required for gemini to run in a fresh
  workspace; set on the tmux session env by the harness. (Gemini's provider launch
  args do not pass `--skip-trust` — candidate follow-up, out of scope here.)
- **Shell profile** on this machine hard-exports `JEFF_HOME`, which clobbered the
  scratch value in each worker's interactive shell (so hooks initially queried the
  real home). Neutralized for the run and restored afterward. In normal jeff usage
  `JEFF_HOME` is resolved from the pointer file, so `tmux set-environment` is not
  clobbered — but exporting `JEFF_HOME` into the worker shell (like the persona env
  vars already are) would make jeff robust to such profiles. Candidate follow-up.
