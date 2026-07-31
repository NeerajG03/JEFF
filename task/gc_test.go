package task

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/crew"
	"github.com/NeerajG03/JEFF/workspace"
	"github.com/NeerajG03/gig"
)

// retireNow closes a task through the real teardown path and back-dates its
// retirement marker so the age gate can be exercised without sleeping.
func retireAged(t *testing.T, store Store, cfg *jeff.Config, taskID, dir string, age time.Duration) {
	t.Helper()
	if err := Teardown(store, cfg, TeardownOpts{TaskID: taskID, Reason: "done"}); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	m := workspace.ReadClosedMarker(dir)
	if m == nil {
		t.Fatalf("workspace %s not retired", dir)
	}
	backdated := workspace.ClosedMarker{TaskID: m.TaskID, Reason: m.Reason, ClosedAt: time.Now().UTC().Add(-age)}
	writeMarker(t, dir, backdated)
}

func writeMarker(t *testing.T, dir string, m workspace.ClosedMarker) {
	t.Helper()
	data := []byte(`{"task_id":"` + m.TaskID + `","reason":"` + m.Reason +
		`","closed_at":"` + m.ClosedAt.Format(time.RFC3339Nano) + `"}` + "\n")
	if err := os.WriteFile(filepath.Join(dir, workspace.ClosedMarkerName), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestGC_CollectsAgedRetiredWorkspace is the happy path: a workspace retired long
// enough ago, with no live worker, is collected and its bytes reported.
func TestGC_CollectsAgedRetiredWorkspace(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "GC collects")

	res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("Pickup: %v", err)
	}
	retireAged(t, store, cfg, task.ID, res.TaskDir, 48*time.Hour)

	gcRes, err := GC(store, cfg, GCOpts{})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}

	if len(gcRes.Workspaces) != 1 {
		t.Fatalf("collected %d workspaces, want 1 (skipped: live=%d open=%d tooNew=%d)",
			len(gcRes.Workspaces), len(gcRes.SkippedLive), len(gcRes.SkippedOpen), len(gcRes.SkippedTooNew))
	}
	if _, err := os.Stat(res.TaskDir); !os.IsNotExist(err) {
		t.Errorf("workspace still present after GC: %v", err)
	}
}

// TestGC_RespectsGracePeriod: a workspace retired moments ago is left alone. This
// is what protects an agent session the crew DB cannot see — anything not started
// by `jeff crew start` has no session row at all.
func TestGC_RespectsGracePeriod(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "GC grace")

	res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("Pickup: %v", err)
	}
	if err := Teardown(store, cfg, TeardownOpts{TaskID: task.ID, Reason: "done"}); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	gcRes, err := GC(store, cfg, GCOpts{}) // default 24h grace
	if err != nil {
		t.Fatalf("GC: %v", err)
	}

	if len(gcRes.Workspaces) != 0 {
		t.Errorf("collected %d workspaces inside the grace period, want 0", len(gcRes.Workspaces))
	}
	if len(gcRes.SkippedTooNew) != 1 {
		t.Errorf("SkippedTooNew = %d, want 1 (the skip must be reported, not silent)", len(gcRes.SkippedTooNew))
	}
	if _, err := os.Stat(res.TaskDir); err != nil {
		t.Errorf("workspace removed despite the grace period: %v", err)
	}
}

// TestGC_LeavesOpenTaskAlone: a live workspace whose task is still open is not
// touched, and is not even reported — it isn't garbage.
func TestGC_LeavesOpenTaskAlone(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "GC open task")

	res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("Pickup: %v", err)
	}

	gcRes, err := GC(store, cfg, GCOpts{MinAge: -1})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if len(gcRes.Workspaces) != 0 {
		t.Errorf("collected %d workspaces for an OPEN task, want 0", len(gcRes.Workspaces))
	}
	if _, err := os.Stat(res.TaskDir); err != nil {
		t.Errorf("live workspace removed: %v", err)
	}
}

