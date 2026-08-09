# PLAN — Debounce the OpenCode worker-stop ping

## Bug

The worker-stop ping fires on **every turn**, not once when the worker actually
falls quiet. It is wired to OpenCode's `session.idle` event, which OpenCode emits
at the end of *every* turn. So a worker that takes N turns pings the orchestrator
N times with `[Worker <id> stopped]: Agent has stopped working…`.

This is the overcorrection of the earlier fix: worker-stop was moved from
`process.exit` (never fired while the process was alive → zero pings) to
`session.idle` (fires every turn → too many pings).

## Fix — debounce to one ping per active→idle transition

The generated OpenCode plugin (`.opencode/plugins/jeff-hooks.ts`) is a single
long-lived closure per session, so a module-level flag persists across events.
Ping once when the worker goes idle; re-arm only after it does real work again
(a tool execution).

Target generated shape:

```js
export const JeffHooksPlugin = async ({ client }) => {
  // ...existing helpers...
  let stopSignalled = false;   // NEW: debounce guard

  return {
    event: async ({ event }) => {
      // ...session.created unchanged...

      if (event.type === "session.idle") {
        if (!stopSignalled) {
          // [worker-stop]
          runFile("tmux", ["send-keys", "-t", "<orch>:orchestrator", "-l", "[Worker <id> stopped]: ..."]);
          // [worker-stop-enter]
          runFile("tmux", ["send-keys", "-t", "<orch>:orchestrator", "Enter"]);
          stopSignalled = true;
        }
        // ...existing inject(parts) unchanged...
      }
    },

    "tool.execute.after": async (input, output) => {
      stopSignalled = false;   // NEW: worker is active again → re-arm the ping
      // ...existing heartbeat / inbox-check / checkpoint-nudge / inbox unchanged...
    },
  };
};
```

Result: exactly one `[Worker stopped]` ping each time the worker finishes a batch
of work and goes quiet; no repeat pings while it stays idle; re-arms when it
resumes tool use.

## Where the code lives

- `hooks/builtin.go` — `buildOpenCodeWorkerStopSnippet(taskID, orchestratorID)`
  (currently emits the two `runFile` tmux calls). The `if (!stopSignalled)` guard
  and `stopSignalled = true` belong around these.
- `hooks/opencode.go` — the plugin generator that:
  1. declares the closure and its helpers (add `let stopSignalled = false;`),
  2. assembles the `session.idle` block (wrap the worker-stop snippet in the guard),
  3. assembles the `tool.execute.after` block (prepend `stopSignalled = false;`).

Decide the cleanest split: the guard variable declaration and the
`tool.execute.after` re-arm line are generator-level (opencode.go), while the
worker-stop snippet body stays in builtin.go. Keep the `[worker-stop]` /
`[worker-stop-enter]` comment markers so existing tests that grep for them pass.

## Constraints

- Only affects OpenCode. Claude/gemini worker-stop delivery is unchanged.
- `memory-session-end` stays on `process.exit` (true end-of-process) — do NOT
  move it and do NOT debounce it.
- If a worker never runs a tool before going idle, one ping still fires (correct).

## Verification

1. `go build ./... && go test ./... && go vet ./...`
2. Add/extend a hooks test (hooks/opencode_test.go or builtin_test.go): assert the
   generated plugin contains `let stopSignalled = false`, that the `session.idle`
   block guards the worker-stop send with `if (!stopSignalled)`, and that
   `tool.execute.after` resets `stopSignalled = false`.
3. Manual: run an opencode worker, let it take several turns; confirm the
   orchestrator pane receives the stop ping ONCE per quiet period, not per turn.
