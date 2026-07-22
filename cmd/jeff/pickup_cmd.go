package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	jeff "github.com/NeerajG03/JEFF"
	jeffembed "github.com/NeerajG03/JEFF/embed"
	"github.com/NeerajG03/JEFF/internal/gitutil"
	"github.com/NeerajG03/JEFF/memory"
	"github.com/NeerajG03/JEFF/persona"
	"github.com/NeerajG03/JEFF/workspace"
	"github.com/NeerajG03/gig"
	"github.com/spf13/cobra"
)

func pickupCmd() *cobra.Command {
	var (
		personaName string
		repos       []string
	)

	cmd := &cobra.Command{
		Use:   "pickup <gig-id>",
		Short: "Claim a task, set up workspace, and launch agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := args[0]

			// Resolve agent from persona, fall back to global config.
			agentTool := cfg.Agent
			if personaName != "" {
				if pa := persona.RegisteredAgent(cfg.Home, personaName); pa != "" {
					agentTool = jeff.AgentTool(pa)
				}
			}

			taskDir, err := pickupTask(taskID, personaName, repos, nil, "", agentTool)
			if err != nil {
				return err
			}

			// Launch agent tool in task directory (foreground, blocks).
			fmt.Fprintf(os.Stderr, "\nLaunching %s in %s...\n", agentTool, taskDir)
			// Resolve persona model for foreground launch.
			model := persona.RegisteredModel(cfg.Home, personaName)
			return launchAgent(taskDir, agentTool, model, personaName)
		},
	}

	cmd.Flags().StringVar(&personaName, "persona", "", "Persona template to use (dickson, eric, hardy, jenko, schmidt)")
	cmd.Flags().StringSliceVar(&repos, "repos", nil, "Repos this task touches (creates worktrees)")
	cmd.ValidArgsFunction = readyTaskCompletion
	cmd.RegisterFlagCompletionFunc("persona", personaCompletion)
	cmd.RegisterFlagCompletionFunc("repos", repoNameCompletion)
	return cmd
}

