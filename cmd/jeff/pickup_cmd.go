package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/hooks"
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

			// 1. Open gig store and claim the task.
			store, err := openGigStore()
			if err != nil {
				return err
			}
			defer store.Close()

			// Ensure JEFF's custom attrs exist.
			if err := jeff.EnsureAttrs(store); err != nil {
				return fmt.Errorf("ensure attrs: %w", err)
			}

			task, err := store.Get(taskID)
			if err != nil {
				return fmt.Errorf("task %s not found: %w", taskID, err)
			}

			claimResult, err := store.Claim(taskID, "jeff")
			if err != nil {
				return fmt.Errorf("claim: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Claimed %s: %s\n", taskID, task.Title)
			if claimResult.ParentProgressed {
				fmt.Fprintf(os.Stderr, "Parent %s → in_progress\n", claimResult.ParentID)
			}

			// 2. Create task workspace.
			td, err := workspace.Create(cfg.Home, taskID, task.Title)
			if err != nil {
				return fmt.Errorf("create workspace: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Workspace: %s\n", td.Path)

			// 3. Set repos attribute if specified.
			if len(repos) > 0 {
				reposJSON, _ := json.Marshal(repos)
				if err := store.SetAttr(taskID, jeff.AttrRepos, string(reposJSON)); err != nil {
					return fmt.Errorf("set repos attr: %w", err)
				}
			}

			// 4. Create worktrees for specified repos.
			taskJSON := buildTaskJSON(store, task)
			for _, repoName := range repos {
				rc := cfg.Repos[repoName]
				branch, err := resolveRepoBranch(rc, taskJSON, taskID)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: branch name for %s: %v, using %s\n", repoName, err, taskID)
					branch = taskID
				}

				opts := workspace.WorktreeOpts{
					JeffHome: cfg.Home,
					RepoName: repoName,
					Branch:   branch,
					TaskDir:  td.Path,
				}
				if rc != nil {
					opts.BaseBranch = rc.BaseBranch
					opts.PostSetup = rc.PostSetup
				}

				wtDir, err := workspace.WorktreeAdd(opts)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: worktree for %s: %v\n", repoName, err)
					continue
				}
				fmt.Fprintf(os.Stderr, "Worktree: %s → %s\n", repoName, wtDir)
			}

			// 5. Generate CLAUDE.md in task directory.
			if err := writeTaskClaudeMD(td.Path, task, personaName); err != nil {
				return fmt.Errorf("write task CLAUDE.md: %w", err)
			}

			// 6. Install task-level hooks (if any).
			reg := hooks.DefaultRegistry()
			mgr := hooks.NewManager(reg)
			hctx := hooks.HookContext{JeffHome: cfg.Home, TargetDir: td.Path, GigHome: cfg.GigHome}
			taskEnabled := hooks.EnabledForSource(cfg.Hooks, hooks.SourceTask, reg)
			if len(taskEnabled) > 0 {
				agent := hooks.AgentTool(cfg.Agent)
				if err := mgr.Sync(td.Path, taskEnabled, agent, hctx); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: task hooks: %v\n", err)
				}
			}

			// 7. Launch agent tool in task directory.
			fmt.Fprintf(os.Stderr, "\nLaunching %s in %s...\n", cfg.Agent, td.Path)
			return launchAgent(td.Path, cfg.Agent)
		},
	}

	cmd.Flags().StringVar(&personaName, "persona", "", "Persona template to use (captain, nerd, jock, scout)")
	cmd.Flags().StringSliceVar(&repos, "repos", nil, "Repos this task touches (creates worktrees)")
	cmd.RegisterFlagCompletionFunc("persona", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return persona.Names(), cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

// writeTaskClaudeMD generates a CLAUDE.md for the task directory combining
// persona instructions, task context, and workspace layout.
func writeTaskClaudeMD(taskDir string, task *gig.Task, personaName string) error {
	var sb strings.Builder

	// Persona section.
	if personaName != "" {
		content, err := persona.Get(personaName)
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

	// Workspace layout with worktrees.
	writeWorkspaceLayout(&sb, taskDir)

	path := filepath.Join(taskDir, "CLAUDE.md")
	return os.WriteFile(path, []byte(sb.String()), 0o644)
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

	return writeTaskClaudeMD(taskDir, task, personaName)
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
		fi, err := os.Lstat(fullPath)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
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

// resolveRepoBranch determines the branch name for a repo.
// Uses the branch_name script if configured, otherwise falls back to defaultBranch.
func resolveRepoBranch(rc *jeff.RepoConfig, taskJSON []byte, defaultBranch string) (string, error) {
	if rc == nil || rc.BranchName == "" {
		return defaultBranch, nil
	}
	return workspace.ResolveBranchName(rc.BranchName, taskJSON, defaultBranch)
}
