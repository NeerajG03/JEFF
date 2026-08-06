package main

import (
	"fmt"
	"strings"

	"github.com/NeerajG03/JEFF/internal/gitutil"
	"github.com/NeerajG03/JEFF/workspace"
	"github.com/NeerajG03/gig"
	"github.com/spf13/cobra"
)

func statusCmd() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Overview of active tasks and their workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openGigStore(cfg)
			if err != nil {
				return err
			}
			defer store.Close()

			dirs, err := workspace.ListActive(cfg.Home, store.Prefix())
			if err != nil {
				return fmt.Errorf("list workspaces: %w", err)
			}

			if len(dirs) == 0 {
				fmt.Println("No task workspaces.")
				return nil
			}

			// Partition into active vs stale.
			var active, stale []statusEntry
			for _, td := range dirs {
				task, err := store.Get(td.TaskID)
				if err != nil {
					stale = append(stale, statusEntry{td: td})
					continue
				}
				e := statusEntry{td: td, task: task}
				e.worktrees = discoverWorktreeStatus(td.Path)
				e.checkpoint, _ = store.LatestCheckpoint(td.TaskID)
				if task.Status.IsTerminal() {
					stale = append(stale, e)
				} else {
					active = append(active, e)
				}
			}

			// Print active tasks.
			if len(active) > 0 {
				fmt.Printf("%s %s\n\n", colorize(cBold, "Active tasks"), colorize(cDim, fmt.Sprintf("(%d)", len(active))))
				for i, e := range active {
					printStatusEntry(e)
					if i < len(active)-1 {
						fmt.Println()
					}
				}
			} else {
				fmt.Println(colorize(cDim, "No active tasks."))
			}

			// Print stale section.
			if len(stale) > 0 {
				if len(active) > 0 {
					fmt.Println()
				}

				if all {
					fmt.Printf("\n%s %s\n\n", colorize(cDim, "Completed"), colorize(cDim, fmt.Sprintf("(%d)", len(stale))))
					for i, e := range stale {
						printStatusEntry(e)
						if i < len(stale)-1 {
							fmt.Println()
						}
					}
				} else {
					ids := make([]string, len(stale))
					for i, e := range stale {
						ids[i] = e.td.TaskID
					}
					fmt.Printf("%s %s\n", colorize(cDim, "Stale workspaces:"), colorize(cDim, strings.Join(ids, ", ")))
					fmt.Printf("%s\n", colorize(cDim, "  Use --all to show, or jeff clean to remove."))
				}
			}

			// Dynamic legend — only show what's present.
			allEntries := append(active, stale...)
			fmt.Println()
			printStatusLegend(allEntries)

			return nil
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Show completed/stale task workspaces too")
	return cmd
}

type worktreeStatus struct {
	repo   string
	branch string
	dirty  bool
}

type statusEntry struct {
	td         *workspace.TaskDir
	task       *gig.Task
	worktrees  []worktreeStatus
	checkpoint *gig.Checkpoint
}

func printStatusEntry(e statusEntry) {
	if e.task == nil {
		fmt.Printf("  %s  %s  %s\n",
			colorize(cDim, "[?]"),
			colorize(cDim, e.td.TaskID),
			colorize(cRed, "(not found in gig)"),
		)
		return
	}

	// Task line: [>] gig-ab12  P1  Task title
	fmt.Printf("  %s  %s  %s  %s\n",
		colorStatus(e.task.Status),
		colorize(cCyan, e.task.ID),
		colorPriority(e.task.Priority),
		e.task.Title,
	)

	// Worktrees.
	for i, wt := range e.worktrees {
		connector := "├──"
		if i == len(e.worktrees)-1 && e.checkpoint == nil {
			connector = "└──"
		}

		branchInfo := colorize(cDim, wt.branch)
		state := colorize(cGreen, "clean")
		if wt.dirty {
			state = colorize(cYellow+cBold, "dirty")
		}

		fmt.Printf("      %s %s/  %s  %s\n", colorize(cDim, connector), wt.repo, branchInfo, state)
	}

	// Checkpoint.
	if e.checkpoint != nil {
		connector := "└──"
		ts := e.checkpoint.CreatedAt.Format("01-02 15:04")
		done := e.checkpoint.Done
		if len(done) > 80 {
			done = done[:77] + "..."
		}
		fmt.Printf("      %s %s %s\n",
			colorize(cDim, connector),
			colorize(cDim, ts),
			colorize(cDim, done),
		)
	}
}

// discoverWorktreeStatus finds symlinked worktrees in a task dir with their
// status. It adapts workspace.ListTaskWorktrees, adding status's dirty probe.
func discoverWorktreeStatus(taskDir string) []worktreeStatus {
	wts, err := workspace.ListTaskWorktrees(taskDir)
	if err != nil {
		return nil
	}

	var result []worktreeStatus
	for _, wt := range wts {
		result = append(result, worktreeStatus{
			repo:   wt.Repo,
			branch: wt.Branch,
			dirty:  isGitDirty(wt.Path),
		})
	}
	return result
}

func statusIconFor(s gig.Status) string {
	switch s {
	case gig.StatusOpen:
		return "[ ]"
	case gig.StatusInProgress:
		return "[>]"
	case gig.StatusBlocked:
		return "[!]"
	case gig.StatusDeferred:
		return "[~]"
	case gig.StatusClosed:
		return "[x]"
	case gig.StatusCancelled:
		return "[-]"
	default:
		return "[?]"
	}
}

func isGitDirty(dir string) bool {
	out, err := gitutil.Output(dir, "status", "--porcelain")
	if err != nil {
		return false
	}
	return len(out) > 0
}

func printStatusLegend(entries []statusEntry) {
	// Collect which statuses and worktree states are present.
	statuses := map[gig.Status]bool{}
	hasDirty, hasClean := false, false
	for _, e := range entries {
		if e.task != nil {
			statuses[e.task.Status] = true
		}
		for _, wt := range e.worktrees {
			if wt.dirty {
				hasDirty = true
			} else {
				hasClean = true
			}
		}
	}

	if len(statuses) == 0 {
		return
	}

	var parts []string
	type legendItem struct {
		status gig.Status
		label  string
	}
	order := []legendItem{
		{gig.StatusOpen, "open"},
		{gig.StatusInProgress, "in_progress"},
		{gig.StatusBlocked, "blocked"},
		{gig.StatusDeferred, "deferred"},
		{gig.StatusClosed, "closed"},
		{gig.StatusCancelled, "cancelled"},
	}
	for _, item := range order {
		if statuses[item.status] {
			parts = append(parts, colorStatus(item.status)+" "+item.label)
		}
	}
	if hasClean {
		parts = append(parts, colorize(cGreen, "clean"))
	}
	if hasDirty {
		parts = append(parts, colorize(cYellow+cBold, "dirty"))
	}

	fmt.Printf("%s  %s\n", colorize(cDim, "Legend:"), strings.Join(parts, "  "))
}
