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
	// Purge deletes the task workspace directory outright instead of retiring
	// it. Off by default: the invoking session is usually anchored to that
	// directory and its hook scripts live inside it, so deleting it breaks every
	// subsequent hook and Bash spawn in that session (#94).
	Purge bool
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

	// 2. Remove worktrees now that the preflight has cleared (or --force). This
	// is the actual reclamation — a checkout is hundreds of MB.
	for _, c := range cleanups {
		// Note if the caller is standing inside this worktree. Unlike the task
		// dir, this one genuinely has to go, so the honest thing is to say so and
		// point at what survives: the branch is untouched by `git worktree
		// remove`, so committed work is still in the repo.
		insideThis := workspace.CwdInside(c.wtPath)
		if err := workspace.WorktreeRemoveByPath(cfg.Home, c.repoName, c.wtPath, opts.Force); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: worktree cleanup %s: %v\n", c.wtPath, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "Removed worktree %s\n", c.wtPath)
		if insideThis {
			fmt.Fprintf(os.Stderr, "  You were inside it — run `cd %s`. Commits are safe on the branch:\n", cfg.Home)
			fmt.Fprintf(os.Stderr, "    jeff worktree add %s <branch>   # to get the checkout back\n", c.repoName)
		}
	}

	// 3. Retire the task workspace — do NOT delete it by default.
	//
	// The worktrees removed above are where the disk cost lives (hundreds of MB
	// each); a task dir is ~20 KB of hook scripts, settings.json and symlinks.
	// It is also the running session's life support: its cwd, the hook scripts
	// themselves, and the settings.json naming them absolutely. Deleting it to
	// reclaim 20 KB broke every hook in the invoking session (#94), so it is
	// retired instead and collected later by `jeff cleanup`.
	retireWorkspace(cfg, td, tdErr, opts)

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

// retireWorkspace marks the task workspace closed (default) or deletes it
// (--purge). Deleting is the old behavior and is kept only as an explicit opt-in;
// it warns first when the caller is standing inside the directory, since that is
// the case that breaks the session.
func retireWorkspace(cfg *jeff.Config, td *workspace.TaskDir, tdErr error, opts TeardownOpts) {
	if tdErr != nil || td == nil {
		// No workspace resolved (already gone, or never created) — nothing to do.
		return
	}

	if opts.Purge {
		if workspace.CwdInside(td.Path) {
			fmt.Fprintf(os.Stderr, "Warning: --purge is deleting %s, which you are currently inside.\n", td.Path)
			fmt.Fprintf(os.Stderr, "  Hooks and shell commands in this session will fail until you: cd %s\n", cfg.Home)
		}
		if err := workspace.Remove(cfg.Home, opts.TaskID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: remove workspace: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Purged workspace for %s\n", opts.TaskID)
		}
		return
	}

	res, err := workspace.Retire(td.Path, opts.TaskID, opts.Reason)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: retire workspace: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "Retired workspace %s (kept — `jeff cleanup` collects it)\n", td.Path)
	if len(res.DanglingRemoved) > 0 {
		fmt.Fprintf(os.Stderr, "  removed dangling symlink(s): %s\n", strings.Join(res.DanglingRemoved, ", "))
	}
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
