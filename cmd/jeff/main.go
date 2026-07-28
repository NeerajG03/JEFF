package main

import (
	"fmt"
	"os"

	"github.com/NeerajG03/JEFF"
	memorycmd "github.com/NeerajG03/JEFF/cmd/jeff/memory"
	"github.com/spf13/cobra"
)

var version = "dev"

var cfg *jeff.Config

func main() {
	rootCmd := &cobra.Command{
		Use:     "jeff",
		Short:   "JEFF — agent workspace manager built on gig",
		Long:    "JEFF supercharges AI agents with structured workspaces, personas, and task lifecycle management.",
		Version: version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Skip config load for commands that must survive an
			// uninitialized home (init, doctor).
			// Use CommandPath to avoid matching subcommands like `project init`.
			switch cmd.CommandPath() {
			case "jeff init", "jeff doctor":
				return nil
			}

			home, err := jeff.ResolveHome()
			if err != nil {
				return fmt.Errorf("resolve JEFF_HOME: %w", err)
			}

			c, err := jeff.LoadConfig(home)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			cfg = c
			jeff.SetOpenCodeModelAliases(cfg.OpenCodeModels)

			// Self-heal: ensure the home pointer exists so it survives
			// upgrades, cache clears, or accidental deletion.
			_ = jeff.WriteHomePointer(home)

			return nil
		},
		// Bare `jeff` with no subcommand: open agent tool at JEFF_HOME.
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg == nil {
				return fmt.Errorf("JEFF is not initialized. Run: jeff init")
			}
			return launchAgent(cfg.Home, cfg.Agent, "", "", effectiveSkipPermissions(cfg, false))
		},
	}

	rootCmd.AddCommand(
		initCmd(),
		pickupCmd(),
		workCmd(),
		doneCmd(),
		memorycmd.Cmd,
		statusCmd(),
		statsCmd(),
		repoCmd(),
		worktreeCmd(),
		checkpointCmd(),
		shipCmd(),
		configCmd(),
		skillCmd(),
		personaCmd(),
		openCmd(),
		projectCmd(),
		completionCmd(),
		crewCmd(),
		orchestratorCmd(),
		dashboardCmd(),
		notifyCmd(),
		doctorCmd(),
		taskCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
