// sessionstart_memory.go — Worker A: hook that fires at agent session start to
// ensure the task's context file has the memory addendum and the agent's
// settings file has native-memory suppressed.
//
// Pickup already calls RunSessionStart directly in Go; this hook exists as a
// belt-and-suspenders guard for session resumes that don't re-run pickup.
package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/NeerajG03/JEFF/memory"
)

// RunSessionStart is the Go entry point for memory session-start logic.
// Called directly from the pickup flow and invocable via `jeff memory session-start`.
//
//  1. memory.EnsureLayout(jeffHome)
//  2. memory.ApplyToTask(jeffHome, taskDir, persona, taskID, repos, agentKind)
//  3. memory.ApplySettings(taskDir, agentKind)
//  4. writes a brief log entry to JEFF_HOME/queue/sessions/<task>-start.log
func RunSessionStart(jeffHome, taskDir, persona, taskID string, repos []string, agentKind string) error {
	if err := memory.EnsureLayout(jeffHome); err != nil {
		return fmt.Errorf("session-start: ensure layout: %w", err)
	}
	if err := memory.ApplyToTask(jeffHome, taskDir, persona, taskID, repos, agentKind); err != nil {
		return fmt.Errorf("session-start: inject addendum: %w", err)
	}
	if err := memory.ApplySettings(taskDir, agentKind); err != nil {
		return fmt.Errorf("session-start: suppress settings: %w", err)
	}
	return writeStartLog(jeffHome, taskID, persona, agentKind)
}

// writeStartLog appends a one-line entry to queue/sessions/<task>-start.log.
func writeStartLog(jeffHome, taskID, persona, agentKind string) error {
	dir := memory.QueueSessionsRoot(jeffHome)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("session-start: mkdir queue: %w", err)
	}
	safeName := strings.NewReplacer(string(filepath.Separator), "_", " ", "_", ":", "_").Replace(taskID)
	logPath := filepath.Join(dir, safeName+"-start.log")

	line := fmt.Sprintf("%s task=%s persona=%s agent=%s\n",
		time.Now().UTC().Format(time.RFC3339), taskID, persona, agentKind)

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("session-start: open log: %w", err)
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}

// sessionStartMemoryHook returns the hook definition for the memory SessionStart hook.
// The generated bash script calls `jeff memory session-start` with params embedded
// at hook generation time, making it safe for session resumes.
func sessionStartMemoryHook() *Hook {
	return &Hook{
		Name:    "memory-session-start",
		Source:  SourceTask,
		Event:   "SessionStart",
		Matcher: "*",
		Timeout: 15,
		Scripts: map[string]func(ctx HookContext) string{
			"claude": buildSessionStartMemoryScript,
			"opencode": buildOpenCodeSessionStartMemorySnippet,
			"gemini": buildSessionStartMemoryScript,
		},
	}
}

func buildOpenCodeSessionStartMemorySnippet(ctx HookContext) string {
	if ctx.TaskID == "" {
		return ""
	}
	repos := strings.Join(ctx.Repos, ",")
	return jsExecFileSnippet("memory-session-start", "jeff",
		"memory", "session-start",
		"--task-dir", ".",
		"--persona", ctx.Persona,
		"--task-id", ctx.TaskID,
		"--repos", repos,
		"--agent", "opencode")
}

func buildSessionStartMemoryScript(ctx HookContext) string {
	repos := strings.Join(ctx.Repos, ",")
	agentKind := "claude" // default; gemini delivery overrides event names but script stays the same

	return `#!/bin/bash
set -euo pipefail

INPUT=$(cat)
JEFF_HOME="${JEFF_HOME:-}"
TASK_DIR="$(pwd)"

# Values embedded at hook-installation time (idempotent for this task).
PERSONA=` + shellQuote(ctx.Persona) + `
TASK_ID=` + shellQuote(ctx.TaskID) + `
REPOS=` + shellQuote(repos) + `
AGENT=` + shellQuote(agentKind) + `

if [ -n "$JEFF_HOME" ]; then
  jeff memory session-start \
    --jeff-home "$JEFF_HOME" \
    --task-dir "$TASK_DIR" \
    --persona "$PERSONA" \
    --task-id "$TASK_ID" \
    --repos "$REPOS" \
    --agent "$AGENT" \
    2>/dev/null || true
fi

echo '{}'
`
}

// shellQuote wraps s in single quotes, escaping any single quotes within.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
