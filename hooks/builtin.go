package hooks

// builtinHooks returns all built-in hook definitions.
func builtinHooks() []*Hook {
	return []*Hook{
		// Home-level hooks.
		gigInstructionsHook(),
		gigReadyTasksHook(),
		jeffReposHook(),
		jeffInstructionsHook(),
		// Task-level hooks.
		taskContextHook(),
		taskCommandsHook(),
		checkpointNudgeHook(),
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
