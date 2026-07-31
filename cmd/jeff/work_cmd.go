package main

import (
	"fmt"
	"os"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/persona"
	"github.com/NeerajG03/JEFF/task"
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

			store, err := openGigStore(cfg)
			if err != nil {
				return err
			}
			defer store.Close()

			// Resuming makes this workspace live again: clear any retirement marker
			// left by a previous `jeff done`, or `jeff cleanup` would collect the
			// directory out from under this session.
			task.Reactivate(store, taskID, taskDir)

			// Regenerate CLAUDE.md (injects latest checkpoint + current memory/worktrees).
			if err := task.RefreshClaudeMD(store, cfg, taskID, taskDir); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: refresh task context: %v\n", err)
			}

			// Resolve persona → agent + model, mirroring pickup.
			personaName := task.ResolvePersona(store, taskID, taskDir)
			agentTool := cfg.Agent
			if personaName != "" {
				if pa := persona.RegisteredAgent(cfg.Home, personaName); pa != "" {
					agentTool = jeff.AgentTool(pa)
				}
			}
			model := persona.RegisteredModel(cfg.Home, personaName)

			fmt.Fprintf(os.Stderr, "Resuming %s in %s...\n", taskID, taskDir)
			repos := task.DetectRepos(taskDir)
			orchestratorID, _, _ := detectOrchestratorID()
			syncTaskHooks(cfg, taskDir, taskID, personaName, repos, orchestratorID)
			return launchAgent(taskDir, agentTool, model, personaName, effectiveSkipPermissions(cfg, safeFlag))
		},
	}
	cmd.Flags().BoolVar(&safeFlag, "safe", false, `Launch the agent with its permission prompts enabled (pass "--safe" to override skip_permissions)`)
	cmd.ValidArgsFunction = activeTaskCompletion
	return cmd
}
