package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/workspace"
	"github.com/spf13/cobra"
)

func doneCmd() *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   "done <gig-id>",
		Short: "Close a task and clean up its workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := args[0]

			store, err := openGigStore()
			if err != nil {
				return err
			}
			defer store.Close()

			// 1. Clean up worktrees for repos associated with this task.
			attr, err := store.GetAttr(taskID, jeff.AttrRepos)
			if err == nil && attr != nil {
				var repos []string
				if json.Unmarshal([]byte(attr.Value), &repos) == nil {
					for _, repoName := range repos {
						branch := taskID
						if err := workspace.WorktreeRemove(cfg.Home, repoName, branch); err != nil {
							fmt.Fprintf(os.Stderr, "Warning: worktree cleanup %s/%s: %v\n", repoName, branch, err)
						} else {
							fmt.Fprintf(os.Stderr, "Removed worktree %s/%s\n", repoName, branch)
						}
					}
				}
			}

			// 2. Remove task workspace directory.
			if err := workspace.Remove(cfg.Home, taskID); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: remove workspace: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "Removed workspace for %s\n", taskID)
			}

			// 3. Close the gig task.
			if err := store.CloseTask(taskID, reason, "jeff"); err != nil {
				return fmt.Errorf("close task: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Closed %s\n", taskID)

			return nil
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "done", "Close reason")
	return cmd
}
