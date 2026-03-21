package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NeerajG03/gig"
)

func TestBuildPRTitle(t *testing.T) {
	task := &gig.Task{ID: "gig-ab12", Title: "Add auth flow"}
	got := buildPRTitle(task)
	want := "[gig-ab12] Add auth flow"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildPRTitle_LongTitle(t *testing.T) {
	task := &gig.Task{ID: "gig-ab12", Title: strings.Repeat("x", 200)}
	got := buildPRTitle(task)
	if len(got) > 120 {
		t.Errorf("title too long: %d chars", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Error("expected ... suffix for truncated title")
	}
}

func TestBuildPRBody_WithCheckpoint(t *testing.T) {
	store, err := gig.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	task, err := store.Create(gig.CreateParams{
		Title:       "Auth task",
		Description: "Implement OAuth2 flow",
		Type:        gig.TypeFeature,
	})
	if err != nil {
		t.Fatal(err)
	}

	store.AddCheckpoint(task.ID, "jeff", gig.CheckpointParams{
		Done:      "Added login endpoint",
		Decisions: "Using JWT tokens",
		Next:      "Add refresh logic",
	})

	body := buildPRBody(store, task)

	for _, want := range []string{
		"Implement OAuth2 flow",
		"## Progress",
		"Added login endpoint",
		"Using JWT tokens",
		"Add refresh logic",
		"Task: " + task.ID,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestBuildPRBody_NoCheckpoint(t *testing.T) {
	store, err := gig.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	task, err := store.Create(gig.CreateParams{
		Title:       "Simple task",
		Description: "Just a description",
		Type:        gig.TypeTask,
	})
	if err != nil {
		t.Fatal(err)
	}

	body := buildPRBody(store, task)

	if !strings.Contains(body, "Just a description") {
		t.Error("missing description")
	}
	if strings.Contains(body, "## Progress") {
		t.Error("should not have progress section without checkpoint")
	}
	if !strings.Contains(body, "Task: "+task.ID) {
		t.Error("missing task ID footer")
	}
}

func TestBuildPRBody_NoDescription(t *testing.T) {
	store, err := gig.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	task, err := store.Create(gig.CreateParams{
		Title: "No desc",
		Type:  gig.TypeTask,
	})
	if err != nil {
		t.Fatal(err)
	}

	store.AddCheckpoint(task.ID, "jeff", gig.CheckpointParams{Done: "Did stuff"})

	body := buildPRBody(store, task)

	if !strings.Contains(body, "Did stuff") {
		t.Error("missing checkpoint content")
	}
	if !strings.Contains(body, "Task: "+task.ID) {
		t.Error("missing task footer")
	}
}

func TestBuildPRBody_Empty(t *testing.T) {
	store, err := gig.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	task, err := store.Create(gig.CreateParams{Title: "Minimal", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}

	body := buildPRBody(store, task)

	if !strings.Contains(body, "Task: "+task.ID) {
		t.Error("missing task footer")
	}
}

func TestDiscoverWorktrees(t *testing.T) {
	dir := t.TempDir()

	// Create fake worktree targets with .jeff-base files.
	for _, name := range []string{"frontend", "backend"} {
		wtDir := filepath.Join(t.TempDir(), name, "gig-ab12")
		os.MkdirAll(wtDir, 0o755)
		os.WriteFile(filepath.Join(wtDir, ".jeff-base"), []byte("origin/main\n"), 0o644)
		os.Symlink(wtDir, filepath.Join(dir, name))
	}

	wts, err := discoverWorktrees(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(wts) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(wts))
	}

	// Verify branch and base are resolved.
	for _, wt := range wts {
		if wt.branch != "gig-ab12" {
			t.Errorf("expected branch gig-ab12, got %q", wt.branch)
		}
		if wt.base != "main" {
			t.Errorf("expected base main, got %q", wt.base)
		}
	}
}

func TestDiscoverWorktrees_Empty(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("test"), 0o644)

	wts, err := discoverWorktrees(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(wts) != 0 {
		t.Errorf("expected 0 worktrees, got %d", len(wts))
	}
}

func TestDiscoverWorktrees_FilterByRepo(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"frontend", "backend"} {
		wtDir := filepath.Join(t.TempDir(), name, "gig-ab12")
		os.MkdirAll(wtDir, 0o755)
		os.Symlink(wtDir, filepath.Join(dir, name))
	}

	wts, err := discoverWorktrees(dir, "frontend")
	if err != nil {
		t.Fatal(err)
	}
	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(wts))
	}
	if wts[0].repo != "frontend" {
		t.Errorf("expected frontend, got %q", wts[0].repo)
	}
}

func TestDiscoverWorktrees_FilterMissing(t *testing.T) {
	dir := t.TempDir()

	wtDir := filepath.Join(t.TempDir(), "frontend", "gig-ab12")
	os.MkdirAll(wtDir, 0o755)
	os.Symlink(wtDir, filepath.Join(dir, "frontend"))

	_, err := discoverWorktrees(dir, "nonexistent")
	if err == nil {
		t.Error("expected error for missing repo filter")
	}
}

func TestDiscoverWorktrees_BaseBranchStripped(t *testing.T) {
	dir := t.TempDir()
	wtDir := filepath.Join(t.TempDir(), "api", "gig-cd34")
	os.MkdirAll(wtDir, 0o755)
	os.WriteFile(filepath.Join(wtDir, ".jeff-base"), []byte("origin/develop\n"), 0o644)
	os.Symlink(wtDir, filepath.Join(dir, "api"))

	wts, err := discoverWorktrees(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if wts[0].base != "develop" {
		t.Errorf("expected base develop (origin/ stripped), got %q", wts[0].base)
	}
}
