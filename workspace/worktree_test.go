package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// runGit runs a git command in dir, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestEnsureExcluded_PlainRepo(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "commit", "--allow-empty", "-m", "init")

	if err := ensureExcluded(dir, ".jeff-base"); err != nil {
		t.Fatalf("ensureExcluded: %v", err)
	}

	excludePath := filepath.Join(dir, ".git", "info", "exclude")
	data, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	if !strings.Contains(string(data), ".jeff-base") {
		t.Errorf("expected .jeff-base in exclude file, got %q", data)
	}

	// Idempotent — running again should not duplicate the entry.
	if err := ensureExcluded(dir, ".jeff-base"); err != nil {
		t.Fatalf("second ensureExcluded: %v", err)
	}
	data, _ = os.ReadFile(excludePath)
	if n := strings.Count(string(data), ".jeff-base"); n != 1 {
		t.Errorf("expected .jeff-base once in exclude file, got %d times", n)
	}

	// A created .jeff-base file should no longer show up in git status.
	os.WriteFile(filepath.Join(dir, ".jeff-base"), []byte("origin/main\n"), 0o644)
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("expected clean git status, got %q", out)
	}
}

func TestEnsureExcluded_LinkedWorktree(t *testing.T) {
	mainDir := t.TempDir()
	runGit(t, mainDir, "init", "-b", "main")
	runGit(t, mainDir, "commit", "--allow-empty", "-m", "init")

	wtDir := filepath.Join(t.TempDir(), "wt")
	runGit(t, mainDir, "worktree", "add", wtDir, "-b", "feature")

	if err := ensureExcluded(wtDir, ".jeff-base"); err != nil {
		t.Fatalf("ensureExcluded: %v", err)
	}

	// info/exclude is resolved via `git rev-parse --git-path` rather than a
	// hardcoded <wt>/.git/info/exclude, since .git inside a worktree is a
	// file (gitdir pointer) — not a directory.
	out, err := exec.Command("git", "-C", wtDir, "rev-parse", "--git-path", "info/exclude").Output()
	if err != nil {
		t.Fatalf("rev-parse --git-path: %v", err)
	}
	excludePath := strings.TrimSpace(string(out))
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(wtDir, excludePath)
	}
	data, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	if !strings.Contains(string(data), ".jeff-base") {
		t.Errorf("expected .jeff-base in exclude file, got %q", data)
	}

	os.WriteFile(filepath.Join(wtDir, ".jeff-base"), []byte("origin/main\n"), 0o644)
	statusOut, err := exec.Command("git", "-C", wtDir, "status", "--porcelain").Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	if strings.TrimSpace(string(statusOut)) != "" {
		t.Errorf("expected clean git status, got %q", statusOut)
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
