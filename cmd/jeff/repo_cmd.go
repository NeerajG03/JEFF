package main

import (
	"fmt"
	"os"

	"github.com/NeerajG03/JEFF"
	"github.com/spf13/cobra"
)

func repoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Manage registered codebases",
	}
	cmd.AddCommand(repoAddCmd(), repoListCmd(), repoRemoveCmd(), repoPostSetupCmd(), repoDescribeCmd(), repoSyncCmd())
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
				if r.Description != "" {
					fmt.Printf("%-20s %s — %s\n", r.Name, r.URL, r.Description)
				} else {
					fmt.Printf("%-20s %s\n", r.Name, r.URL)
				}
			}
			return nil
		},
	}
}

func repoRemoveCmd() *cobra.Command {
	var deleteFiles bool

	cmd := &cobra.Command{
		Use:               "remove <name>",
		Short:             "Unregister a codebase",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: repoNameCompletion,
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

func repoSyncCmd() *cobra.Command {
	var repoName string

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Pull latest main from origin for all repos (or --repo for one)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoName != "" {
				r, err := jeff.SyncRepo(cfg, repoName)
				if err != nil {
					return err
				}
				printSyncResult(r)
				return nil
			}

			results := jeff.SyncAllRepos(cfg)
			for _, r := range results {
				printSyncResult(r)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repoName, "repo", "", "Sync a specific repo only")
	cmd.RegisterFlagCompletionFunc("repo", repoNameCompletion)
	return cmd
}

func printSyncResult(r *jeff.SyncResult) {
	if r.Err != nil {
		fmt.Fprintf(os.Stderr, "%-20s failed: %v\n", r.Name, r.Err)
	} else if r.Updated {
		fmt.Printf("%-20s updated (%d new commits)\n", r.Name, r.Behind)
	} else {
		fmt.Printf("%-20s already up to date\n", r.Name)
	}
}

func repoDescribeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "describe <name> <description>",
		Short: "Set a description for a repo",
		Args:  cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return repoNameCompletion(cmd, args, toComplete)
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := jeff.SetDescription(cfg, args[0], args[1]); err != nil {
				return err
			}
			fmt.Printf("Set description for %s → %s\n", args[0], args[1])
			return nil
		},
	}
}

func repoPostSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "post-setup <name> <script-path>",
		Short: "Set a post-setup script for worktree creation",
		Long:  "The script receives two arguments: src_dir (repo clone) and dest_dir (new worktree).",
		Args:  cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return repoNameCompletion(cmd, args, toComplete)
			}
			// Second arg is a file path — let the shell handle it.
			return nil, cobra.ShellCompDirectiveDefault
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := jeff.SetPostSetup(cfg, args[0], args[1]); err != nil {
				return err
			}
			fmt.Printf("Set post-setup for %s → %s\n", args[0], args[1])
			return nil
		},
	}
}
