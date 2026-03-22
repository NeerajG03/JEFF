package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/NeerajG03/JEFF"
	jeffembed "github.com/NeerajG03/JEFF/embed"
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
				home = filepath.Join(cwd, "jeff")
			} else {
				h, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("get home: %w", err)
				}
				home = filepath.Join(h, ".jeff")
			}

			// Check if JEFF is already initialized — either at the target
			// location or anywhere the resolver can find.
			existing, err := jeff.ResolveHome()
			if err == nil && existing != home {
				// Check if resolved home actually exists on disk.
				if _, err := os.Stat(jeff.ConfigPath(existing)); err == nil {
					return fmt.Errorf("JEFF is already initialized at %s\nTo reinitialize here, first remove the global pointer: rm ~/.config/jeff/home", existing)
				}
			}

			// If target location already has jeff.yaml, just re-sync hooks.
			if _, err := os.Stat(jeff.ConfigPath(home)); err == nil {
				c, err := jeff.LoadConfig(home)
				if err != nil {
					return fmt.Errorf("load config: %w", err)
				}
				if err := syncHomeHooks(home, c); err != nil {
					return fmt.Errorf("sync hooks: %w", err)
				}
				fmt.Printf("JEFF already initialized at %s (hooks synced)\n", home)
				return nil
			}

			// Create directory structure.
			dirs := []string{
				home,
				filepath.Join(home, "repos"),
				filepath.Join(home, "tasks"),
				filepath.Join(home, "worktrees"),
				filepath.Join(home, "exports"),
				filepath.Join(home, "scripts"),
				filepath.Join(home, ".skills"),
				filepath.Join(home, ".claude"),
				filepath.Join(home, ".opencode"),
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
			// Determine display path for the Home section.
			var homePath string
			if here {
				homePath = "jeff/"
			} else {
				homePath = "~/.jeff/"
			}
			if err := jeffembed.WriteClaudeMD(home, homePath, false); err != nil {
				return fmt.Errorf("write CLAUDE.md: %w", err)
			}

			// Write empty skills registry if missing.
			writeIfMissing(filepath.Join(home, ".skills", "skills.json"),
				"{\"$schema\":\"https://raw.githubusercontent.com/NeerajG03/JEFF/main/schemas/skills.json\",\"skills\":{}}\n")

			// Write default agent settings files if missing.
			writeIfMissing(filepath.Join(home, ".claude", "settings.json"), jeffembed.DefaultClaudeSettings)
			writeIfMissing(filepath.Join(home, ".claude", "settings.local.json"), "{}\n")
			writeIfMissing(filepath.Join(home, ".opencode", "settings.json"), "{}\n")
			writeIfMissing(filepath.Join(home, ".opencode", "settings.local.json"), "{}\n")

			// Install hooks.
			if err := syncHomeHooks(home, &c); err != nil {
				return fmt.Errorf("install hooks: %w", err)
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
			fmt.Println("  hooks/      — session hooks (configure in jeff.yaml)")
			return nil
		},
	}

	cmd.Flags().BoolVar(&here, "here", false, "Initialize in current directory instead of ~/.jeff/")
	return cmd
}

// writeIfMissing writes content to path only if the file doesn't exist.
func writeIfMissing(path, content string) {
	if _, err := os.Stat(path); err == nil {
		return
	}
	os.WriteFile(path, []byte(content), 0o644)
}

