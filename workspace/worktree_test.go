package workspace

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NeerajG03/JEFF/internal/testutil"
)

// setupGitRepo creates a minimal real git repo with one commit under
// jeffHome/repos/<repoName>, so tests can exercise real `git worktree` /
// `git status` behavior. BaseBranch "main" (no remote prefix) avoids the
// `git fetch` step WorktreeAdd runs for remote-qualified base branches.
func setupGitRepo(t *testing.T, jeffHome, repoName string) string {
	t.Helper()
	repoDir := filepath.Join(jeffHome, "repos", repoName)
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "initial")
	return repoDir
}

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

// addFileRemote configures repoDir's "origin" remote to point at a second
// local repo seeded with the given branches (each branched off one initial
// commit, with distinct content per branch when supplied via branchContent),
// so tests can exercise the real `git fetch` path without network access.
func addFileRemote(t *testing.T, repoDir string) (remoteDir string, run func(dir string, args ...string) string) {
	t.Helper()
	remoteDir = t.TempDir()
	run = func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v (in %s): %v\n%s", args, dir, err, out)
		}
		return string(out)
	}
	run(remoteDir, "init", "-q", "-b", "main")
	run(remoteDir, "config", "user.email", "t@example.com")
	run(remoteDir, "config", "user.name", "T")
	if err := os.WriteFile(filepath.Join(remoteDir, "f"), []byte("main\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	run(remoteDir, "add", ".")
	run(remoteDir, "commit", "-q", "-m", "init")
	run(repoDir, "remote", "add", "origin", remoteDir)
	return remoteDir, run
}

// TestWorktreeAddSlashedLocalBase is the regression test for gig-1553: a
// local branch whose name happens to contain a "/" (a routine naming
// convention: "cb-15329/propagation-in-publish", "feature/x", ...) must not
// be treated as "<remote>/<branch>" when no remote of that name exists.
// Before the fix, WorktreeAdd always ran `git fetch <first-segment>`, which
// fails when "cb-15329" is not a configured remote and aborts the whole
// worktree creation.
func TestWorktreeAddSlashedLocalBase(t *testing.T) {
	home := testutil.TempHome(t, "tasks")
	repoDir := setupGitRepo(t, home, "repo1")
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("branch", "cb-15329/propagation-in-publish")

	wtDir, err := WorktreeAdd(WorktreeOpts{
		JeffHome:   home,
		RepoName:   "repo1",
		Branch:     "new-work",
		BaseBranch: "cb-15329/propagation-in-publish",
	})
	if err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	if _, err := os.Stat(wtDir); err != nil {
		t.Errorf("worktree not created: %v", err)
	}
}

// TestWorktreeAddRemoteQualifiedBaseStillFetches confirms a genuine
// "origin/<branch>" base still triggers `git fetch origin` and resolves
// against the fetched remote-tracking ref, not a stale local one.
func TestWorktreeAddRemoteQualifiedBaseStillFetches(t *testing.T) {
	home := testutil.TempHome(t, "tasks")
	repoDir := setupGitRepo(t, home, "repo1")
	remoteDir, run := addFileRemote(t, repoDir)
	run(remoteDir, "checkout", "-q", "-b", "release")
	if err := os.WriteFile(filepath.Join(remoteDir, "f"), []byte("release\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	run(remoteDir, "commit", "-q", "-am", "release commit")

	wtDir, err := WorktreeAdd(WorktreeOpts{
		JeffHome:   home,
		RepoName:   "repo1",
		Branch:     "new-work",
		BaseBranch: "origin/release",
	})
	if err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(wtDir, "f"))
	if err != nil {
		t.Fatalf("read f: %v", err)
	}
	if string(got) != "release\n" {
		t.Errorf("worktree did not branch from fetched origin/release; f = %q", got)
	}
}

// TestWorktreeAddTwoSlashBase confirms a base with two slashes
// ("origin/feature/x") still resolves: only the first segment is checked
// against the remote list.
func TestWorktreeAddTwoSlashBase(t *testing.T) {
	home := testutil.TempHome(t, "tasks")
	repoDir := setupGitRepo(t, home, "repo1")
	remoteDir, run := addFileRemote(t, repoDir)
	run(remoteDir, "checkout", "-q", "-b", "feature/x")
	if err := os.WriteFile(filepath.Join(remoteDir, "f"), []byte("feature-x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	run(remoteDir, "commit", "-q", "-am", "feature/x commit")

	wtDir, err := WorktreeAdd(WorktreeOpts{
		JeffHome:   home,
		RepoName:   "repo1",
		Branch:     "new-work",
		BaseBranch: "origin/feature/x",
	})
	if err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(wtDir, "f"))
	if err != nil {
		t.Fatalf("read f: %v", err)
	}
	if string(got) != "feature-x\n" {
		t.Errorf("worktree did not branch from origin/feature/x; f = %q", got)
	}
}

// TestWorktreeAddLocalBranchPrecedenceOverRemoteName pins the documented
// ambiguity rule: when a local branch's literal name equals a remote-
// qualified ref (a local branch named "origin/shared" vs. remote "origin"'s
// "shared" branch, both resolvable as "origin/shared"), the local branch
// wins. Without disambiguation, `git worktree add` refuses outright with
// "ambiguous object name" instead of picking one — the fix resolves
// "origin/shared" against refs/heads/ first. The remote is still fetched
// (it IS a configured remote), but ref resolution prefers the local branch.
func TestWorktreeAddLocalBranchPrecedenceOverRemoteName(t *testing.T) {
	home := testutil.TempHome(t, "tasks")
	repoDir := setupGitRepo(t, home, "repo1")
	remoteDir, run := addFileRemote(t, repoDir)
	run(remoteDir, "checkout", "-q", "-b", "shared")
	if err := os.WriteFile(filepath.Join(remoteDir, "f"), []byte("remote-shared\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	run(remoteDir, "commit", "-q", "-am", "remote shared commit")

	localRun := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	localRun("checkout", "-q", "-b", "origin/shared")
	if err := os.WriteFile(filepath.Join(repoDir, "f"), []byte("local-shared\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	localRun("add", "f")
	localRun("commit", "-q", "-m", "local origin/shared commit")
	localRun("checkout", "-q", "main")

	wtDir, err := WorktreeAdd(WorktreeOpts{
		JeffHome:   home,
		RepoName:   "repo1",
		Branch:     "new-work",
		BaseBranch: "origin/shared",
	})
	if err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(wtDir, "f"))
	if err != nil {
		t.Fatalf("read f: %v", err)
	}
	if string(got) != "local-shared\n" {
		t.Errorf("expected local branch 'origin/shared' to take precedence, got f = %q", got)
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

func TestWorktreeRemoveDirty(t *testing.T) {
	home := testutil.TempHome(t)
	setupGitRepo(t, home, "backend")

	wtDir, err := WorktreeAdd(WorktreeOpts{JeffHome: home, RepoName: "backend", Branch: "gig-dirty", BaseBranch: "main"})
	if err != nil {
		t.Fatalf("worktree add: %v", err)
	}

	// Dirty the worktree with an untracked file.
	if err := os.WriteFile(filepath.Join(wtDir, "scratch.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatalf("write scratch file: %v", err)
	}

	// Without force: refuse with ErrWorktreeDirty, worktree untouched.
	err = WorktreeRemove(home, "backend", "gig-dirty", false)
	if !errors.Is(err, ErrWorktreeDirty) {
		t.Fatalf("expected ErrWorktreeDirty, got %v", err)
	}
	if !strings.Contains(err.Error(), "scratch.txt") {
		t.Errorf("error should list dirty paths, got: %v", err)
	}
	if _, statErr := os.Stat(wtDir); statErr != nil {
		t.Errorf("worktree should still exist after refused removal: %v", statErr)
	}

	// With force: succeeds.
	if err := WorktreeRemove(home, "backend", "gig-dirty", true); err != nil {
		t.Fatalf("force remove: %v", err)
	}
	if _, statErr := os.Stat(wtDir); !os.IsNotExist(statErr) {
		t.Errorf("worktree dir should be gone after force removal")
	}

	// git worktree list in the repo should no longer show it (prune ran).
	repoDir := filepath.Join(home, "repos", "backend")
	out, err := exec.Command("git", "-C", repoDir, "worktree", "list").CombinedOutput()
	if err != nil {
		t.Fatalf("git worktree list: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "gig-dirty") {
		t.Errorf("expected worktree pruned from git metadata, still present:\n%s", out)
	}
}

func TestWorktreeRemoveClean(t *testing.T) {
	home := testutil.TempHome(t)
	setupGitRepo(t, home, "backend")

	wtDir, err := WorktreeAdd(WorktreeOpts{JeffHome: home, RepoName: "backend", Branch: "gig-clean", BaseBranch: "main"})
	if err != nil {
		t.Fatalf("worktree add: %v", err)
	}

	// No changes — removal without force should succeed.
	if err := WorktreeRemove(home, "backend", "gig-clean", false); err != nil {
		t.Fatalf("clean remove without force: %v", err)
	}
	if _, statErr := os.Stat(wtDir); !os.IsNotExist(statErr) {
		t.Errorf("worktree dir should be gone after clean removal")
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

func TestListTaskWorktrees_IgnoresRegularFiles(t *testing.T) {
	home := t.TempDir()
	taskDir := t.TempDir()

	// Regular file — should be ignored.
	os.WriteFile(filepath.Join(taskDir, "CLAUDE.md"), []byte("test"), 0o644)

	// Regular dir — should be ignored.
	os.MkdirAll(filepath.Join(taskDir, ".claude"), 0o755)

	// A real worktree symlinked in — should be found, with its real branch.
	setupGitRepo(t, home, "myrepo")
	wtDir, err := WorktreeAdd(WorktreeOpts{JeffHome: home, RepoName: "myrepo", Branch: "gig-lt01", BaseBranch: "main", TaskDir: taskDir})
	if err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}

	result, err := ListTaskWorktrees(taskDir)
	if err != nil {
		t.Fatalf("ListTaskWorktrees: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 worktree, got %d: %+v", len(result), result)
	}
	if result[0].Repo != "myrepo" {
		t.Errorf("expected Repo=myrepo, got %q", result[0].Repo)
	}
	if result[0].Branch != "gig-lt01" {
		t.Errorf("expected Branch=gig-lt01, got %q", result[0].Branch)
	}
	if result[0].Path != wtDir {
		t.Errorf("expected Path=%q, got %q", wtDir, result[0].Path)
	}
}
