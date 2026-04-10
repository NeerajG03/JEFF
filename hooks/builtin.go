package hooks

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
	}
}

// gigInstructionsHook injects gig CLI reference into the agent session.
func gigInstructionsHook() *Hook {
	return &Hook{
		Name:    "gig-instructions",
		Source:  SourceHome,
		Event:   "SessionStart",
		Matcher: "*",
		ClaudeScript: func(ctx HookContext) string {
			return claudeSessionStartStatic(gigInstructionsContext)
		},
		OpenCodeSnippet: func(ctx HookContext) string {
			return jsStaticSnippet("gig-instructions", gigInstructionsContext)
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
		ClaudeScript: func(ctx HookContext) string {
			return claudeSessionStartDynamic(`## Tasks ready for pickup
` + "$(gig ready 2>/dev/null || echo '(no tasks)')")
		},
		OpenCodeSnippet: func(ctx HookContext) string {
			return jsDynamicSnippet("gig-ready-tasks", `gig ready 2>/dev/null`)
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
		ClaudeScript: func(ctx HookContext) string {
			return claudeSessionStartDynamic(`## Registered repos
` + "$(jeff repo list 2>/dev/null || echo '(none)')")
		},
		OpenCodeSnippet: func(ctx HookContext) string {
			return jsDynamicSnippet("jeff-repos", `jeff repo list 2>/dev/null`)
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
		ClaudeScript: func(ctx HookContext) string {
			return claudeSessionStartStatic(jeffInstructionsContext)
		},
		OpenCodeSnippet: func(ctx HookContext) string {
			return jsStaticSnippet("jeff-instructions", jeffInstructionsContext)
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
	return `  // [` + name + `]
  parts.push(` + "`" + content + "`" + `);`
}

// jsDynamicSnippet returns a JS snippet that runs a command and contributes the output.
func jsDynamicSnippet(name, command string) string {
	return `  // [` + name + `]
  try {
    parts.push(execSync("` + command + `", { encoding: "utf-8" }).trim());
  } catch { /* skip if unavailable */ }`
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
		ClaudeScript: func(ctx HookContext) string {
			if ctx.TaskID == "" {
				return claudeSessionStartStatic("(no task context — TaskID not set)")
			}
			return claudeSessionStartDynamic(`## Current Task
` + "$(gig show " + ctx.TaskID + " 2>/dev/null || echo '(task not found)')")
		},
		OpenCodeSnippet: func(ctx HookContext) string {
			if ctx.TaskID == "" {
				return ""
			}
			return jsDynamicSnippet("task-context", `gig show `+ctx.TaskID+` 2>/dev/null`)
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
		ClaudeScript: func(ctx HookContext) string {
			return claudeSessionStartStatic(taskCommandsContext)
		},
		OpenCodeSnippet: func(ctx HookContext) string {
			return jsStaticSnippet("task-commands", taskCommandsContext)
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
		ClaudeScript: func(ctx HookContext) string {
			return buildCheckpointNudgeScript(ctx.CheckpointPatterns)
		},
		OpenCodeSnippet: func(ctx HookContext) string {
			// PostToolUse is Claude Code specific; no OpenCode equivalent.
			return ""
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
		ClaudeScript: func(ctx HookContext) string {
			return claudeSessionStartDynamic(`## Active Crew Sessions
` + "$(jeff crew list 2>/dev/null || echo '(no active sessions)')")
		},
		OpenCodeSnippet: func(ctx HookContext) string {
			return jsDynamicSnippet("crew-context", `jeff crew list 2>/dev/null`)
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
		ClaudeScript: func(ctx HookContext) string {
			return buildInboxCheckScript(ctx.TaskID)
		},
		OpenCodeSnippet: func(ctx HookContext) string {
			// PostToolUse hook; no OpenCode equivalent.
			return ""
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
		ClaudeScript: func(ctx HookContext) string {
			return buildWorkerHeartbeatScript(ctx.TaskID)
		},
		OpenCodeSnippet: func(ctx HookContext) string {
			return ""
		},
	}
}

func buildWorkerHeartbeatScript(taskID string) string {
	if taskID == "" {
		return `#!/bin/bash
set -euo pipefail
cat > /dev/null
`
	}

	return `#!/bin/bash
set -euo pipefail

INPUT=$(cat)

# Touch last_seen as heartbeat signal.
# Stall detection is handled by jeff daemon, not here.
jeff crew touch ` + taskID + ` 2>/dev/null || true
`
}

// workerStopHook fires when the worker's Claude Code session ends.
// Signals the orchestrator that the worker has stopped so it can check
// if this was intentional (jeff done) or unexpected (crash/timeout).
func workerStopHook() *Hook {
	return &Hook{
		Name:    "worker-stop",
		Source:  SourceTask,
		Event:   "Stop",
		Matcher: "*",
		Timeout: 5,
		ClaudeScript: func(ctx HookContext) string {
			return buildWorkerStopScript(ctx.TaskID, ctx.OrchestratorID)
		},
		OpenCodeSnippet: func(ctx HookContext) string {
			return ""
		},
	}
}

func buildWorkerStopScript(taskID, orchestratorID string) string {
	if taskID == "" || orchestratorID == "" {
		return `#!/bin/bash
set -euo pipefail
cat > /dev/null
`
	}

	// Send directly to orchestrator pane via tmux — no DB lookup needed.
	// The orchestrator is always in the "orchestrator" window of its session.
	target := orchestratorID + ":orchestrator"
	message := "[Worker " + taskID + " stopped]: Session ended — please check if this was intentional."

	return `#!/bin/bash
set -euo pipefail

INPUT=$(cat)

# Signal orchestrator directly via tmux — DB may already be cleaned up.
tmux send-keys -t "` + target + `" -l "` + message + `" \; send-keys -t "` + target + `" Enter 2>/dev/null || true
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
		ClaudeScript: func(ctx HookContext) string {
			return buildOrchestratorInboxScript(ctx.OrchestratorID)
		},
		OpenCodeSnippet: func(ctx HookContext) string {
			return ""
		},
	}
}

func buildOrchestratorInboxScript(orchestratorID string) string {
	// If no orchestrator ID is configured, detect from tmux session name.
	detection := `ORCH_ID="` + orchestratorID + `"`
	if orchestratorID == "" {
		detection = `# Auto-detect orchestrator ID from tmux session name.
ORCH_ID=""
if [ -n "${TMUX:-}" ]; then
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

