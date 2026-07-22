# FINDINGS — Intercept Worker "Ask User" and Redirect to `jeff crew ask`

> **⚠️ ORCHESTRATOR REVIEW CORRECTION (added post-research).**
> The "Core Finding" below is **partly wrong for Claude Code.** Claude Code DOES
> have a discrete **`AskUserQuestion`** tool (and `ExitPlanMode`), which prompt the
> user and ARE hookable via `PreToolUse` with `matcher: "AskUserQuestion"`. A
> `claude` worker calling `AskUserQuestion` renders an interactive prompt in its
> pane that nobody answers — the exact hang this task targets — and a `PreToolUse`
> deny hook can catch it cleanly and return "use `jeff crew ask`". So for Claude
> workers the PRIMARY ask path is a hookable TOOL, not just plain text. The
> plain-text-question path (below) remains real as a SECONDARY, un-hookable case
> needing the instruction fallback. Re-verify the equivalent for OpenCode
> (`ExitPlanMode`-like tools) and Gemini before implementing. The rest of the
> analysis (block mechanisms, delivery wiring, instruction fallback, SourceTask
> hitting the orchestrator too) is sound.

## Core Finding: "Ask user" is NOT a hookable tool/event in any agent

Across all three agents (Claude Code, OpenCode, Gemini CLI), **asking the user a question is plain assistant text output — not a tool call**. There is no `AskUserQuestion`, `ask_user`, or similar discrete tool in any agent's built-in toolset. This means **hooks cannot directly intercept the act of asking**.

