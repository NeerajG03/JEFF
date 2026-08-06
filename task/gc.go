// gc.go — collecting retired task workspaces and orphaned worktrees.
//
// Nothing ever reaped a task dir before this: workspace.Remove had exactly two
// callers, `jeff done` (which deleted the live session's own directory, #94) and a
// pickup rollback. With `done` now retiring workspaces instead of deleting them,
// something has to collect them, and that something should also pick up the
// expensive garbage `done` could not remove — worktrees left behind when a close
// refused on uncommitted changes or crashed part-way.
//
// The safety rules, in order of importance:
//
//  1. Never remove a workspace a live worker is anchored to.
//  2. Never discard uncommitted work. Dirty worktrees are reported and skipped
//     unless Force is set.
//  3. Never remove a workspace whose task is still open.
//  4. Respect a grace period. A plain agent session outside tmux has no crew row
//     at all, so rule 1 cannot see it; MinAge is what protects those.
package task

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/crew"
	"github.com/NeerajG03/JEFF/hooks"
	"github.com/NeerajG03/JEFF/workspace"
)

// DefaultGCMinAge is how long a retired workspace is left alone before it is
// collected. It exists for sessions the crew DB cannot see (anything started
// outside `jeff crew start`): keeping ~20 KB for a day is a trivial price for not
// pulling the rug out from under one.
const DefaultGCMinAge = 24 * time.Hour

// GCOpts configures a collection pass.
type GCOpts struct {
	DryRun bool
	// Force allows removing an orphaned worktree that has uncommitted changes.
	// Without it, dirty worktrees are only reported.
	Force bool
	// MinAge is the grace period for retired workspaces. Zero means
	// DefaultGCMinAge; use a negative value to disable the gate entirely.
	MinAge time.Duration
}

// GCItem is one thing collected, or one thing deliberately left alone.
type GCItem struct {
	Path   string
	TaskID string
	Bytes  int64
	// Reason is set only for skipped items, explaining why.
	Reason string
}

// GCResult reports a collection pass.
type GCResult struct {
	Workspaces       []GCItem // retired workspaces collected (or, in a dry run, would be)
	Worktrees        []GCItem // orphaned worktrees collected
	SkippedLive      []GCItem // a live worker is anchored here
	SkippedOpen      []GCItem // the task is not closed
	SkippedTooNew    []GCItem // inside the grace period
	SkippedDirty     []GCItem // orphaned worktree with uncommitted changes — needs Force
	BytesReclaimed   int64
	BytesRecoverable int64 // what the skipped-dirty worktrees would free
}

// taskIDPatternFor builds the pattern matching a gig task id under the store's
// configured prefix ("" means the gig default). It is applied ONLY to the
// branch portion of a worktree path (see taskIDForWorktree) — never the whole
// path. Built from the configured prefix rather than a package-level "gig-"
// literal: under a custom prefix the literal never matched, so orphaned
// worktrees were never attributed and never collected (#97).
func taskIDPatternFor(prefix string) *regexp.Regexp {
	if prefix == "" {
		prefix = workspace.DefaultTaskIDPrefix
	}
	return regexp.MustCompile(`(` + regexp.QuoteMeta(prefix) + `-[a-z0-9]+(?:\.[0-9]+)*)`)
}

// taskIDForWorktree recovers the task id a worktree belongs to, from the BRANCH
// portion of its path only. prefix is the store's configured task-ID prefix.
//
// Scoping matters for deletion safety. Worktrees live at
// worktrees/<repo>/<branch>/ and a branch may contain slashes, so the id can sit
// at any depth *within the branch* — but matching the whole path lets the repo
// name win: a repo legitimately named "gig-app" made
// worktrees/gig-app/jeff/gig-b222-real-task resolve to "gig-app". If that
// spurious id happened to be a closed task, the GC would delete a clean worktree
// belonging to a genuinely open one. Found in review of #96.
func taskIDForWorktree(jeffHome, wtPath, prefix string) string {
	rel, err := filepath.Rel(filepath.Join(jeffHome, "worktrees"), wtPath)
	if err != nil {
		return ""
	}
	// A path outside the worktrees root is not ours to attribute; Rel happily
	// returns a ../.. walk, which would otherwise still match an id.
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 2 {
		return "" // no branch portion at all — not a worktree we can attribute
	}
	branch := filepath.Join(parts[1:]...) // drop the <repo> segment
	return taskIDPatternFor(prefix).FindString(branch)
}

