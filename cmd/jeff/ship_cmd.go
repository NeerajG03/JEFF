package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	jeff "github.com/NeerajG03/JEFF"
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

			if !dryRun {
				if _, err := exec.LookPath("gh"); err != nil {
					return fmt.Errorf("gh CLI not found — jeff ship needs it to create PRs.\nInstall: https://cli.github.com, then run 'gh auth login'.\n(Use --dry-run to preview without gh.)")
				}
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
			var results []shipResult
			for _, wt := range worktrees {
				fmt.Fprintf(os.Stderr, "\n── %s (branch: %s → %s)\n", wt.repo, wt.branch, wt.base)

				res := shipResult{repo: wt.repo}

				if dirty, lines := countDirty(wt.wtDir); dirty > 0 {
					res.dirty = dirty
					fmt.Fprintf(os.Stderr, "  WARNING: %d uncommitted change(s) will NOT be shipped:\n", dirty)
					for i, l := range lines {
						if i == 5 {
							fmt.Fprintf(os.Stderr, "    …and %d more\n", dirty-5)
							break
						}
						fmt.Fprintf(os.Stderr, "    %s\n", l)
					}
				}

				if tracked, err := jeffBaseTracked(wt.wtDir); err == nil && tracked {
					fmt.Fprintf(os.Stderr, "  WARNING: .jeff-base is tracked in this repo — run 'git rm --cached .jeff-base' to stop shipping it in PRs.\n")
				}

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
						res.prURL = url
					}
					res.skipped = true
					results = append(results, res)
					continue
				}

				if dryRun {
					fmt.Fprintf(os.Stderr, "  [dry-run] Would push %s and create PR → %s\n", wt.branch, wt.base)
					fmt.Fprintf(os.Stderr, "  [dry-run] Title: %s\n", title)
					res.pushed = true
					results = append(results, res)
					continue
				}

				// Push.
				if err := pushBranch(wt.wtDir, wt.branch); err != nil {
					fmt.Fprintf(os.Stderr, "  Push failed: %v\n", err)
					res.err = fmt.Errorf("push failed: %w", err)
					results = append(results, res)
					continue
				}
				res.pushed = true
				fmt.Fprintf(os.Stderr, "  Pushed %s\n", wt.branch)

				// Check for existing PR.
				url, _ := prExists(wt.wtDir, wt.branch)
				if url != "" {
					fmt.Fprintf(os.Stderr, "  PR exists: %s\n", url)
					res.prURL = url
					results = append(results, res)
					continue
				}

				// Create PR.
				prURL, err := createPR(wt.wtDir, wt.branch, wt.base, title, body, draft)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  PR creation failed: %v\n", err)
					res.err = fmt.Errorf("PR creation failed: %w", err)
					results = append(results, res)
					continue
				}
				fmt.Fprintf(os.Stderr, "  PR created: %s\n", prURL)
				res.prURL = prURL
				res.newPR = true
				results = append(results, res)
			}

			summary, shipErr := summarizeShip(results)
			fmt.Fprintf(os.Stderr, "\n%s\n", summary)

			if dryRun {
				return nil
			}

			if err := recordShipResults(store, taskID, results); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}

			return shipErr
		},
	}

	cmd.Flags().StringVar(&repoFilter, "repo", "", "Ship only this repo (default: all)")
	cmd.Flags().BoolVar(&draft, "draft", false, "Create draft PRs")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would happen without acting")
	cmd.Flags().StringVar(&prTitle, "title", "", "Override PR title")
	cmd.Flags().StringVar(&prBody, "body", "", "Override PR body")
	cmd.ValidArgsFunction = activeTaskCompletion
	_ = cmd.RegisterFlagCompletionFunc("repo", repoNameCompletion)

	return cmd
}

// shipWorktree holds the info needed to ship one worktree.
type shipWorktree struct {
	repo   string // symlink name (repo name)
	branch string // git branch
	base   string // PR target branch
	wtDir  string // resolved worktree path
}

