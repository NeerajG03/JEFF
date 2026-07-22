// memory_propose_nudge.go — one-shot nudge to propose memory before the session ends.
// Fires on Stop (claude) / AfterAgent (gemini).
// The .nudged sentinel persists across session resumes — one nudge per task lifetime.
// To re-trigger, remove the sentinel manually: rm .nudged
package hooks

// memoryProposeNudgeHook returns the hook that nudges the worker to propose memory
// before exiting. Fires on Stop; blocks once per task via a .nudged sentinel.
func memoryProposeNudgeHook() *Hook {
	return &Hook{
		Name:    "memory-propose-nudge",
		Source:  SourceTask,
		Event:   "Stop",
		Matcher: "*",
		Timeout: 5,
		OpenCodeEvent: "session.idle",
		Scripts: map[string]func(ctx HookContext) string{
			"claude": buildMemoryProposeNudgeScript,
			"opencode": buildOpenCodeMemoryProposeNudgeSnippet,
			"gemini": buildMemoryProposeNudgeScript,
		},
	}
}

func buildOpenCodeMemoryProposeNudgeSnippet(_ HookContext) string {
	return `        // [memory-propose-nudge]
        if (!run("test -f .nudged")) {
          run("touch .nudged");
          parts.push("Before exiting: did anything surface this session worth remembering? If yes, run: jeff memory propose --name <slug> --type <user|feedback|project|reference> --description \\\"<one-liner>\\\" --body \\\"<details>\\\". Otherwise just continue.");
        }`
}

func buildMemoryProposeNudgeScript(_ HookContext) string {
	return `#!/bin/bash
set -euo pipefail

INPUT=$(cat)

SENTINEL="$(pwd)/.nudged"

# One nudge per task — sentinel persists across session resumes.
# To re-trigger, remove it manually: rm .nudged
if [ -f "$SENTINEL" ]; then
  echo '{}'
  exit 0
fi

touch "$SENTINEL"

REASON='Before exiting: did anything surface this session worth remembering? If yes, run: jeff memory propose --name <slug> --type <user|feedback|project|reference> --description "<one-liner>" --body "<details>". Otherwise just continue.'

jq -n --arg reason "$REASON" '{"decision": "block", "reason": $reason}'
`
}
