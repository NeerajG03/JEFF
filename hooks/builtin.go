package hooks

import "strconv"

// builtinHooks returns all built-in hook definitions.
func builtinHooks() []*Hook {
	return []*Hook{
		// Home-level hooks.
		gigInstructionsHook(),
		gigReadyTasksHook(),
		jeffReposHook(),
		jeffInstructionsHook(),
		crewContextHook(),
		orchestratorInboxHook(),
		// Task-level hooks.
		taskContextHook(),
		taskCommandsHook(),
		checkpointNudgeHook(),
		inboxReplayHook(),
		workerHeartbeatHook(),
		workerStopHook(),
		sessionCaptureHook(),
		// Memory hooks.
		sessionStartMemoryHook(),
		sessionEndMemoryHook(),
	}
}

// gigInstructionsHook injects gig CLI reference into the agent session.
func gigInstructionsHook() *Hook {
	return &Hook{
		Name:    "gig-instructions",
		Source:  SourceHome,
		Event:   "SessionStart",
		Matcher: "*",
		Scripts: bashBoth(
			func(ctx HookContext) string {
				return claudeSessionStartStatic(gigInstructionsContext)
			},
			func(ctx HookContext) string {
				return jsStaticSnippet("gig-instructions", gigInstructionsContext)
			},
		),
	}
}

// gigReadyTasksHook injects the output of `gig ready` into the agent session.
func gigReadyTasksHook() *Hook {
	return &Hook{
		Name:    "gig-ready-tasks",
		Source:  SourceHome,
		Event:   "SessionStart",
		Matcher: "*",
		Scripts: bashBoth(
			func(ctx HookContext) string {
				return claudeSessionStartDynamic(`## Tasks ready for pickup
` + "$(gig ready 2>/dev/null || echo '(no tasks)')")
			},
			func(ctx HookContext) string {
				return jsDynamicSnippet("gig-ready-tasks", `gig ready 2>/dev/null`)
			},
		),
	}
}

// jeffReposHook injects the list of registered repos into the agent session.
func jeffReposHook() *Hook {
	return &Hook{
		Name:    "jeff-repos",
		Source:  SourceHome,
		Event:   "SessionStart",
		Matcher: "*",
		Scripts: bashBoth(
			func(ctx HookContext) string {
				return claudeSessionStartDynamic(`## Registered repos
` + "$(jeff repo list 2>/dev/null || echo '(none)')")
			},
			func(ctx HookContext) string {
				return jsDynamicSnippet("jeff-repos", `jeff repo list 2>/dev/null`)
			},
		),
	}
}

// jeffInstructionsHook injects jeff CLI reference into the agent session.
func jeffInstructionsHook() *Hook {
	return &Hook{
		Name:    "jeff-instructions",
		Source:  SourceHome,
		Event:   "SessionStart",
		Matcher: "*",
		Scripts: bashBoth(
			func(ctx HookContext) string {
				return claudeSessionStartStatic(jeffInstructionsContext)
			},
			func(ctx HookContext) string {
				return jsStaticSnippet("jeff-instructions", jeffInstructionsContext)
			},
		),
	}
}

// claudeSessionStartStatic wraps static content (no shell expansion) in a
// Claude Code SessionStart hook script. Uses a heredoc so backticks, single
// quotes, and double quotes are all passed through literally.
// bashBoth returns a Scripts map registering the same bash generator for
// the claude and gemini deliveries (their script bodies are identical today;
// the gemini delivery remaps event names and timeout units at install time).
func bashBoth(fn func(HookContext) string, opencode ...func(HookContext) string) map[string]func(HookContext) string {
	m := map[string]func(HookContext) string{"claude": fn, "gemini": fn}
	if len(opencode) > 0 {
		m["opencode"] = opencode[0]
	}
	return m
}

func claudeSessionStartStatic(content string) string {
	return `#!/bin/bash
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"[jeff] jq not installed - hooks degraded. Run: jeff doctor"}}'
  exit 0
fi
INPUT=$(cat)

read -r -d '' CONTEXT <<'HEREDOC' || true
` + content + `
HEREDOC

jq -n \
  --arg ctx "$CONTEXT" \
  '{
    hookSpecificOutput: {
      hookEventName: "SessionStart",
      additionalContext: $ctx
    }
  }'
`
}

