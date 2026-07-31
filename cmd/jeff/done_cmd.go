package main

import (
	"github.com/NeerajG03/JEFF/task"
	"github.com/spf13/cobra"
)

func doneCmd() *cobra.Command {
	var reason string
	var force bool
	var purge bool

	cmd := &cobra.Command{
		Use:   "done [gig-id]",
		Short: "Close a task and clean up its workspace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, _, err := resolveTaskID(args)
			if err != nil {
				return err
			}

			store, err := openGigStore(cfg)
			if err != nil {
				return err
			}
			defer store.Close()

			return task.Teardown(store, cfg, task.TeardownOpts{
				TaskID: taskID,
				Reason: reason,
				Force:  force,
				Purge:  purge,
			})
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "done", "Close reason")
	cmd.Flags().BoolVar(&force, "force", false, "Discard uncommitted worktree changes instead of refusing to close")
	cmd.Flags().BoolVar(&purge, "purge", false, "Delete the task workspace dir too (breaks hooks in a session anchored to it)")
	cmd.ValidArgsFunction = activeTaskCompletion
	return cmd
}