// storePrefix returns the store's configured task-ID prefix, defaulting when
// the store is unreachable — the same conservative stance as taskIsTerminal.
func storePrefix(store Store) string {
	if store == nil {
		return workspace.DefaultTaskIDPrefix
	}
	if p := store.Prefix(); p != "" {
		return p
	}
	return workspace.DefaultTaskIDPrefix
}

// Reactivate clears a workspace's retirement marker when its task is live again.
//
// Retirement is written by `jeff done`, but a task dir can be brought back to life
// by more than one path: `jeff pickup` (via workspace.Create), `jeff work`, and
// `jeff crew resume`. A marker left behind on a live workspace hides it from
// `jeff status` and from hook sync, and — worst — `jeff cleanup` eventually deletes
// it out from under the running session, which is the very failure #94 was about.
// Found in review of #96: only the Create path was wired.
//
// A workspace whose task is still terminal keeps its marker: poking at a closed
// task must not resurrect its directory into the live set.
func Reactivate(store Store, taskID, taskDir string) {
	if taskDir == "" || !workspace.IsRetired(taskDir) {
		return
	}
	if taskIsTerminal(store, taskID) {
		return
	}
	workspace.Unretire(taskDir)
}

// GC collects retired task workspaces and orphaned worktrees. It is safe to run
// at any time and reports everything it chose not to touch.
func GC(store Store, cfg *jeff.Config, opts GCOpts) (*GCResult, error) {
	minAge := opts.MinAge
	if minAge == 0 {
		minAge = DefaultGCMinAge
	}

	res := &GCResult{}
	anchored := anchoredTaskDirs(cfg.Home)
	prefix := storePrefix(store)

	// --- Retired task workspaces ---
	dirs, err := workspace.List(cfg.Home, prefix)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	for _, td := range dirs {
		marker := workspace.ReadClosedMarker(td.Path)
		terminal := taskIsTerminal(store, td.TaskID)

		// A workspace with no retirement marker whose task is still open is simply
		// live work — not this pass's business, and not worth reporting.
		if marker == nil && !terminal {
			continue
		}

		item := GCItem{Path: td.Path, TaskID: td.TaskID, Bytes: workspace.DirSize(td.Path)}

		// Retired but the task is open again (reopened after close): leave it, and
		// say why rather than silently skipping.
		if !terminal {
			item.Reason = "task is not closed"
			res.SkippedOpen = append(res.SkippedOpen, item)
			continue
		}
		if _, live := anchored[canonical(td.Path)]; live {
			item.Reason = "a live worker is anchored here"
			res.SkippedLive = append(res.SkippedLive, item)
			continue
		}
		if minAge > 0 {
			closedAt := retiredAt(marker, td.Path)
			if age := time.Since(closedAt); age < minAge {
				item.Reason = fmt.Sprintf("retired %s ago, grace period is %s", age.Round(time.Minute), minAge)
				res.SkippedTooNew = append(res.SkippedTooNew, item)
				continue
			}
		}

		if !opts.DryRun {
			hooks.UninstallAllFromDir(td.Path)
			if err := os.RemoveAll(td.Path); err != nil {
				item.Reason = fmt.Sprintf("remove failed: %v", err)
				res.SkippedOpen = append(res.SkippedOpen, item)
				continue
			}
		}
		res.Workspaces = append(res.Workspaces, item)
		res.BytesReclaimed += item.Bytes
	}

	// --- Orphaned worktrees (the expensive garbage) ---
	for _, wt := range allWorktrees(cfg.Home) {
		taskID := taskIDForWorktree(cfg.Home, wt, prefix)
		item := GCItem{Path: wt, TaskID: taskID, Bytes: workspace.DirSize(wt)}

		if taskID == "" || !taskIsTerminal(store, taskID) {
			continue // not identifiably finished work — leave it entirely alone
		}
		if dirty := workspace.DirtyPaths(wt, 5); len(dirty) > 0 && !opts.Force {
			item.Reason = "uncommitted changes: " + strings.Join(dirty, "; ")
			res.SkippedDirty = append(res.SkippedDirty, item)
			res.BytesRecoverable += item.Bytes
			continue
		}

		if !opts.DryRun {
			repoName := repoNameForWorktree(cfg.Home, wt)
			if err := workspace.WorktreeRemoveByPath(cfg.Home, repoName, wt, opts.Force); err != nil {
				item.Reason = fmt.Sprintf("remove failed: %v", err)
				res.SkippedDirty = append(res.SkippedDirty, item)
				continue
			}
		}
		res.Worktrees = append(res.Worktrees, item)
		res.BytesReclaimed += item.Bytes
	}

	return res, nil
}

