package task

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NeerajG03/JEFF/workspace"
	"github.com/NeerajG03/gig"
)

func TestTeardown_HappyPath(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "Teardown happy")

	res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("Pickup: %v", err)
	}

	if err := Teardown(store, cfg, TeardownOpts{TaskID: task.ID, Reason: "done"}); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	// Task closed.
	got, _ := store.Get(task.ID)
	if got.Status != gig.StatusClosed {
		t.Errorf("expected closed, got %s", got.Status)
	}

	// Workspace RETIRED, not removed: `done` keeps the ~20 KB task dir because a
	// live session's cwd and hook scripts live in it (#94). The worktree — the
	// part that actually costs disk — is gone.
	if _, err := os.Stat(res.TaskDir); err != nil {
		t.Errorf("expected workspace kept, stat err: %v", err)
	}
	if !workspace.IsRetired(res.TaskDir) {
		t.Error("expected workspace to carry a .closed marker after teardown")
	}
	marker := workspace.ReadClosedMarker(res.TaskDir)
	if marker == nil || marker.TaskID != task.ID || marker.Reason != "done" {
		t.Errorf("closed marker = %+v, want task_id=%s reason=done", marker, task.ID)
	}
	if marker != nil && marker.ClosedAt.IsZero() {
		t.Error("closed marker has no timestamp; the GC grace period depends on it")
	}
}

func TestTeardown_MissingWorkspaceStillCloses(t *testing.T) {
	store, cfg, _ := fixture(t, false)
	task := newOpenTask(t, store, "Teardown no workspace")
	if _, err := store.Claim(task.ID, "jeff"); err != nil {
		t.Fatal(err)
	}

	// No workspace was ever built — teardown must still close the task.
	if err := Teardown(store, cfg, TeardownOpts{TaskID: task.ID, Reason: "done"}); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	got, _ := store.Get(task.ID)
	if got.Status != gig.StatusClosed {
		t.Errorf("expected closed, got %s", got.Status)
	}
}

// TestTeardown_KeepsHookScriptsAndSettings is the regression test for #94.
//
// `jeff done` used to RemoveAll the task workspace, which is where the invoking
// session's hook scripts and the settings.json naming them absolutely both live.
// The session's cwd died with it, so every subsequent hook and Bash spawn failed —
// the cwd as `ENOENT posix_spawn '/bin/sh'`, and then, once cwd was recovered, the
// scripts themselves as "No such file or directory". 27 hook errors in one observed
// session.
//
// Both losses are asserted here, because a fix that only preserves the cwd would
// still leave the scripts gone.
func TestTeardown_KeepsHookScriptsAndSettings(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "Teardown keeps session runtime")

	res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("Pickup: %v", err)
	}

	// Stand in the workspace, exactly as an agent session does. `jeff done` with
	// no args resolves its task from cwd, so this is the NORMAL invocation.
	t.Chdir(res.TaskDir)

	hookScript := filepath.Join(res.TaskDir, "hooks", "worker-heartbeat.sh")
	if err := os.MkdirAll(filepath.Dir(hookScript), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hookScript, []byte("#!/bin/bash\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(res.TaskDir, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{"hooks":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Teardown(store, cfg, TeardownOpts{TaskID: task.ID, Reason: "done"}); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	// 1. The session's cwd still exists.
	if _, err := os.Stat(res.TaskDir); err != nil {
		t.Errorf("task dir deleted under the session's cwd: %v", err)
	}
	// 2. The hook scripts the session is configured to run still exist. This is
	//    the half no chdir-based fix could ever address.
	if _, err := os.Stat(hookScript); err != nil {
		t.Errorf("hook script deleted — every hook in the session would fail: %v", err)
	}
	if _, err := os.Stat(settings); err != nil {
		t.Errorf("settings.json deleted: %v", err)
	}
	// 3. And the task is still closed — behavior preserved.
	if got, _ := store.Get(task.ID); got.Status != gig.StatusClosed {
		t.Errorf("status = %s, want closed", got.Status)
	}
}

// TestTeardown_PurgeRemovesWorkspace covers the explicit opt-in to the old
// behavior.
func TestTeardown_PurgeRemovesWorkspace(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "Teardown purge")

	res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("Pickup: %v", err)
	}

	if err := Teardown(store, cfg, TeardownOpts{TaskID: task.ID, Reason: "done", Purge: true}); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	if _, err := os.Stat(res.TaskDir); !os.IsNotExist(err) {
		t.Errorf("--purge should delete the workspace, stat err: %v", err)
	}
	if _, err := workspace.Open(cfg.Home, task.ID); err == nil {
		t.Error("expected workspace.Open to fail after --purge")
	}
}

