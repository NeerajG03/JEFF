package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/NeerajG03/JEFF/internal/gitutil"
	"github.com/NeerajG03/JEFF/workspace"
	"github.com/NeerajG03/gig"
	"github.com/spf13/cobra"
)

func shipCmd() *cobra.Command {
	var (
		repoFilter string
		draft      bool
		dryRun     bool
		prTitle    string
		prBody     string
	)

	cmd := &cobra.Command{
		Use:   "ship [gig-id]",
		Short: "Push branches and create PRs for all repos on a task",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, taskDir, err := resolveTaskID(args)
			if err != nil {
				return err
			}
			if taskDir == "" {
				return fmt.Errorf("no workspace found for %s", taskID)
			}

			store, err := openGigStore()
			if err != nil {
				return err
			}
			defer store.Close()

			task, err := store.GetFull(taskID)
			if err != nil {
				return fmt.Errorf("get task: %w", err)
			}

			// Discover worktrees in the task dir.
			worktrees, err := discoverWorktrees(taskDir, repoFilter)
			if err != nil {
				return err
			}
			if len(worktrees) == 0 {
				fmt.Println("No worktrees found to ship.")
				return nil
			}

			// Build PR content.
			title := prTitle
			if title == "" {
				title = buildPRTitle(task)
			}
			body := prBody
			if body == "" {
				body = buildPRBody(store, task)
			}

			// Ship each worktree.
			var shipped, skipped int
			for _, wt := range worktrees {
				fmt.Fprintf(os.Stderr, "\n── %s (branch: %s → %s)\n", wt.repo, wt.branch, wt.base)

				has, err := hasUnpushedCommits(wt.wtDir, wt.branch)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  Warning: check commits: %v\n", err)
				}

				if !has {
					fmt.Fprintf(os.Stderr, "  Nothing to push.\n")

					// Still check if we need to create a PR for already-pushed commits.
					url, _ := prExists(wt.wtDir, wt.branch)
					if url != "" {
						fmt.Fprintf(os.Stderr, "  PR exists: %s\n", url)
					}
					skipped++
					continue
				}

				if dryRun {
					fmt.Fprintf(os.Stderr, "  [dry-run] Would push %s and create PR → %s\n", wt.branch, wt.base)
					fmt.Fprintf(os.Stderr, "  [dry-run] Title: %s\n", title)
					shipped++
					continue
				}

				// Push.
				if err := pushBranch(wt.wtDir, wt.branch); err != nil {
					fmt.Fprintf(os.Stderr, "  Push failed: %v\n", err)
					continue
				}
				fmt.Fprintf(os.Stderr, "  Pushed %s\n", wt.branch)

				// Check for existing PR.
				url, _ := prExists(wt.wtDir, wt.branch)
				if url != "" {
					fmt.Fprintf(os.Stderr, "  PR exists: %s\n", url)
					shipped++
					continue
				}

				// Create PR.
				prURL, err := createPR(wt.wtDir, wt.branch, wt.base, title, body, draft)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  PR creation failed: %v\n", err)
					continue
				}
				fmt.Fprintf(os.Stderr, "  PR created: %s\n", prURL)
				shipped++
			}

			fmt.Fprintf(os.Stderr, "\nShipped %d, skipped %d\n", shipped, skipped)
			return nil
		},
	}

	cmd.Flags().StringVar(&repoFilter, "repo", "", "Ship only this repo (default: all)")
	cmd.Flags().BoolVar(&draft, "draft", false, "Create draft PRs")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would happen without acting")
	cmd.Flags().StringVar(&prTitle, "title", "", "Override PR title")
	cmd.Flags().StringVar(&prBody, "body", "", "Override PR body")

	return cmd
}

// shipWorktree holds the info needed to ship one worktree.
type shipWorktree struct {
	repo   string // symlink name (repo name)
	branch string // git branch
	base   string // PR target branch
	wtDir  string // resolved worktree path
}

// discoverWorktrees finds worktree symlinks in the task dir and resolves their branches.
func discoverWorktrees(taskDir, repoFilter string) ([]shipWorktree, error) {
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		return nil, fmt.Errorf("read task dir: %w", err)
	}

	var result []shipWorktree
	for _, e := range entries {
		fullPath := filepath.Join(taskDir, e.Name())
		if !gitutil.IsSymlink(fullPath) {
			continue
		}

		if repoFilter != "" && e.Name() != repoFilter {
			continue
		}

		target, err := os.Readlink(fullPath)
		if err != nil {
			continue
		}

		branch := filepath.Base(target)
		base := workspace.ReadBaseBranch(target)

		// Strip "origin/" prefix from base for PR target.
		prBase := base
		if after, ok := strings.CutPrefix(base, "origin/"); ok {
			prBase = after
		}

		result = append(result, shipWorktree{
			repo:   e.Name(),
			branch: branch,
			base:   prBase,
			wtDir:  target,
		})
	}

	if repoFilter != "" && len(result) == 0 {
		return nil, fmt.Errorf("repo %q not found as a worktree in this task", repoFilter)
	}

	return result, nil
}

// buildPRTitle generates a PR title from the task.
func buildPRTitle(task *gig.Task) string {
	title := fmt.Sprintf("[%s] %s", task.ID, task.Title)
	if len(title) > 120 {
		title = title[:117] + "..."
	}
	return title
}

// buildPRBody generates a PR body from the task description and latest checkpoint.
func buildPRBody(store *gig.Store, task *gig.Task) string {
	var sb strings.Builder

	if task.Description != "" {
		sb.WriteString(task.Description)
		sb.WriteString("\n\n")
	}

	cp, _ := store.LatestCheckpoint(task.ID)
	if cp != nil {
		sb.WriteString("## Progress\n\n")
		if cp.Done != "" {
			sb.WriteString("**Done:** " + cp.Done + "\n")
		}
		if cp.Decisions != "" {
			sb.WriteString("**Decisions:** " + cp.Decisions + "\n")
		}
		if cp.Next != "" {
			sb.WriteString("**Next:** " + cp.Next + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("---\nTask: " + task.ID)
	return sb.String()
}

// hasUnpushedCommits checks if the branch has commits not yet on the remote.
func hasUnpushedCommits(wtDir, branch string) (bool, error) {
	out, err := gitutil.Output(wtDir, "log", "origin/"+branch+"..HEAD", "--oneline")
	if err != nil {
		// If origin/branch doesn't exist, all local commits are unpushed.
		return true, nil
	}
	return len(bytes.TrimSpace(out)) > 0, nil
}

// pushBranch pushes the current branch to origin.
func pushBranch(wtDir, branch string) error {
	return gitutil.Run(wtDir, "push", "-u", "origin", branch)
}

// prExists checks if a PR already exists for the branch. Returns the URL or "".
func prExists(wtDir, branch string) (string, error) {
	cmd := exec.Command("gh", "pr", "list", "--head", branch, "--json", "url", "--jq", ".[0].url")
	cmd.Dir = wtDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	url := strings.TrimSpace(string(out))
	return url, nil
}

// createPR creates a pull request via gh CLI.
func createPR(wtDir, branch, base, title, body string, draft bool) (string, error) {
	args := []string{"pr", "create", "--head", branch, "--base", base, "--title", title, "--body", body}
	if draft {
		args = append(args, "--draft")
	}
	cmd := exec.Command("gh", args...)
	cmd.Dir = wtDir

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%w: %s", err, string(exitErr.Stderr))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