// discoverWorktrees finds worktree symlinks in the task dir and resolves their
// branches. It adapts workspace.ListTaskWorktrees, keeping ship's own concerns:
// the repo filter, the .jeff-base self-heal, and stripping "origin/" from the
// PR base branch.
func discoverWorktrees(taskDir, repoFilter string) ([]shipWorktree, error) {
	wts, err := workspace.ListTaskWorktrees(taskDir)
	if err != nil {
		return nil, err
	}

	var result []shipWorktree
	for _, wt := range wts {
		if repoFilter != "" && wt.Repo != repoFilter {
			continue
		}

		if err := workspace.EnsureJeffBaseExcluded(wt.Path); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: exclude .jeff-base for %s: %v\n", wt.Repo, err)
		}

		if wt.Branch == "" {
			fmt.Fprintf(os.Stderr, "Warning: could not detect branch for %s\n", wt.Repo)
			continue
		}

		// Strip "origin/" prefix from base for PR target.
		prBase := wt.Base
		if after, ok := strings.CutPrefix(wt.Base, "origin/"); ok {
			prBase = after
		}

		result = append(result, shipWorktree{
			repo:   wt.Repo,
			branch: wt.Branch,
			base:   prBase,
			wtDir:  wt.Path,
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

// shipResult records the outcome of shipping one worktree.
type shipResult struct {
	repo    string
	prURL   string
	err     error
	dirty   int // count of uncommitted paths
	pushed  bool
	newPR   bool // a PR was created this run (vs. pre-existing)
	skipped bool // nothing to push
}

// countDirty returns the number of uncommitted paths in a worktree and their
// `git status --porcelain` lines, so ship can warn about work that won't ship.
func countDirty(wtDir string) (int, []string) {
	out, err := gitutil.Output(wtDir, "status", "--porcelain")
	if err != nil || len(bytes.TrimSpace(out)) == 0 {
		return 0, nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return len(lines), lines
}

// jeffBaseTracked reports whether .jeff-base is committed to the repo. The
// exclude file only hides untracked files — if .jeff-base was already
// committed on an old branch, it still ships in the PR.
func jeffBaseTracked(wtDir string) (bool, error) {
	cmd := exec.Command("git", "ls-files", "--error-unmatch", ".jeff-base")
	cmd.Dir = wtDir
	if err := cmd.Run(); err != nil {
		return false, nil
	}
	return true, nil
}

// summarizeShip builds the "Shipped X, skipped Y, failed Z" summary line and
// returns a non-nil error naming every failed repo when at least one exists.
func summarizeShip(results []shipResult) (string, error) {
	var shipped, skipped int
	var failed []string
	for _, r := range results {
		switch {
		case r.err != nil:
			failed = append(failed, fmt.Sprintf("%s: %v", r.repo, r.err))
		case r.skipped:
			skipped++
		default:
			shipped++
		}
	}

	summary := fmt.Sprintf("Shipped %d, skipped %d, failed %d", shipped, skipped, len(failed))
	if len(failed) > 0 {
		return summary, fmt.Errorf("ship incomplete:\n  %s", strings.Join(failed, "\n  "))
	}
	return summary, nil
}

// recordShipResults writes newly-created PR URLs back into gig: an attr
// (repo -> PR URL) plus a comment, so stats/orchestrators can see ship status.
// A comment is only posted when at least one PR is new this run, so re-running
// `jeff ship` on an already-shipped task doesn't spam duplicate comments.
func recordShipResults(store *gig.Store, taskID string, results []shipResult) error {
	prURLs := map[string]string{}
	hasNew := false
	for _, r := range results {
		if r.prURL != "" {
			prURLs[r.repo] = r.prURL
		}
		if r.newPR {
			hasNew = true
		}
	}
	if len(prURLs) == 0 {
		return nil
	}

	if err := jeff.EnsureAttrs(store); err != nil {
		return fmt.Errorf("ensure attrs: %w", err)
	}
	data, err := json.Marshal(prURLs)
	if err != nil {
		return fmt.Errorf("marshal PR URLs: %w", err)
	}
	if err := store.SetAttr(taskID, jeff.AttrPRURLs, string(data)); err != nil {
		return fmt.Errorf("record PR URLs: %w", err)
	}

	if !hasNew {
		return nil
	}

	var lines []string
	for repo, url := range prURLs {
		lines = append(lines, fmt.Sprintf("%s: %s", repo, url))
	}
	sort.Strings(lines)
	if _, err := store.AddComment(taskID, "jeff", "Shipped:\n"+strings.Join(lines, "\n")); err != nil {
		return fmt.Errorf("record ship comment: %w", err)
	}
	return nil
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
