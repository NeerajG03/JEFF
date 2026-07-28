package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/crew"
	"github.com/NeerajG03/JEFF/identity"
	"github.com/spf13/cobra"
)

func orchestratorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "orchestrator",
		Aliases: []string{"orch"},
		Short:   "Create and manage orchestrator sessions",
		Long:    "Launch a new orchestrator tmux session (jeff-N) with Claude Code, then start workers as additional tabs.",
	}

	cmd.AddCommand(
		orchestratorInitCmd(),
		orchestratorStartCmd(),
		orchestratorListCmd(),
		orchestratorInfoCmd(),
		orchestratorAttachCmd(),
		orchestratorStopCmd(),
	)

	return cmd
}

func orchestratorInitCmd() *cobra.Command {
	var name string
	var global bool
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a durable orchestrator identity for this project (or machine)",
		Long: `Write a durable orchestrator identity file so this orchestrator has a stable
id that survives shell restarts and works outside tmux (Cursor, VS Code, a plain
terminal, CI). Workers started afterwards bind to this identity.

  jeff orchestrator init            write .jeff/orchestrator.json in the current directory
  jeff orchestrator init --name X   set a human-readable name (default: tmux session name or basename of cwd)
  jeff orchestrator init --global   write the machine-wide default (inside JEFF_HOME, default-orchestrator.json)
  jeff orchestrator init --force    overwrite an existing identity file

IDENTITY IS PER-DIRECTORY: the identity file is written under the current
working directory — each project should run init in its own root. List and
info commands show the directory the identity is bound to.

If run inside a tmux pane that hosts an existing jeff orchestrator, you are
offered the chance to adopt its id so already-bound workers keep their
orchestrator.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			// Resolve where the identity file lives. The global default is written
			// inside JEFF_HOME (resolved via jeff.ResolveHome), not under $HOME,
			// so JEFF_HOME env var / pointer file is honored.
			var dir, path string
			if global {
				jeffHome, err := jeff.ResolveHome()
				if err != nil {
					return fmt.Errorf("resolve JEFF_HOME: %w", err)
				}
				dir = jeffHome
				path = identity.GlobalFilePath(jeffHome)
			} else {
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("get cwd: %w", err)
				}
				dir = wd
				path = identity.ProjectFilePath(wd)
			}

			// Refuse on an existing file unless --force (write is atomic below,
			// so an interrupted overwrite never corrupts the existing file).
			if !force {
				if _, err := os.Stat(path); err == nil {
					return fmt.Errorf("identity already exists at %s\nRun `jeff orchestrator init --force` to overwrite", path)
				}
			}

			// Tmux binding is an optional enhancement. Record the pane only for a
			// per-project identity created inside tmux; the global default stays
			// host-agnostic (no pane).
			tmuxPane, tmuxSession := "", ""
			if !global && os.Getenv("TMUX") != "" {
				tmuxPane = os.Getenv("TMUX_PANE")
				tmuxSession = currentTmuxSessionName(tmuxPane)
			}

			// Default name: tmux session name (in tmux) else basename of dir.
			// The global default is host-wide, so it gets a fixed clear name.
			resolvedName := name
			if resolvedName == "" {
				switch {
				case global:
					resolvedName = "default"
				case tmuxSession != "":
					resolvedName = tmuxSession
				default:
					resolvedName = filepath.Base(dir)
				}
			}

			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return fmt.Errorf("open crew store: %w", err)
			}
			defer cs.Close()

			// Adopt flow: if this pane/session already hosts a jeff orchestrator,
			// offer to reuse its id so workers already bound to it keep their
			// orchestrator (continuity for running orchestrators).
			adoptID := ""
			if !global && tmuxPane != "" {
				if cand := findAdoptableOrchestrator(cs, tmuxPane, tmuxSession); cand != nil {
					if promptAdopt(cmd, cand.ID) {
						adoptID = cand.ID
					}
				}
			}

			id := identity.Generate(identity.GenerateOpts{
				ID:       adoptID,
				Name:     resolvedName,
				Dir:      dir,
				TmuxPane: tmuxPane,
			})
			if err := identity.Write(path, id); err != nil {
				return err
			}

			// Bridge: ensure an orchestrators DB row exists for this identity so
			// worker start / scoping / signalling resolve. The identity itself
			// lives on disk; this row is only the DB-side handle. When adopting,
			// the row already exists — don't clobber its tmux binding.
			if adoptID == "" {
				orch := &crew.Orchestrator{
					ID:          id.ID,
					Dir:         dir, // record project directory for list/info visibility
					TmuxSession: "", // durable identity: workers host in the shared session
					TmuxWindow:  "",
					TmuxPane:    tmuxPane, // enables direct notification routing when set
					StartedAt:   time.Now().UTC(),
					Status:      "running",
				}
				if err := cs.PutOrchestrator(orch); err != nil {
					return fmt.Errorf("register orchestrator identity: %w", err)
				}
			}

			fmt.Printf("Wrote %s\n", path)
			fmt.Printf("  id:   %s\n", id.ID)
			fmt.Printf("  name: %s\n", id.Name)
			fmt.Printf("  dir:  %s\n", dir)
			if id.TmuxPane != "" {
				fmt.Printf("  tmux_pane: %s\n", id.TmuxPane)
			}
			if adoptID != "" {
				fmt.Printf("  adopted existing orchestrator %s\n", adoptID)
			}
			fmt.Fprintln(os.Stderr, "Workers started here now bind to this orchestrator.")
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "human-readable name (default: tmux session name or basename of cwd)")
	cmd.Flags().BoolVar(&global, "global", false, "write the machine-wide default inside JEFF_HOME (default-orchestrator.json)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing identity file")
	return cmd
}

// findAdoptableOrchestrator returns a running orchestrator that this tmux
// pane/session already hosts, or nil. It first tries the durable pane binding,
// then a session-name match (jeff orchestrator sessions are named after their id).
func findAdoptableOrchestrator(cs *crew.Store, tmuxPane, tmuxSession string) *crew.Orchestrator {
	if id, _ := cs.OrchestratorByPane(tmuxPane); id != "" {
		if o, err := cs.GetOrchestrator(id); err == nil {
			return o
		}
	}
	if tmuxSession != "" {
		if o, err := cs.GetOrchestrator(tmuxSession); err == nil && o.Status == "running" {
			return o
		}
	}
	return nil
}

// promptAdopt asks the user whether to adopt an existing orchestrator id.
// Defaults to yes (empty input). Non-y answers decline.
func promptAdopt(cmd *cobra.Command, orchID string) bool {
	fmt.Fprintf(cmd.OutOrStdout(), "adopt existing tmux orchestrator %s for this project? [Y/n] ", orchID)
	reader := bufio.NewReader(cmd.InOrStdin())
	line, _ := reader.ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "" || answer == "y" || answer == "yes"
}

func orchestratorStartCmd() *cobra.Command {
	var (
		name          string
		agentOverride string
		modelOverride string
	)
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Launch a new orchestrator session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doOrchestratorStart(name, agentOverride, modelOverride)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "custom name suffix for the session (e.g. --name work → jeff-work)")
	cmd.Flags().StringVar(&agentOverride, "agent", "", "Agent backend (claude, gemini, opencode; default: config agent)")
	cmd.Flags().StringVar(&modelOverride, "model", "", "Model name; auto-routes backend (sonnet/opus/haiku/claude-* → claude, pro/flash/flash-lite/auto/gemini-* → gemini, provider/model → opencode)")
	return cmd
}

// doOrchestratorStart is the shared orchestrator start logic, extracted so it
// can be reused by both orchestratorStartCmd and crewUpCmd.
func doOrchestratorStart(name, agentOverride, modelOverride string) error {
	cs, err := crew.Open(cfg.Home)
	if err != nil {
		return fmt.Errorf("open crew store: %w", err)
	}
	defer cs.Close()

	// Resolve agent: --agent flag takes priority over global config.
	agentTool := cfg.Agent
	if agentOverride != "" {
		agentTool = jeff.AgentTool(agentOverride)
		if !agentTool.IsValid() {
			return fmt.Errorf("unknown agent %q (valid: %s)", agentOverride, strings.Join(jeff.AgentTool("").ValidNames(), ", "))
		}
	}

	// Resolve model: --model flag.
	model := modelOverride

	// Auto-route backend from model name when --model is explicitly supplied.
	if modelOverride != "" {
		if inferred := jeff.InferBackend(modelOverride); inferred != "" {
			agentTool = inferred
		} else if !jeff.IsValidModel(agentTool, modelOverride) {
			return fmt.Errorf("%s", jeff.UnknownModelError(modelOverride))
		}
	}

	provider := jeff.GetProvider(agentTool)
	if provider == nil {
		return fmt.Errorf("no provider registered for agent %q", agentTool)
	}
	// No --safe flag on orchestrator start (out of scope for this knob);
	// resolve from config only so `skip_permissions: false` still applies.
	launchArgs := provider.BuildLaunchArgs(jeff.LaunchOpts{Model: model, SkipPermissions: effectiveSkipPermissions(cfg, false)})
	launchCmd := provider.Command()
	for _, arg := range launchArgs {
		launchCmd += " " + shellQuote(arg)
	}
	orch, err := crew.StartOrchestratorWithLaunchCmd(cs, cfg.Home, string(agentTool), model, name, launchCmd)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Orchestrator %s started (tmux session: %s, agent: %s", orch.ID, orch.TmuxSession, orch.Agent)
	if orch.Model != "" {
		fmt.Fprintf(os.Stderr, ", model: %s", orch.Model)
	}
	fmt.Fprintf(os.Stderr, ")\n")
	fmt.Fprintf(os.Stderr, "Attach with: jeff orchestrator attach %s\n", orch.ID)
	fmt.Fprintf(os.Stderr, "Start workers with: jeff crew start <task-id> \"Work on this task\" --orchestrator %s\n", orch.ID)

	data, _ := json.Marshal(orch)
	fmt.Println(string(data))
	return nil
}

func orchestratorListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List orchestrator sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			// Refresh state from tmux before listing.
			gigStore, _ := openGigStore(cfg)
			if gigStore != nil {
				defer gigStore.Close()
			}
			if err := crew.Refresh(cs, func(taskID string) bool {
				if gigStore == nil {
					return false
				}
				t, err := gigStore.Get(taskID)
				return err == nil && t.Status.IsTerminal()
			}); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: refresh crew state: %v\n", err)
			}

			orchs, err := cs.ListOrchestrators(false)
			if err != nil {
				return err
			}

			if len(orchs) == 0 {
				fmt.Fprintln(os.Stderr, "(no orchestrator sessions)")
				return nil
			}

			fmt.Fprintf(os.Stdout, "%-12s %-12s %-10s %-10s %-24s %-24s %s\n", "ID", "SESSION", "STATUS", "AGENT", "MODEL", "DIR", "STARTED")
			for _, o := range orchs {
				started := relativeTime(o.StartedAt)
				agent := o.Agent
				if agent == "" {
					agent = "-"
				}
				model := o.Model
				if model == "" {
					model = "-"
				}
				dir := o.Dir
				if dir == "" {
					dir = "-"
				}
				fmt.Fprintf(os.Stdout, "%-12s %-12s %-10s %-10s %-24s %-24s %s\n", o.ID, o.TmuxSession, o.Status, agent, model, dir, started)
			}
			return nil
		},
	}
}

func orchestratorInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info [orchestrator-id]",
		Short: "Show all tasks worked under an orchestrator session",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			// Auto-detect orchestrator if no arg given.
			orchID := ""
			if len(args) > 0 {
				orchID = args[0]
			} else {
				id, _, derr := detectOrchestratorID()
				if derr != nil {
					return derr
				}
				orchID = id
			}
			if orchID == "" {
				return fmt.Errorf("no orchestrator specified and no identity found (run `jeff orchestrator init`)")
			}

			orch, err := cs.GetOrchestrator(orchID)
			if err != nil {
				return fmt.Errorf("orchestrator not found: %w", err)
			}

			agent := orch.Agent
			if agent == "" {
				agent = "-"
			}
			model := orch.Model
			if model == "" {
				model = "-"
			}
			dir := orch.Dir
			if dir == "" {
				dir = "-"
			}
			fmt.Fprintf(os.Stdout, "Orchestrator: %s (session: %s, status: %s, agent: %s, model: %s, dir: %s, started: %s)\n\n",
				orch.ID, orch.TmuxSession, orch.Status, agent, model, dir, relativeTime(orch.StartedAt))

			gigStore, _ := openGigStore(cfg)
			if gigStore != nil {
				defer gigStore.Close()
			}

			workers, err := cs.WorkersForOrchestrator(orchID)
			if err != nil {
				return err
			}

			if len(workers) == 0 {
				fmt.Fprintln(os.Stderr, "(no workers)")
				return nil
			}

			fmt.Fprintf(os.Stdout, "%-12s %-10s %-8s %-12s %-12s %s\n",
				"TASK", "PERSONA", "MODEL", "STATUS", "STARTED", "LAST CHECKPOINT")

			for _, sess := range workers {
				started := relativeTime(sess.StartedAt)
				ckpt := "(none)"

				if gigStore != nil {
					if cp, err := gigStore.LatestCheckpoint(sess.TaskID); err == nil && cp != nil {
						summary := cp.Done
						if len(summary) > 50 {
							summary = summary[:47] + "..."
						}
						ckpt = fmt.Sprintf("%q (%s ago)", summary, relativeTime(cp.CreatedAt))
					}
				}

				model := sess.Model
				if model == "" {
					model = "-"
				}

				status := crewStatusLabel(sess.Status)
				visPad := 12 - visibleLen(status)
				if visPad < 0 {
					visPad = 0
				}
				fmt.Fprintf(os.Stdout, "%-12s %-10s %-8s %s%-*s %-12s %s\n",
					sess.TaskID, sess.Persona, model, status, visPad, "", started, ckpt)
			}
			return nil
		},
	}
	cmd.ValidArgsFunction = orchestratorCompletion
	return cmd
}

func orchestratorAttachCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attach <orchestrator-id>",
		Short: "Attach to an orchestrator's tmux session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			orch, err := cs.GetOrchestrator(args[0])
			if err != nil {
				return fmt.Errorf("orchestrator not found: %w", err)
			}

			fmt.Fprintf(os.Stderr, "Attaching to %s...\n", orch.TmuxSession)
			return crew.AttachToSession(orch.TmuxSession, "")
		},
	}
	cmd.ValidArgsFunction = orchestratorCompletion
	return cmd
}

func orchestratorStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <orchestrator-id>",
		Short: "Stop an orchestrator and all its workers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			orchID := args[0]
			fmt.Fprintf(os.Stderr, "Stopping orchestrator %s...\n", orchID)

			if err := crew.StopOrchestrator(cs, orchID); err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Orchestrator %s stopped.\n", orchID)
			return nil
		},
	}
	cmd.ValidArgsFunction = orchestratorCompletion
	return cmd
}
