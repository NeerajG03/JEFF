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

// configEnumCmd creates a get/set command for a string enum config field.
func configEnumCmd[T ~string](use, short string, validNames []string,
	get func() string, set func(string),
) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Println(get())
				return nil
			}
			val := args[0]
			valid := false
			for _, n := range validNames {
				if n == val {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("invalid value %q, must be one of: %s", val, strings.Join(validNames, ", "))
			}
			set(val)
			if err := jeff.SaveConfig(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Printf("%s set to %s\n", cmd.Name(), val)
			return nil
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return validNames, cobra.ShellCompDirectiveNoFileComp
		},
	}
}

func configAgentCmd() *cobra.Command {
	return configEnumCmd[jeff.AgentTool](
		"agent [claude|opencode]", "Get or set the preferred agent tool",
		jeff.AgentTool("").ValidNames(),
		func() string { return string(cfg.Agent) },
		func(v string) { cfg.Agent = jeff.AgentTool(v) },
	)
}

func configIDECmd() *cobra.Command {
	return configEnumCmd[jeff.IDE](
		"ide [vscode|cursor|windsurf|nvim]", "Get or set the preferred IDE",
		jeff.IDE("").ValidNames(),
		func() string {
			if cfg.IDE == "" {
				return "vscode (default)"
			}
			return string(cfg.IDE)
		},
		func(v string) { cfg.IDE = jeff.IDE(v) },
	)
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
			homeEnabled := hooks.EnabledForSource(cfg.Hooks, hooks.SourceHome, reg)
			taskEnabled := hooks.EnabledForSource(cfg.Hooks, hooks.SourceTask, reg)

			for _, h := range reg.All() {
				var enabled bool
				if h.Source == hooks.SourceHome {
					enabled = homeEnabled[h.Name]
				} else {
					enabled = taskEnabled[h.Name]
				}
				status := "[ ]"
				if enabled {
					status = "[x]"
				}
				fmt.Printf("%s %-22s %-6s %s/%s\n", status, h.Name, h.Source, h.Event, h.Matcher)
			}
			return nil
		},
	}
}

// configHooksToggleCmd creates an enable or disable command for hooks.
func configHooksToggleCmd(verb string, value bool) *cobra.Command {
	return &cobra.Command{
		Use:   verb + " <hook-name>",
		Short: strings.ToUpper(verb[:1]) + verb[1:] + " a hook",
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
			cfg.Hooks[name] = value
			if err := jeff.SaveConfig(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Printf("%sd %s\n", strings.ToUpper(verb[:1]) + verb[1:], name)
			return syncHooksFromConfig()
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return hooks.DefaultRegistry().Names(), cobra.ShellCompDirectiveNoFileComp
		},
	}
}

func configHooksEnableCmd() *cobra.Command  { return configHooksToggleCmd("enable", true) }
func configHooksDisableCmd() *cobra.Command { return configHooksToggleCmd("disable", false) }

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

// syncHomeHooks syncs home-level hooks to disk for the given config.
func syncHomeHooks(home string, c *jeff.Config) error {
	reg := hooks.DefaultRegistry()
	mgr := hooks.NewManager(reg)
	ctx := hooks.HookContext{JeffHome: home, TargetDir: home, GigHome: c.GigHome}
	enabled := hooks.EnabledForSource(c.Hooks, hooks.SourceHome, reg)
	deliveryKey := jeff.GetProvider(c.Agent).HookDeliveryKey()
	return mgr.Sync(home, enabled, deliveryKey, ctx)
}

// syncHooksFromConfig syncs hooks to disk using current config.
func syncHooksFromConfig() error {
	if err := syncHomeHooks(cfg.Home, cfg); err != nil {
		return fmt.Errorf("sync hooks: %w", err)
	}
	fmt.Println("Hooks synced")
	return nil
}
