package main

import (
	"fmt"
	"os"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/persona"
	"github.com/spf13/cobra"
)

func workCmd() *cobra.Command {
	var safeFlag bool

	cmd := &cobra.Command{
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

			store, err := openGigStore()
			if err != nil {
				return err
			}
			defer store.Close()

			// Regenerate CLAUDE.md (injects latest checkpoint + current memory/worktrees).
			if err := refreshTaskClaudeMD(taskDir, store, taskID); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: refresh task context: %v\n", err)
			}

			// Resolve persona → agent + model, mirroring pickup.
			personaName := resolveTaskPersona(store, taskID, taskDir)
			agentTool := cfg.Agent
			if personaName != "" {
				if pa := persona.RegisteredAgent(cfg.Home, personaName); pa != "" {
					agentTool = jeff.AgentTool(pa)
				}
			}
			model := persona.RegisteredModel(cfg.Home, personaName)

			fmt.Fprintf(os.Stderr, "Resuming %s in %s...\n", taskID, taskDir)
			return launchAgent(taskDir, agentTool, model, personaName, effectiveSkipPermissions(cfg, safeFlag))
		},
	}
	cmd.Flags().BoolVar(&safeFlag, "safe", false, `Launch the agent with its permission prompts enabled (pass "--safe" to override skip_permissions)`)
	cmd.ValidArgsFunction = activeTaskCompletion
	return cmd
}