// claudeSessionStartDynamic wraps content with shell expansions (e.g. $(gig ready))
// in a Claude Code SessionStart hook script. Uses double quotes so expansions are evaluated.
func claudeSessionStartDynamic(content string) string {
	return `#!/bin/bash
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"[jeff] jq not installed - hooks degraded. Run: jeff doctor"}}'
  exit 0
fi
INPUT=$(cat)

CONTEXT="` + content + `"

jq -n \
  --arg ctx "$CONTEXT" \
  '{
    hookSpecificOutput: {
      hookEventName: "SessionStart",
      additionalContext: $ctx
    }
  }'
`
}

// jsStaticSnippet returns a JS snippet that contributes static text.
func jsStaticSnippet(name, content string) string {
	return `        // [` + name + `]
        parts.push(` + strconv.Quote(content) + `);`
}

// jsDynamicSnippet returns a JS snippet that runs a command and contributes the output.
func jsDynamicSnippet(name, command string) string {
	return `        // [` + name + `]
        { const value = run(` + strconv.Quote(command) + `); if (value) parts.push(value); }`
}

func jsToolDynamicSnippet(name, command string) string {
	return `      // [` + name + `]
      { const value = run(` + strconv.Quote(command) + `); if (value) parts.push(value); }`
}

func jsExecFileSnippet(name, file string, args ...string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = strconv.Quote(arg)
	}
	joined := ""
	for i, arg := range quoted {
		if i > 0 {
			joined += ", "
		}
		joined += arg
	}
	return `        // [` + name + `]
        runFile(` + strconv.Quote(file) + `, [` + joined + `]);`
}

const gigInstructionsContext = `## Gig Task Management

` + "`gig`" + ` is a CLI task tracker. Use it to track progress and understand context.

- ` + "`gig show <id>`" + `                      — task details, checkpoint, deps, subtasks
- ` + "`gig list [--tree]`" + `                  — tasks and their status
- ` + "`gig create \"<title>\" [--parent <id>]`" + ` — create a task or subtask
- ` + "`gig update <id> --claim`" + `            — claim a task (sets assignee + in_progress)
- ` + "`gig update <id> --status <s>`" + `       — update status (open, in_progress, blocked)
- ` + "`gig close <id>`" + `                     — mark done (children must be closed first)
- ` + "`gig comment <id> \"<text>\"`" + `         — leave notes on a task
- ` + "`gig checkpoint <id> --done \"...\"`" + `  — save a progress snapshot
- ` + "`gig dep add <from> blocks <to>`" + `     — declare a dependency
- ` + "`gig search \"<query>\"`" + `              — find tasks by title/description

Run ` + "`gig <command> --help`" + ` for full flags and options.`

const jeffInstructionsContext = `## JEFF Commands

- ` + "`jeff checkpoint --done \"...\" [--next ...]`" + `        — save structured progress snapshot
- ` + "`jeff worktree add <repo> <branch> [--base <ref>]`" + ` — create a worktree
- ` + "`jeff ship`" + `                                        — push branches + create PRs for all repos. Use this to create PRs
- ` + "`jeff status`" + `                                      — overview of all active tasks and workspaces

### Crew Management (orchestrator)

- ` + "`jeff crew start <id> [--persona p] [--repos r1,r2]`" + ` — launch a worker in tmux
- ` + "`jeff crew resume <id>`" + `                               — resume a worker in tmux
- ` + "`jeff crew list`" + `                                      — show active workers
- ` + "`jeff crew status <id>`" + `                               — worker detail + pane output
- ` + "`jeff crew send <id> \"msg\" [--interrupt]`" + ` — message a worker
- ` + "`jeff crew events [--since 5m]`" + `                       — recent gig activity across workers
- ` + "`jeff crew stop <id>`" + `                                 — stop a worker
- ` + "`jeff crew ack <msg-id> [\"response\"]`" + `                — acknowledge an orchestrator message

Run ` + "`jeff <command> --help`" + ` for full flags and options.`

// --- Task-level hooks ---

