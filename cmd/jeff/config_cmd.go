package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/NeerajG03/JEFF"
	jeffembed "github.com/NeerajG03/JEFF/embed"
	"github.com/NeerajG03/JEFF/hooks"
	"github.com/spf13/cobra"
)

func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "View and update JEFF configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("agent: %s\n", cfg.Agent)
			if cfg.IDE != "" {
				fmt.Printf("ide:   %s\n", cfg.IDE)
			} else {
				fmt.Printf("ide:   (not set, default: vscode)\n")
			}
			fmt.Printf("home:  %s\n", cfg.Home)
			return nil
		},
	}

	cmd.AddCommand(configAgentCmd())
	cmd.AddCommand(configIDECmd())
	cmd.AddCommand(configHooksCmd())
	cmd.AddCommand(configResetClaudeMDCmd())
	return cmd
}

func configAgentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "agent [claude|opencode]",
		Short: "Get or set the preferred agent tool",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Println(cfg.Agent)
				return nil
			}

			val := jeff.AgentTool(args[0])
			if !val.IsValid() {
				var names []string
				for _, t := range jeff.ValidAgentTools {
					names = append(names, string(t))
				}
				return fmt.Errorf("invalid agent %q, must be one of: %s", args[0], strings.Join(names, ", "))
			}

			cfg.Agent = val
			if err := jeff.SaveConfig(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Printf("agent set to %s\n", val)
			return nil
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			var names []string
			for _, t := range jeff.ValidAgentTools {
				names = append(names, string(t))
			}
			return names, cobra.ShellCompDirectiveNoFileComp
		},
	}
}

func configIDECmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ide [vscode|cursor|windsurf|nvim]",
		Short: "Get or set the preferred IDE",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				if cfg.IDE == "" {
					fmt.Println("vscode (default)")
				} else {
					fmt.Println(cfg.IDE)
				}
				return nil
			}

			val := jeff.IDE(args[0])
			if !val.IsValid() {
				var names []string
				for _, i := range jeff.ValidIDEs {
					names = append(names, string(i))
				}
				return fmt.Errorf("invalid IDE %q, must be one of: %s", args[0], strings.Join(names, ", "))
			}

			cfg.IDE = val
			if err := jeff.SaveConfig(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Printf("ide set to %s\n", val)
			return nil
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			var names []string
			for _, i := range jeff.ValidIDEs {
				names = append(names, string(i))
			}
			return names, cobra.ShellCompDirectiveNoFileComp
		},
	}
}

func configHooksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hooks",
		Short: "Manage session hooks",
	}
	cmd.AddCommand(configHooksListCmd())
	cmd.AddCommand(configHooksEnableCmd())
	cmd.AddCommand(configHooksDisableCmd())
	cmd.AddCommand(configHooksSyncCmd())
	return cmd
}

func configHooksListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all hooks and their status",
		RunE: func(cmd *cobra.Command, args []string) error {
			reg := hooks.DefaultRegistry()
			enabled := hooks.EnabledForSource(cfg.Hooks, hooks.SourceHome, reg)

			for _, h := range reg.All() {
				status := "[ ]"
				if enabled[h.Name] {
					status = "[x]"
				}
				fmt.Printf("%s %-20s %s/%s\n", status, h.Name, h.Event, h.Matcher)
			}
			return nil
		},
	}
}

func configHooksEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <hook-name>",
		Short: "Enable a hook",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			reg := hooks.DefaultRegistry()
			if reg.Get(name) == nil {
				return fmt.Errorf("unknown hook %q, available: %s", name, strings.Join(reg.Names(), ", "))
			}

			if cfg.Hooks == nil {
				cfg.Hooks = make(map[string]bool)
			}
			cfg.Hooks[name] = true
			if err := jeff.SaveConfig(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Printf("Enabled %s\n", name)
			return syncHooksFromConfig()
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return hooks.DefaultRegistry().Names(), cobra.ShellCompDirectiveNoFileComp
		},
	}
}

func configHooksDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <hook-name>",
		Short: "Disable a hook",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			reg := hooks.DefaultRegistry()
			if reg.Get(name) == nil {
				return fmt.Errorf("unknown hook %q, available: %s", name, strings.Join(reg.Names(), ", "))
			}

			if cfg.Hooks == nil {
				cfg.Hooks = make(map[string]bool)
			}
			cfg.Hooks[name] = false
			if err := jeff.SaveConfig(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Printf("Disabled %s\n", name)
			return syncHooksFromConfig()
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return hooks.DefaultRegistry().Names(), cobra.ShellCompDirectiveNoFileComp
		},
	}
}

func configHooksSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Re-sync hooks to disk from config",
		RunE: func(cmd *cobra.Command, args []string) error {
			return syncHooksFromConfig()
		},
	}
}

func configResetClaudeMDCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset-claude-md",
		Short: "Regenerate CLAUDE.md from default template",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Backup existing CLAUDE.md before overwriting.
			src := filepath.Join(cfg.Home, "CLAUDE.md")
			if _, err := os.Stat(src); err == nil {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("get home dir: %w", err)
				}
				backupDir := filepath.Join(home, ".config", "jeff", "backups")
				if err := os.MkdirAll(backupDir, 0o755); err != nil {
					return fmt.Errorf("create backup dir: %w", err)
				}
				ts := time.Now().Format("20060102-150405")
				backupPath := filepath.Join(backupDir, "CLAUDE.md."+ts)
				data, err := os.ReadFile(src)
				if err != nil {
					return fmt.Errorf("read existing CLAUDE.md: %w", err)
				}
				if err := os.WriteFile(backupPath, data, 0o644); err != nil {
					return fmt.Errorf("write backup: %w", err)
				}
				fmt.Printf("Backed up to %s\n", backupPath)
			}

			homePath := cfg.Home + "/"
			if err := jeffembed.WriteClaudeMD(cfg.Home, homePath, true); err != nil {
				return fmt.Errorf("reset CLAUDE.md: %w", err)
			}
			fmt.Printf("Reset %s/CLAUDE.md\n", cfg.Home)
			return nil
		},
	}
}

// syncHooksFromConfig syncs hooks to disk using current config.
func syncHooksFromConfig() error {
	reg := hooks.DefaultRegistry()
	mgr := hooks.NewManager(reg)
	ctx := hooks.HookContext{JeffHome: cfg.Home, TargetDir: cfg.Home, GigHome: cfg.GigHome}
	enabled := hooks.EnabledForSource(cfg.Hooks, hooks.SourceHome, reg)
	agent := hooks.AgentTool(cfg.Agent)
	if err := mgr.Sync(cfg.Home, enabled, agent, ctx); err != nil {
		return fmt.Errorf("sync hooks: %w", err)
	}
	fmt.Println("Hooks synced")
	return nil
}
