package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/persona"
	"github.com/NeerajG03/JEFF/task"
	"github.com/NeerajG03/JEFF/workspace"
	"github.com/spf13/cobra"
)

func pickupCmd() *cobra.Command {
	var (
		personaName   string
		agentOverride string
		repos         []string
		testFlag      bool
	)

	cmd := &cobra.Command{
		Use:   "pickup <gig-id>",
		Short: "Claim a task, set up workspace, and launch agent",
		Long: `Claim a gig task, set up its workspace with worktrees, hooks, skills,
and memory, then launch the configured agent.

Use --test to prepare the workspace and verify everything is correct without
starting the agent.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := args[0]

			// Resolve agent: --agent flag > persona > global config.
			agentTool := cfg.Agent
			if agentOverride != "" {
				agentTool = jeff.AgentTool(agentOverride)
				if !agentTool.IsValid() {
					return fmt.Errorf("unknown agent %q (valid: %s)", agentOverride, strings.Join(jeff.AgentTool("").ValidNames(), ", "))
				}
			} else if personaName != "" {
				if pa := persona.RegisteredAgent(cfg.Home, personaName); pa != "" {
					agentTool = jeff.AgentTool(pa)
				}
			}

			// Open the gig store here and close it BEFORE launchAgent blocks for
			// the whole interactive session — otherwise the SQLite DB stays
			// locked for hours. Ordering: open -> EnsureAttrs -> Pickup -> Close ->
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

			// --test mode: verify workspace and exit (implies no launch).
			if testFlag {
				return verifyPickup(res.TaskDir, agentTool)
			}

			// Launch agent tool in task directory (foreground, blocks).
			fmt.Fprintf(os.Stderr, "\nLaunching %s in %s...\n", agentTool, res.TaskDir)
			model := persona.RegisteredModel(cfg.Home, personaName)
			if err := launchAgent(res.TaskDir, agentTool, model, personaName, effectiveSkipPermissions(cfg, false)); err != nil {
				fmt.Fprintf(os.Stderr, "Error: Agent launch failed: %v\n", err)
				fmt.Fprintf(os.Stderr, "The workspace at %s was set up successfully — you can reopen it with: jeff open %s\n", res.TaskDir, taskID)
				return &exitCode{code: 1}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&personaName, "persona", "", "Persona template to use ("+strings.Join(persona.Names(), ", ")+")")
	cmd.Flags().StringVar(&agentOverride, "agent", "", "Agent backend ("+strings.Join(jeff.AgentTool("").ValidNames(), ", ")+"; default: config agent)")
	cmd.Flags().StringSliceVar(&repos, "repos", nil, "Repos this task touches (creates worktrees)")
	cmd.Flags().BoolVar(&testFlag, "test", false, "Prepare workspace, verify correctness, and skip agent launch")
	cmd.ValidArgsFunction = readyTaskCompletion
	_ = cmd.RegisterFlagCompletionFunc("persona", personaCompletion)
	_ = cmd.RegisterFlagCompletionFunc("agent", agentCompletion)
	_ = cmd.RegisterFlagCompletionFunc("repos", repoNameCompletion)
	return cmd
}

// verifyPickup checks that the workspace was set up correctly.
func verifyPickup(taskDir string, agent jeff.AgentTool) error {
	var failures []string
	numWorktrees := 0

	info, err := os.Stat(taskDir)
	if err != nil {
		failures = append(failures, fmt.Sprintf("workspace directory: %v", err))
	} else if !info.IsDir() {
		failures = append(failures, "workspace path is not a directory")
	}

	claudePath := filepath.Join(taskDir, "CLAUDE.md")
	info, err = os.Stat(claudePath)
	if err != nil {
		failures = append(failures, fmt.Sprintf("CLAUDE.md: %v", err))
	} else if info.Size() == 0 {
		failures = append(failures, "CLAUDE.md is empty")
	}

	wts, err := workspace.ListTaskWorktrees(taskDir)
	if err != nil {
		failures = append(failures, fmt.Sprintf("list worktrees: %v", err))
	} else {
		numWorktrees = len(wts)
		for _, wt := range wts {
			if _, err := os.Stat(wt.Path); err != nil {
				failures = append(failures, fmt.Sprintf("worktree %s: symlink target missing: %v", wt.Repo, err))
			}
		}
	}

	if p := jeff.GetProvider(agent); p != nil {
		configDir := filepath.Join(taskDir, p.ConfigDir())
		if _, err := os.Stat(configDir); err != nil {
			failures = append(failures, fmt.Sprintf("agent config dir (%s): %v", p.ConfigDir(), err))
		}
	}

	if len(failures) > 0 {
		fmt.Fprintf(os.Stderr, "\nVerification failed (%d issue(s)):\n", len(failures))
		for _, f := range failures {
			fmt.Fprintf(os.Stderr, "  - %s\n", f)
		}
		fmt.Fprintf(os.Stderr, "\nWorkspace: %s\n", taskDir)
		return &exitCode{code: 1}
	}

	fmt.Fprintf(os.Stderr, "\nVerification passed\n")
	fmt.Fprintf(os.Stderr, "  Workspace: %s\n", taskDir)
	if numWorktrees > 0 {
		fmt.Fprintf(os.Stderr, "  Worktrees: %d\n", numWorktrees)
	}
	return nil
}
