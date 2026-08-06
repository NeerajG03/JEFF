package task

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/crew"
	"github.com/NeerajG03/JEFF/hooks"
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
	// discovered marks a worktree found by inspection (task-dir symlink or
	// on-disk branch attribution) rather than the task's repos attribute — a
	// repo attached mid-task that was never registered (#98).
	discovered bool
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
	var repos []string
	if attr, err := store.GetAttr(opts.TaskID, jeff.AttrRepos); err == nil && attr != nil {
		_ = json.Unmarshal([]byte(attr.Value), &repos)
	}
	// Resolve cleanups even when the attribute is empty or unreadable — repos
	// attached mid-task may exist only as symlinks or on-disk worktrees (#98).
	cleanups := resolveWorktreeCleanups(cfg.Home, opts.TaskID, storePrefix(store), repos, td, tdErr)

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
		note := ""
		if c.discovered {
			note = " (repo was not registered on the task — attached mid-task?)"
		}
		fmt.Fprintf(os.Stderr, "Removed worktree %s%s\n", c.wtPath, note)
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
		hooks.UninstallAllFromDir(td.Path)
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
// the task. Repos come from three sources, unioned (#98):
//
//  1. The task's repos attribute — written at pickup, extended by
//     `jeff worktree add --task-dir`. Path preference: the task-dir symlink
//     target (via ListTaskWorktrees), falling back to the legacy branch=taskID
//     path (matching the old WorktreeRemove(jeffHome, repoName, taskID)
//     behavior).
//  2. Task-dir symlinks with no attribute entry — repos attached mid-task by
//     binaries that predate registration on add.
//  3. On-disk worktrees whose branch carries the task id but were never
//     symlinked in (`jeff worktree add` without --task-dir).
//
// Before #98 only the attribute was consulted, so a repo added mid-task closed
// with the task but silently kept its worktree (and branch) on disk.
func resolveWorktreeCleanups(jeffHome, taskID, prefix string, repos []string, td *workspace.TaskDir, tdErr error) []worktreeCleanup {
	// Build repo → resolved worktree path from the live symlinks.
	linked := map[string]string{}
	if tdErr == nil {
		if wts, err := workspace.ListTaskWorktrees(td.Path); err == nil {
			for _, wt := range wts {
				linked[wt.Repo] = wt.Path
			}
		}
	}

	var cleanups []worktreeCleanup
	seen := map[string]bool{} // canonical worktree path → already queued
	add := func(repoName, wtPath string, discovered bool) {
		key := canonical(wtPath)
		if seen[key] {
			return
		}
		seen[key] = true
		cleanups = append(cleanups, worktreeCleanup{repoName: repoName, wtPath: wtPath, discovered: discovered})
	}

	// 1. Repos registered on the task.
	inAttr := map[string]bool{}
	for _, repoName := range repos {
		inAttr[repoName] = true
		wtPath := linked[repoName]
		if wtPath == "" {
			wtPath = filepath.Join(jeffHome, "worktrees", repoName, taskID)
		}
		add(repoName, wtPath, false)
	}

	// 2. Repos symlinked into the task dir but missing from the attribute.
	linkedNames := make([]string, 0, len(linked))
	for repoName := range linked {
		linkedNames = append(linkedNames, repoName)
	}
	sort.Strings(linkedNames) // deterministic cleanup (and output) order
	for _, repoName := range linkedNames {
		if !inAttr[repoName] {
			add(repoName, linked[repoName], true)
		}
	}

	// 3. On-disk worktrees whose branch carries this task's id. Attribution is
	// exact (taskIDForWorktree scopes the match to the branch portion), so a
	// subtask's worktree (gig-ab12.1) never rides along with its parent's
	// teardown (gig-ab12).
	for _, wt := range allWorktrees(jeffHome) {
		if taskIDForWorktree(jeffHome, wt, prefix) == taskID {
			add(repoNameForWorktree(jeffHome, wt), wt, true)
		}
	}
	return cleanups
}
