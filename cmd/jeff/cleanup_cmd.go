package main

import (
	"fmt"
	"time"

	"github.com/NeerajG03/JEFF/crew"
	"github.com/NeerajG03/JEFF/task"
	"github.com/spf13/cobra"
)

// cleanupCmd collects what `jeff done` deliberately leaves behind.
//
// `done` retires a task workspace rather than deleting it, because the invoking
// agent session is anchored to that directory and its hook scripts live inside it
// (#94). This is where those ~20 KB directories are finally collected — and where
// the expensive garbage `done` could not remove gets picked up too: worktrees left
// behind by a close that refused on uncommitted changes or died part-way.
func cleanupCmd() *cobra.Command {
	var dryRun, force bool
	var olderThan time.Duration

	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Collect retired task workspaces and orphaned worktrees",
		Long: `Collect finished workspaces and reclaim disk.

Removes:
  • retired task workspaces (closed by ` + "`jeff done`" + `, no live worker anchored,
    older than --older-than)
  • orphaned worktrees whose task is closed — the actual disk cost

Never removes a worktree with uncommitted changes unless --force is given, and
never removes a workspace a live worker is anchored to.

Also reconciles crew state (dead tmux windows, stale session rows), the same
sweep as ` + "`jeff crew cleanup`" + `.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			store, err := openGigStore(cfg)
			if err != nil {
				return err
			}
			defer store.Close()

			res, err := task.GC(store, cfg, task.GCOpts{
				DryRun: dryRun,
				Force:  force,
				MinAge: olderThan,
			})
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			verb := "Removed"
			if dryRun {
				verb = "Would remove"
			}

			printGCGroup(cmd, fmt.Sprintf("%s task workspaces", verb), res.Workspaces)
			printGCGroup(cmd, fmt.Sprintf("%s orphaned worktrees", verb), res.Worktrees)
			printGCGroup(cmd, "Kept — live worker anchored", res.SkippedLive)
			printGCGroup(cmd, "Kept — task still open", res.SkippedOpen)
			printGCGroup(cmd, "Kept — inside grace period", res.SkippedTooNew)
			printGCGroup(cmd, "Kept — UNCOMMITTED CHANGES (pass --force to discard)", res.SkippedDirty)

			total := len(res.Workspaces) + len(res.Worktrees)
			if total == 0 {
				fmt.Fprintln(out, "Nothing to collect.")
			} else {
				fmt.Fprintf(out, "\n%s %d item(s), %s reclaimed.\n", verb, total, humanBytes(res.BytesReclaimed))
			}
			if res.BytesRecoverable > 0 {
				fmt.Fprintf(out, "%s held by worktrees with uncommitted changes (review, then --force).\n",
					humanBytes(res.BytesRecoverable))
			}

			// Crew reconciliation, so one command leaves the whole home tidy.
			if cs, err := crew.Open(cfg.Home); err == nil {
				defer cs.Close()
				if cr, err := crew.Cleanup(cs, cfg.Home, dryRun); err == nil {
					n := len(cr.OrphanedWindows) + len(cr.StaleSessions) + len(cr.StaleOrch)
					if n > 0 {
						fmt.Fprintf(out, "Crew: reconciled %d item(s).\n", n)
					}
					if cr.SkippedNoState {
						fmt.Fprintln(out, "Crew: orphan sweep skipped — the crew DB has no state while worker windows exist (likely a different JEFF_HOME).")
					}
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report what would be collected without removing anything")
	cmd.Flags().BoolVar(&force, "force", false, "Also remove orphaned worktrees with uncommitted changes (discards that work)")
	cmd.Flags().DurationVar(&olderThan, "older-than", 0,
		fmt.Sprintf("Grace period before a retired workspace is collected (default %s; 0 uses the default, negative disables)", task.DefaultGCMinAge))
	return cmd
}

func printGCGroup(cmd *cobra.Command, title string, items []task.GCItem) {
	if len(items) == 0 {
		return
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%s (%d):\n", title, len(items))
	for _, it := range items {
		fmt.Fprintf(out, "  %-10s %s  %s\n", it.TaskID, humanBytes(it.Bytes), it.Path)
		if it.Reason != "" {
			fmt.Fprintf(out, "             %s\n", it.Reason)
		}
	}
}

// humanBytes renders a byte count for humans reading a cleanup report.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KB", "MB", "GB", "TB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PB", value/unit)
}
