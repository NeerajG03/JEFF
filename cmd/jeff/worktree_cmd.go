package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/NeerajG03/JEFF/task"
	"github.com/NeerajG03/JEFF/workspace"
	"github.com/spf13/cobra"
)

func worktreeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worktree",
		Short: "Manage git worktrees",
	}
	cmd.AddCommand(worktreeAddCmd(), worktreeRmCmd(), worktreeListCmd())
	return cmd
}

func worktreeAddCmd() *cobra.Command {
	var taskDir, baseBranch string

	cmd := &cobra.Command{
		Use:   "add <repo> <branch>",
		Short: "Create a git worktree and optionally symlink into task dir",
		Args:  cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return repoNameCompletion(cmd, args, toComplete)
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			repoName, branch := args[0], args[1]

			opts := workspace.WorktreeOpts{
				JeffHome:   cfg.Home,
				RepoName:   repoName,
				Branch:     branch,
				BaseBranch: baseBranch,
				TaskDir:    taskDir,
			}
			// Fill from repo config if not overridden by flags.
			if rc, ok := cfg.Repos[repoName]; ok {
				if opts.BaseBranch == "" {
					opts.BaseBranch = rc.BaseBranch
				}
				opts.PostSetup = rc.PostSetup
			}

			wtDir, err := workspace.WorktreeAdd(opts)
			if err != nil {
				return err
			}
			fmt.Printf("Worktree created: %s\n", wtDir)
			if taskDir != "" {
				fmt.Printf("Symlinked into: %s/%s\n", taskDir, repoName)

				// Refresh task CLAUDE.md so it reflects the new worktree.
				// ExtractTaskID reads the id off the directory's BASE name, so a
				// relative --task-dir (notably `.`, run from inside the task dir)
				// has to be resolved first: Base(".") is "." and ExtractTaskID
				// falls through to returning it verbatim, which then fails the
				// lookup with "task not found" and skips the refresh entirely.
				idSource := taskDir
				if abs, err := filepath.Abs(taskDir); err == nil {
					idSource = abs
				}
				taskID := workspace.ExtractTaskID(idSource, gigTaskPrefix(cfg))
				if taskID != "" {
					store, err := openGigStore(cfg)
					if err == nil {
						defer store.Close()
						// Register the repo on the task, so `jeff done` cleans this
						// worktree up and task stats count the repo (#98).
						if added, err := task.AddTaskRepo(store, taskID, repoName); err != nil {
							fmt.Fprintf(os.Stderr, "Warning: register repo on %s: %v\n", taskID, err)
						} else if added {
							fmt.Printf("Registered %s on %s\n", repoName, taskID)
						}
						if err := task.RefreshClaudeMD(store, cfg, taskID, taskDir); err != nil {
							fmt.Fprintf(os.Stderr, "Warning: refresh CLAUDE.md: %v\n", err)
						}
					}
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&taskDir, "task-dir", "", "Symlink worktree into this task directory")
	cmd.Flags().StringVar(&baseBranch, "base", "", "Base branch to branch from (default: repo config or origin/main)")
	return cmd
}

func worktreeRmCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "rm <repo> <branch>",
		Short: "Remove a git worktree",
		Args:  cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return repoNameCompletion(cmd, args, toComplete)
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := workspace.WorktreeRemove(cfg.Home, args[0], args[1], force); err != nil {
				return err
			}
			fmt.Printf("Removed worktree %s/%s\n", args[0], args[1])
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Remove even if the worktree has uncommitted changes")
	return cmd
}

func worktreeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "list <repo>",
		Short:             "List worktrees for a repo",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: repoNameCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			branches, err := workspace.WorktreeList(cfg.Home, args[0])
			if err != nil {
				return err
			}
			if len(branches) == 0 {
				fmt.Printf("No worktrees for %s\n", args[0])
				return nil
			}
			for _, b := range branches {
				fmt.Println(b)
			}
			return nil
		},
	}
}
