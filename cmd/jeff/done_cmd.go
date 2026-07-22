package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/crew"
	"github.com/NeerajG03/JEFF/workspace"
	"github.com/spf13/cobra"
)

// worktreeCleanup is a repo's resolved worktree path, ready for a dirty
// preflight check and (if clean, or --force) removal.
type worktreeCleanup struct {
	repoName string
	wtPath   string
}

// resolveWorktreeCleanups resolves the worktree path for each repo attached
// to the task. It prefers the task dir symlink target; if that's unavailable
// it reconstructs the legacy branch=taskID path (matching the old
// WorktreeRemove(jeffHome, repoName, taskID) behavior).
func resolveWorktreeCleanups(jeffHome, taskID string, repos []string, td *workspace.TaskDir, tdErr error) []worktreeCleanup {
	cleanups := make([]worktreeCleanup, 0, len(repos))
	for _, repoName := range repos {
		var wtPath string
		if tdErr == nil {
			link := filepath.Join(td.Path, repoName)
			if target, lerr := os.Readlink(link); lerr == nil {
				wtPath = target
			}
		}
		if wtPath == "" {
			wtPath = filepath.Join(jeffHome, "worktrees", repoName, taskID)
		}
		cleanups = append(cleanups, worktreeCleanup{repoName: repoName, wtPath: wtPath})
	}
	return cleanups
}

func doneCmd() *cobra.Command {
	var reason string
	var force bool

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

			// 0. Signal orchestrator before cleanup (best-effort). Harmless to
			// run before the dirty preflight.
			if cs, cerr := crew.Open(cfg.Home); cerr == nil {
				msg := fmt.Sprintf("completed — %s", reason)
				if err := crew.SignalOrchestrator(cs, taskID, msg); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: signal orchestrator: %v\n", err)
				}
				cs.Close()
			}

			// Resolve the worktree path for each repo associated with this
			// task from the task dir symlink (taskDir/<repoName> → real
			// worktree path) instead of reconstructing from the task ID,
			// which doesn't match the branch name generated during pickup.
			td, tdErr := workspace.Open(cfg.Home, taskID)
			var cleanups []worktreeCleanup
			attr, err := store.GetAttr(taskID, jeff.AttrRepos)
			if err == nil && attr != nil {
				var repos []string
				if json.Unmarshal([]byte(attr.Value), &repos) == nil {
					cleanups = resolveWorktreeCleanups(cfg.Home, taskID, repos, td, tdErr)
				}
			}

			// 1. Dirty preflight across ALL repos before removing any — a
			// mid-loop refuse would leave asymmetric state (some worktrees
			// gone, some not).
			if !force {
				var dirty []string
				for _, c := range cleanups {
					if paths := workspace.DirtyPaths(c.wtPath, 5); len(paths) > 0 {
						dirty = append(dirty, fmt.Sprintf("%s (%s):\n    %s", c.repoName, c.wtPath, strings.Join(paths, "\n    ")))
					}
				}
				if len(dirty) > 0 {
					return fmt.Errorf("refusing to close %s — uncommitted changes:\n  %s\n(commit/ship first, or pass --force to discard)", taskID, strings.Join(dirty, "\n  "))
				}
			}

			// 2. Remove worktrees now that the preflight has cleared (or --force).
			for _, c := range cleanups {
				if err := workspace.WorktreeRemoveByPath(cfg.Home, c.repoName, c.wtPath, force); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: worktree cleanup %s: %v\n", c.wtPath, err)
				} else {
					fmt.Fprintf(os.Stderr, "Removed worktree %s\n", c.wtPath)
				}
			}

			// 3. Remove task workspace directory.
			if err := workspace.Remove(cfg.Home, taskID); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: remove workspace: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "Removed workspace for %s\n", taskID)
			}

			// 4. Close the gig task.
			if err := store.SetAttr(taskID, jeff.AttrOutcome, reason); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: set outcome attr: %v\n", err)
			}
			if err := store.CloseTask(taskID, reason, "jeff"); err != nil {
				return fmt.Errorf("close task: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Closed %s\n", taskID)

			// 5. Run crew cleanup to reconcile tmux windows and worktrees.
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
	cmd.Flags().BoolVar(&force, "force", false, "Discard uncommitted worktree changes instead of refusing to close")
	cmd.ValidArgsFunction = activeTaskCompletion
	return cmd
}
