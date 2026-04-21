package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NeerajG03/JEFF/internal/testutil"
)

func TestSymlinkIntoTask(t *testing.T) {
	home := testutil.TempHome(t, "tasks")
	taskDir := filepath.Join(home, "tasks", "gig-ab12-test")
	os.MkdirAll(taskDir, 0o755)

	wtDir := filepath.Join(home, "worktrees", "backend", "feat-auth")
	os.MkdirAll(wtDir, 0o755)

	err := symlinkIntoTask(taskDir, "backend", wtDir)
	if err != nil {
		t.Fatalf("symlink: %v", err)
	}

	link := filepath.Join(taskDir, "backend")
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("stat symlink: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink")
	}

	target, _ := os.Readlink(link)
	if target != wtDir {
		t.Errorf("expected symlink to %s, got %s", wtDir, target)
	}
}

func TestSymlinkIntoTaskIdempotent(t *testing.T) {
	home := testutil.TempHome(t, "tasks")
	taskDir := filepath.Join(home, "tasks", "gig-ab12-test")
	os.MkdirAll(taskDir, 0o755)

	wtDir := filepath.Join(home, "worktrees", "backend", "feat-auth")
	os.MkdirAll(wtDir, 0o755)

	// Create symlink twice — should not error.
	symlinkIntoTask(taskDir, "backend", wtDir)
	err := symlinkIntoTask(taskDir, "backend", wtDir)
	if err != nil {
		t.Fatalf("second symlink should succeed: %v", err)
	}
}

func TestWorktreeListEmpty(t *testing.T) {
	home := testutil.TempHome(t, "tasks")
	branches, err := WorktreeList(home, "nonexistent")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(branches) != 0 {
		t.Errorf("expected 0 branches, got %d", len(branches))
	}
}

func TestWorktreeAddMissingRepo(t *testing.T) {
	home := testutil.TempHome(t, "tasks")
	_, err := WorktreeAdd(WorktreeOpts{JeffHome: home, RepoName: "nonexistent", Branch: "feat-x"})
	if err == nil {
		t.Error("expected error for missing repo")
	}
}

func TestReadBaseBranch_Default(t *testing.T) {
	dir := t.TempDir()
	got := ReadBaseBranch(dir)
	if got != defaultBaseBranch {
		t.Errorf("expected %q, got %q", defaultBaseBranch, got)
	}
}

func TestReadBaseBranch_Written(t *testing.T) {
	dir := t.TempDir()
	writeBaseBranch(dir, "origin/develop")
	got := ReadBaseBranch(dir)
	if got != "origin/develop" {
		t.Errorf("expected origin/develop, got %q", got)
	}
}

func TestReadBaseBranch_Empty(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".jeff-base"), []byte("\n"), 0o644)
	got := ReadBaseBranch(dir)
	if got != defaultBaseBranch {
		t.Errorf("expected default for empty file, got %q", got)
	}
}

func TestFindExistingWorktree_None(t *testing.T) {
	home := testutil.TempHome(t, "tasks")
	_, ok := FindExistingWorktree(home, "nonexistent")
	if ok {
		t.Error("expected false for missing repo worktrees")
	}
}

func TestFindExistingWorktree_Found(t *testing.T) {
	home := testutil.TempHome(t, "tasks")
	wtDir := filepath.Join(home, "worktrees", "backend", "gig-ab12")
	os.MkdirAll(wtDir, 0o755)

	got, ok := FindExistingWorktree(home, "backend")
	if !ok {
		t.Fatal("expected worktree to be found")
	}
	if got != wtDir {
		t.Errorf("expected %s, got %s", wtDir, got)
	}
}

func TestFindExistingWorktree_MostRecent(t *testing.T) {
	home := testutil.TempHome(t, "tasks")

	older := filepath.Join(home, "worktrees", "backend", "gig-ab12")
	os.MkdirAll(older, 0o755)

	// Ensure the second dir has a newer mtime.
	newer := filepath.Join(home, "worktrees", "backend", "gig-cd34")
	os.MkdirAll(newer, 0o755)
	// Touch newer dir to guarantee mtime ordering.
	now := time.Now().Add(time.Second)
	os.Chtimes(newer, now, now)

	got, ok := FindExistingWorktree(home, "backend")
	if !ok {
		t.Fatal("expected worktree to be found")
	}
	if got != newer {
		t.Errorf("expected most-recent worktree %s, got %s", newer, got)
	}
}

