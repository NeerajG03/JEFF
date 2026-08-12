package main

import (
	"os"
	"path/filepath"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/crew"
	"github.com/NeerajG03/JEFF/persona"
	"github.com/NeerajG03/JEFF/skill"
	"github.com/NeerajG03/JEFF/workspace"
	"github.com/spf13/cobra"
)

func completionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish]",
		Short: "Generate shell completion script",
		Long: `Generate shell completion scripts for jeff.

To load completions:

  bash:
    source <(jeff completion bash)

  zsh:
    source <(jeff completion zsh)
    # Or install permanently:
    jeff completion zsh > "${fpath[1]}/_jeff"

  fish:
    jeff completion fish | source
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish"},
		Args:                  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(cmd.OutOrStdout(), true)
			case "zsh":
				return cmd.Root().GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return cmd.Root().GenFishCompletion(cmd.OutOrStdout(), true)
			default:
				return cmd.Help()
			}
		},
	}
	return cmd
}

// ── Gig task completions ─────────────────────────────────────────────

// readyTaskCompletion completes gig task IDs that are open and unblocked (ready for pickup).
func readyTaskCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	store, err := openGigStore(cfg)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer store.Close()

	tasks, err := store.Ready("")
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var ids []string
	for _, t := range tasks {
		ids = append(ids, t.ID+"\t"+t.Title)
	}
	return ids, cobra.ShellCompDirectiveNoFileComp
}

// activeTaskCompletion completes gig task IDs that have a workspace (in_progress tasks assigned to jeff).
func activeTaskCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if cfg == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	dirs, err := workspace.ListActive(cfg.Home, gigTaskPrefix(cfg))
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Optionally enrich with task title from gig.
	store, _ := openGigStore(cfg)
	if store != nil {
		defer store.Close()
	}

	var ids []string
	for _, td := range dirs {
		desc := td.Slug
		if store != nil {
			if t, err := store.Get(td.TaskID); err == nil {
				desc = t.Title
			}
		}
		ids = append(ids, td.TaskID+"\t"+desc)
	}
	return ids, cobra.ShellCompDirectiveNoFileComp
}

// ── Orchestrator completions ─────────────────────────────────────────

// orchestratorCompletion completes orchestrator IDs — only running sessions (attachable).
func orchestratorCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if cfg == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cs, err := crew.Open(cfg.Home)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer cs.Close()

	orchs, err := cs.ListOrchestrators(true) // activeOnly: only running sessions are attachable
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var ids []string
	for _, o := range orchs {
		ids = append(ids, o.ID+"\t"+o.TmuxSession)
	}
	return ids, cobra.ShellCompDirectiveNoFileComp
}

// ── Repo completions ─────────────────────────────────────────────────

// repoNameCompletion completes registered repo names.
func repoNameCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if cfg == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	for name, rc := range cfg.Repos {
		desc := name
		if rc != nil && rc.Description != "" {
			desc = rc.Description
		}
		names = append(names, name+"\t"+desc)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// ── Skill completions ────────────────────────────────────────────────

// skillNameCompletion completes registered skill names.
func skillNameCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if cfg == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	skills, err := skill.List(cfg.Home)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	for _, s := range skills {
		names = append(names, s.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// ── Project completions ──────────────────────────────────────────────

// projectNameCompletion completes project names from JEFF_HOME/projects/.
func projectNameCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if cfg == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	projectsDir := filepath.Join(cfg.Home, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// ── Persona completions ──────────────────────────────────────────────

// personaCompletion completes persona names with role descriptions.
// Uses the registry if available, falls back to embedded names.
func personaCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if cfg != nil && cfg.Home != "" {
		if names := persona.RegisteredNamesWithDescriptions(cfg.Home); len(names) > 0 {
			return names, cobra.ShellCompDirectiveNoFileComp
		}
	}
	return persona.NamesWithDescriptions(), cobra.ShellCompDirectiveNoFileComp
}

// ── Gig type/status completions ──────────────────────────────────────

// gigTypeCompletion completes gig task type values.
func gigTypeCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{"task", "bug", "feature", "epic", "chore"}, cobra.ShellCompDirectiveNoFileComp
}

// ── IDE completions ──────────────────────────────────────────────────

// ideCompletion completes supported IDE names.
func ideCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{
		"vscode\tVisual Studio Code",
		"cursor\tCursor",
		"windsurf\tWindsurf",
	}, cobra.ShellCompDirectiveNoFileComp
}

// ── Agent completions ────────────────────────────────────────────────

// agentCompletion completes supported agent CLI names.
func agentCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	agents := jeff.RegisteredAgents()
	var names []string
	for _, a := range agents {
		var desc string
		switch a {
		case jeff.AgentClaudeCode:
			desc = "Claude Code CLI"
		case jeff.AgentOpenCode:
			desc = "OpenCode AI CLI"
		case jeff.AgentGemini:
			desc = "Gemini CLI"
		default:
			desc = string(a)
		}
		names = append(names, string(a)+"\t"+desc)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
