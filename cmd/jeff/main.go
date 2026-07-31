package main

import (
	"errors"
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

			// NOTE: this path must never write the home pointer. It used to
			// "self-heal" it on every command, which meant a one-shot
			// `JEFF_HOME=/tmp/x jeff status` permanently repointed the pointer
			// file for every future shell — a transient override promoting
			// itself to the persistent default. The pointer is written only by
			// the selection path: `jeff init` and `jeff home use`.

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
		homeCmd(),
		cleanupCmd(),
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
		var ece *exitCode
		if errors.As(err, &ece) {
			if ece.msg != "" {
				fmt.Fprintln(os.Stderr, ece.msg)
			}
			os.Exit(ece.code)
		}
		os.Exit(1)
	}
}