// TestTeardown_DropsDanglingRepoSymlink: the worktree is reclaimed, so the symlink
// pointing at it would dangle. Retirement removes it rather than leaving a broken
// link for tooling and humans to trip over.
func TestTeardown_DropsDanglingRepoSymlink(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "Teardown dangling symlink")

	res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("Pickup: %v", err)
	}
	link := filepath.Join(res.TaskDir, repo)
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("expected a repo symlink at %s: %v", link, err)
	}

	if err := Teardown(store, cfg, TeardownOpts{TaskID: task.ID, Reason: "done"}); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("dangling symlink %s should have been removed, lstat err: %v", link, err)
	}
}

// TestTeardown_RemovesMidTaskSymlinkedWorktree is the main regression test for
// #98: a repo attached after pickup (as `jeff worktree add <repo> <branch>
// --task-dir .` does — worktree + symlink, historically WITHOUT registering the
// repo on the task) must still have its worktree removed by teardown. The
// branch deliberately does not carry the task id, so only the task-dir symlink
// can attribute it.
func TestTeardown_RemovesMidTaskSymlinkedWorktree(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "Teardown mid-task repo")

	res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("Pickup: %v", err)
	}
	registeredWT := filepath.Join(cfg.Home, "worktrees", repo, task.ID)
	if _, err := os.Stat(registeredWT); err != nil {
		t.Fatalf("precondition: pickup worktree missing: %v", err)
	}

	setupRepo(t, cfg.Home, "frontend")
	midTaskWT, err := workspace.WorktreeAdd(workspace.WorktreeOpts{
		JeffHome:   cfg.Home,
		RepoName:   "frontend",
		Branch:     "feature-x",
		BaseBranch: "main",
		TaskDir:    res.TaskDir,
	})
	if err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}

	if err := Teardown(store, cfg, TeardownOpts{TaskID: task.ID, Reason: "done"}); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	if _, err := os.Stat(registeredWT); !os.IsNotExist(err) {
		t.Errorf("registered repo's worktree should be removed, stat err: %v", err)
	}
	if _, err := os.Stat(midTaskWT); !os.IsNotExist(err) {
		t.Errorf("mid-task repo's worktree should be removed (#98), stat err: %v", err)
	}
}

// TestTeardown_RemovesUnlinkedTaskBranchWorktree covers the discovery half that
// symlinks can't: a worktree created WITHOUT --task-dir has no symlink in the
// task dir and no attribute entry, but its branch carries the task id, so
// teardown attributes it from disk (#98).
func TestTeardown_RemovesUnlinkedTaskBranchWorktree(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "Teardown unlinked worktree")

	if _, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}}); err != nil {
		t.Fatalf("Pickup: %v", err)
	}

	setupRepo(t, cfg.Home, "frontend")
	unlinkedWT, err := workspace.WorktreeAdd(workspace.WorktreeOpts{
		JeffHome:   cfg.Home,
		RepoName:   "frontend",
		Branch:     task.ID,
		BaseBranch: "main",
		// no TaskDir — nothing links this worktree to the task but its branch
	})
	if err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}

	if err := Teardown(store, cfg, TeardownOpts{TaskID: task.ID, Reason: "done"}); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	if _, err := os.Stat(unlinkedWT); !os.IsNotExist(err) {
		t.Errorf("unlinked task-branch worktree should be removed (#98), stat err: %v", err)
	}
}

