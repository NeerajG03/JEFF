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
		inboxCheckHook(),
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
		Scripts: map[string]func(ctx HookContext) string{
			"claude": func(ctx HookContext) string {
				return claudeSessionStartStatic(gigInstructionsContext)
			},
			"opencode": func(ctx HookContext) string {
				return jsStaticSnippet("gig-instructions", gigInstructionsContext)
			},
			"gemini": func(ctx HookContext) string {
				return claudeSessionStartStatic(gigInstructionsContext)
			},
		},
	}
}

// gigReadyTasksHook injects the output of `gig ready` into the agent session.
func gigReadyTasksHook() *Hook {
	return &Hook{
		Name:    "gig-ready-tasks",
		Source:  SourceHome,
		Event:   "SessionStart",
		Matcher: "*",
		Scripts: map[string]func(ctx HookContext) string{
			"claude": func(ctx HookContext) string {
				return claudeSessionStartDynamic(`## Tasks ready for pickup
` + "$(gig ready 2>/dev/null || echo '(no tasks)')")
			},
			"opencode": func(ctx HookContext) string {
				return jsDynamicSnippet("gig-ready-tasks", `gig ready 2>/dev/null`)
			},
			"gemini": func(ctx HookContext) string {
				return claudeSessionStartDynamic(`## Tasks ready for pickup
` + "$(gig ready 2>/dev/null || echo '(no tasks)')")
			},
		},
	}
}

// jeffReposHook injects the list of registered repos into the agent session.
func jeffReposHook() *Hook {
	return &Hook{
		Name:    "jeff-repos",
		Source:  SourceHome,
		Event:   "SessionStart",
		Matcher: "*",
		Scripts: map[string]func(ctx HookContext) string{
			"claude": func(ctx HookContext) string {
				return claudeSessionStartDynamic(`## Registered repos
` + "$(jeff repo list 2>/dev/null || echo '(none)')")
			},
			"opencode": func(ctx HookContext) string {
				return jsDynamicSnippet("jeff-repos", `jeff repo list 2>/dev/null`)
			},
			"gemini": func(ctx HookContext) string {
				return claudeSessionStartDynamic(`## Registered repos
` + "$(jeff repo list 2>/dev/null || echo '(none)')")
			},
		},
	}
}

// jeffInstructionsHook injects jeff CLI reference into the agent session.
func jeffInstructionsHook() *Hook {
	return &Hook{
		Name:    "jeff-instructions",
		Source:  SourceHome,
		Event:   "SessionStart",
		Matcher: "*",
		Scripts: map[string]func(ctx HookContext) string{
			"claude": func(ctx HookContext) string {
				return claudeSessionStartStatic(jeffInstructionsContext)
			},
			"opencode": func(ctx HookContext) string {
				return jsStaticSnippet("jeff-instructions", jeffInstructionsContext)
			},
			"gemini": func(ctx HookContext) string {
				return claudeSessionStartStatic(jeffInstructionsContext)
			},
		},
	}
}

