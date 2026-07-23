package task

import (
	"os"
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

	// Workspace removed.
	if _, err := os.Stat(res.TaskDir); !os.IsNotExist(err) {
		t.Errorf("expected workspace removed, stat err: %v", err)
	}
	if _, err := workspace.Open(cfg.Home, task.ID); err == nil {
		t.Error("expected workspace.Open to fail after teardown")
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