// TestTeardown_DirtyMidTaskWorktreeRefusesClose: the dirty preflight must cover
// discovered worktrees too, or teardown would discard uncommitted mid-task work
// that the pre-#98 code at least left on disk.
func TestTeardown_DirtyMidTaskWorktreeRefusesClose(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "Teardown dirty mid-task repo")

	res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("Pickup: %v", err)
	}

	setupRepo(t, cfg.Home, "frontend")
	midTaskWT, err := workspace.WorktreeAdd(workspace.WorktreeOpts{
		JeffHome:   cfg.Home,
		RepoName:   "frontend",
		Branch:     "feature-x",
		BaseBranch: "main",
		TaskDir:    res.TaskDir,
	})
	if err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	if err := os.WriteFile(filepath.Join(midTaskWT, "wip.txt"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Teardown(store, cfg, TeardownOpts{TaskID: task.ID, Reason: "done"}); err == nil {
		t.Fatal("expected Teardown to refuse while a mid-task worktree is dirty")
	}

	if _, err := os.Stat(midTaskWT); err != nil {
		t.Errorf("dirty worktree must survive a refused teardown: %v", err)
	}
	if got, _ := store.Get(task.ID); got.Status == gig.StatusClosed {
		t.Error("task must not close when teardown refuses")
	}
}

// TestTeardown_LeavesOtherTasksWorktreesAlone: disk discovery is scoped to
// branches carrying THIS task's id — a sibling task's worktree (including a
// subtask's, whose id merely extends the parent's) must survive.
func TestTeardown_LeavesOtherTasksWorktreesAlone(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "Teardown scoping")
	other := newOpenTask(t, store, "Unrelated task")

	if _, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}}); err != nil {
		t.Fatalf("Pickup: %v", err)
	}

	otherWT, err := workspace.WorktreeAdd(workspace.WorktreeOpts{
		JeffHome:   cfg.Home,
		RepoName:   repo,
		Branch:     other.ID,
		BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	subtaskWT, err := workspace.WorktreeAdd(workspace.WorktreeOpts{
		JeffHome:   cfg.Home,
		RepoName:   repo,
		Branch:     task.ID + ".1",
		BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}

	if err := Teardown(store, cfg, TeardownOpts{TaskID: task.ID, Reason: "done"}); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	if _, err := os.Stat(otherWT); err != nil {
		t.Errorf("another task's worktree must survive this teardown: %v", err)
	}
	if _, err := os.Stat(subtaskWT); err != nil {
		t.Errorf("a subtask's worktree must survive its parent's teardown: %v", err)
	}
}

// TestTeardown_RepickupUnretiresWorkspace: a closed task can be reopened and
// picked up again, and workspace.Create reuses the existing directory. If the
// .closed marker survived that, the live workspace would be hidden from
// `jeff status` and hook sync — and `jeff cleanup` would eventually delete it out
// from under the session.
func TestTeardown_RepickupUnretiresWorkspace(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "Reopen after close")

	res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("Pickup: %v", err)
	}
	if err := Teardown(store, cfg, TeardownOpts{TaskID: task.ID, Reason: "done"}); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	if !workspace.IsRetired(res.TaskDir) {
		t.Fatal("precondition: workspace should be retired after done")
	}

	// Reopen and pick it up again.
	if err := store.UpdateStatus(task.ID, gig.StatusOpen, "test"); err != nil {
		t.Fatal(err)
	}
	again, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("second Pickup: %v", err)
	}

	if workspace.IsRetired(again.TaskDir) {
		t.Errorf("%s is still marked retired after re-pickup; it would be hidden from status and eventually collected", again.TaskDir)
	}
}
