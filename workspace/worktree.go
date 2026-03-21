package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/NeerajG03/JEFF/internal/gitutil"
)

// WorktreeOpts holds options for creating a worktree.
type WorktreeOpts struct {
	JeffHome   string
	RepoName   string
	Branch     string // branch to create (e.g. "gig-ab12")
	BaseBranch string // base to branch from (default: "origin/main")
	TaskDir    string // symlink into this task dir (optional)
	PostSetup  string // script to run after creation (optional)
}

const defaultBaseBranch = "origin/main"

// WorktreeAdd creates a git worktree for the given repo and branch under
// jeffHome/worktrees/<repo>/<branch>/, then symlinks it into the task directory.
// The base branch determines what the new branch is created from.
// A .jeff-base file is written to the worktree recording the base branch for jeff ship.
func WorktreeAdd(opts WorktreeOpts) (string, error) {
	repoDir := filepath.Join(opts.JeffHome, "repos", opts.RepoName)
	if _, err := os.Stat(repoDir); err != nil {
		return "", fmt.Errorf("repo %q not found at %s", opts.RepoName, repoDir)
	}

	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		baseBranch = defaultBaseBranch
	}

	wtDir := filepath.Join(opts.JeffHome, "worktrees", opts.RepoName, opts.Branch)
	if _, err := os.Stat(wtDir); err == nil {
		// Worktree already exists — just symlink into task dir.
		if opts.TaskDir != "" {
			return wtDir, symlinkIntoTask(opts.TaskDir, opts.RepoName, wtDir)
		}
		return wtDir, nil
	}

	if err := os.MkdirAll(filepath.Dir(wtDir), 0o755); err != nil {
		return "", fmt.Errorf("create worktree parent: %w", err)
	}

	// Fetch remote if base branch references a remote (e.g. "origin/main").
	if remote, _, ok := strings.Cut(baseBranch, "/"); ok {
		if err := gitutil.Run(repoDir, "fetch", remote); err != nil {
			return "", err
		}
	}

	// Create worktree branching from baseBranch.
	if err := gitutil.Run(repoDir, "worktree", "add", wtDir, "-b", opts.Branch, baseBranch); err != nil {
		// Branch may already exist — try without -b.
		if err := gitutil.Run(repoDir, "worktree", "add", wtDir, opts.Branch); err != nil {
			return "", fmt.Errorf("git worktree add: %w", err)
		}
	}

	// Record the base branch so jeff ship knows the PR target.
	writeBaseBranch(wtDir, baseBranch)

	// Run post-setup script if configured.
	if opts.PostSetup != "" {
		if err := runPostSetup(opts.PostSetup, repoDir, wtDir); err != nil {
			return wtDir, fmt.Errorf("post-setup script: %w", err)
		}
	}

	if opts.TaskDir != "" {
		return wtDir, symlinkIntoTask(opts.TaskDir, opts.RepoName, wtDir)
	}
	return wtDir, nil
}

// writeBaseBranch records the base branch in a .jeff-base file inside the worktree.
func writeBaseBranch(wtDir, baseBranch string) {
	os.WriteFile(filepath.Join(wtDir, ".jeff-base"), []byte(baseBranch+"\n"), 0o644)
}

// ReadBaseBranch reads the base branch from a worktree's .jeff-base file.
// Returns defaultBaseBranch if the file doesn't exist.
func ReadBaseBranch(wtDir string) string {
	data, err := os.ReadFile(filepath.Join(wtDir, ".jeff-base"))
	if err != nil {
		return defaultBaseBranch
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return defaultBaseBranch
	}
	return s
}

// runPostSetup executes a user-provided script after worktree creation.
// The script receives src_dir (repo clone) and dest_dir (worktree) as arguments.
func runPostSetup(script, srcDir, destDir string) error {
	cmd := exec.Command("sh", script, srcDir, destDir)
	cmd.Dir = destDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// WorktreeRemove removes a git worktree and its symlink.
func WorktreeRemove(jeffHome, repoName, branch string) error {
	wtDir := filepath.Join(jeffHome, "worktrees", repoName, branch)

	repoDir := filepath.Join(jeffHome, "repos", repoName)
	if err := gitutil.Run(repoDir, "worktree", "remove", wtDir); err != nil {
		// Fallback: force remove if dirty.
		os.RemoveAll(wtDir)
	}

	// Clean up empty parent dir.
	parent := filepath.Dir(wtDir)
	entries, _ := os.ReadDir(parent)
	if len(entries) == 0 {
		os.Remove(parent)
	}

	return nil
}

// WorktreeList returns all worktrees for a repo.
func WorktreeList(jeffHome, repoName string) ([]string, error) {
	wtBase := filepath.Join(jeffHome, "worktrees", repoName)
	entries, err := os.ReadDir(wtBase)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read worktrees: %w", err)
	}

	var branches []string
	for _, e := range entries {
		if e.IsDir() {
			branches = append(branches, e.Name())
		}
	}
	return branches, nil
}

// symlinkIntoTask creates a symlink from taskDir/<repoName> → worktree path.
func symlinkIntoTask(taskDir, repoName, wtDir string) error {
	link := filepath.Join(taskDir, repoName)

	// Remove existing symlink if present.
	if gitutil.IsSymlink(link) {
		os.Remove(link)
	} else if _, err := os.Lstat(link); err == nil {
		return fmt.Errorf("%s exists and is not a symlink", link)
	}

	return os.Symlink(wtDir, link)
}