// claudeSessionStartStatic wraps static content (no shell expansion) in a
// Claude Code SessionStart hook script. Uses a heredoc so backticks, single
// quotes, and double quotes are all passed through literally.
func claudeSessionStartStatic(content string) string {
	return `#!/bin/bash
set -euo pipefail

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
- ` + "`jeff crew send <id> \"msg\" [--type nudge|status|divert|normal]`" + ` — message a worker
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
		Scripts: map[string]func(ctx HookContext) string{
			"claude": func(ctx HookContext) string {
				if ctx.TaskID == "" {
					return claudeSessionStartStatic("(no task context — TaskID not set)")
				}
				return claudeSessionStartDynamic(`## Current Task
` + "$(gig show " + ctx.TaskID + " 2>/dev/null || echo '(task not found)')")
			},
			"opencode": func(ctx HookContext) string {
				if ctx.TaskID == "" {
					return ""
				}
				return jsDynamicSnippet("task-context", `gig show `+ctx.TaskID+` 2>/dev/null`)
			},
			"gemini": func(ctx HookContext) string {
				if ctx.TaskID == "" {
					return claudeSessionStartStatic("(no task context — TaskID not set)")
				}
				return claudeSessionStartDynamic(`## Current Task
` + "$(gig show " + ctx.TaskID + " 2>/dev/null || echo '(task not found)')")
			},
		},
	}
}

// taskCommandsHook injects task-dir-specific commands and good practices.
func taskCommandsHook() *Hook {
	return &Hook{
		Name:    "task-commands",
		Source:  SourceTask,
		Event:   "SessionStart",
		Matcher: "*",
		Scripts: map[string]func(ctx HookContext) string{
			"claude": func(ctx HookContext) string {
				return claudeSessionStartStatic(taskCommandsContext)
			},
			"opencode": func(ctx HookContext) string {
				return jsStaticSnippet("task-commands", taskCommandsContext)
			},
			"gemini": func(ctx HookContext) string {
				return claudeSessionStartStatic(taskCommandsContext)
			},
		},
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
				return buildCheckpointNudgeScript(ctx.CheckpointPatterns)
			},
			"opencode": func(ctx HookContext) string {
				return buildOpenCodeCheckpointNudgeSnippet(ctx.CheckpointPatterns)
			},
			// Gemini maps PostToolUse → AfterTool; same bash script works.
			"gemini": func(ctx HookContext) string {
				return buildCheckpointNudgeScript(ctx.CheckpointPatterns)
			},
		},
	}
}

// buildCheckpointNudgeScript generates a bash script that checks the tool
// input command against a list of regex patterns. If any match, it outputs
// a checkpoint reminder via the hook protocol.
func buildCheckpointNudgeScript(patterns []string) string {
	if len(patterns) == 0 {
		// No patterns configured — script exits silently.
		return `#!/bin/bash
set -euo pipefail
cat > /dev/null
echo '{}'
`
	}

	// Build a combined ERE pattern: (pat1|pat2|pat3)
	combined := "(" + patterns[0]
	for _, p := range patterns[1:] {
		combined += "|" + p
	}
	combined += ")"

	return `#!/bin/bash
set -euo pipefail

INPUT=$(cat)

# Extract the command from tool input.
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // ""')

if [ -z "$COMMAND" ]; then
  exit 0
fi

# Check against configured checkpoint patterns.
if echo "$COMMAND" | grep -qE '` + combined + `'; then
  jq -n '{
    hookSpecificOutput: {
      hookEventName: "PostToolUse",
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
		Scripts: map[string]func(ctx HookContext) string{
			"claude": func(ctx HookContext) string {
				return claudeSessionStartDynamic(`## Active Crew Sessions
` + "$(jeff crew list 2>/dev/null || echo '(no active sessions)')")
			},
			"opencode": func(ctx HookContext) string {
				return jsDynamicSnippet("crew-context", `jeff crew list 2>/dev/null`)
			},
			"gemini": func(ctx HookContext) string {
				return claudeSessionStartDynamic(`## Active Crew Sessions
` + "$(jeff crew list 2>/dev/null || echo '(no active sessions)')")
			},
		},
	}
}

// inboxCheckHook checks for pending orchestrator messages and surfaces
// them to the worker agent. Only nudge-type messages go through this hook;
// status/divert/normal messages are delivered directly via tmux.
func inboxCheckHook() *Hook {
	return &Hook{
		Name:    "inbox-check",
		Source:  SourceTask,
		Event:   "PostToolUse",
		Matcher: "*",
		Timeout: 3,
		Scripts: map[string]func(ctx HookContext) string{
			"claude": func(ctx HookContext) string {
				return buildInboxCheckScript(ctx.TaskID)
			},
			"opencode": func(ctx HookContext) string {
				return buildOpenCodeInboxCheckSnippet(ctx.TaskID)
			},
			"gemini": func(ctx HookContext) string {
				return buildInboxCheckScript(ctx.TaskID)
			},
		},
	}
}

