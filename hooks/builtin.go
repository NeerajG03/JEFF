package hooks

// builtinHooks returns all built-in hook definitions.
func builtinHooks() []*Hook {
	return []*Hook{
		gigInstructionsHook(),
		gigReadyTasksHook(),
		jeffReposHook(),
		jeffInstructionsHook(),
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
` + "$(jeff repo list 2>/dev/null | awk '{print $1}' || echo '(none)')")
		},
		OpenCodeSnippet: func(ctx HookContext) string {
			return jsDynamicSnippet("jeff-repos", `jeff repo list 2>/dev/null | awk '{print $1}'`)
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

You have access to ` + "`gig`" + ` — a CLI task tracker.

### Quick reference
- ` + "`gig list [--tree]`" + `           — list open tasks
- ` + "`gig show <id>`" + `              — task details + latest checkpoint
- ` + "`gig ready [--id <parent>]`" + `  — what's available to pick up
- ` + "`gig create \"<title>\"`" + `       — create a task
- ` + "`gig update <id> --claim`" + `    — claim a task
- ` + "`gig close <id>`" + `             — close a task
- ` + "`gig comment <id> \"<text>\"`" + ` — add a comment
- ` + "`gig comments <id>`" + `          — view comments
- ` + "`gig checkpoints <id>`" + `       — view progress snapshots`

const jeffInstructionsContext = `## JEFF Commands

- ` + "`jeff pickup <gig-id> [--persona <name>]`" + ` — claim task, setup workspace, start working
- ` + "`jeff work [<gig-id>]`" + `                     — resume work in existing task dir
- ` + "`jeff checkpoint --done \"...\" [--next ...]`" + ` — save structured progress snapshot
- ` + "`jeff worktree add <repo> <branch>`" + `        — create worktree, symlink to task dir
- ` + "`jeff ship`" + `                                — push branches + create PRs
- ` + "`jeff done [<gig-id>]`" + `                     — close task, cleanup workspace
- ` + "`jeff status`" + `                              — overview of all active tasks`
