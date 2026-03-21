package workspace

import (
	"os"
	"testing"

	"github.com/NeerajG03/JEFF/internal/testutil"
)

func TestCreateAndOpen(t *testing.T) {
	home := testutil.TempHome(t, "tasks")

	td, err := Create(home, "gig-ab12", "Refactor auth module")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if td.TaskID != "gig-ab12" {
		t.Errorf("expected task ID gig-ab12, got %s", td.TaskID)
	}
	if td.Slug != "gig-ab12-refactor-auth-module" {
		t.Errorf("unexpected slug: %s", td.Slug)
	}

	// Verify directory exists.
	if _, err := os.Stat(td.Path); err != nil {
		t.Errorf("task dir should exist: %v", err)
	}

	// Open should find it.
	opened, err := Open(home, "gig-ab12")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if opened.Path != td.Path {
		t.Errorf("expected %s, got %s", td.Path, opened.Path)
	}
}

func TestOpenNotFound(t *testing.T) {
	home := testutil.TempHome(t, "tasks")
	_, err := Open(home, "gig-xxxx")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestRemove(t *testing.T) {
	home := testutil.TempHome(t, "tasks")
	Create(home, "gig-ab12", "Some task")

	err := Remove(home, "gig-ab12")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}

	_, err = Open(home, "gig-ab12")
	if err == nil {
		t.Error("task dir should be gone after remove")
	}
}

func TestList(t *testing.T) {
	home := testutil.TempHome(t, "tasks")
	Create(home, "gig-ab12", "Task one")
	Create(home, "gig-cd34", "Task two")

	dirs, err := List(home)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(dirs) != 2 {
		t.Fatalf("expected 2 task dirs, got %d", len(dirs))
	}
}

func TestListEmpty(t *testing.T) {
	home := testutil.TempHome(t, "tasks")
	dirs, err := List(home)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(dirs) != 0 {
		t.Errorf("expected 0 task dirs, got %d", len(dirs))
	}
}

func TestMakeSlug(t *testing.T) {
	tests := []struct {
		id, title, want string
	}{
		{"gig-ab12", "Refactor auth", "gig-ab12-refactor-auth"},
		{"gig-ab12", "", "gig-ab12"},
		{"gig-ab12", "Fix bug #123!", "gig-ab12-fix-bug-123"},
		{"gig-ab12.1", "Sub task", "gig-ab12.1-sub-task"},
	}
	for _, tt := range tests {
		got := makeSlug(tt.id, tt.title)
		if got != tt.want {
			t.Errorf("makeSlug(%q, %q) = %q, want %q", tt.id, tt.title, got, tt.want)
		}
	}
}

func TestExtractTaskID(t *testing.T) {
	tests := []struct {
		slug, want string
	}{
		{"gig-ab12-refactor-auth", "gig-ab12"},
		{"gig-ab12.1-sub-task", "gig-ab12.1"},
		{"gig-ab12", "gig-ab12"},
		{"random-dir", "random-dir"},
	}
	for _, tt := range tests {
		got := ExtractTaskID(tt.slug)
		if got != tt.want {
			t.Errorf("ExtractTaskID(%q) = %q, want %q", tt.slug, got, tt.want)
		}
	}
}
