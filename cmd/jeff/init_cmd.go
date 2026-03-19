package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/neerajg/JEFF"
	jeffembed "github.com/neerajg/JEFF/embed"
	"github.com/spf13/cobra"
)

func initCmd() *cobra.Command {
	var here bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize JEFF home directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			var home string
			if here {
				cwd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("get cwd: %w", err)
				}
				home = filepath.Join(cwd, ".jeff")
			} else {
				h, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("get home: %w", err)
				}
				home = filepath.Join(h, ".jeff")
			}

			// Check if JEFF is already initialized elsewhere.
			existing, err := jeff.ResolveHome()
			if err == nil && existing != home {
				if _, err := os.Stat(existing); err == nil {
					return fmt.Errorf("JEFF is already initialized at %s\nTo reinitialize here, first remove the global pointer: rm ~/.config/jeff/home", existing)
				}
			}

			// Check if this location is already initialized.
			if _, err := os.Stat(jeff.ConfigPath(home)); err == nil {
				fmt.Printf("JEFF already initialized at %s (skipping)\n", home)
				return nil
			}

			// Create directory structure.
			dirs := []string{
				home,
				filepath.Join(home, "repos"),
				filepath.Join(home, "tasks"),
				filepath.Join(home, "worktrees"),
				filepath.Join(home, "exports"),
			}
			for _, d := range dirs {
				if err := os.MkdirAll(d, 0o755); err != nil {
					return fmt.Errorf("create %s: %w", d, err)
				}
			}

			// Write default config if missing.
			c := jeff.DefaultConfig()
			c.Home = home
			if _, err := os.Stat(jeff.ConfigPath(home)); os.IsNotExist(err) {
				if err := jeff.SaveConfig(&c); err != nil {
					return fmt.Errorf("write config: %w", err)
				}
			}

			// Write default CLAUDE.md if missing.
			if err := jeffembed.WriteClaudeMD(home, false); err != nil {
				return fmt.Errorf("write CLAUDE.md: %w", err)
			}

			// Write global pointer so `jeff` always finds home.
			if err := jeff.WriteHomePointer(home); err != nil {
				return fmt.Errorf("write home pointer: %w", err)
			}

			fmt.Printf("Initialized JEFF at %s\n", home)
			fmt.Println("  repos/      — register codebases with: jeff repo add <url>")
			fmt.Println("  tasks/      — task workspaces created by: jeff pickup <gig-id>")
			fmt.Println("  worktrees/  — git worktrees managed by: jeff worktree add")
			fmt.Println("  exports/    — artifacts and generated files")
			fmt.Println("  CLAUDE.md   — agent instructions (editable)")
			return nil
		},
	}

	cmd.Flags().BoolVar(&here, "here", false, "Initialize in current directory instead of ~/.jeff/")
	return cmd
}
