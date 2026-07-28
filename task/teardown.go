package task

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/crew"
	"github.com/NeerajG03/JEFF/workspace"
)

// TeardownOpts configures a task teardown.
type TeardownOpts struct {
	TaskID string
	Reason string
	Force  bool // discard uncommitted worktree changes instead of refusing
}

// worktreeCleanup is a repo's resolved worktree path, ready for a dirty
// preflight check and (if clean, or --force) removal.
type worktreeCleanup struct {
	repoName string
	wtPath   string
}

// Teardown closes a task and cleans up its workspace, mirroring Pickup. It
// preserves the best-effort semantics of `jeff done`: every step warns and
// continues except CloseTask, which is the only hard failure. When Force is
// false it refuses (returns an error) if any worktree has uncommitted changes,
// before removing anything, so a mid-loop refuse can't leave asymmetric state.
func Teardown(store Store, cfg *jeff.Config, opts TeardownOpts) error {
	// 0. Signal orchestrator before cleanup (best-effort).
	if cs, cerr := crew.Open(cfg.Home); cerr == nil {
		msg := fmt.Sprintf("completed — %s", opts.Reason)
		if err := crew.SignalOrchestrator(cs, opts.TaskID, msg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: signal orchestrator: %v\n", err)
		}
		cs.Close()
	}

	// Resolve the worktree path for each repo associated with this task from
	// the task dir symlink instead of reconstructing from the task ID, which
	// doesn't match the branch name generated during pickup.
	td, tdErr := workspace.Open(cfg.Home, opts.TaskID)
	var cleanups []worktreeCleanup
	if attr, err := store.GetAttr(opts.TaskID, jeff.AttrRepos); err == nil && attr != nil {
		var repos []string
		if json.Unmarshal([]byte(attr.Value), &repos) == nil {
			cleanups = resolveWorktreeCleanups(cfg.Home, opts.TaskID, repos, td, tdErr)
		}
	}

	// 1. Dirty preflight across ALL repos before removing any.
	if !opts.Force {
		var dirty []string
		for _, c := range cleanups {
			if paths := workspace.DirtyPaths(c.wtPath, 5); len(paths) > 0 {
				dirty = append(dirty, fmt.Sprintf("%s (%s):\n    %s", c.repoName, c.wtPath, strings.Join(paths, "\n    ")))
			}
		}
		if len(dirty) > 0 {
			return fmt.Errorf("refusing to close %s — uncommitted changes:\n  %s\n(commit/ship first, or pass --force to discard)", opts.TaskID, strings.Join(dirty, "\n  "))
		}
	}

	// 2. Remove worktrees now that the preflight has cleared (or --force).
	for _, c := range cleanups {
		if err := workspace.WorktreeRemoveByPath(cfg.Home, c.repoName, c.wtPath, opts.Force); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: worktree cleanup %s: %v\n", c.wtPath, err)
		} else {
			fmt.Fprintf(os.Stderr, "Removed worktree %s\n", c.wtPath)
		}
	}

	// 3. Remove task workspace directory.
	if err := workspace.Remove(cfg.Home, opts.TaskID); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: remove workspace: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "Removed workspace for %s\n", opts.TaskID)
	}

	// 4. Close the gig task (the only hard-fail step).
	if err := store.SetAttr(opts.TaskID, jeff.AttrOutcome, opts.Reason); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: set outcome attr: %v\n", err)
	}
	if err := store.CloseTask(opts.TaskID, opts.Reason, "jeff"); err != nil {
		return fmt.Errorf("close task: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Closed %s\n", opts.TaskID)

	// 5. Run crew cleanup to reconcile tmux windows and worktrees. Cleanup
	// only ever kills windows whose pane is dead, so this cannot take down
	// other live workers even if this process sees a diverged DB view.
	if cs, err := crew.Open(cfg.Home); err == nil {
		defer cs.Close()
		if result, err := crew.Cleanup(cs, cfg.Home, false); err == nil {
			cleaned := len(result.OrphanedWindows) + len(result.StaleSessions) + len(result.StaleOrch)
			if cleaned > 0 {
				fmt.Fprintf(os.Stderr, "Crew cleanup: reconciled %d items\n", cleaned)
			}
		}
	}

	return nil
}

// resolveWorktreeCleanups resolves the worktree path for each repo attached to
// the task. It prefers the task dir symlink target (via ListTaskWorktrees); if
// that's unavailable it reconstructs the legacy branch=taskID path (matching the
// old WorktreeRemove(jeffHome, repoName, taskID) behavior).
func resolveWorktreeCleanups(jeffHome, taskID string, repos []string, td *workspace.TaskDir, tdErr error) []worktreeCleanup {
	// Build repo → resolved worktree path from the live symlinks.
	linked := map[string]string{}
	if tdErr == nil {
		if wts, err := workspace.ListTaskWorktrees(td.Path); err == nil {
			for _, wt := range wts {
				linked[wt.Repo] = wt.Path
			}
		}
	}

	cleanups := make([]worktreeCleanup, 0, len(repos))
	for _, repoName := range repos {
		wtPath := linked[repoName]
		if wtPath == "" {
			wtPath = filepath.Join(jeffHome, "worktrees", repoName, taskID)
		}
		cleanups = append(cleanups, worktreeCleanup{repoName: repoName, wtPath: wtPath})
	}
	return cleanups
}
