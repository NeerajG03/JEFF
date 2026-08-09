# RESEARCH — Intercept a worker's "ask the user" and redirect to `jeff crew ask`

## Problem

A worker agent (claude / gemini / opencode) sometimes asks the user a question in
its own tmux window — "Should I do A or B?", a permission prompt, a clarification.
**Nobody sees it:** the user isn't watching the worker pane, and the question never
reaches the orchestrator either. The worker hangs indefinitely.

## Goal of THIS task — RESEARCH ONLY, do not implement

Produce a research document (`roadmaps/RESEARCH-Block-Worker-Ask-FINDINGS.md`)
that answers: **how can JEFF add a hook that fires whenever a worker uses an
"ask the user" tool/mechanism, and instead returns an error telling the worker:
"you cannot ask questions as a worker — use `jeff crew ask <task-id> \"...\"` to
ask the orchestrator"?**

Cover all THREE agents. Do NOT change production code. A throwaway experiment
(spawning a test agent to observe tool names/events) is fine, but the deliverable
is the findings doc, not a feature.

## Questions to answer, per agent (claude, gemini, opencode)

1. **Is "asking the user" a discrete, interceptable TOOL/event, or is it plain
   text output?** Only tool/event-based asks can be hooked. Identify the exact
   tool name / event name:
   - Claude Code: is there an `AskUserQuestion` (or similar) tool? Does a
     `PreToolUse` hook with a matcher fire on it? (JEFF already uses PreToolUse-style
     hooks — see hooks/ package.)
   - OpenCode: can a plugin's `tool.execute.before` intercept an ask/question tool?
     What is it called? Is there a permission/ask event instead?
   - Gemini: what is the pre-tool event name (JEFF maps PostToolUse→AfterTool for
     gemini in delivery_gemini.go — is there a BeforeTool? what ask tool exists)?

2. **Can the hook BLOCK the tool and feed an error back to the agent?** For each
   agent, what is the exact mechanism to (a) deny/abort the tool call and (b)
   return a message the agent will read (so it learns to use `jeff crew ask`)?
   - Claude: PreToolUse hook returning a deny/`permissionDecision` with reason.
   - OpenCode: throwing / returning from `tool.execute.before`.
   - Gemini: its BeforeTool equivalent.

3. **What asks are NOT interceptable?** If an agent asks in plain assistant text
   (no tool call), a hook cannot catch it. Document which ask-paths are tool-based
   (hookable) vs free-text (not), per agent.

4. **Fallback: instruction-based prevention.** Independent of hooks, what CLAUDE.md
   / persona-prompt wording would tell workers to NEVER ask the user directly and
   ALWAYS use `jeff crew ask`? Where would it live (embed/CLAUDE.md, persona
   templates, task CLAUDE.md)? This is the safety net for the un-hookable case.

5. **How would this slot into JEFF's hook system?** Sketch (do not build): a new
   builtin task-level hook in hooks/builtin.go, delivered to all three agents via
   the existing claude/opencode/gemini delivery paths. Reference how existing
   hooks like `inbox-check` are defined and delivered (hooks/CLAUDE.md,
   hooks/builtin.go, hooks/claude.go, hooks/opencode.go, delivery_gemini.go).

## Deliverable

`roadmaps/RESEARCH-Block-Worker-Ask-FINDINGS.md` with:
- A per-agent table: ask-tool/event name · hookable? · block+message mechanism · gaps.
- A recommended approach (hook where possible + instruction fallback where not).
- A rough implementation sketch (files to touch) — for a FUTURE task, not now.
- Cite exact tool/event names and any docs/experiments used. If something can't be
  confirmed, say so rather than guessing.
