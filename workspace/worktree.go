package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// WorktreeAdd creates a git worktree for the given repo and branch under
// jeffHome/worktrees/<repo>/<branch>/, then symlinks it into the task directory.
// If postSetup is non-empty, it is executed as a shell script with src_dir and dest_dir args.
func WorktreeAdd(jeffHome, repoName, branch, taskDir, postSetup string) (string, error) {
	repoDir := filepath.Join(jeffHome, "repos", repoName)
	if _, err := os.Stat(repoDir); err != nil {
		return "", fmt.Errorf("repo %q not found at %s", repoName, repoDir)
	}

	wtDir := filepath.Join(jeffHome, "worktrees", repoName, branch)
	if _, err := os.Stat(wtDir); err == nil {
		// Worktree already exists — just symlink into task dir.
		if taskDir != "" {
			return wtDir, symlinkIntoTask(taskDir, repoName, wtDir)
		}
		return wtDir, nil
	}

	if err := os.MkdirAll(filepath.Dir(wtDir), 0o755); err != nil {
		return "", fmt.Errorf("create worktree parent: %w", err)
	}

	cmd := exec.Command("git", "worktree", "add", wtDir, "-b", branch)
	cmd.Dir = repoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Branch may already exist — try without -b.
		cmd = exec.Command("git", "worktree", "add", wtDir, branch)
		cmd.Dir = repoDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("git worktree add: %w", err)
		}
	}

	// Run post-setup script if configured.
	if postSetup != "" {
		if err := runPostSetup(postSetup, repoDir, wtDir); err != nil {
			return wtDir, fmt.Errorf("post-setup script: %w", err)
		}
	}

	if taskDir != "" {
		return wtDir, symlinkIntoTask(taskDir, repoName, wtDir)
	}
	return wtDir, nil
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
	cmd := exec.Command("git", "worktree", "remove", wtDir)
	cmd.Dir = repoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
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
	if fi, err := os.Lstat(link); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			os.Remove(link)
		} else {
			return fmt.Errorf("%s exists and is not a symlink", link)
		}
	}

	return os.Symlink(wtDir, link)
}