// TestGC_ReopenedTaskIsNotCollected: retired, then reopened and picked up again.
// The workspace is live and must survive even with the age gate disabled — the
// marker is cleared by Create, so nothing marks it as garbage.
func TestGC_ReopenedTaskIsNotCollected(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "GC reopened")

	res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("Pickup: %v", err)
	}
	retireAged(t, store, cfg, task.ID, res.TaskDir, 48*time.Hour)

	if err := store.UpdateStatus(task.ID, gig.StatusOpen, "test"); err != nil {
		t.Fatal(err)
	}
	again, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("second Pickup: %v", err)
	}

	gcRes, err := GC(store, cfg, GCOpts{MinAge: -1})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if len(gcRes.Workspaces) != 0 {
		t.Errorf("collected a reopened, live workspace (%d) — the session would lose its hooks", len(gcRes.Workspaces))
	}
	if _, err := os.Stat(again.TaskDir); err != nil {
		t.Errorf("live workspace removed after re-pickup: %v", err)
	}
}

// TestGC_DryRunRemovesNothing: the report must be identical to a real pass, but the
// filesystem untouched.
func TestGC_DryRunRemovesNothing(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "GC dry run")

	res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("Pickup: %v", err)
	}
	retireAged(t, store, cfg, task.ID, res.TaskDir, 48*time.Hour)

	gcRes, err := GC(store, cfg, GCOpts{DryRun: true})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if len(gcRes.Workspaces) != 1 {
		t.Errorf("dry run reported %d workspaces, want 1", len(gcRes.Workspaces))
	}
	if _, err := os.Stat(res.TaskDir); err != nil {
		t.Errorf("dry run deleted %s: %v", res.TaskDir, err)
	}
}

// TestGC_UnreachableStoreDeletesNothing: an unreadable task must count as
// "not terminal", so a gig failure can never turn into a deletion.
func TestGC_UnreachableStoreDeletesNothing(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "GC store failure")

	res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("Pickup: %v", err)
	}
	retireAged(t, store, cfg, task.ID, res.TaskDir, 48*time.Hour)

	// nil store: every lookup fails.
	gcRes, err := GC(nil, cfg, GCOpts{MinAge: -1})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if len(gcRes.Workspaces) != 0 {
		t.Errorf("collected %d workspaces with an unusable store, want 0", len(gcRes.Workspaces))
	}
	if _, err := os.Stat(res.TaskDir); err != nil {
		t.Errorf("workspace deleted while the store was unusable: %v", err)
	}
}

