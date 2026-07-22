package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/NeerajG03/JEFF/internal/gitutil"
)

// ErrWorktreeDirty is returned when a worktree removal is refused because it
// has uncommitted changes and force was not requested.
var ErrWorktreeDirty = errors.New("worktree has uncommitted changes")

// DirtyPaths returns up to max uncommitted paths in the worktree ("" clean).
// Exported so callers (e.g. `jeff done`) can preflight-check multiple
// worktrees for dirtiness before removing any of them.
func DirtyPaths(wtDir string, max int) []string {
	out, err := gitutil.Output(wtDir, "status", "--porcelain")
	if err != nil || len(bytes.TrimSpace(out)) == 0 {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > max {
		lines = lines[:max]
	}
	return lines
}

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
	if err := ensureExcluded(wtDir, ".jeff-base"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: exclude .jeff-base: %v\n", err)
	}

	// Run post-setup script if configured.
	if opts.PostSetup != "" {
		if err := runPostSetup(opts.PostSetup, PostSetupContext{
			SrcDir:  repoDir,
			DestDir: wtDir,
			Repo:    opts.RepoName,
			Branch:  opts.Branch,
		}); err != nil {
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
	_ = os.WriteFile(filepath.Join(wtDir, ".jeff-base"), []byte(baseBranch+"\n"), 0o644)
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

// ensureExcluded appends pattern to the worktree's local git exclude file if absent.
// Worktrees have their own info/exclude under the worktree gitdir, resolved via
// `git rev-parse --git-path` — for linked worktrees the gitdir lives under the
// main repo's .git/worktrees/<name>/, and a hardcoded <wt>/.git/info/exclude
// path is wrong since .git in a worktree is a file, not a directory.
func ensureExcluded(wtDir, pattern string) error {
	out, err := gitutil.Output(wtDir, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return err
	}
	excludePath := strings.TrimSpace(string(out))
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(wtDir, excludePath)
	}
	data, _ := os.ReadFile(excludePath)
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == pattern {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(pattern + "\n")
	return err
}

// EnsureJeffBaseExcluded self-heals worktrees created by older jeff binaries
// that predate the .jeff-base exclude, so it stops appearing in `git status`.
func EnsureJeffBaseExcluded(wtDir string) error {
	return ensureExcluded(wtDir, ".jeff-base")
}

// PostSetupContext is the JSON payload sent to post-setup scripts on stdin.
type PostSetupContext struct {
	SrcDir  string `json:"src_dir"`  // repo clone directory
	DestDir string `json:"dest_dir"` // worktree directory
	Repo    string `json:"repo"`     // repo name
	Branch  string `json:"branch"`   // worktree branch
}

// runPostSetup executes a user-provided script after worktree creation.
// The script receives a JSON context on stdin with src_dir, dest_dir, repo, and branch.
func runPostSetup(script string, ctx PostSetupContext) error {
	data, err := json.Marshal(ctx)
	if err != nil {
		return fmt.Errorf("marshal post-setup context: %w", err)
	}
	cmd := exec.Command(script)
	cmd.Dir = ctx.DestDir
	cmd.Stdin = bytes.NewReader(data)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// WorktreeRemove removes a git worktree by reconstructing its path from
// jeffHome/worktrees/<repoName>/<branch>. Refuses on uncommitted changes
// unless force is true.
func WorktreeRemove(jeffHome, repoName, branch string, force bool) error {
	wtDir := filepath.Join(jeffHome, "worktrees", repoName, branch)
	return worktreeRemoveDir(jeffHome, repoName, wtDir, force)
}

// WorktreeRemoveByPath removes a git worktree at the given absolute path.
// Use this when the worktree path is already known (e.g. resolved from a
// task dir symlink) instead of reconstructing from a branch name. Refuses on
// uncommitted changes unless force is true.
func WorktreeRemoveByPath(jeffHome, repoName, wtDir string, force bool) error {
	return worktreeRemoveDir(jeffHome, repoName, wtDir, force)
}

func worktreeRemoveDir(jeffHome, repoName, wtDir string, force bool) error {
	repoDir := filepath.Join(jeffHome, "repos", repoName)

	if !force {
		if paths := DirtyPaths(wtDir, 20); len(paths) > 0 {
			return fmt.Errorf("%w:\n  %s\n(commit/ship first, or pass --force to discard)", ErrWorktreeDirty, strings.Join(paths, "\n  "))
		}
	}

	if err := gitutil.Run(repoDir, "worktree", "remove", wtDir); err != nil {
		// Fallback: force remove if dirty.
		os.RemoveAll(wtDir)
	}

	// Clean up dangling .git/worktrees/<name> metadata left by the RemoveAll
	// fallback above, so a future WorktreeAdd at this path doesn't fail with
	// "already registered".
	_ = gitutil.Run(repoDir, "worktree", "prune")

	// Clean up empty parent dirs up to the worktrees/<repoName> level.
	parent := filepath.Dir(wtDir)
	wtBase := filepath.Join(jeffHome, "worktrees", repoName)
	for parent != wtBase && strings.HasPrefix(parent, wtBase+string(filepath.Separator)) {
		entries, _ := os.ReadDir(parent)
		if len(entries) != 0 {
			break
		}
		os.Remove(parent)
		parent = filepath.Dir(parent)
	}
	// Also clean the repo-level dir if empty.
	entries, _ := os.ReadDir(wtBase)
	if len(entries) == 0 {
		os.Remove(wtBase)
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

// FindExistingWorktree returns the path of the most recently modified worktree for repoName.
// Returns ("", false) if none exist. Used to share an existing worktree instead of creating
// a duplicate (e.g. for a readonly worker joining a crew that already checked out the repo).
func FindExistingWorktree(jeffHome, repoName string) (string, bool) {
	wtBase := filepath.Join(jeffHome, "worktrees", repoName)
	entries, err := os.ReadDir(wtBase)
	if err != nil {
		return "", false
	}
	var best string
	var bestMod time.Time
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(wtBase, e.Name())
		if info.ModTime().After(bestMod) {
			bestMod = info.ModTime()
			best = path
		}
	}
	if best == "" {
		return "", false
	}
	return best, true
}

// ReadonlyLink creates a symlink in taskDir pointing to the repo for read-only access.
// No git worktree is created and no post-setup hooks are run.
// If an existing worktree is found (e.g. created by another crew member), the symlink
// points there; otherwise it falls back to the main repo clone (repos/<name>).
func ReadonlyLink(jeffHome, repoName, taskDir string) (string, error) {
	repoDir := filepath.Join(jeffHome, "repos", repoName)
	if _, err := os.Stat(repoDir); err != nil {
		return "", fmt.Errorf("repo %q not found at %s", repoName, repoDir)
	}

	// Prefer an existing worktree — more likely to have up-to-date work-in-progress.
	target := repoDir
	if wtDir, ok := FindExistingWorktree(jeffHome, repoName); ok {
		target = wtDir
	}

	if taskDir != "" {
		return target, symlinkIntoTask(taskDir, repoName, target)
	}
	return target, nil
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
