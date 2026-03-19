package main

import (
	"fmt"

	"github.com/NeerajG03/JEFF"
	"github.com/spf13/cobra"
)

func repoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Manage registered codebases",
	}
	cmd.AddCommand(repoAddCmd(), repoListCmd(), repoRemoveCmd(), repoPostSetupCmd())
	return cmd
}

func repoAddCmd() *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "add <url>",
		Short: "Register and clone a codebase",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := jeff.AddRepo(cfg, args[0], name)
			if err != nil {
				return err
			}
			fmt.Printf("Added repo %s → %s\n", repo.Name, repo.Path)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Short name for the repo (derived from URL if omitted)")
	return cmd
}

func repoListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered codebases",
		RunE: func(cmd *cobra.Command, args []string) error {
			repos := jeff.ListRepos(cfg)
			if len(repos) == 0 {
				fmt.Println("No repos registered. Use: jeff repo add <url>")
				return nil
			}
			for _, r := range repos {
				fmt.Printf("%-20s %s\n", r.Name, r.URL)
			}
			return nil
		},
	}
}

func repoRemoveCmd() *cobra.Command {
	var deleteFiles bool

	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Unregister a codebase",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := jeff.RemoveRepo(cfg, args[0], deleteFiles); err != nil {
				return err
			}
			fmt.Printf("Removed repo %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVar(&deleteFiles, "delete", false, "Also delete the cloned files")
	return cmd
}

func repoPostSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "post-setup <name> <script-path>",
		Short: "Set a post-setup script for worktree creation",
		Long:  "The script receives two arguments: src_dir (repo clone) and dest_dir (new worktree).",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := jeff.SetPostSetup(cfg, args[0], args[1]); err != nil {
				return err
			}
			fmt.Printf("Set post-setup for %s → %s\n", args[0], args[1])
			return nil
		},
	}
}
