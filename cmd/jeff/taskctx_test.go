package main

import (
	"os"
	"path/filepath"
	"testing"

	jeff "github.com/NeerajG03/JEFF"
)

func setupTaskCtxTest(t *testing.T) (string, func()) {
	t.Helper()
	// Resolve symlinks so macOS /var → /private/var doesn't cause mismatches.
	home, _ := filepath.EvalSymlinks(t.TempDir())

	// Create tasks dir with a task workspace.
	taskDir := filepath.Join(home, "tasks", "gig-ab12-test-task")
	os.MkdirAll(taskDir, 0o755)

	// Write a minimal jeff.json so LoadConfig works.
	os.WriteFile(filepath.Join(home, "jeff.json"), []byte(`{"agent":"claude","repos":{}}`+"\n"), 0o644)

	// Set the global cfg.
	oldCfg := cfg
	c, _ := jeff.LoadConfig(home)
	cfg = c

	return home, func() { cfg = oldCfg }
}

func TestResolveTaskID_FromArgs(t *testing.T) {
	home, cleanup := setupTaskCtxTest(t)
	defer cleanup()

	taskID, taskDir, err := resolveTaskID([]string{"gig-ab12"})
	if err != nil {
		t.Fatal(err)
	}
	if taskID != "gig-ab12" {
		t.Errorf("expected gig-ab12, got %q", taskID)
	}
	expected := filepath.Join(home, "tasks", "gig-ab12-test-task")
	if taskDir != expected {
		t.Errorf("expected %s, got %s", expected, taskDir)
	}
}

func TestResolveTaskID_FromCwd(t *testing.T) {
	home, cleanup := setupTaskCtxTest(t)
	defer cleanup()

	taskDir := filepath.Join(home, "tasks", "gig-ab12-test-task")

	// Change to task dir.
	oldCwd, _ := os.Getwd()
	os.Chdir(taskDir)
	defer os.Chdir(oldCwd)

	taskID, dir, err := resolveTaskID(nil)
	if err != nil {
		t.Fatal(err)
	}
	if taskID != "gig-ab12" {
		t.Errorf("expected gig-ab12, got %q", taskID)
	}
	if dir != taskDir {
		t.Errorf("expected %s, got %s", taskDir, dir)
	}
}

func TestResolveTaskID_FromCwdSubdir(t *testing.T) {
	home, cleanup := setupTaskCtxTest(t)
	defer cleanup()

	// Create a subdirectory inside the task dir (like a worktree).
	subdir := filepath.Join(home, "tasks", "gig-ab12-test-task", "frontend")
	os.MkdirAll(subdir, 0o755)

	oldCwd, _ := os.Getwd()
	os.Chdir(subdir)
	defer os.Chdir(oldCwd)

	taskID, dir, err := resolveTaskID(nil)
	if err != nil {
		t.Fatal(err)
	}
	if taskID != "gig-ab12" {
		t.Errorf("expected gig-ab12, got %q", taskID)
	}
	expected := filepath.Join(home, "tasks", "gig-ab12-test-task")
	if dir != expected {
		t.Errorf("expected %s, got %s", expected, dir)
	}
}

func TestResolveTaskID_NoCwd(t *testing.T) {
	_, cleanup := setupTaskCtxTest(t)
	defer cleanup()

	// cwd is some random dir, not a task workspace.
	oldCwd, _ := os.Getwd()
	os.Chdir(t.TempDir())
	defer os.Chdir(oldCwd)

	_, _, err := resolveTaskID(nil)
	if err == nil {
		t.Error("expected error when not in a task workspace")
	}
}

func TestResolveTaskID_FromSymlinkedWorktree(t *testing.T) {
	home, cleanup := setupTaskCtxTest(t)
	defer cleanup()

	// Create a worktree directory outside the tasks dir (simulating
	// /jeff/worktrees/<repo>/<branch>/) and symlink it into the task dir.
	worktreeDir := filepath.Join(home, "worktrees", "myrepo", "gig-ab12-branch")
	os.MkdirAll(worktreeDir, 0o755)
	symlinkPath := filepath.Join(home, "tasks", "gig-ab12-test-task", "myrepo")
	os.Symlink(worktreeDir, symlinkPath)

	// Simulate what the shell does: update $PWD to the logical path through
	// the symlink before cd-ing into the physical directory.
	// os.Getwd() honours $PWD when it matches the current directory's inode,
	// so this makes it return the logical (task-dir) path, not the worktree path.
	oldCwd, _ := os.Getwd()
	oldPWD := os.Getenv("PWD")
	os.Setenv("PWD", symlinkPath)
	os.Chdir(symlinkPath)
	defer func() {
		os.Chdir(oldCwd)
		os.Setenv("PWD", oldPWD)
	}()

	taskID, dir, err := resolveTaskID(nil)
	if err != nil {
		t.Fatal(err)
	}
	if taskID != "gig-ab12" {
		t.Errorf("expected gig-ab12, got %q", taskID)
	}
	expected := filepath.Join(home, "tasks", "gig-ab12-test-task")
	if dir != expected {
		t.Errorf("expected %s, got %s", expected, dir)
	}
}

func TestResolveTaskID_ArgOverridesCwd(t *testing.T) {
	home, cleanup := setupTaskCtxTest(t)
	defer cleanup()

	// Create a second task.
	os.MkdirAll(filepath.Join(home, "tasks", "gig-cd34-other-task"), 0o755)

	// Stand in gig-ab12's dir but ask for gig-cd34.
	oldCwd, _ := os.Getwd()
	os.Chdir(filepath.Join(home, "tasks", "gig-ab12-test-task"))
	defer os.Chdir(oldCwd)

	taskID, _, err := resolveTaskID([]string{"gig-cd34"})
	if err != nil {
		t.Fatal(err)
	}
	if taskID != "gig-cd34" {
		t.Errorf("expected gig-cd34, got %q", taskID)
	}
}