// buildInboxCheckScript generates a bash script that checks jeff.db
// for pending nudge messages and surfaces them via the hook protocol.
func buildInboxCheckScript(taskID string) string {
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

# Quick check: any pending messages?
PENDING=$(jeff crew inbox ` + taskID + ` --count 2>/dev/null || echo "0")
[ "$PENDING" = "0" ] && exit 0

# Fetch messages formatted for agent consumption.
MESSAGES=$(jeff crew inbox ` + taskID + ` --format agent 2>/dev/null)

jq -n \
  --arg ctx "$MESSAGES" \
  '{
    hookSpecificOutput: {
      hookEventName: "PostToolUse",
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
		Scripts: map[string]func(ctx HookContext) string{
			"claude": func(ctx HookContext) string {
				return buildWorkerHeartbeatScript(ctx.TaskID)
			},
			"opencode": func(ctx HookContext) string {
				if ctx.TaskID == "" {
					return ""
				}
				return jsToolDynamicSnippet("worker-heartbeat", "jeff crew touch "+ctx.TaskID+" 2>/dev/null")
			},
			"gemini": func(ctx HookContext) string {
				return buildWorkerHeartbeatScript(ctx.TaskID)
			},
		},
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
jeff crew touch ` + taskID + ` 2>/dev/null || true

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
		Scripts: map[string]func(ctx HookContext) string{
			"claude": func(ctx HookContext) string {
				return buildWorkerStopScript(ctx.TaskID, ctx.OrchestratorID)
			},
			"opencode": func(ctx HookContext) string {
				return buildOpenCodeWorkerStopSnippet(ctx.TaskID, ctx.OrchestratorID)
			},
			"gemini": func(ctx HookContext) string {
				return buildWorkerStopScript(ctx.TaskID, ctx.OrchestratorID)
			},
		},
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
		Scripts: map[string]func(ctx HookContext) string{
			"claude": func(ctx HookContext) string {
				return buildSessionCaptureScript(ctx.TaskID)
			},
			"opencode": func(ctx HookContext) string {
				if ctx.TaskID == "" {
					return ""
				}
				return `        // [session-capture]
        if (id) runFile("jeff", ["crew", "session-id", ` + strconv.Quote(ctx.TaskID) + `, id]);`
			},
			"gemini": func(ctx HookContext) string {
				return buildSessionCaptureScript(ctx.TaskID)
			},
		},
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

jeff crew session-id ` + taskID + ` "$SESSION_ID" 2>/dev/null || true

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

	// Send directly to orchestrator pane via tmux — no DB lookup needed.
	// The orchestrator is always in the "orchestrator" window of its session.
	target := orchestratorID + ":orchestrator"
	message := "[Worker " + taskID + " stopped]: Agent has stopped working — the tmux session is still active."

	return `#!/bin/bash
set -euo pipefail

INPUT=$(cat)

# Signal orchestrator directly via tmux — DB may already be cleaned up.
# Two separate tmux invocations: paste text first, then send Enter.
# Chaining with \; in a single invocation drops the Enter (same class of bug as gig-4040).
tmux send-keys -t "` + target + `" -l "` + message + `" 2>/dev/null || true
tmux send-keys -t "` + target + `" Enter 2>/dev/null || true

echo '{}'
`
}

// orchestratorInboxHook fires after every tool use at JEFF_HOME level.
// It detects if we're running inside an orchestrator session (jeff-N),
// and if so, checks for pending to_orchestrator messages from workers.
func orchestratorInboxHook() *Hook {
	return &Hook{
		Name:    "orchestrator-inbox",
		Source:  SourceHome,
		Event:   "PostToolUse",
		Matcher: "*",
		Timeout: 5,
		Scripts: map[string]func(ctx HookContext) string{
			"claude": func(ctx HookContext) string {
				return buildOrchestratorInboxScript(ctx.OrchestratorID)
			},
			"opencode": func(ctx HookContext) string {
				return buildOpenCodeOrchestratorInboxSnippet(ctx.OrchestratorID)
			},
			"gemini": func(ctx HookContext) string {
				return buildOrchestratorInboxScript(ctx.OrchestratorID)
			},
		},
	}
}

func buildOrchestratorInboxScript(orchestratorID string) string {
	// If no orchestrator ID is configured, detect from tmux session name.
	detection := `ORCH_ID="` + orchestratorID + `"`
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

INPUT=$(cat)

` + detection + `

# Quick check: any pending messages for this orchestrator?
PENDING=$(jeff crew orchestrator-inbox "$ORCH_ID" --count 2>/dev/null || echo "0")
[ "$PENDING" = "0" ] && exit 0

# Fetch messages formatted for orchestrator consumption.
MESSAGES=$(jeff crew orchestrator-inbox "$ORCH_ID" --format agent 2>/dev/null)

jq -n \
  --arg ctx "$MESSAGES" \
  '{
    hookSpecificOutput: {
      hookEventName: "PostToolUse",
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
      { const count = run(` + strconv.Quote("jeff crew inbox "+taskID+" --count 2>/dev/null") + `);
        if (count && count !== "0") {
          const messages = run(` + strconv.Quote("jeff crew inbox "+taskID+" --format agent 2>/dev/null") + `);
          if (messages) parts.push(messages);
        }
      }`
}

func buildOpenCodeWorkerStopSnippet(taskID, orchestratorID string) string {
	if taskID == "" || orchestratorID == "" {
		return ""
	}
	target := orchestratorID + ":orchestrator"
	message := "[Worker " + taskID + " stopped]: Agent has stopped working — the tmux session is still active."
	return jsExecFileSnippet("worker-stop", "tmux", "send-keys", "-t", target, "-l", message) + "\n" +
		jsExecFileSnippet("worker-stop-enter", "tmux", "send-keys", "-t", target, "Enter")
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
