package main

import (
	"fmt"
	"os"
	"strings"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/persona"
	"github.com/NeerajG03/JEFF/task"
	"github.com/spf13/cobra"
)

func pickupCmd() *cobra.Command {
	var (
		personaName string
		repos       []string
		safeFlag    bool
		testFlag    bool
	)

	cmd := &cobra.Command{
		Use:   "pickup <gig-id>",
		Short: "Claim a task, set up workspace, and launch agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := args[0]

			// Resolve agent from persona, fall back to global config.
			agentTool := cfg.Agent
			if personaName != "" {
				if pa := persona.RegisteredAgent(cfg.Home, personaName); pa != "" {
					agentTool = jeff.AgentTool(pa)
				}
			}

			// Open the gig store here and close it BEFORE launchAgent blocks for
			// the whole interactive session — otherwise the SQLite DB stays
			// locked for hours. Ordering: open → EnsureAttrs → Pickup → Close →
			// launch (preserving the old pickupTask lifecycle).
			store, err := openGigStore(cfg)
			if err != nil {
				return err
			}
			if err := jeff.EnsureAttrs(store); err != nil {
				store.Close()
				return fmt.Errorf("ensure attrs: %w", err)
			}
			res, err := task.Pickup(store, cfg, task.PickupOpts{
				TaskID:        taskID,
				Persona:       personaName,
				Repos:         repos,
				AgentOverride: agentTool,
			})
			store.Close()
			if err != nil {
				return err
			}

			if testFlag {
				fmt.Fprintf(os.Stderr, "\nTest mode — workspace ready at %s\n", res.TaskDir)
				fmt.Fprintf(os.Stderr, "Verify:\n")
				fmt.Fprintf(os.Stderr, "  • Task dir:   %s\n", res.TaskDir)
				fmt.Fprintf(os.Stderr, "  • CLAUDE.md:  %s\n", res.TaskDir+"/CLAUDE.md")
				fmt.Fprintf(os.Stderr, "  • Worktrees:  ls %s/\n", res.TaskDir)
				for range repos {
					fmt.Fprintf(os.Stderr, "  • Skills:     ls %s/.claude/skills/\n", res.TaskDir)
				}
				return nil
			}

			// Launch agent tool in task directory (foreground, blocks).
			fmt.Fprintf(os.Stderr, "\nLaunching %s in %s...\n", agentTool, res.TaskDir)
			// Resolve persona model for foreground launch.
			model := persona.RegisteredModel(cfg.Home, personaName)
			return launchAgent(res.TaskDir, agentTool, model, personaName, effectiveSkipPermissions(cfg, safeFlag))
		},
	}

	cmd.Flags().StringVar(&personaName, "persona", "", "Persona template to use ("+strings.Join(persona.Names(), ", ")+")")
	cmd.Flags().StringSliceVar(&repos, "repos", nil, "Repos this task touches (creates worktrees)")
	cmd.Flags().BoolVar(&safeFlag, "safe", false, `Launch the agent with its permission prompts enabled (pass "--safe" to override skip_permissions)`)
	cmd.Flags().BoolVar(&testFlag, "test", false, "Prepare the workspace and verify structure, then exit without launching the agent (useful for CI, non-interactive setup, or staging a workspace)")
	cmd.ValidArgsFunction = readyTaskCompletion
	_ = cmd.RegisterFlagCompletionFunc("persona", personaCompletion)
	_ = cmd.RegisterFlagCompletionFunc("repos", repoNameCompletion)
	return cmd
}