// taskContextHook injects `gig show <task-id>` output so the agent knows
// what task it is working on, including status, checkpoints, and subtasks.
func taskContextHook() *Hook {
	return &Hook{
		Name:    "task-context",
		Source:  SourceTask,
		Event:   "SessionStart",
		Matcher: "*",
		Scripts: bashBoth(
			func(ctx HookContext) string {
				if ctx.TaskID == "" {
					return claudeSessionStartStatic("(no task context — TaskID not set)")
				}
				return claudeSessionStartDynamic(`## Current Task
` + "$(gig show " + shellQuote(ctx.TaskID) + " 2>/dev/null || echo '(task not found)')")
			},
			func(ctx HookContext) string {
				if ctx.TaskID == "" {
					return ""
				}
				return jsDynamicSnippet("task-context", `gig show `+shellQuote(ctx.TaskID)+` 2>/dev/null`)
			},
		),
	}
}

// taskCommandsHook injects task-dir-specific commands and good practices.
func taskCommandsHook() *Hook {
	return &Hook{
		Name:    "task-commands",
		Source:  SourceTask,
		Event:   "SessionStart",
		Matcher: "*",
		Scripts: bashBoth(
			func(ctx HookContext) string {
				return claudeSessionStartStatic(taskCommandsContext)
			},
			func(ctx HookContext) string {
				return jsStaticSnippet("task-commands", taskCommandsContext)
			},
		),
	}
}

// checkpointNudgeHook fires after Bash tool use and checks if the command
// matches any user-configured checkpoint patterns. If so, it reminds the
// agent to run jeff checkpoint.
func checkpointNudgeHook() *Hook {
	return &Hook{
		Name:    "checkpoint-nudge",
		Source:  SourceTask,
		Event:   "PostToolUse",
		Matcher: "Bash",
		Timeout: 5,
		Scripts: map[string]func(ctx HookContext) string{
                        "claude": func(ctx HookContext) string {
                                return buildCheckpointNudgeScript(ctx.CheckpointPatterns, "PostToolUse")
                        },
                        "opencode": func(ctx HookContext) string {
                                return buildOpenCodeCheckpointNudgeSnippet(ctx.CheckpointPatterns)
                        },
                        "gemini": func(ctx HookContext) string {
                                return buildCheckpointNudgeScript(ctx.CheckpointPatterns, "AfterTool")
                        },
                },
	}
}

// buildCheckpointNudgeScript generates a bash script that checks the tool
// input command against a list of regex patterns. If any match, it outputs
// a checkpoint reminder via the hook protocol. eventName is emitted as
// hookSpecificOutput.hookEventName and must match the agent's settings key.
func buildCheckpointNudgeScript(patterns []string, eventName string) string {
	if len(patterns) == 0 {
		// No patterns configured — script exits silently.
		return `#!/bin/bash
set -euo pipefail
cat > /dev/null
echo '{}'
`
	}

	// Build a combined ERE (Extended Regular Expression) pattern:
	// CheckpointPatterns are evaluated as EREs by grep -E. (pat1|pat2|pat3)
	combined := "(" + patterns[0]
	for _, p := range patterns[1:] {
		combined += "|" + p
	}
	combined += ")"

	return `#!/bin/bash
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo '{}'
  exit 0
fi
INPUT=$(cat)

# Extract the command from tool input.
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // ""')

if [ -z "$COMMAND" ]; then
  exit 0
fi

# Check against configured checkpoint patterns.
if echo "$COMMAND" | grep -qE ` + shellQuote(combined) + `; then
  jq -n \
    --arg ev "` + eventName + `" \
    '{
    hookSpecificOutput: {
      hookEventName: $ev,
      additionalContext: "You just completed a significant action. Consider running jeff checkpoint --done ... --next ... to save a progress snapshot for the user."
    }
  }'
fi
`
}

const taskCommandsContext = `## Task Commands

Commands available in this task workspace:

- ` + "`jeff worktree add <repo> <branch> --task-dir .`" + ` — get a new repo worktree for this task
- ` + "`jeff checkpoint --done \"...\" [--next \"...\"] [--decisions \"...\"]`" + ` — save a structured progress snapshot
- ` + "`jeff ship`" + `                                          — push all worktrees and create PRs
- ` + "`jeff done`" + `                                          — close the task and clean up workspace + worktrees
- ` + "`gig comment <id> \"...\"`" + `                             — leave notes on the task
- ` + "`gig update <id> --status blocked`" + `                   — flag blockers

## Good Practices

- **Checkpoint after logical blocks** — committed code, passing tests, finished a subtask. Run ` + "`jeff checkpoint --done \"...\" --next \"...\"`" + ` to keep the user informed without them having to read diffs.
- **Ship when ready for review** — ` + "`jeff ship`" + ` pushes all worktrees and creates PRs.
- **Mark done when complete** — ` + "`jeff done`" + ` closes the task and cleans up.`

