// inject.go — Worker A: composes the per-session memory addendum and applies it
// to the task's CLAUDE.md or GEMINI.md. Idempotent via HTML sentinel comments.
package memory

import (
	"fmt"
	"github.com/NeerajG03/JEFF"
	"os"
	"path/filepath"
	"sort"
	"strings"

	jeffembed "github.com/NeerajG03/JEFF/embed"
)

const maxIndexEntriesPerScope = 30

const (
	addendumStartSentinel = "<!-- jeff-memory-addendum -->"
	addendumEndSentinel   = "<!-- /jeff-memory-addendum -->"
)

// ApplyToTask injects the memory addendum into the task's context file.
//
// For agentKind=="claude", "opencode", or any unrecognised kind, the target is CLAUDE.md.
// For agentKind=="gemini", the target is GEMINI.md (often a symlink to CLAUDE.md).
//
// jeffHome is used to look up canonical memory entries in scope (persona,
// each repo, orchestrator) so the addendum can carry a name/description
// index, not just the static "how to propose" boilerplate.
//
// Idempotent: if the addendum sentinels are already present the block is
// replaced in place; running twice produces the same file.
func ApplyToTask(jeffHome, taskDir, persona, taskID string, repos []string, agentKind string) error {
	tmpl := addendumTemplate(agentKind)
	index := buildMemoryIndex(jeffHome, persona, repos)
	addendum := renderAddendum(tmpl, persona, taskID, repos, index)
	target := contextFilePath(taskDir, agentKind)
	return applyAddendum(target, addendum)
}

// addendumTemplate returns the raw template string for the given agent kind.
func addendumTemplate(agentKind string) string {
	name := "CLAUDE.md"
	p := jeff.GetProvider(jeff.AgentTool(agentKind))
	if p != nil {
		name = p.ContextFileName()
	}
	if name == "GEMINI.md" {
		return jeffembed.MemoryContextGemini
	}
	return jeffembed.MemoryContextClaude
}

// contextFilePath returns the absolute path of the context file to update.
func contextFilePath(taskDir, agentKind string) string {
	name := "CLAUDE.md"
	p := jeff.GetProvider(jeff.AgentTool(agentKind))
	if p != nil {
		name = p.ContextFileName()
	}
	return filepath.Join(taskDir, name)
}

// renderAddendum substitutes the template variables in the addendum text,
// including the pre-rendered memory index block.
func renderAddendum(tmpl, persona, taskID string, repos []string, memoryIndex string) string {
	reposStr := strings.Join(repos, ", ")
	if reposStr == "" {
		reposStr = "(none)"
	}
	s := tmpl
	s = strings.ReplaceAll(s, "{{persona}}", persona)
	s = strings.ReplaceAll(s, "{{task_id}}", taskID)
	s = strings.ReplaceAll(s, "{{repos}}", reposStr)
	s = strings.ReplaceAll(s, "{{memory_index}}", memoryIndex)
	return s
}

// buildMemoryIndex renders the name/description index grouped by scope:
// persona, then each repo in scope, then orchestrator (global rules). A
// scope section is omitted entirely when it has zero canonical entries. If
// no scope has any entries, the returned string is empty (no "Read full
// body" pointer to an empty index).
func buildMemoryIndex(jeffHome, persona string, repos []string) string {
	var sb strings.Builder
	any := false

	if persona != "" {
		if entries, err := ListScope(jeffHome, "persona:"+persona, "accepted"); err == nil && len(entries) > 0 {
			fmt.Fprintf(&sb, "## Persona memory (%s)\n", persona)
			overflow := writeIndexBulletsCap(&sb, entries)
			sb.WriteString("\n")
			if overflow > 0 {
				fmt.Fprintf(&sb, "- …and %d more — run 'jeff memory list --scope persona:%s'\n\n", overflow, persona)
			}
			any = true
		}
	}

	for _, repo := range repos {
		if entries, err := ListScope(jeffHome, "repo:"+repo, "accepted"); err == nil && len(entries) > 0 {
			fmt.Fprintf(&sb, "## Repo memory (%s)\n", repo)
			overflow := writeIndexBulletsCap(&sb, entries)
			sb.WriteString("\n")
			if overflow > 0 {
				fmt.Fprintf(&sb, "- …and %d more — run 'jeff memory list --scope repo:%s'\n\n", overflow, repo)
			}
			any = true
		}
	}

	if entries, err := ListScope(jeffHome, "orchestrator", "accepted"); err == nil && len(entries) > 0 {
		sb.WriteString("## Orchestrator memory (global rules)\n")
		overflow := writeIndexBulletsCap(&sb, entries)
		sb.WriteString("\n")
		if overflow > 0 {
			fmt.Fprintf(&sb, "- …and %d more — run 'jeff memory list --scope orchestrator'\n\n", overflow)
		}
		any = true
	}

	if !any {
		return ""
	}

	sb.WriteString("Read full body with `jeff memory show <name>` when the topic is relevant.\n\n")
	return sb.String()
}

// writeIndexBulletsCap appends one "`slug` — description" bullet per entry, sorted
// by importance descending then valid_from descending, capped at maxIndexEntriesPerScope.
// Returns the number of entries that were truncated (0 if within limit).
func writeIndexBulletsCap(sb *strings.Builder, entries []Entry) int {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].FM.Importance != entries[j].FM.Importance {
			return entries[i].FM.Importance > entries[j].FM.Importance
		}
		return entries[i].FM.ValidFrom.After(entries[j].FM.ValidFrom)
	})
	limit := maxIndexEntriesPerScope
	overflow := 0
	if len(entries) > limit {
		overflow = len(entries) - limit
		entries = entries[:limit]
	}
	for _, e := range entries {
		fmt.Fprintf(sb, "- `%s` — %s\n", e.FM.Name, e.FM.Description)
	}
	return overflow
}

// applyAddendum writes addendum into targetPath, replacing any existing
// sentinel block or appending if none is found. Normalises the addendum to
// end with exactly one newline before writing.
func applyAddendum(targetPath, addendum string) error {
	// Resolve symlinks so writing to GEMINI.md (a symlink → CLAUDE.md)
	// updates the real file instead of replacing the link.
	if resolved, err := filepath.EvalSymlinks(targetPath); err == nil {
		targetPath = resolved
	}

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

	// Write atomically: tmp file + rename (atomic on POSIX).
	tmpPath := targetPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("inject: write tmp: %w", err)
	}
	return os.Rename(tmpPath, targetPath)
}
