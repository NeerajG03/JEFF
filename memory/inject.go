// inject.go — Worker A: composes the per-session memory addendum and applies it
// to the task's CLAUDE.md or GEMINI.md. Idempotent via HTML sentinel comments.
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	jeffembed "github.com/NeerajG03/JEFF/embed"
)

const (
	addendumStartSentinel = "<!-- jeff-memory-addendum -->"
	addendumEndSentinel   = "<!-- /jeff-memory-addendum -->"
)

// ApplyToTask injects the memory addendum into the task's context file.
//
// For agentKind=="claude" (or any unrecognised kind), the target is CLAUDE.md.
// For agentKind=="gemini", the target is GEMINI.md (often a symlink to CLAUDE.md).
//
// Idempotent: if the addendum sentinels are already present the block is
// replaced in place; running twice produces the same file.
func ApplyToTask(taskDir, persona, taskID string, repos []string, agentKind string) error {
	tmpl := addendumTemplate(agentKind)
	addendum := renderAddendum(tmpl, persona, taskID, repos)
	target := contextFilePath(taskDir, agentKind)
	return applyAddendum(target, addendum)
}

// addendumTemplate returns the raw template string for the given agent kind.
func addendumTemplate(agentKind string) string {
	if agentKind == "gemini" {
		return jeffembed.MemoryContextGemini
	}
	return jeffembed.MemoryContextClaude
}

// contextFilePath returns the absolute path of the context file to update.
func contextFilePath(taskDir, agentKind string) string {
	name := "CLAUDE.md"
	if agentKind == "gemini" {
		name = "GEMINI.md"
	}
	return filepath.Join(taskDir, name)
}

// renderAddendum substitutes the three template variables in the addendum text.
func renderAddendum(tmpl, persona, taskID string, repos []string) string {
	reposStr := strings.Join(repos, ", ")
	if reposStr == "" {
		reposStr = "(none)"
	}
	s := tmpl
	s = strings.ReplaceAll(s, "{{persona}}", persona)
	s = strings.ReplaceAll(s, "{{task_id}}", taskID)
	s = strings.ReplaceAll(s, "{{repos}}", reposStr)
	return s
}

// applyAddendum writes addendum into targetPath, replacing any existing
// sentinel block or appending if none is found. Normalises the addendum to
// end with exactly one newline before writing.
func applyAddendum(targetPath, addendum string) error {
	existing, err := os.ReadFile(targetPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inject: read %s: %w", filepath.Base(targetPath), err)
	}
	content := string(existing)

	// Normalise: addendum ends with exactly one newline.
	addendum = strings.TrimRight(addendum, "\n") + "\n"

	startIdx := strings.Index(content, addendumStartSentinel)
	endIdx := strings.LastIndex(content, addendumEndSentinel)

	if startIdx >= 0 && endIdx > startIdx {
		// Replace existing block (from start sentinel through end sentinel + newline).
		end := endIdx + len(addendumEndSentinel)
		if end < len(content) && content[end] == '\n' {
			end++
		}
		content = content[:startIdx] + addendum + content[end:]
	} else {
		// Append with a blank separator line.
		if len(content) > 0 {
			if !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			content += "\n"
		}
		content += addendum
	}

	return os.WriteFile(targetPath, []byte(content), 0o644)
}