// --- Crew hooks ---

// crewContextHook injects crew management commands and active session list
// into the orchestrator's session at JEFF_HOME level.
func crewContextHook() *Hook {
	return &Hook{
		Name:    "crew-context",
		Source:  SourceHome,
		Event:   "SessionStart",
		Matcher: "*",
		Scripts: bashBoth(
			func(ctx HookContext) string {
				return claudeSessionStartDynamic(`## Active Crew Sessions
` + "$(jeff crew list 2>/dev/null || echo '(no active sessions)')")
			},
			func(ctx HookContext) string {
				return jsDynamicSnippet("crew-context", `jeff crew list 2>/dev/null`)
			},
		),
	}
}

// inboxReplayHook is the SessionStart recovery path (Model B). Delivery happens
// directly via the pane keystroke in crew.Send; the inbox is a durable LOG. This
// hook replays any log rows still unacked at launch/resume — i.e. messages typed
// while the worker's pane was dead — attributes them, and acks them. It is the
// SOLE remaining hook-driven surfacing of message content: there is deliberately
// no PostToolUse / turn-end re-surfacing (that was the double-delivery).
func inboxReplayHook() *Hook {
	return &Hook{
		Name:    "inbox-replay",
		Source:  SourceTask,
		Event:   "SessionStart",
		Matcher: "*",
		Timeout: 5,
		Scripts: bashBoth(
			func(ctx HookContext) string {
				return buildInboxReplayScript(ctx.TaskID, "SessionStart")
			},
			func(ctx HookContext) string {
				// SessionStart → session.created (see openCodeEventName).
				return buildOpenCodeInboxCheckSnippet(ctx.TaskID)
			},
		),
	}
}

// buildInboxReplayScript generates the SessionStart replay script. It surfaces
// any unacked log rows via `jeff crew inbox --format agent` (which frames them as
// "[Orchestrator <msg-id>]: <content>" — identical to the direct-send framing —
// and acks them, so each is replayed exactly once). eventName is emitted as
// hookSpecificOutput.hookEventName and MUST match the agent's settings key.
func buildInboxReplayScript(taskID, eventName string) string {
	if taskID == "" {
		return `#!/bin/bash
set -euo pipefail
cat > /dev/null
echo '{}'
`
	}

	return `#!/bin/bash
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"[jeff] jq not installed - hooks degraded. Run: jeff doctor"}}'
  exit 0
fi
INPUT=$(cat)

# Recovery only: replay log rows left unacked because they were typed while this
# worker's pane was dead. Live-delivered messages were acked at send time, so in
# the normal case this is empty and nothing is re-surfaced.
PENDING=$(jeff crew inbox ` + shellQuote(taskID) + ` --count 2>/dev/null || echo "0")
[ "$PENDING" = "0" ] && exit 0

# --format agent frames + acks, so each row replays exactly once.
MESSAGES=$(jeff crew inbox ` + shellQuote(taskID) + ` --format agent 2>/dev/null)

jq -n \
  --arg ctx "$MESSAGES" \
  --arg ev "` + eventName + `" \
  '{
    hookSpecificOutput: {
      hookEventName: $ev,
      additionalContext: $ctx
    }
  }'
`
}

// --- Signal hooks ---

// workerHeartbeatHook fires after every tool use in a worker session.
// It touches the session's last_seen timestamp as a heartbeat signal.
func workerHeartbeatHook() *Hook {
	return &Hook{
		Name:    "worker-heartbeat",
		Source:  SourceTask,
		Event:   "PostToolUse",
		Matcher: "*",
		Timeout: 3,
		Scripts: bashBoth(
			func(ctx HookContext) string {
				return buildWorkerHeartbeatScript(ctx.TaskID)
			},
			func(ctx HookContext) string {
				if ctx.TaskID == "" {
					return ""
				}
				return jsToolDynamicSnippet("worker-heartbeat", "jeff crew touch "+shellQuote(ctx.TaskID)+" 2>/dev/null")
			},
		),
	}
}

