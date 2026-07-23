package task

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	jeff "github.com/NeerajG03/JEFF"
	jeffembed "github.com/NeerajG03/JEFF/embed"
	"github.com/NeerajG03/JEFF/memory"
	"github.com/NeerajG03/JEFF/persona"
	"github.com/NeerajG03/JEFF/workspace"
	"github.com/NeerajG03/gig"
)

// WriteClaudeMD generates a CLAUDE.md for the task directory combining persona
// instructions, task context, memory, workspace layout, and scratchpad guide.
// store may be nil (the checkpoint section is then omitted).
func WriteClaudeMD(taskDir, jeffHome string, store Store, task *gig.Task, personaName string, repos []string) error {
	var sb strings.Builder

	// Persona section — try registry first, fall back to embedded.
	if personaName != "" {
		content, err := persona.GetTemplate(jeffHome, personaName)
		if err != nil {
			// Fall back to embedded template.
			content, err = persona.Get(personaName)
		}
		if err == nil {
			sb.WriteString(content)
			sb.WriteString("\n\n---\n\n")
		}
	}

	// Task context.
	sb.WriteString("# Current Task\n\n")
	fmt.Fprintf(&sb, "- **ID:** %s\n", task.ID)
	fmt.Fprintf(&sb, "- **Title:** %s\n", task.Title)
	if task.Description != "" {
		fmt.Fprintf(&sb, "- **Description:** %s\n", task.Description)
	}
	fmt.Fprintf(&sb, "- **Priority:** P%d\n", task.Priority)
	fmt.Fprintf(&sb, "- **Type:** %s\n", task.Type)
	if task.ParentID != "" {
		fmt.Fprintf(&sb, "- **Parent:** %s\n", task.ParentID)
	}
	if len(task.Labels) > 0 {
		fmt.Fprintf(&sb, "- **Labels:** %s\n", strings.Join(task.Labels, ", "))
	}
	sb.WriteString("\n")

	// Latest checkpoint — so resumed sessions start with prior progress.
	if store != nil {
		if cp, err := store.LatestCheckpoint(task.ID); err == nil && cp != nil {
			sb.WriteString("## Resuming: Last Checkpoint\n\n")
			fmt.Fprintf(&sb, "_Recorded %s_\n\n", cp.CreatedAt.Format("2006-01-02 15:04"))
			if cp.Done != "" {
				sb.WriteString("- **Done:** " + cp.Done + "\n")
			}
			if cp.Decisions != "" {
				sb.WriteString("- **Decisions:** " + cp.Decisions + "\n")
			}
			if cp.Next != "" {
				sb.WriteString("- **Next:** " + cp.Next + "\n")
			}
			if cp.Blockers != "" {
				sb.WriteString("- **Blockers:** " + cp.Blockers + "\n")
			}
			if len(cp.Files) > 0 {
				sb.WriteString("- **Files touched:** " + strings.Join(cp.Files, ", ") + "\n")
			}
			sb.WriteString("\n")
		}
	}

	// Persona memory (gated by JEFF_MEMORY_DISABLE).
	if !memory.Disabled(jeffHome) && personaName != "" {
		content, err := memory.LoadPersonaMemory(jeffHome, personaName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: load persona memory: %v\n", err)
		}
		if content != "" {
			sb.WriteString("## Persona Memory\n\n")
			sb.WriteString(content)
			sb.WriteString("\n\n")
			fmt.Fprintf(&sb, "Detail files: `%s`\n\n", memory.PersonaMemoryDir(jeffHome, personaName))
		}
	}

	// Repo learnings (gated by JEFF_MEMORY_DISABLE).
	if !memory.Disabled(jeffHome) {
		for _, repoName := range repos {
			content, err := memory.LoadRepoLearnings(jeffHome, repoName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: load repo learnings for %s: %v\n", repoName, err)
				continue
			}
			if content != "" {
				fmt.Fprintf(&sb, "## Repo Learnings: %s\n\n", repoName)
				sb.WriteString(content)
				sb.WriteString("\n\n")
				fmt.Fprintf(&sb, "Detail files: `%s`\n\n", memory.RepoLearningsDir(jeffHome, repoName))
			}
		}
	}

	// Workspace layout with worktrees.
	writeWorkspaceLayout(&sb, taskDir)

	// Good practices.
	sb.WriteString("## Good Practices\n\n")
	sb.WriteString("- **Checkpoint after logical blocks** — committed code, passing tests, finished a subtask. ")
	sb.WriteString("Run `jeff checkpoint --done \"...\" --next \"...\"` to keep the user informed without them having to read diffs.\n")
	sb.WriteString("- **Ship when ready for review** — `jeff ship` pushes all worktrees and creates PRs.\n")
	sb.WriteString("- **Mark done when complete** — `jeff done` closes the task and cleans up.\n")
	sb.WriteString("\n")

	// Scratchpad & memory guide (gated by JEFF_MEMORY_DISABLE).
	if !memory.Disabled(jeffHome) && (personaName != "" || len(repos) > 0) {
		writeScratchpadGuide(&sb, taskDir, jeffHome, personaName, repos)
	}

	path := filepath.Join(taskDir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return err
	}

	// Create context file aliases (e.g. GEMINI.md → CLAUDE.md) for all registered providers.
	for _, agent := range jeff.RegisteredAgents() {
		p := jeff.GetProvider(agent)
		if p == nil {
			continue
		}
		if aliases := p.ContextFileAliases(); len(aliases) > 0 {
			if err := jeffembed.CreateContextAliases(taskDir, aliases); err != nil {
				return fmt.Errorf("create context aliases for %s: %w", agent, err)
			}
		}
	}

	return nil
}

