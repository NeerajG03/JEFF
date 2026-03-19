package main

import (
	"fmt"

	"github.com/NeerajG03/gig"
	"github.com/spf13/cobra"
)

func checkpointCmd() *cobra.Command {
	var (
		done      string
		decisions string
		next      string
		blockers  string
		files     []string
		taskID    string
	)

	cmd := &cobra.Command{
		Use:   "checkpoint",
		Short: "Save a structured progress snapshot to the current task",
		RunE: func(cmd *cobra.Command, args []string) error {
			if taskID == "" {
				return fmt.Errorf("--task is required (automatic detection from CWD coming soon)")
			}

			store, err := openGigStore()
			if err != nil {
				return err
			}
			defer store.Close()

			cp, err := store.AddCheckpoint(taskID, "jeff", gig.CheckpointParams{
				Done:      done,
				Decisions: decisions,
				Next:      next,
				Blockers:  blockers,
				Files:     files,
			})
			if err != nil {
				return err
			}

			fmt.Printf("Checkpoint added to %s (%s)\n", taskID, cp.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&taskID, "task", "", "Task ID to checkpoint")
	cmd.Flags().StringVar(&done, "done", "", "What was accomplished (required)")
	cmd.Flags().StringVar(&decisions, "decisions", "", "Key decisions and reasoning")
	cmd.Flags().StringVar(&next, "next", "", "What should happen next")
	cmd.Flags().StringVar(&blockers, "blockers", "", "Current blockers")
	cmd.Flags().StringSliceVar(&files, "files", nil, "File paths touched or referenced")
	cmd.MarkFlagRequired("done")
	return cmd
}
