package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/spf13/cobra"
)

func projectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage JEFF projects",
	}

	cmd.AddCommand(projectInitCmd(), projectOpenCmd(), projectListCmd())
	return cmd
}

func projectInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init <name>",
		Short: "Create a new project with a CLAUDE.md",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			projectDir := filepath.Join(cfg.Home, "projects", name)

			if _, err := os.Stat(projectDir); err == nil {
				return fmt.Errorf("project %q already exists at %s", name, projectDir)
			}

			if err := os.MkdirAll(projectDir, 0o755); err != nil {
				return fmt.Errorf("create project dir: %w", err)
			}

			// Ask the user what the project is about.
			fmt.Printf("What is project %q about?\n> ", name)
			reader := bufio.NewReader(os.Stdin)
			description, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("read input: %w", err)
			}
			description = strings.TrimSpace(description)

			// Write CLAUDE.md.
			content := fmt.Sprintf("# %s\n\n%s\n", name, description)
			claudePath := filepath.Join(projectDir, "CLAUDE.md")
			if err := os.WriteFile(claudePath, []byte(content), 0o644); err != nil {
				return fmt.Errorf("write CLAUDE.md: %w", err)
			}

			fmt.Printf("Project %q created at %s\n", name, projectDir)
			fmt.Printf("  CLAUDE.md — edit to add more context\n")
			fmt.Printf("  Open with: jeff project open %s\n", name)
			return nil
		},
	}
}

func projectOpenCmd() *cobra.Command {
	var agentOverride string
	cmd := &cobra.Command{
		Use:               "open <name>",
		Short:             "Open a project in the configured agent",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: projectNameCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			projectDir := filepath.Join(cfg.Home, "projects", name)

			if _, err := os.Stat(projectDir); err != nil {
				return fmt.Errorf("project %q not found — run: jeff project init %s", name, name)
			}

			agentTool := cfg.Agent
			if agentOverride != "" {
				agentTool = jeff.AgentTool(agentOverride)
				if !agentTool.IsValid() {
					return fmt.Errorf("unknown agent %q (valid: %s)", agentOverride, strings.Join(jeff.AgentTool("").ValidNames(), ", "))
				}
			}

			return launchAgent(projectDir, agentTool, "", "", effectiveSkipPermissions(cfg, false))
		},
	}
	cmd.Flags().StringVar(&agentOverride, "agent", "", "Agent backend ("+strings.Join(jeff.AgentTool("").ValidNames(), ", ")+"; default: config agent)")
	_ = cmd.RegisterFlagCompletionFunc("agent", agentCompletion)
	return cmd
}

func projectListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all projects",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectsDir := filepath.Join(cfg.Home, "projects")
			entries, err := os.ReadDir(projectsDir)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Println("No projects yet. Create one with: jeff project init <name>")
					return nil
				}
				return fmt.Errorf("read projects dir: %w", err)
			}

			if len(entries) == 0 {
				fmt.Println("No projects yet. Create one with: jeff project init <name>")
				return nil
			}

			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				fmt.Println(e.Name())
			}
			return nil
		},
	}
}