// anchoredTaskDirs returns the task dirs of workers whose tmux pane is still
// alive, keyed by canonical path. A failure to open the crew DB is deliberately
// non-fatal but conservative: callers get an empty set, and the MinAge gate is
// then the only thing standing between a retired dir and collection.
func anchoredTaskDirs(jeffHome string) map[string]struct{} {
	out := map[string]struct{}{}
	cs, err := crew.Open(jeffHome)
	if err != nil {
		return out
	}
	defer cs.Close()

	sessions, err := cs.ListSessions(true, "")
	if err != nil {
		return out
	}
	for _, sess := range sessions {
		if sess.TaskDir == "" {
			continue
		}
		out[canonical(sess.TaskDir)] = struct{}{}
	}
	return out
}

// allWorktrees lists every worktree directory under jeffHome/worktrees. A
// worktree is <worktrees>/<repo>/<branch...>/ and branches may contain slashes, so
// a directory qualifies when it holds a .git entry. GC filters these down to
// orphans; teardown uses the same enumeration to discover worktrees whose
// branch carries the closing task's id (#98).
func allWorktrees(jeffHome string) []string {
	root := filepath.Join(jeffHome, "worktrees")
	var out []string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if _, err := os.Stat(filepath.Join(p, ".git")); err == nil {
			out = append(out, p)
			return filepath.SkipDir // don't descend into a checkout
		}
		return nil
	})
	return out
}

// repoNameForWorktree recovers the repo name from a worktree path: it is the
// first segment under jeffHome/worktrees.
func repoNameForWorktree(jeffHome, wtPath string) string {
	rel, err := filepath.Rel(filepath.Join(jeffHome, "worktrees"), wtPath)
	if err != nil {
		return ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// taskIsTerminal reports whether gig considers the task closed or cancelled. An
// unreadable task is treated as NOT terminal, so an unreachable store can never
// cause a deletion.
func taskIsTerminal(store Store, taskID string) bool {
	if store == nil || taskID == "" {
		return false
	}
	t, err := store.Get(taskID)
	if err != nil || t == nil {
		return false
	}
	return t.Status.IsTerminal()
}

// retiredAt returns when a workspace was retired: the marker's timestamp when
// usable, else the directory's mtime, so a dir with no (or a corrupt) marker still
// ages out instead of leaking forever.
func retiredAt(marker *workspace.ClosedMarker, path string) time.Time {
	if marker != nil && !marker.ClosedAt.IsZero() {
		return marker.ClosedAt
	}
	if info, err := os.Stat(path); err == nil {
		return info.ModTime()
	}
	return time.Time{}
}

// canonical resolves a path for comparison, matching workspace.PathContains'
// treatment of symlinks.
func canonical(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}
