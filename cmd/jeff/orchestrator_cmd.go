package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/NeerajG03/JEFF/crew"
	"github.com/spf13/cobra"
)

func orchestratorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "orchestrator",
		Short: "Create and manage orchestrator sessions",
		Long:  "Launch a new orchestrator tmux session (jeff-N) with Claude Code, then start workers as additional tabs.",
	}

	cmd.AddCommand(
		orchestratorStartCmd(),
		orchestratorListCmd(),
		orchestratorAttachCmd(),
	)

	return cmd
}

func orchestratorStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Launch a new orchestrator session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return fmt.Errorf("open crew store: %w", err)
			}
			defer cs.Close()

			orch, err := crew.StartOrchestrator(cs, cfg.Home, string(cfg.Agent))
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Orchestrator %s started (tmux session: %s)\n", orch.ID, orch.TmuxSession)
			fmt.Fprintf(os.Stderr, "Attach with: jeff orchestrator attach %s\n", orch.ID)
			fmt.Fprintf(os.Stderr, "Start workers with: jeff crew start <task-id> --orchestrator %s\n", orch.ID)

			data, _ := json.Marshal(orch)
			fmt.Println(string(data))
			return nil
		},
	}
}

func orchestratorListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List orchestrator sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			orchs, err := cs.ListOrchestrators(false)
			if err != nil {
				return err
			}

			if len(orchs) == 0 {
				fmt.Fprintln(os.Stderr, "(no orchestrator sessions)")
				return nil
			}

			fmt.Fprintf(os.Stdout, "%-12s %-12s %-10s %s\n", "ID", "SESSION", "STATUS", "STARTED")
			for _, o := range orchs {
				started := relativeTime(o.StartedAt)
				fmt.Fprintf(os.Stdout, "%-12s %-12s %-10s %s\n", o.ID, o.TmuxSession, o.Status, started)
			}
			return nil
		},
	}
}

func orchestratorAttachCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "attach <orchestrator-id>",
		Short: "Attach to an orchestrator's tmux session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			orch, err := cs.GetOrchestrator(args[0])
			if err != nil {
				return fmt.Errorf("orchestrator not found: %w", err)
			}

			fmt.Fprintf(os.Stderr, "Attaching to %s...\n", orch.TmuxSession)
			return crew.AttachToSession(orch.TmuxSession, "")
		},
	}
}