// writeScratchpadGuide appends the scratchpad and memory usage instructions.
func writeScratchpadGuide(sb *strings.Builder, taskDir, jeffHome, personaName string, repos []string) {
	sb.WriteString("## Scratchpad\n\n")

	sb.WriteString("### Reading Memory\n")
	if personaName != "" {
		sb.WriteString("- **Persona memory** above contains knowledge from past sessions — scan before starting.\n")
	}
	if len(repos) > 0 {
		sb.WriteString("- **Repo learnings** above contain repo-specific setup and quirks — scan before starting.\n")
	}
	sb.WriteString("- When a topic is relevant to your current work, read the detail file for deeper context.\n\n")

	sb.WriteString("### Writing to Scratchpad\n")
	sb.WriteString("During your work, you'll discover things worth remembering — corrections from the user, ")
	sb.WriteString("repo quirks, commands that were wrong, debugging insights, outdated skill/hook info.\n\n")
	fmt.Fprintf(sb, "Append raw observations to: `%s`\n\n", memory.ScratchpadPath(taskDir))
	sb.WriteString("Format — just append, don't overthink structure:\n\n")
	sb.WriteString("```\n## <short title>\n[persona] or [repo:<name>]\n<what you observed, what went wrong, what the user corrected>\n```\n\n")

	sb.WriteString("### What belongs in the scratchpad\n")
	sb.WriteString("- User corrections (\"don't do X\", \"always do Y\")\n")
	sb.WriteString("- Commands or skill info that was outdated/wrong\n")
	sb.WriteString("- Repo setup quirks that tripped you up\n")
	sb.WriteString("- Code patterns the user prefers\n")
	sb.WriteString("- Debugging insights that took time to discover\n")
	if personaName != "" {
		// Try registry hint first, fall back to embedded.
		hint := persona.RegisteredMemoryHint(jeffHome, personaName)
		if hint == "" {
			hint = persona.MemoryHint(personaName)
		}
		if hint != "" {
			fmt.Fprintf(sb, "\n**As %s, especially capture:** %s\n", personaName, hint)
		}
	}
	sb.WriteString("\n")

	sb.WriteString("### What does NOT belong\n")
	sb.WriteString("- Code structure or file paths (read the code instead)\n")
	sb.WriteString("- Anything already in CLAUDE.md or git history\n")
	sb.WriteString("- Task-specific implementation details\n\n")

	sb.WriteString("After the task, run `/learn` to curate scratchpad observations into persistent memory.\n")
}

