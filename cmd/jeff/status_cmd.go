package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/NeerajG03/JEFF/workspace"
	"github.com/NeerajG03/gig"
	"github.com/spf13/cobra"
)

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Overview of all active tasks and their workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openGigStore()
			if err != nil {
				return err
			}
			defer store.Close()

			// List task workspaces.
			dirs, err := workspace.List(cfg.Home)
			if err != nil {
				return fmt.Errorf("list workspaces: %w", err)
			}

			if len(dirs) == 0 {
				fmt.Println("No active task workspaces.")
				return nil
			}

			for _, td := range dirs {
				// Get task info from gig.
				task, err := store.Get(td.TaskID)
				if err != nil {
					fmt.Printf("%-16s  (task not found in gig)\n", td.TaskID)
					continue
				}

				statusIcon := statusIconFor(task.Status)
				fmt.Printf("%s %-16s %s\n", statusIcon, td.TaskID, task.Title)

				// Check for dirty worktrees.
				entries, _ := os.ReadDir(td.Path)
				for _, e := range entries {
					if !isSymlink(filepath.Join(td.Path, e.Name())) {
						continue
					}
					target, err := os.Readlink(filepath.Join(td.Path, e.Name()))
					if err != nil {
						continue
					}
					dirty := isGitDirty(target)
					dirtyMark := ""
					if dirty {
						dirtyMark = " (dirty)"
					}
					fmt.Printf("  └── %s → %s%s\n", e.Name(), target, dirtyMark)
				}

				// Show latest checkpoint if any.
				cp, _ := store.LatestCheckpoint(td.TaskID)
				if cp != nil {
					fmt.Printf("  last checkpoint: %s — %s\n", cp.CreatedAt.Format("2006-01-02 15:04"), cp.Done)
				}
			}

			return nil
		},
	}
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

func isSymlink(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeSymlink != 0
}

func isGitDirty(dir string) bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(out) > 0
}
