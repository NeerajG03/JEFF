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
	var here, update bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize JEFF home directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if update {
				return runUpdate()
			}
			return runInit(here)
		},
	}

	cmd.Flags().BoolVar(&here, "here", false, "Initialize in current directory instead of ~/.jeff/")
	cmd.Flags().BoolVar(&update, "update", false, "Sync existing home (create missing dirs, hooks, settings)")
	cmd.MarkFlagsMutuallyExclusive("here", "update")
	return cmd
}

// runInit performs first-time initialization.
func runInit(here bool) error {
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

	// Block if already initialized elsewhere.
	existing, err := jeff.ResolveHome()
	if err == nil && existing != home {
		if _, err := os.Stat(jeff.ConfigPath(existing)); err == nil {
			return fmt.Errorf("JEFF is already initialized at %s\nRun `jeff init --update` to sync, or remove the pointer: rm ~/.config/jeff/home", existing)
		}
	}

	// Block if already initialized at this location.
	if _, err := os.Stat(jeff.ConfigPath(home)); err == nil {
		return fmt.Errorf("JEFF is already initialized at %s\nRun `jeff init --update` to sync", home)
	}

	// Create directory structure.
	ensureDirs(home)

	// Write default config.
	c := jeff.DefaultConfig()
	c.Home = home
	if err := jeff.SaveConfig(&c); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// Write default files.
	var homePath string
	if here {
		homePath = "jeff/"
	} else {
		homePath = "~/.jeff/"
	}
	if err := jeffembed.WriteClaudeMD(home, homePath, false); err != nil {
		return fmt.Errorf("write CLAUDE.md: %w", err)
	}

	writeDefaults(home)

	// Install hooks.
	if err := syncHomeHooks(home, &c); err != nil {
		return fmt.Errorf("install hooks: %w", err)
	}

	// Write global pointer.
	if err := jeff.WriteHomePointer(home); err != nil {
		return fmt.Errorf("write home pointer: %w", err)
	}

	fmt.Printf("Initialized JEFF at %s\n", home)
	fmt.Println("  repos/      — register codebases with: jeff repo add <url>")
	fmt.Println("  tasks/      — task workspaces created by: jeff pickup <gig-id>")
	fmt.Println("  worktrees/  — git worktrees managed by: jeff worktree add")
	fmt.Println("  exports/    — artifacts and generated files")
	fmt.Println("  CLAUDE.md   — agent instructions (editable)")
	fmt.Println("  hooks/      — session hooks (configure in jeff.json)")
	return nil
}

// runUpdate syncs an existing JEFF home — creates missing dirs, files, and hooks.
func runUpdate() error {
	home, err := jeff.ResolveHome()
	if err != nil {
		return fmt.Errorf("JEFF is not initialized. Run `jeff init` first")
	}
	if _, err := os.Stat(jeff.ConfigPath(home)); err != nil {
		return fmt.Errorf("JEFF is not initialized at %s. Run `jeff init` first", home)
	}

	c, err := jeff.LoadConfig(home)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Ensure all directories exist (picks up new ones added in upgrades).
	ensureDirs(home)

	// Write missing default files (never overwrites existing).
	writeDefaults(home)

	// Sync hooks.
	if err := syncHomeHooks(home, c); err != nil {
		return fmt.Errorf("sync hooks: %w", err)
	}

	// Ensure home pointer.
	if err := jeff.WriteHomePointer(home); err != nil {
		return fmt.Errorf("write home pointer: %w", err)
	}

	fmt.Printf("JEFF updated at %s (dirs, hooks, settings synced)\n", home)
	return nil
}

// ensureDirs creates all expected directories under home (idempotent).
func ensureDirs(home string) {
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
		os.MkdirAll(d, 0o755)
	}
}

// writeDefaults writes default files that don't already exist.
func writeDefaults(home string) {
	writeIfMissing(filepath.Join(home, ".skills", "skills.json"),
		"{\"$schema\":\"https://raw.githubusercontent.com/NeerajG03/JEFF/main/schemas/skills.json\",\"skills\":{}}\n")
	writeIfMissing(filepath.Join(home, ".claude", "settings.json"), jeffembed.DefaultClaudeSettings)
	writeIfMissing(filepath.Join(home, ".claude", "settings.local.json"), "{}\n")
	writeIfMissing(filepath.Join(home, ".opencode", "settings.json"), "{}\n")
	writeIfMissing(filepath.Join(home, ".opencode", "settings.local.json"), "{}\n")
}

// writeIfMissing writes content to path only if the file doesn't exist.
func writeIfMissing(path, content string) {
	if _, err := os.Stat(path); err == nil {
		return
	}
	os.WriteFile(path, []byte(content), 0o644)
}
