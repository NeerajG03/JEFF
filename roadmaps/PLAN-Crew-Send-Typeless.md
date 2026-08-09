# PLAN — Typeless `jeff crew send`

## Goal

`jeff crew send` should have **no message types**. Today it has four
(`nudge`, `status`, `divert`, `normal`) and `nudge`/`normal` are byte-for-byte
identical internally. Collapse to a single default send behavior. Preserve the
one genuinely distinct capability (interrupt) as a **flag**, not a type.

```
# before
jeff crew send gig-x "msg" --type nudge|status|divert|normal

# after
jeff crew send gig-x "msg"              # the one way (store + deliver to pane)
jeff crew send gig-x "msg" --interrupt  # same, but Ctrl-C the agent first (was --type divert)
```

## Current behavior (crew/lifecycle.go:460-523 `Send`)

| type | behavior |
|------|----------|
| `nudge` | `store.SendMessage` + `sendCommandForSession` (type into pane) |
| `normal` | **identical to nudge** |
| `status` | store + type `"/btw " + content` (sidechain); CLI then captures pane response |
| `divert` | store + `SendInterrupt` (Ctrl-C) + settle delay + type |

Every type stores to the inbox AND types into the pane. `nudge`==`normal`.
`status` just prefixes `/btw`. `divert` interrupts first.

## Scope

### 1. Collapse `Send` — crew/lifecycle.go

Replace the 4-case switch with one behavior. New signature:

```go
// Send stores the message in the inbox and delivers it to the worker's pane.
// If interrupt is true, the agent is interrupted (Ctrl-C) before delivery.
func Send(store *Store, taskID, content string, interrupt bool) (*Message, error)
```

- Always: `store.SendMessage(msg)` then, if `interrupt`, `SendInterrupt(target)` +
  `time.Sleep(interruptSettleDelay(sess.Agent))`, then
  `sendCommandForSession(target, content, sess.Agent)`.
- Drop the `/btw` sidechain (`status`) — it is redundant with `jeff crew ask`,
  which is the dedicated question/response path. Removing it removes the
  pane-response-capture branch from the CLI too.
- The stored `Message.Type` field: set to a single constant. Keep the
  `msg_type` DB column (no migration) — just always write one value.

### 2. MessageType constants — crew/crew.go

- Keep the `MessageType` type alias and ONE constant (rename `MsgNormal` →
  `MsgMessage = "message"`, or keep `MsgNormal` as the single value — pick one
  and use it everywhere). Delete `MsgNudge`, `MsgStatus`, `MsgDivert`.
- The scan at crew.go:618-629 reads whatever string is in old rows — no constant
  needed for back-compat, so existing rows with "nudge"/"divert" still load fine.
- `Ask` (lifecycle.go:633) currently sets `Type: MsgNormal` — update to the
  single constant.

### 3. CLI — cmd/jeff/crew_cmd.go (crewSendCmd, ~583-635)

- Remove `--type` flag; add `--interrupt` bool flag (default false).
- Call `crew.Send(cs, taskID, content, interrupt)`.
- Replace the 4-case output switch with one line
  (`"Sent message to <id>"`, or `"Interrupted and sent to <id>"` when
  `--interrupt`). Delete the `status` 10s pane-capture block.
- Update the command `Long` help to describe the single behavior + `--interrupt`.

### 4. TUI — tui/input.go, tui/tui.go

- tui/input.go:9 — delete `msgTypes` slice and the type-cycling
  (`typeIdx`, `msgType()`, the `[type]` label at input.go:77).
- tui/tui.go:292-295 — call `crew.Send(store, taskID, content, false)`
  (TUI has no interrupt affordance; keep it simple).

### 5. Docs / hook text

- hooks/builtin.go:225 — update the embedded help line
  `jeff crew send <id> "msg" [--type ...]` → `jeff crew send <id> "msg" [--interrupt]`.
- Any other doc mentioning `--type` for crew send.

### 6. Tests

- Update any test referencing `MsgNudge`/`MsgStatus`/`MsgDivert` or the old
  `Send` signature (search `crew.Send(`, `Msg` in `*_test.go`).
- Add a test: `Send(..., interrupt=false)` stores one message and delivers;
  `Send(..., interrupt=true)` calls the interrupt path. Mirror existing
  lifecycle_test.go patterns (in-memory store, stub tmux where needed).

## Verification

`go build ./... && go test ./... && go vet ./...` — all green.
Grep to prove the flag is gone: `grep -rn '"type"' cmd/jeff/crew_cmd.go` returns
nothing for send; `grep -rn 'MsgNudge\|MsgStatus\|MsgDivert' .` returns nothing.

## Out of scope (Phase 2, do NOT do here)

The larger delivery unification — making the inbox the single content channel
with a content-free wake keystroke, wiring `inbox-check` into
SessionStart/turn-end, fixing the gemini `hookEventName` mismatch, and making
worker→orchestrator signals durable — is tracked separately. This task ONLY
collapses the `crew send` type system. Keep the existing store+type delivery
mechanism unchanged.

## Design decision to confirm

Interrupt is preserved as `--interrupt` (a modifier) rather than deleted, because
"stop the agent now" is a real capability. If the user prefers zero flags, drop
`--interrupt` too and interrupt becomes unavailable via `crew send`.
