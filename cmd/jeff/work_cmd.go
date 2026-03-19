package main

import (
	"fmt"
	"os"

	"github.com/NeerajG03/JEFF/workspace"
	"github.com/spf13/cobra"
)

func workCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "work [gig-id]",
		Short: "Resume work on an existing task — launch agent in task dir",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			td, err := workspace.Open(cfg.Home, args[0])
			if err != nil {
				return fmt.Errorf("no workspace for %s: %w", args[0], err)
			}

			fmt.Fprintf(os.Stderr, "Resuming %s in %s...\n", args[0], td.Path)
			return launchAgent(td.Path, cfg.Agent)
		},
	}
}
