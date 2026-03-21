package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func workCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "work [gig-id]",
		Short: "Resume work on an existing task — launch agent in task dir",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, taskDir, err := resolveTaskID(args)
			if err != nil {
				return err
			}
			if taskDir == "" {
				return fmt.Errorf("no workspace found for %s", taskID)
			}

			fmt.Fprintf(os.Stderr, "Resuming %s in %s...\n", taskID, taskDir)
			return launchAgent(taskDir, cfg.Agent)
		},
	}
}
