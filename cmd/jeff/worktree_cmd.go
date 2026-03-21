package main

import (
	"fmt"
	"os"

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
	var taskDir string

	cmd := &cobra.Command{
		Use:   "add <repo> <branch>",
		Short: "Create a git worktree and optionally symlink into task dir",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Look up post-setup script from repo config.
			var postSetup string
			if rc, ok := cfg.Repos[args[0]]; ok && rc.PostSetup != "" {
				postSetup = rc.PostSetup
			}
			wtDir, err := workspace.WorktreeAdd(cfg.Home, args[0], args[1], taskDir, postSetup)
			if err != nil {
				return err
			}
			fmt.Printf("Worktree created: %s\n", wtDir)
			if taskDir != "" {
				fmt.Printf("Symlinked into: %s/%s\n", taskDir, args[0])

				// Refresh task CLAUDE.md so it reflects the new worktree.
				taskID := workspace.ExtractTaskID(taskDir)
				if taskID != "" {
					store, err := openGigStore()
					if err == nil {
						defer store.Close()
						if err := refreshTaskClaudeMD(taskDir, store, taskID); err != nil {
							fmt.Fprintf(os.Stderr, "Warning: refresh CLAUDE.md: %v\n", err)
						}
					}
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&taskDir, "task-dir", "", "Symlink worktree into this task directory")
	return cmd
}

func worktreeRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <repo> <branch>",
		Short: "Remove a git worktree",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := workspace.WorktreeRemove(cfg.Home, args[0], args[1]); err != nil {
				return err
			}
			fmt.Printf("Removed worktree %s/%s\n", args[0], args[1])
			return nil
		},
	}
}

func worktreeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <repo>",
		Short: "List worktrees for a repo",
		Args:  cobra.ExactArgs(1),
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
