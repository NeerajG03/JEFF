package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/NeerajG03/JEFF"
	jeffembed "github.com/NeerajG03/JEFF/embed"
	"github.com/NeerajG03/JEFF/hooks"
	"github.com/NeerajG03/JEFF/task"
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
	cmd.AddCommand(configGigHomeCmd())
	cmd.AddCommand(configHooksCmd())
	cmd.AddCommand(configResetClaudeMDCmd())
	cmd.AddCommand(configOpenCodeCmd())
	return cmd
}

func configOpenCodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "opencode",
		Short: "Manage OpenCode --model aliases",
		Long: "Manage OpenCode --model aliases.\n\n" +
			"With no aliases registered, any \"provider/model\"-shaped --model value is\n" +
			"recognized as OpenCode (e.g. opencode-go/kimi-k2.7-code). Once you register\n" +
			"at least one alias, ONLY registered names or actual provider/model ids are\n" +
			"recognized as OpenCode — this lets you catch typos, at the cost of needing\n" +
			"to register every model you plan to use via --model.",
	}
	cmd.AddCommand(configOpenCodeAddCmd())
	cmd.AddCommand(configOpenCodeListCmd())
	cmd.AddCommand(configOpenCodeRemoveCmd())
	return cmd
}

func configOpenCodeAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <name> <provider/model>",
		Short: "Register a --model alias for an OpenCode provider/model id",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, actual := args[0], args[1]
			if err := jeff.AddOpenCodeModel(cfg, name, actual); err != nil {
				return err
			}
			fmt.Printf("Registered opencode model %s -> %s\n", name, actual)
			fmt.Println("Note: only registered opencode --model aliases/ids will be recognized from now on.")
			return nil
		},
	}
}

func configOpenCodeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered OpenCode --model aliases",
		RunE: func(cmd *cobra.Command, args []string) error {
			models := jeff.ListOpenCodeModels(cfg)
			if len(models) == 0 {
				fmt.Println("No opencode models registered. Use: jeff config opencode add <name> <provider/model>")
				return nil
			}
			names := make([]string, 0, len(models))
			for name := range models {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				fmt.Printf("%-20s %s\n", name, models[name])
			}
			return nil
		},
	}
}

func configOpenCodeRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Unregister an OpenCode --model alias",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := jeff.RemoveOpenCodeModel(cfg, args[0]); err != nil {
				return err
			}
			fmt.Printf("Removed opencode model %s\n", args[0])
			return nil
		},
	}
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
		fmt.Sprintf("agent [%s]", strings.Join(jeff.AgentTool("").ValidNames(), "|")), "Get or set the preferred agent tool",
		jeff.AgentTool("").ValidNames(),
		func() string { return string(cfg.Agent) },
		func(v string) { cfg.Agent = jeff.AgentTool(v) },
	)
}

func configIDECmd() *cobra.Command {
	return configEnumCmd[jeff.IDE](
		"ide [vscode|cursor|windsurf|nvim|zed]", "Get or set the preferred IDE",
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

func configGigHomeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gig-home [path]",
		Short: "Get or set the gig home directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Println(resolveGigHome(cfg))
				return nil
			}
			val := args[0]
			fi, err := os.Stat(val)
			if err != nil {
				return fmt.Errorf("invalid path %q: %w", val, err)
			}
			if !fi.IsDir() {
				return fmt.Errorf("invalid path %q: not a directory", val)
			}
			cfg.GigHome = val
			if err := jeff.SaveConfig(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			if !isGigStoreInitialized(cfg) {
				fmt.Fprintf(os.Stderr, "Warning: %s does not contain a gig store — run `gig init --prefix <name>` first\n", val)
			}
			fmt.Printf("gig-home set to %s\n", val)
			return nil
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveFilterDirs
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
			fmt.Printf("%sd %s\n", strings.ToUpper(verb[:1])+verb[1:], name)
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
	var syncTasks bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Re-sync hooks to disk from config",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := syncHooksFromConfig(); err != nil {
				return err
			}
			if syncTasks {
				for _, ws := range taskWorkspaces(cfg.Home, gigTaskPrefix(cfg)) {
					personaName := task.DetectPersona(ws.Dir)
					repos := task.DetectRepos(ws.Dir)
					syncTaskHooks(cfg, ws.Dir, ws.TaskID, personaName, repos, "")
					fmt.Printf("Synced hooks for %s\n", ws.Name)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&syncTasks, "tasks", false, "Also sync hooks in all task workspaces")
	return cmd
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
	ctx := hooks.HookContext{JeffHome: home, TargetDir: home, GigHome: resolveGigHome(c)}
	enabled := hooks.EnabledForSource(c.Hooks, hooks.SourceHome, reg)
	// Sync hooks for ALL registered agents so home is ready for any agent.
	for _, agent := range jeff.RegisteredAgents() {
		p := jeff.GetProvider(agent)
		if p == nil {
			continue
		}
		if err := mgr.Sync(home, enabled, p.HookDeliveryKey(), ctx); err != nil {
			return fmt.Errorf("sync home hooks (%s): %w", agent, err)
		}
	}
	return nil
}

// syncHooksFromConfig syncs hooks to disk using current config.
func syncHooksFromConfig() error {
	if err := syncHomeHooks(cfg.Home, cfg); err != nil {
		return fmt.Errorf("sync hooks: %w", err)
	}
	fmt.Println("Hooks synced")
	return nil
}
