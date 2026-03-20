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

			if err := store.Claim(taskID, "jeff"); err != nil {
				return fmt.Errorf("claim: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Claimed %s: %s\n", taskID, task.Title)

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
			for _, repoName := range repos {
				branch := taskID // use task ID as branch name
				var postSetup string
				if rc, ok := cfg.Repos[repoName]; ok && rc.PostSetup != "" {
					postSetup = rc.PostSetup
				}
				wtDir, err := workspace.WorktreeAdd(cfg.Home, repoName, branch, td.Path, postSetup)
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

	cmd.Flags().StringVar(&personaName, "persona", "jock", "Persona template to use (captain, nerd, jock, scout)")
	cmd.Flags().StringSliceVar(&repos, "repos", nil, "Repos this task touches (creates worktrees)")
	cmd.RegisterFlagCompletionFunc("persona", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return persona.Names(), cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

// writeTaskClaudeMD generates a CLAUDE.md for the task directory combining
// persona instructions, task context, and JEFF/gig CLI reference.
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
	sb.WriteString(fmt.Sprintf("**ID:** %s\n", task.ID))
	sb.WriteString(fmt.Sprintf("**Title:** %s\n", task.Title))
	if task.Description != "" {
		sb.WriteString(fmt.Sprintf("**Description:** %s\n", task.Description))
	}
	sb.WriteString(fmt.Sprintf("**Priority:** P%d\n", task.Priority))
	if task.ParentID != "" {
		sb.WriteString(fmt.Sprintf("**Parent:** %s\n", task.ParentID))
	}
	sb.WriteString("\n")

	path := filepath.Join(taskDir, "CLAUDE.md")
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}