func buildWorkerHeartbeatScript(taskID string) string {
	if taskID == "" {
		return `#!/bin/bash
set -euo pipefail
cat > /dev/null
echo '{}'
`
	}

	return `#!/bin/bash
set -euo pipefail

INPUT=$(cat)

# Touch last_seen as heartbeat signal.
# Stall detection is handled by jeff daemon, not here.
jeff crew touch ` + shellQuote(taskID) + ` 2>/dev/null || true

echo '{}'
`
}

// workerStopHook fires when the worker's agent session ends.
// Signals the orchestrator that the worker has stopped so it can check
// if this was intentional (jeff done) or unexpected (crash/timeout).
func workerStopHook() *Hook {
	return &Hook{
		Name:    "worker-stop",
		Source:  SourceTask,
		Event:   "Stop",
		Matcher: "*",
		Timeout: 5,
		// OpenCode: "Stop" maps to process.exit by default, which only fires
		// when the whole process ends — so an idle worker never pings the
		// orchestrator. session.idle is OpenCode's turn-end signal and matches
		// Claude's Stop-on-every-turn behavior, so the orchestrator is pinged
		// each time the agent finishes working.
		OpenCodeEvent: "session.idle",
		Scripts: bashBoth(
			func(ctx HookContext) string {
				return buildWorkerStopScript(ctx.TaskID, ctx.OrchestratorID)
			},
			func(ctx HookContext) string {
				return buildOpenCodeWorkerStopSnippet(ctx.TaskID, ctx.OrchestratorID)
			},
		),
	}
}

// sessionCaptureHook fires at SessionStart for task-level workers.
// It reads the session_id from the hook input JSON and stores it in the crew DB
// via `jeff crew session-id`, enabling `jeff crew resume` to use --resume.
func sessionCaptureHook() *Hook {
	return &Hook{
		Name:    "session-capture",
		Source:  SourceTask,
		Event:   "SessionStart",
		Matcher: "*",
		Timeout: 5,
		Scripts: bashBoth(
			func(ctx HookContext) string {
				return buildSessionCaptureScript(ctx.TaskID)
			},
			func(ctx HookContext) string {
				if ctx.TaskID == "" {
					return ""
				}
				return `        // [session-capture]
        if (id) runFile("jeff", ["crew", "session-id", ` + strconv.Quote(ctx.TaskID) + `, id]);`
			},
		),
	}
}

func buildSessionCaptureScript(taskID string) string {
	if taskID == "" {
		return `#!/bin/bash
set -euo pipefail
cat > /dev/null
echo '{}'
`
	}

	return `#!/bin/bash
set -euo pipefail

INPUT=$(cat)
SESSION_ID=$(echo "$INPUT" | jq -r '.session_id // ""')

if [ -z "$SESSION_ID" ]; then
  echo '{}'
  exit 0
fi

jeff crew session-id ` + shellQuote(taskID) + ` "$SESSION_ID" 2>/dev/null || true

echo '{}'
`
}

func buildWorkerStopScript(taskID, orchestratorID string) string {
	if taskID == "" || orchestratorID == "" {
		return `#!/bin/bash
set -euo pipefail
cat > /dev/null
echo '{}'
`
	}

	// Durable + real-time in one call: `jeff crew worker-stopped` persists a
	// de-duplicated to_orchestrator row (recovered by the orchestrator-inbox poll
	// even if the pane is dead) AND wakes the orchestrator pane. The de-dup means
	// Claude's per-turn Stop can't spam the orchestrator with "[Worker stopped]".
	return `#!/bin/bash
set -euo pipefail

INPUT=$(cat)

jeff crew worker-stopped ` + shellQuote(taskID) + ` 2>/dev/null || true

echo '{}'
`
}

// orchestratorInboxHook is the orchestrator-side mirror of inbox-replay (Model B).
// Worker→orchestrator delivery happens directly by typing framed content into the
// orchestrator pane (Ask / SignalOrchestrator / worker-stop); this hook does NOT
// re-surface those mid-session. It runs at SessionStart only, replaying any
// to_orchestrator log rows left unacked because they were typed while the
// orchestrator pane was dead — so a stop-ping sent to a down orchestrator is
// recovered exactly once on relaunch.
func orchestratorInboxHook() *Hook {
	return &Hook{
		Name:    "orchestrator-inbox",
		Source:  SourceHome,
		Event:   "SessionStart",
		Matcher: "*",
		Timeout: 5,
		Scripts: bashBoth(
			func(ctx HookContext) string {
				return buildOrchestratorInboxScript(ctx.OrchestratorID)
			},
			func(ctx HookContext) string {
				return buildOpenCodeOrchestratorInboxSnippet(ctx.OrchestratorID)
			},
		),
	}
}

