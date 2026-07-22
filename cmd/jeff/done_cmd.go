package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/crew"
	"github.com/NeerajG03/JEFF/workspace"
	"github.com/spf13/cobra"
)

func doneCmd() *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   "done [gig-id]",
		Short: "Close a task and clean up its workspace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, _, err := resolveTaskID(args)
			if err != nil {
				return err
			}

			store, err := openGigStore()
			if err != nil {
				return err
			}
			defer store.Close()

			// 0. Signal orchestrator before cleanup (best-effort).
			if cs, cerr := crew.Open(cfg.Home); cerr == nil {
				msg := fmt.Sprintf("completed — %s", reason)
				if err := crew.SignalOrchestrator(cs, taskID, msg); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: signal orchestrator: %v\n", err)
				}
				cs.Close()
			}

			// 1. Clean up worktrees for repos associated with this task.
			//    Resolve the actual worktree path from the task dir symlink
			//    (taskDir/<repoName> → real worktree path) instead of
			//    reconstructing from the task ID, which doesn't match the
			//    branch name generated during pickup.
			td, tdErr := workspace.Open(cfg.Home, taskID)
			attr, err := store.GetAttr(taskID, jeff.AttrRepos)
			if err == nil && attr != nil {
				var repos []string
				if json.Unmarshal([]byte(attr.Value), &repos) == nil {
					for _, repoName := range repos {
						var wtPath string
						if tdErr == nil {
							link := filepath.Join(td.Path, repoName)
							if target, lerr := os.Readlink(link); lerr == nil {
								wtPath = target
							}
						}
						if wtPath != "" {
							if err := workspace.WorktreeRemoveByPath(cfg.Home, repoName, wtPath); err != nil {
								fmt.Fprintf(os.Stderr, "Warning: worktree cleanup %s: %v\n", wtPath, err)
							} else {
								fmt.Fprintf(os.Stderr, "Removed worktree %s\n", wtPath)
							}
						} else {
							// Fallback: try the old branch=taskID approach.
							if err := workspace.WorktreeRemove(cfg.Home, repoName, taskID); err != nil {
								fmt.Fprintf(os.Stderr, "Warning: worktree cleanup %s/%s: %v\n", repoName, taskID, err)
							} else {
								fmt.Fprintf(os.Stderr, "Removed worktree %s/%s\n", repoName, taskID)
							}
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

			// 4. Run crew cleanup to reconcile tmux windows and worktrees.
			if cs, err := crew.Open(cfg.Home); err == nil {
				defer cs.Close()
				if result, err := crew.Cleanup(cs, cfg.Home, false); err == nil && !result.IsClean() {
					cleaned := len(result.OrphanedWindows) + len(result.StaleSessions) + len(result.StaleOrch)
					fmt.Fprintf(os.Stderr, "Crew cleanup: reconciled %d items\n", cleaned)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "done", "Close reason")
	cmd.ValidArgsFunction = activeTaskCompletion
	return cmd
}