// writeTaskClaudeMD generates a CLAUDE.md for the task directory combining
// persona instructions, task context, memory, workspace layout, and scratchpad guide.
func writeTaskClaudeMD(taskDir, jeffHome string, task *gig.Task, personaName string, repos []string) error {
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
	sb.WriteString(fmt.Sprintf("- **ID:** %s\n", task.ID))
	sb.WriteString(fmt.Sprintf("- **Title:** %s\n", task.Title))
	if task.Description != "" {
		sb.WriteString(fmt.Sprintf("- **Description:** %s\n", task.Description))
	}
	sb.WriteString(fmt.Sprintf("- **Priority:** P%d\n", task.Priority))
	sb.WriteString(fmt.Sprintf("- **Type:** %s\n", task.Type))
	if task.ParentID != "" {
		sb.WriteString(fmt.Sprintf("- **Parent:** %s\n", task.ParentID))
	}
	if len(task.Labels) > 0 {
		sb.WriteString(fmt.Sprintf("- **Labels:** %s\n", strings.Join(task.Labels, ", ")))
	}
	sb.WriteString("\n")

	// Persona memory.
	if personaName != "" {
		content, err := memory.LoadPersonaMemory(jeffHome, personaName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: load persona memory: %v\n", err)
		}
		if content != "" {
			sb.WriteString("## Persona Memory\n\n")
			sb.WriteString(content)
			sb.WriteString("\n\n")
			sb.WriteString(fmt.Sprintf("Detail files: `%s`\n\n", memory.PersonaMemoryDir(jeffHome, personaName)))
		}
	}

	// Repo learnings.
	for _, repoName := range repos {
		content, err := memory.LoadRepoLearnings(jeffHome, repoName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: load repo learnings for %s: %v\n", repoName, err)
			continue
		}
		if content != "" {
			sb.WriteString(fmt.Sprintf("## Repo Learnings: %s\n\n", repoName))
			sb.WriteString(content)
			sb.WriteString("\n\n")
			sb.WriteString(fmt.Sprintf("Detail files: `%s`\n\n", memory.RepoLearningsDir(jeffHome, repoName)))
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

	// Scratchpad & memory guide.
	if personaName != "" || len(repos) > 0 {
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
	sb.WriteString(fmt.Sprintf("Append raw observations to: `%s`\n\n", memory.ScratchpadPath(taskDir)))
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
			sb.WriteString(fmt.Sprintf("\n**As %s, especially capture:** %s\n", personaName, hint))
		}
	}
	sb.WriteString("\n")

	sb.WriteString("### What does NOT belong\n")
	sb.WriteString("- Code structure or file paths (read the code instead)\n")
	sb.WriteString("- Anything already in CLAUDE.md or git history\n")
	sb.WriteString("- Task-specific implementation details\n\n")

	sb.WriteString("After the task, run `/learn` to curate scratchpad observations into persistent memory.\n")
}

// refreshTaskClaudeMD regenerates the task CLAUDE.md, preserving the persona
// that was used at pickup. Called after worktrees are added/removed.
func refreshTaskClaudeMD(taskDir string, store *gig.Store, taskID string) error {
	task, err := store.Get(taskID)
	if err != nil {
		return err
	}

	// Detect persona from existing CLAUDE.md (it's above the --- separator).
	personaName := detectPersona(taskDir)

	// Detect repos from existing worktree symlinks.
	repos := detectRepos(taskDir)

	return writeTaskClaudeMD(taskDir, cfg.Home, task, personaName, repos)
}

// detectRepos returns repo names from worktree symlinks in the task directory.
func detectRepos(taskDir string) []string {
	wts := listWorktreeSymlinks(taskDir)
	var repos []string
	for _, wt := range wts {
		repos = append(repos, wt.name)
	}
	return repos
}

// detectPersona reads the existing CLAUDE.md and returns the persona name
// if one was used, or "" if none.
func detectPersona(taskDir string) string {
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

// writeWorkspaceLayout appends the workspace directory layout to sb,
// showing symlinked worktrees and their branches.
func writeWorkspaceLayout(sb *strings.Builder, taskDir string) {
	worktrees := listWorktreeSymlinks(taskDir)
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
		sb.WriteString(connector + wt.name + "/")
		if wt.branch != "" {
			sb.WriteString("  (branch: " + wt.branch + ")")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("```\n\n")
}

type worktreeInfo struct {
	name   string // symlink name (repo name)
	branch string // git branch name, if detectable
}

// listWorktreeSymlinks scans taskDir for symlinks (which are worktrees).
func listWorktreeSymlinks(taskDir string) []worktreeInfo {
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		return nil
	}

	var result []worktreeInfo
	for _, e := range entries {
		fullPath := filepath.Join(taskDir, e.Name())
		if !gitutil.IsSymlink(fullPath) {
			continue
		}
		target, err := os.Readlink(fullPath)
		if err != nil {
			continue
		}
		// Branch is the last component of the worktree path
		// (jeff stores worktrees as worktrees/<repo>/<branch>).
		branch := filepath.Base(target)
		result = append(result, worktreeInfo{name: e.Name(), branch: branch})
	}
	return result
}

// buildTaskJSON marshals a task (with attrs) to JSON for branch naming scripts.
// Uses store.GetFull which populates the Attrs field — same output as gig show --json.
func buildTaskJSON(store *gig.Store, task *gig.Task) []byte {
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

// resolveRepoBranch determines the branch name for a repo.
// Uses the branch_name script if configured, otherwise falls back to defaultBranch.
func resolveRepoBranch(rc *jeff.RepoConfig, taskJSON []byte, defaultBranch string) (string, error) {
	if rc == nil || rc.BranchName == "" {
		return defaultBranch, nil
	}
	return workspace.ResolveBranchName(rc.BranchName, taskJSON, defaultBranch)
}