func TestReadonlyLink_MissingRepo(t *testing.T) {
	home := testutil.TempHome(t, "tasks")
	taskDir := filepath.Join(home, "tasks", "gig-ab12-test")
	os.MkdirAll(taskDir, 0o755)

	_, err := ReadonlyLink(home, "nonexistent", taskDir)
	if err == nil {
		t.Error("expected error for missing repo")
	}
}

func TestReadonlyLink_FallsBackToMainClone(t *testing.T) {
	home := testutil.TempHome(t, "tasks")
	repoDir := filepath.Join(home, "repos", "backend")
	os.MkdirAll(repoDir, 0o755)

	taskDir := filepath.Join(home, "tasks", "gig-ab12-test")
	os.MkdirAll(taskDir, 0o755)

	target, err := ReadonlyLink(home, "backend", taskDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target != repoDir {
		t.Errorf("expected symlink to main clone %s, got %s", repoDir, target)
	}

	link := filepath.Join(taskDir, "backend")
	resolved, _ := os.Readlink(link)
	if resolved != repoDir {
		t.Errorf("symlink target: expected %s, got %s", repoDir, resolved)
	}
}

func TestReadonlyLink_PrefersExistingWorktree(t *testing.T) {
	home := testutil.TempHome(t, "tasks")
	repoDir := filepath.Join(home, "repos", "backend")
	os.MkdirAll(repoDir, 0o755)

	wtDir := filepath.Join(home, "worktrees", "backend", "gig-xy99")
	os.MkdirAll(wtDir, 0o755)

	taskDir := filepath.Join(home, "tasks", "gig-ab12-test")
	os.MkdirAll(taskDir, 0o755)

	target, err := ReadonlyLink(home, "backend", taskDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target != wtDir {
		t.Errorf("expected symlink to existing worktree %s, got %s", wtDir, target)
	}

	link := filepath.Join(taskDir, "backend")
	resolved, _ := os.Readlink(link)
	if resolved != wtDir {
		t.Errorf("symlink target: expected %s, got %s", wtDir, resolved)
	}
}

func TestReadonlyLink_NoTaskDir(t *testing.T) {
	home := testutil.TempHome(t, "tasks")
	repoDir := filepath.Join(home, "repos", "backend")
	os.MkdirAll(repoDir, 0o755)

	target, err := ReadonlyLink(home, "backend", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target != repoDir {
		t.Errorf("expected %s, got %s", repoDir, target)
	}
}

func TestResolveBranchName_NoScript(t *testing.T) {
	got, err := ResolveBranchName("", nil, "gig-ab12")
	if err != nil {
		t.Fatal(err)
	}
	if got != "gig-ab12" {
		t.Errorf("expected gig-ab12, got %q", got)
	}
}

func TestResolveBranchName_WithScript(t *testing.T) {
	// Create a script that reads task JSON and outputs a branch name.
	script := filepath.Join(t.TempDir(), "branch.sh")
	os.WriteFile(script, []byte(`#!/bin/bash
TASK=$(cat)
TYPE=$(echo "$TASK" | jq -r '.type')
ID=$(echo "$TASK" | jq -r '.id')
echo "${TYPE}/${ID}"
`), 0o755)

	taskJSON := []byte(`{"id":"gig-ab12","type":"feature"}`)
	got, err := ResolveBranchName(script, taskJSON, "fallback")
	if err != nil {
		t.Fatal(err)
	}
	if got != "feature/gig-ab12" {
		t.Errorf("expected feature/gig-ab12, got %q", got)
	}
}

func TestResolveBranchName_ScriptEmpty(t *testing.T) {
	script := filepath.Join(t.TempDir(), "empty.sh")
	os.WriteFile(script, []byte("#!/bin/bash\necho ''"), 0o755)

	_, err := ResolveBranchName(script, []byte("{}"), "fallback")
	if err == nil {
		t.Error("expected error for empty output")
	}
}

func TestResolveBranchName_ScriptFails(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fail.sh")
	os.WriteFile(script, []byte("#!/bin/bash\nexit 1"), 0o755)

	_, err := ResolveBranchName(script, []byte("{}"), "fallback")
	if err == nil {
		t.Error("expected error for failing script")
	}
}
