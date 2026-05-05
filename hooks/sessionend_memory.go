// sessionend_memory.go — Worker A: hook that fires when an agent session ends.
// Copies the session transcript and writes a queue entry for marlowe to process.
// No LLM calls — pure file I/O.
package hooks

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/NeerajG03/JEFF/memory"
)

// RunSessionEnd is the Go entry point for memory session-end logic.
// Called from `jeff memory session-end` (invoked by the Stop bash hook).
//
//  1. Copies transcriptPath → JEFF_HOME/transcripts/<task>/<ts>.jsonl
//  2. Lists proposals/<persona>/<task>/*.md and collects names
//  3. Writes queue/sessions/<task>-<ts>.json via memory.WriteQueueEntry
//
// Idempotent: each call creates a new queue entry with a fresh timestamp.
// No LLM is spawned.
func RunSessionEnd(jeffHome, taskID, persona string, repos []string, agentKind, transcriptPath, reason string) error {
	now := time.Now().UTC()

	// Copy transcript (best-effort — queue entry is still written on failure).
	copiedPath := ""
	if transcriptPath != "" {
		var err error
		copiedPath, err = copyTranscript(jeffHome, taskID, transcriptPath, now)
		if err != nil {
			fmt.Fprintf(os.Stderr, "session-end: copy transcript: %v\n", err)
		}
	}

	// Collect proposal names for this persona+task.
	proposals, err := listProposalNames(jeffHome, persona, taskID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "session-end: list proposals: %v\n", err)
	}

	entry := memory.SessionQueueEntry{
		Task:           taskID,
		Persona:        persona,
		Repos:          repos,
		TranscriptPath: copiedPath,
		Reason:         reason,
		Proposals:      proposals,
		EndedAt:        now,
	}
	if _, err := memory.WriteQueueEntry(jeffHome, entry); err != nil {
		return fmt.Errorf("session-end: write queue entry: %w", err)
	}
	return nil
}

// copyTranscript copies src to JEFF_HOME/transcripts/<task>/<ts>.jsonl and
// returns the destination path.
func copyTranscript(jeffHome, taskID, src string, ts time.Time) (string, error) {
	taskSlug := strings.NewReplacer(
		string(filepath.Separator), "_", " ", "_", ":", "_",
	).Replace(taskID)

	destDir := filepath.Join(memory.TranscriptsRoot(jeffHome), taskSlug)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir transcripts: %w", err)
	}

	ext := filepath.Ext(src)
	if ext == "" {
		ext = ".jsonl"
	}
	dest := filepath.Join(destDir, ts.Format("20060102T150405Z")+ext)

	if err := copyFile(src, dest); err != nil {
		return "", fmt.Errorf("copy %s → %s: %w", src, dest, err)
	}
	return dest, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// listProposalNames returns the slug names of proposals for (persona, task).
func listProposalNames(jeffHome, persona, taskID string) ([]string, error) {
	proposals, err := memory.ListProposals(jeffHome, persona, taskID)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(proposals))
	for i, p := range proposals {
		names[i] = p.Slug
	}
	return names, nil
}

// sessionEndMemoryHook returns the hook definition for the memory SessionEnd hook.
// Fires on the Stop event; calls `jeff memory session-end` via the bash script.
func sessionEndMemoryHook() *Hook {
	return &Hook{
		Name:    "memory-session-end",
		Source:  SourceTask,
		Event:   "Stop",
		Matcher: "*",
		Timeout: 15,
		Scripts: map[string]func(ctx HookContext) string{
			"claude": buildSessionEndMemoryScript,
			"gemini": buildSessionEndMemoryScript,
		},
	}
}

func buildSessionEndMemoryScript(ctx HookContext) string {
	repos := strings.Join(ctx.Repos, ",")
	agentKind := "claude"

	return `#!/bin/bash
set -euo pipefail

INPUT=$(cat)
JEFF_HOME="${JEFF_HOME:-}"

# Values embedded at hook-installation time.
TASK_ID=` + shellQuote(ctx.TaskID) + `
PERSONA=` + shellQuote(ctx.Persona) + `
REPOS=` + shellQuote(repos) + `
AGENT=` + shellQuote(agentKind) + `

TRANSCRIPT=$(echo "$INPUT" | jq -r '.transcript_path // ""')
REASON=$(echo "$INPUT" | jq -r '.stop_reason // "unknown"')

if [ -n "$JEFF_HOME" ]; then
  jeff memory session-end \
    --jeff-home "$JEFF_HOME" \
    --task "$TASK_ID" \
    --persona "$PERSONA" \
    --repos "$REPOS" \
    --transcript "$TRANSCRIPT" \
    --reason "$REASON" \
    --agent "$AGENT" \
    2>/dev/null || true
fi

echo '{}'
`
}