*(See correction above: this is inaccurate for Claude Code's `AskUserQuestion`/`ExitPlanMode` tools.)*

What hooks CAN intercept: **tool calls that produce interactive output** (e.g., `Bash` with `read -p`, `Write` with a question written to stdout). This is a secondary path.

---

## 1. Per-Agent Analysis

### 1a. Claude Code

| Property | Detail |
|---|---|
| **Hook events available** | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PreCompact`, `Stop`, `SubagentStop`, `Notification` |
| **Does JEFF currently use PreToolUse?** | **No.** JEFF only uses `SessionStart`, `PostToolUse`, `Stop`. `PreToolUse` is completely absent from `hook.go`'s `Event` doc, `delivery.go`, and all built-in hooks. |
| **Ask mechanism** | Plain text in assistant response. **NOT a tool call.** |
| **Blocking mechanism** | Exit code 2 (stderr = reason) or `{"decision": "block", "reason": "..."}` in stdout JSON. Available on `PreToolUse`, `Stop`. |
| **Hookable ask path** | If the agent uses `Bash` to `echo` a question + `read`, a `PreToolUse` hook with `matcher: "Bash"` can block it. But the agent usually just outputs text — no tool is invoked. |
| **Key gap** | No `PreToolUse` delivery path exists in JEFF. Would need a `claudePreToolUseScript()` generator + a new delivery. |

### 1b. Gemini CLI

| Property | Detail |
|---|---|
| **Hook events available** | `SessionStart`, `SessionEnd`, `BeforeAgent`, `AfterAgent`, `BeforeModel`, `AfterModel`, `BeforeToolSelection`, **`BeforeTool`**, `AfterTool`, `PreCompress`, `Notification` |
| **Does JEFF currently use BeforeTool?** | **No.** `delivery_gemini.go` maps only: `PostToolUse → AfterTool`, `Stop → AfterAgent`, `PreCompact → PreCompress`. `BeforeTool` is unmapped. |
| **Ask mechanism** | Plain text output. **NOT a tool call.** |
| **Blocking mechanism** | Exit code 2 or `{"decision": "deny", "reason": "..."}`. Available on `BeforeTool` (blocks tool + turn continues), `BeforeAgent` (blocks entire turn). |
| **Hookable ask path** | Same as Claude — only if the agent uses a tool like `run_shell_command` to produce interactive output. |
| **Key gap** | No `BeforeTool` mapping in JEFF's `geminiEventMap`. Would need to add it + a `claudePreToolUseScript()` equivalent. |

### 1c. OpenCode (V2 Plugin API)

| Property | Detail |
|---|---|
| **Runtime hooks available** | `ctx.tool.hook("execute.before", cb)`, `ctx.tool.hook("execute.after", cb)`, `ctx.session.hook("request", cb)`, `ctx.aisdk.hook(...)` |
| **Does JEFF use execute.before?** | **No.** JEFF's generated plugin (`opencode.go`) only emits `tool.execute.after` and `session.created`/`session.idle` handlers. |
| **Ask mechanism** | Plain text output. **NOT a tool call.** |
| **Blocking mechanism** | Throwing an error in `execute.before` or setting `event.input` to invalid values. "A hook failure fails the operation it intercepts." |
| **Hookable ask path** | Only if agent uses a tool interactively. The `plan_enter`/`plan_exit` tools prompt the user — these could be blocked via `execute.before` with `matcher: "plan_enter|plan_exit"`. |
| **Key gap** | JEFF needs to add `ctx.tool.hook("execute.before", ...)` registration to the generated plugin template in `opencode.go`. |

### 1d. Community `ask_user` tool

There is a community custom tool for OpenCode ([Whiteknight07/opencode-ask-user](https://github.com/Whiteknight07/opencode-ask-user)) that registers an `ask_user` tool. **This is NOT built-in** and must be explicitly installed. If a worker has this plugin, it becomes hookable via `execute.before` with `matcher: "ask_user"`. JEFF could consider bundling or recommending this, but it's not standard.

---

## 2. Hook Delivery Comparison

### JEFF's current hook delivery

| Layer | Claude Code | Gemini CLI | OpenCode |
|---|---|---|---|
| **Script type** | `.sh` file in `hooks/<name>.sh` | Same `.sh` file (shared) | `.ts` plugin in `.opencode/plugins/jeff-hooks.ts` |
| **Settings file** | `.claude/settings.json` | `.gemini/settings.json` | N/A (JS plugin) |
| **Event mapping** | Direct | `delivery_gemini.go:geminiEventMap` | `opencode.go:openCodeEventName()` |
| **Pre-tool used?** | No | No | No |
| **Post-tool used?** | Yes (`checkpoint-nudge`, `inbox-check`, `worker-heartbeat`, `orchestrator-inbox`) | Yes (same scripts, mapped `PostToolUse→AfterTool`) | Yes (`tool.execute.after` in generated plugin) |
| **Blocking used?** | Only in `memory-propose-nudge` (Stop event) | Same | No blocking (only context injection) |

### What would need to be added for PreToolUse/BeforeTool blocking

| Component | Claude | Gemini | OpenCode |
|---|---|---|---|
| **Event constant** | `"PreToolUse"` exists in `hook.go` Event doc | Map `"PreToolUse" → "BeforeTool"` in `geminiEventMap` | `"tool.execute.before"` in `openCodeEventName()` |
| **Script generator** | New `claudePreToolUseScript(content)` — reads `tool_name`/`tool_input` from stdin, returns `{"decision": "deny", "reason": "..."}` | Same (reuses Claude's script) | New `execute.before` handler in `generateOpenCodePlugin()` |
| **Matcher** | `h.Matcher` matches tool name (e.g. `"Bash"`, `"plan_enter\|plan_exit"`) | Same regex matcher | Check `event.tool` in JS callback |
| **Output contract** | `{"decision": "block", "reason": "..."}` or exit code 2 | `{"decision": "deny", "reason": "..."}` or exit code 2 | Throw error in callback |

---

## 3. What Is and Isn't Hookable

| Ask path | Hookable? | Details |
|---|---|---|
| **Plain text question** ("Should I do A or B?") | **No** | Pure LLM output — no tool event fires. Only instruction-based prevention works. |
| **Bash tool with interactive prompt** (`echo "..?" && read answer`) | **Yes** (Partial) | `PreToolUse`/`BeforeTool`/`execute.before` with `matcher: "Bash"` can block. But detecting "is this an interactive command?" requires regex heuristics. |
| **Write tool with question in content** | **Yes** (Partial) | `PreToolUse`/`BeforeTool` with `matcher: "Write"` can block writes that look like they're asking the user. |
| **OpenCode plan_enter/plan_exit** | **Yes** | These are tools that prompt the user. Hookable via `matcher: "plan_enter\|plan_exit"` (Claude/Gemini) or `event.tool === "plan_enter"` (OpenCode). |
| **Gemini tool permission prompt** | **No** | Gemini's `Notification` event fires for `ToolPermission`, but it's explicitly "Observability Only — cannot block alerts or grant permissions." |
| **Claude Code permission popup** | **No** | Not hookable. The `Notification` event in Claude is only for system notifications. |
| **Subagent ask** | **Varies** | If a subagent asks via text = no. If it uses a tool = yes, if JEFF hooks propagate to subagents (they don't today). |

---

## 4. Instruction-Based Fallback (Un-hookable Paths)

Since plain-text questions cannot be hooked, **system prompt instructions are the only defense**. Recommended injection points:

| Location | Content | Scope |
|---|---|---|
| `embed/CLAUDE.md` (JEFF_HOME) | *"You are a worker agent. Never ask the user questions directly. Use \`jeff crew ask <task-id> "..."\` to communicate with the orchestrator."* | All JEFF sessions at home level |
| `persona/templates/` (each persona) | Same instruction in each persona's base prompt | Per-persona |
| `hooks/builtin.go` new `NoAskInstructions` hook | Injects the instruction at `SessionStart` for all agents | Task-level |
| `task-commands` hook (existing) | Append the instruction to the existing `taskCommandsContext` constant | Task-level (already exists, just add text) |

### Why instruction-only is fragile
- LLMs can ignore instructions, especially under pressure or when prompted by the orchestrator to "just ask the user"
- No enforcement mechanism — a hook failure is deterministic, an instruction violation is probabilistic
- Must be paired with monitoring (orchestrator detects worker is stuck/hanging and intervenes)

---

## 5. Recommended Approach

### Layer 1: Hook-based interception (partial coverage)

| Priority | Agent | Hook | Matcher | Behavior |
|---|---|---|---|---|
| P0 | Claude | `PreToolUse` | `"Bash"` | Block Bash calls that match interactive patterns (heuristic regex on command: `read$`, `-p`, etc.) |
| P0 | Gemini | `BeforeTool` | `"run_shell_command"` | Same heuristic blocking |
| P0 | OpenCode | `execute.before` | Check `event.tool === "bash"` | Same heuristic blocking |
| P1 | Claude | `PreToolUse` | `"Write\|Edit"` | Block writes containing question-like patterns |
| P1 | OpenCode | `execute.before` | `"plan_enter\|plan_exit"` | Block plan confirmations with a message to use `jeff crew ask` |

### Layer 2: Instruction-based prevention (comprehensive coverage)

- Add "never ask the user — use `jeff crew ask`" to:
  - `embed/CLAUDE.md` (the template)
  - Each persona template in `persona/templates/`
  - The `task-commands` hook content in `builtin.go`
  - A new dedicated home-level hook `worker-no-ask` that injects at `SessionStart`

### Layer 3: Post-hoc detection (monitoring)

- The `PostToolUse`/`AfterTool`/`tool.execute.after` hooks can inspect tool output for question-like agent text and inject a nudge via `additionalContext`
- The `orchestrator-inbox` hook already runs after every tool use — it could be extended to also check for worker idle patterns suggesting it's waiting for user input

---

## 6. Implementation Sketch (for a future task)

### New or modified files

| File | Change |
|---|---|
| `hooks/hook.go` | Add `"PreToolUse"` to Event doc string; possibly add a `BlockResponse` type |
| `hooks/builtin.go` | Add `workerNoAskHook()` — a new hook definition with `Event: "PreToolUse"`, `Matcher: "Bash"`. Generate bash script that inspects `tool_input.command` for interactive patterns and returns `{"decision": "block", "reason": "Use jeff crew ask instead"}`. |
| `hooks/builtin.go` | Add `claudePreToolUseScript()` — similar to `claudeSessionStartDynamic()` but reads `tool_name`/`tool_input` from stdin JSON |
| `hooks/builtin.go` | Add `workerNoAskOpenCodeSnippet()` — inserts a `tool.execute.before` handler in the generated plugin |
| `hooks/delivery_gemini.go` | Add `"PreToolUse" → "BeforeTool"` to `geminiEventMap` |
| `hooks/opencode.go` | Update `openCodeEventName()` to handle `"PreToolUse"` → `"tool.execute.before"`. Update `generateOpenCodePlugin()` to emit `tool.execute.before` blocks alongside existing `tool.execute.after`. |
| `hooks/claude.go` | The `addHookToSettings` function already supports any event name + matcher — no change needed for Claude delivery |
| `hooks/registry.go` | `builtinHooks()` already includes all hooks — just add the new one to the slice |
| `embed/CLAUDE.md` | Add "never ask the user" instruction |
| `persona/templates/*.md` | Add "never ask the user" instruction to each persona |

### Example: `worker-no-ask` hook definition

```go
func workerNoAskHook() *Hook {
    return &Hook{
        Name:    "worker-no-ask",
        Source:  SourceTask,
        Event:   "PreToolUse",
        Matcher: "Bash",
        Timeout: 3,
        Scripts: map[string]func(ctx HookContext) string{
            "claude":  buildWorkerNoAskClaudeScript,
            "opencode": buildWorkerNoAskOpenCodeSnippet,
            "gemini":  buildWorkerNoAskClaudeScript, // same bash script, mapped to BeforeTool
        },
    }
}

// Block Bash calls that look like they're asking the user.
func buildWorkerNoAskClaudeScript(ctx HookContext) string {
    return `#!/bin/bash
set -euo pipefail
INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // ""')
# Heuristic: interactive patterns
if echo "$COMMAND" | grep -qE '(read\b|select\b|dialog|--interactive|-p\s+["\x27])'; then
    jq -n '{
        "decision": "block",
        "reason": "BLOCKED: Workers cannot ask the user directly. Use jeff crew ask <task-id> \"your question\" to communicate through the orchestrator."
    }'
    exit 0
fi
echo '{}'
`
}
```

---

## 7. Summary Table

| Question | Claude Code | OpenCode | Gemini CLI |
|---|---|---|---|
| Ask-the-user: tool or text? | **Text** (no `AskUserQuestion` tool) | **Text** (no built-in `ask_user` tool) | **Text** (no ask tool) |
| Pre-tool event exists? | `PreToolUse` ✓ | `tool.execute.before` ✓ | `BeforeTool` ✓ |
| JEFF uses it? | No | No | No |
| Can hook block tool? | Yes: exit code 2 or `{"decision":"block","reason":"..."}` | Yes: throw in callback or return block | Yes: exit code 2 or `{"decision":"deny","reason":"..."}` |
| Can hook block non-tool ask? | No — only text-based | No — only text-based | No — only text-based |
| `plan_enter` hookable? | Yes (it's a tool) | Yes (it's a tool) | Yes (it's a tool) |
| Permission prompts hookable? | No | No | Notification event is advisory-only |
| Fallback needed? | Yes — instructions in CLAUDE.md/persona | Yes — instructions in CLAUDE.md/persona | Yes — instructions in GEMINI.md/persona |

---

## 8. Key Blockers & Unknowns

1. **Heuristic reliability**: Detecting "asking the user" from a Bash command string is heuristic — false positives (blocking legitimate `read`) and false negatives (missing clever asks) are inevitable. The implementation should bias toward false positives (block + explain) since the worker can retry using `jeff crew ask`.

2. **No Claude Code `PreToolUse` in JEFF yet**: The entire `PreToolUse` delivery path needs to be built from scratch. `delivery_claude.go`'s `installClaudeScript` already supports arbitrary events/matchers, so only the script generator (`buildWorkerNoAskClaudeScript`) and event mapping are needed.

3. **OpenCode V2 API is beta**: The `@opencode-ai/plugin/v2` package is marked beta. The `execute.before` hook shape may change. JEFF's generated plugin would need version detection or maintenance tracking for API changes.

4. **Subagent asks invisible**: If a subagent asks the user, JEFF hooks in the parent session don't propagate. The subagent has its own hook environment. The `PreToolUse` hook would need to be installed in the subagent's `.claude/settings.json` as well.

5. **`worker-no-ask` applies to orchestrator too**: If the hook is task-level (SourceTask), it fires for the orchestrator agent as well. Need `SourceWorker` or a `Target` field to restrict to worker sessions only, or add an orchestrator-ID check in the script.
