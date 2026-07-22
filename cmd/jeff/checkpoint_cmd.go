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
		taskFlag  string
	)

	cmd := &cobra.Command{
		Use:   "checkpoint",
		Short: "Save a structured progress snapshot to the current task",
		RunE: func(cmd *cobra.Command, args []string) error {
			var taskID string
			if taskFlag != "" {
				taskID = taskFlag
			} else {
				id, _, err := resolveTaskID(nil)
				if err != nil {
					return fmt.Errorf("cannot detect task: %w\nUse --task to specify explicitly", err)
				}
				taskID = id
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

	cmd.Flags().StringVar(&taskFlag, "task", "", "Task ID (auto-detected from cwd if omitted)")
	cmd.Flags().StringVar(&done, "done", "", "What was accomplished (required)")
	cmd.Flags().StringVar(&decisions, "decisions", "", "Key decisions and reasoning")
	cmd.Flags().StringVar(&next, "next", "", "What should happen next")
	cmd.Flags().StringVar(&blockers, "blockers", "", "Current blockers")
	cmd.Flags().StringSliceVar(&files, "files", nil, "File paths touched or referenced")
	_ = cmd.MarkFlagRequired("done")
	_ = cmd.RegisterFlagCompletionFunc("task", activeTaskCompletion)
	return cmd
}