// TestTaskIDForWorktree pins the attribution used to decide whether an orphaned
// worktree may be deleted. From review of #96: matching a gig id anywhere in the
// full path let a REPO name win over the branch, so a repo named "gig-app" could
// misattribute a worktree — and if that spurious id resolved to a closed task, a
// clean worktree belonging to an OPEN task would be deleted.
func TestTaskIDForWorktree(t *testing.T) {
	home := filepath.FromSlash("/home/.jeff")
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "plain branch",
			path: filepath.Join(home, "worktrees", "frontend", "gig-ab12-some-slug"),
			want: "gig-ab12",
		},
		{
			name: "branch with slashes",
			path: filepath.Join(home, "worktrees", "frontend", "jeff", "gig-ab12-some-slug"),
			want: "gig-ab12",
		},
		{
			name: "subtask id",
			path: filepath.Join(home, "worktrees", "backend", "gig-ab12.3-sub"),
			want: "gig-ab12.3",
		},
		{
			name: "repo name must NOT win over the branch",
			path: filepath.Join(home, "worktrees", "gig-app", "jeff", "gig-b222-real-task"),
			want: "gig-b222",
		},
		{
			name: "repo name alone is not an attribution",
			path: filepath.Join(home, "worktrees", "gig-app", "main"),
			want: "",
		},
		{
			name: "no branch portion",
			path: filepath.Join(home, "worktrees", "frontend"),
			want: "",
		},
		{
			name: "outside the worktrees root",
			path: filepath.FromSlash("/elsewhere/gig-ab12"),
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := taskIDForWorktree(home, tt.path); got != tt.want {
				t.Errorf("taskIDForWorktree(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestGC_SkipsWorkspaceWithLiveAnchoredWorker covers rule #1 of the GC — "never
// remove a workspace a live worker is anchored to" — which review of #96 found had
// zero coverage despite being the most safety-critical rule in the change.
//
// The workspace is retired and well past the grace period, so ONLY the anchor check
// stands between it and deletion. If the path keying between crew's stored TaskDir
// and workspace.List ever drifts, this fails.
func TestGC_SkipsWorkspaceWithLiveAnchoredWorker(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "GC live anchor")

	res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("Pickup: %v", err)
	}
	retireAged(t, store, cfg, task.ID, res.TaskDir, 72*time.Hour)

	// Register a running worker anchored to this workspace, as `jeff crew start` does.
	cs, err := crew.Open(cfg.Home)
	if err != nil {
		t.Fatalf("crew.Open: %v", err)
	}
	if err := cs.PutSession(&crew.Session{
		TaskID:      task.ID,
		TmuxSession: "jeff-test",
		WindowName:  "w",
		TaskDir:     res.TaskDir,
		Status:      "running",
		StartedAt:   time.Now().UTC(),
		LastSeen:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("PutSession: %v", err)
	}
	cs.Close()

	gcRes, err := GC(store, cfg, GCOpts{})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}

	if len(gcRes.Workspaces) != 0 {
		t.Errorf("collected %d workspace(s) with a LIVE worker anchored — that session loses its hooks and cwd", len(gcRes.Workspaces))
	}
	if len(gcRes.SkippedLive) != 1 {
		t.Errorf("SkippedLive = %d, want 1; the anchor check did not match. Path keying between crew.Session.TaskDir and workspace.List may have drifted", len(gcRes.SkippedLive))
	}
	if _, err := os.Stat(res.TaskDir); err != nil {
		t.Errorf("workspace with a live worker was deleted: %v", err)
	}
}

// TestGC_CollectsAfterWorkerStops: the same workspace becomes collectable once its
// worker reaches a terminal status, so the anchor check protects rather than leaks.
func TestGC_CollectsAfterWorkerStops(t *testing.T) {
	store, cfg, repo := fixture(t, true)
	task := newOpenTask(t, store, "GC anchor released")

	res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
	if err != nil {
		t.Fatalf("Pickup: %v", err)
	}
	retireAged(t, store, cfg, task.ID, res.TaskDir, 72*time.Hour)

	cs, err := crew.Open(cfg.Home)
	if err != nil {
		t.Fatalf("crew.Open: %v", err)
	}
	sess := &crew.Session{
		TaskID: task.ID, TmuxSession: "jeff-test", WindowName: "w",
		TaskDir: res.TaskDir, Status: "stopped",
		StartedAt: time.Now().UTC(), LastSeen: time.Now().UTC(),
	}
	if err := cs.PutSession(sess); err != nil {
		t.Fatalf("PutSession: %v", err)
	}
	cs.Close()

	gcRes, err := GC(store, cfg, GCOpts{})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if len(gcRes.Workspaces) != 1 {
		t.Errorf("collected %d workspaces after the worker stopped, want 1 (live=%d)", len(gcRes.Workspaces), len(gcRes.SkippedLive))
	}
}

// TestReactivate covers the paths that bring a retired workspace back to life
// (`jeff work`, `jeff crew resume`) and the one case that must NOT: a task that is
// still closed.
func TestReactivate(t *testing.T) {
	store, cfg, repo := fixture(t, true)

	t.Run("reopened task un-retires", func(t *testing.T) {
		task := newOpenTask(t, store, "Reactivate reopened")
		res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
		if err != nil {
			t.Fatal(err)
		}
		if err := Teardown(store, cfg, TeardownOpts{TaskID: task.ID, Reason: "done"}); err != nil {
			t.Fatal(err)
		}
		if err := store.UpdateStatus(task.ID, gig.StatusInProgress, "test"); err != nil {
			t.Fatal(err)
		}

		Reactivate(store, task.ID, res.TaskDir)

		if workspace.IsRetired(res.TaskDir) {
			t.Error("workspace still retired after Reactivate on a live task; cleanup would delete it under the session")
		}
	})

	t.Run("still-closed task keeps its marker", func(t *testing.T) {
		task := newOpenTask(t, store, "Reactivate closed")
		res, err := Pickup(store, cfg, PickupOpts{TaskID: task.ID, Repos: []string{repo}})
		if err != nil {
			t.Fatal(err)
		}
		if err := Teardown(store, cfg, TeardownOpts{TaskID: task.ID, Reason: "done"}); err != nil {
			t.Fatal(err)
		}

		Reactivate(store, task.ID, res.TaskDir)

		if !workspace.IsRetired(res.TaskDir) {
			t.Error("Reactivate resurrected a workspace whose task is still closed; it would never be collected")
		}
	})
}
