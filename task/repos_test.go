package task

import (
	"testing"

	jeff "github.com/NeerajG03/JEFF"
)

// TestAddTaskRepo pins the registration half of #98: `jeff worktree add
// --task-dir` appends the repo to the task's repos attribute, so teardown and
// stats keep a single source of truth.
func TestAddTaskRepo(t *testing.T) {
	store, _, _ := fixture(t, false)
	task := newOpenTask(t, store, "add repo attr")

	attrValue := func() string {
		t.Helper()
		attr, err := store.GetAttr(task.ID, jeff.AttrRepos)
		if err != nil || attr == nil {
			t.Fatalf("GetAttr: attr=%v err=%v", attr, err)
		}
		return attr.Value
	}

	// First add creates the attribute.
	added, err := AddTaskRepo(store, task.ID, "backend")
	if err != nil {
		t.Fatalf("AddTaskRepo: %v", err)
	}
	if !added {
		t.Error("expected added=true on first registration")
	}
	if got := attrValue(); got != `["backend"]` {
		t.Errorf("attr = %s, want [\"backend\"]", got)
	}

	// A second repo appends, preserving order.
	if _, err := AddTaskRepo(store, task.ID, "frontend"); err != nil {
		t.Fatalf("AddTaskRepo: %v", err)
	}
	if got := attrValue(); got != `["backend","frontend"]` {
		t.Errorf("attr = %s, want [\"backend\",\"frontend\"]", got)
	}

	// Re-adding an existing repo is a no-op.
	added, err = AddTaskRepo(store, task.ID, "backend")
	if err != nil {
		t.Fatalf("AddTaskRepo duplicate: %v", err)
	}
	if added {
		t.Error("expected added=false for an already-registered repo")
	}
	if got := attrValue(); got != `["backend","frontend"]` {
		t.Errorf("attr changed on duplicate add: %s", got)
	}
}
