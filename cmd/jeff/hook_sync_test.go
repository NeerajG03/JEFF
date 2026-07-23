package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTaskWorkspaces verifies the sync --tasks walk: it picks up every task
// directory under <home>/tasks whose name carries a gig- ID and skips files
// and non-gig directories.
func TestTaskWorkspaces(t *testing.T) {
	home := t.TempDir()
	tasksDir := filepath.Join(home, "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Two eligible task dirs, one non-gig dir, and a stray file.
	mkdir := func(name string) {
		if err := os.MkdirAll(filepath.Join(tasksDir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mkdir("gig-1d9d.7-r6-hooks-hardening")
	mkdir("gig-abcd-some-other-task")
	mkdir("notes-scratch") // non-gig dir, must be skipped
	if err := os.WriteFile(filepath.Join(tasksDir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := taskWorkspaces(home)
	if len(got) != 2 {
		t.Fatalf("expected 2 task workspaces, got %d: %+v", len(got), got)
	}

	byID := map[string]taskWorkspace{}
	for _, ws := range got {
		byID[ws.TaskID] = ws
	}
	if _, ok := byID["gig-1d9d.7"]; !ok {
		t.Errorf("expected gig-1d9d.7 in results, got %+v", got)
	}
	if _, ok := byID["gig-abcd"]; !ok {
		t.Errorf("expected gig-abcd in results, got %+v", got)
	}
	for _, ws := range got {
		if filepath.Dir(ws.Dir) != tasksDir {
			t.Errorf("Dir %q not under tasks dir %q", ws.Dir, tasksDir)
		}
		if ws.Name == "" {
			t.Errorf("empty Name in %+v", ws)
		}
	}
}

// TestTaskWorkspacesMissingDir confirms a missing tasks/ directory yields nil
// rather than an error or panic.
func TestTaskWorkspacesMissingDir(t *testing.T) {
	home := t.TempDir() // no tasks/ subdir
	if got := taskWorkspaces(home); got != nil {
		t.Errorf("expected nil for missing tasks dir, got %+v", got)
	}
}