// RefreshClaudeMD regenerates the task CLAUDE.md, preserving the persona that
// was used at pickup. Called after worktrees are added/removed.
func RefreshClaudeMD(store Store, cfg *jeff.Config, taskID, taskDir string) error {
	task, err := store.Get(taskID)
	if err != nil {
		return err
	}

	// Resolve persona: gig attr set at pickup, falling back to CLAUDE.md
	// prefix detection for workspaces created by older binaries.
	personaName := ResolvePersona(store, taskID, taskDir)

	// Detect repos from existing worktree symlinks.
	repos := DetectRepos(taskDir)

	return WriteClaudeMD(taskDir, cfg.Home, store, task, personaName, repos)
}

// ResolvePersona returns the persona for a task: the gig attr written at
// pickup, falling back to CLAUDE.md prefix detection for workspaces created by
// older binaries.
func ResolvePersona(store Store, taskID, taskDir string) string {
	if attr, err := store.GetAttr(taskID, jeff.AttrPersona); err == nil && attr != nil && attr.Value != "" {
		return attr.Value
	}
	return DetectPersona(taskDir)
}

// DetectRepos returns repo names from worktree symlinks in the task directory.
func DetectRepos(taskDir string) []string {
	wts, _ := workspace.ListTaskWorktrees(taskDir)
	var repos []string
	for _, wt := range wts {
		repos = append(repos, wt.Repo)
	}
	return repos
}

// DetectPersona reads the existing CLAUDE.md and returns the persona name if one
// was used, or "" if none.
func DetectPersona(taskDir string) string {
	data, err := os.ReadFile(filepath.Join(taskDir, "CLAUDE.md"))
	if err != nil {
		return ""
	}
	content := string(data)
	// Persona content appears before "---" separator and before "# Current Task".
	if !strings.Contains(content, "\n---\n") {
		return ""
	}
	// Check each known persona.
	for _, name := range persona.Names() {
		p, err := persona.Get(name)
		if err != nil {
			continue
		}
		if strings.HasPrefix(content, p) {
			return name
		}
	}
	return ""
}

// writeWorkspaceLayout appends the workspace directory layout to sb, showing
// symlinked worktrees and their branches.
func writeWorkspaceLayout(sb *strings.Builder, taskDir string) {
	worktrees, _ := workspace.ListTaskWorktrees(taskDir)
	if len(worktrees) == 0 {
		return
	}

	sb.WriteString("## Workspace\n\n")
	sb.WriteString("```\n")
	sb.WriteString(filepath.Base(taskDir) + "/\n")
	for i, wt := range worktrees {
		connector := "├── "
		if i == len(worktrees)-1 {
			connector = "└── "
		}
		sb.WriteString(connector + wt.Repo + "/")
		if wt.Branch != "" {
			sb.WriteString("  (branch: " + wt.Branch + ")")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("```\n\n")
}

// buildTaskJSON marshals a task (with attrs) to JSON for branch naming scripts.
// Uses store.GetFull which populates the Attrs field — same output as gig show --json.
func buildTaskJSON(store Store, task *gig.Task) []byte {
	full, err := store.GetFull(task.ID)
	if err != nil {
		full = task
	}
	data, _ := json.Marshal(full)
	return data
}

// appendReadonlyNote appends a warning to CLAUDE.md listing repos the agent must not modify.
func appendReadonlyNote(taskDir string, reposReadonly []string) error {
	claudeMDPath := filepath.Join(taskDir, "CLAUDE.md")
	f, err := os.OpenFile(claudeMDPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open CLAUDE.md: %w", err)
	}
	defer f.Close()

	names := make([]string, len(reposReadonly))
	for i, r := range reposReadonly {
		names[i] = "`" + r + "`"
	}
	note := fmt.Sprintf("\n> **Read-only repos:** %s — symlinked for reading only. Do not commit changes to these repos.\n",
		strings.Join(names, ", "))
	_, err = f.WriteString(note)
	return err
}

// resolveRepoBranch determines the branch name for a repo. Uses the branch_name
// script if configured, otherwise falls back to defaultBranch.
func resolveRepoBranch(rc *jeff.RepoConfig, taskJSON []byte, defaultBranch string) (string, error) {
	if rc == nil || rc.BranchName == "" {
		return defaultBranch, nil
	}
	return workspace.ResolveBranchName(rc.BranchName, taskJSON, defaultBranch)
}