func buildOrchestratorInboxScript(orchestratorID string) string {
	// If no orchestrator ID is configured, detect from tmux session name.
	detection := `ORCH_ID=` + shellQuote(orchestratorID)
	if orchestratorID == "" {
		detection = `# Auto-detect orchestrator ID from env var or tmux session name.
ORCH_ID=""
if [ -n "${JEFF_ORCHESTRATOR_SESSION:-}" ]; then
  ORCH_ID="$JEFF_ORCHESTRATOR_SESSION"
elif [ -n "${TMUX:-}" ]; then
  SESSION_NAME=$(tmux display-message -t "${TMUX_PANE:--}" -p '#{session_name}' 2>/dev/null || true)
  if echo "$SESSION_NAME" | grep -qE '^jeff-[a-z0-9][a-z0-9-]*$'; then
    ORCH_ID="$SESSION_NAME"
  fi
fi
[ -z "$ORCH_ID" ] && exit 0`
	}

	return `#!/bin/bash
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"[jeff] jq not installed - hooks degraded. Run: jeff doctor"}}'
  exit 0
fi
INPUT=$(cat)

` + detection + `

# Recovery only: replay to_orchestrator rows left unacked because they were typed
# while this orchestrator's pane was dead. Live-delivered signals were acked at
# send time, so in the normal case this is empty.
PENDING=$(jeff crew orchestrator-inbox "$ORCH_ID" --count 2>/dev/null || echo "0")
[ "$PENDING" = "0" ] && exit 0

# --format agent frames + acks, so each row replays exactly once.
MESSAGES=$(jeff crew orchestrator-inbox "$ORCH_ID" --format agent 2>/dev/null)

jq -n \
  --arg ctx "$MESSAGES" \
  '{
    hookSpecificOutput: {
      hookEventName: "SessionStart",
      additionalContext: $ctx
    }
  }'
`
}

func buildOpenCodeCheckpointNudgeSnippet(patterns []string) string {
	if len(patterns) == 0 {
		return ""
	}
	combined := "(" + patterns[0]
	for _, pattern := range patterns[1:] {
		combined += "|" + pattern
	}
	combined += ")"
	bt := "`"
	return `      // [checkpoint-nudge]
      const command = input.tool === "bash" ? (input.args?.command ?? "") : "";
      if (command && new RegExp(` + strconv.Quote(combined) + `).test(command)) {
        parts.push("You just completed a significant action. Consider running ` + bt + `jeff checkpoint --done ... --next ...` + bt + ` to save a progress snapshot for the user.");
      }`
}

func buildOpenCodeInboxCheckSnippet(taskID string) string {
	if taskID == "" {
		return ""
	}
	return `      // [inbox-check]
      { const count = run(` + strconv.Quote("jeff crew inbox "+shellQuote(taskID)+" --count 2>/dev/null") + `);
        if (count && count !== "0") {
          const messages = run(` + strconv.Quote("jeff crew inbox "+shellQuote(taskID)+" --format agent 2>/dev/null") + `);
          if (messages) parts.push(messages);
        }
      }`
}

func buildOpenCodeWorkerStopSnippet(taskID, orchestratorID string) string {
	if taskID == "" || orchestratorID == "" {
		return ""
	}
	// Durable + real-time in one call (see buildWorkerStopScript). The stopSignalled
	// debounce in the generated plugin still guards against per-idle repetition.
	return jsExecFileSnippet("worker-stop", "jeff", "crew", "worker-stopped", taskID)
}

func buildOpenCodeOrchestratorInboxSnippet(orchestratorID string) string {
	commandID := orchestratorID
	if commandID == "" {
		commandID = "${JEFF_ORCHESTRATOR_SESSION:-}"
	}
	if commandID == "" {
		return ""
	}
	return `      // [orchestrator-inbox]
      { const id = ` + strconv.Quote(commandID) + `;
        if (id) {
          const count = run("jeff crew orchestrator-inbox " + id + " --count 2>/dev/null");
          if (count && count !== "0") {
            const messages = run("jeff crew orchestrator-inbox " + id + " --format agent 2>/dev/null");
            if (messages) parts.push(messages);
          }
        }
      }`
}
