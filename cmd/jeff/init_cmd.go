package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/NeerajG03/JEFF"
	jeffembed "github.com/NeerajG03/JEFF/embed"
	"github.com/NeerajG03/JEFF/memory"
	"github.com/NeerajG03/JEFF/persona"
	"github.com/NeerajG03/JEFF/skill"
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

	// Create context file aliases (e.g. GEMINI.md → CLAUDE.md) at home level.
	for _, agent := range jeff.RegisteredAgents() {
		if p := jeff.GetProvider(agent); p != nil {
			if aliases := p.ContextFileAliases(); len(aliases) > 0 {
				_ = jeffembed.CreateContextAliases(home, aliases)
			}
		}
	}

	// Always alias .gemini/skills → .claude/skills, regardless of whether
	// the gemini agent is registered. Skills should be in sync across agents.
	if err := jeffembed.EnsureGeminiSkillsAlias(home); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: alias .gemini/skills: %v\n", err)
	}
	if err := jeffembed.EnsureOpenCodeSkillsAlias(home); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: alias .opencode/skills: %v\n", err)
	}

	writeDefaults(home)

	// Seed default personas.
	if err := persona.SeedDefaults(home); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: seed personas: %v\n", err)
	}

	// Seed built-in skills.
	if err := skill.SeedDefaults(home); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: seed skills: %v\n", err)
	}

	// Seed persona-tagged embedded skills.
	if err := skill.SeedPersonaSkills(home); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: seed persona skills: %v\n", err)
	}

	// Install hooks.
	if err := syncHomeHooks(home, &c); err != nil {
		return fmt.Errorf("install hooks: %w", err)
	}

	// Write global pointer.
	if err := jeff.WriteHomePointer(home); err != nil {
		return fmt.Errorf("write home pointer: %w", err)
	}

	// Initialize memory subsystem.
	if err := memory.Initialize(home); err != nil {
		return fmt.Errorf("init memory: %w", err)
	}

	fmt.Printf("Initialized JEFF at %s\n", home)
	fmt.Println("  repos/      — register codebases with: jeff repo add <url>")
	fmt.Println("  tasks/      — task workspaces created by: jeff pickup <gig-id>")
	fmt.Println("  worktrees/  — git worktrees managed by: jeff worktree add")
	fmt.Println("  exports/    — artifacts and generated files")
	fmt.Println("  CLAUDE.md   — agent instructions (editable)")
	fmt.Println("  hooks/      — session hooks (configure in jeff.json)")
	fmt.Println("  memory/     — canonical memory (see docs/usage.md)")
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

	// Create context file aliases (e.g. GEMINI.md → CLAUDE.md) at home level.
	for _, agent := range jeff.RegisteredAgents() {
		if p := jeff.GetProvider(agent); p != nil {
			if aliases := p.ContextFileAliases(); len(aliases) > 0 {
				_ = jeffembed.CreateContextAliases(home, aliases)
			}
		}
	}

	// Always alias .gemini/skills → .claude/skills, regardless of whether
	// the gemini agent is registered. Skills should be in sync across agents.
	if err := jeffembed.EnsureGeminiSkillsAlias(home); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: alias .gemini/skills: %v\n", err)
	}
	if err := jeffembed.EnsureOpenCodeSkillsAlias(home); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: alias .opencode/skills: %v\n", err)
	}

	// Seed default personas (adds any new built-in personas, doesn't overwrite existing).
	if err := persona.SeedDefaults(home); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: seed personas: %v\n", err)
	}

	// Refresh built-in skills (updates files, clears persona injection tags).
	if err := skill.SeedDefaults(home); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: seed skills: %v\n", err)
	}

	// Refresh persona-tagged embedded skills.
	if err := skill.SeedPersonaSkills(home); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: seed persona skills: %v\n", err)
	}

	// Sync hooks.
	if err := syncHomeHooks(home, c); err != nil {
		return fmt.Errorf("sync hooks: %w", err)
	}

	// Ensure home pointer.
	if err := jeff.WriteHomePointer(home); err != nil {
		return fmt.Errorf("write home pointer: %w", err)
	}

	// Update memory subsystem (additive — never clobbers user edits).
	memReport, err := memory.Update(home)
	if err != nil {
		return fmt.Errorf("update memory: %w", err)
	}

	fmt.Printf("JEFF updated at %s (dirs, hooks, personas, providers, config synced)\n", home)
	fmt.Printf("  memory: %d new, %d skipped\n", len(memReport.Created), len(memReport.Skipped))
	if len(memReport.Migrations) > 0 {
		fmt.Println("  migration hints:")
		for _, h := range memReport.Migrations {
			fmt.Printf("    • %s\n", h)
		}
		fmt.Println("  Move legacy directories manually (source → dest under memory/...).")
	}
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
		filepath.Join(home, "projects"),
		filepath.Join(home, ".skills"),
		filepath.Join(home, ".personas"),
	}
	for _, d := range dirs {
		_ = os.MkdirAll(d, 0o755)
	}
	// Create agent-specific config dirs via providers.
	for _, agent := range jeff.RegisteredAgents() {
		if p := jeff.GetProvider(agent); p != nil {
			_ = p.EnsureHomeDirs(home)
		}
	}
}

// writeDefaults writes default files that don't already exist.
func writeDefaults(home string) {
	writeIfMissing(filepath.Join(home, ".skills", "skills.json"),
		"{\"$schema\":\"https://raw.githubusercontent.com/NeerajG03/JEFF/main/schemas/skills.json\",\"skills\":{}}\n")
	// Write agent-specific default files via providers.
	for _, agent := range jeff.RegisteredAgents() {
		if p := jeff.GetProvider(agent); p != nil {
			_ = p.WriteHomeDefaults(home)
		}
	}
}

// writeIfMissing writes content to path only if the file doesn't exist.
func writeIfMissing(path, content string) {
	if _, err := os.Stat(path); err == nil {
		return
	}
	_ = os.WriteFile(path, []byte(content), 0o644)
}
