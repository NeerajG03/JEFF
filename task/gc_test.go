package task

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	jeff "github.com/NeerajG03/JEFF"
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
