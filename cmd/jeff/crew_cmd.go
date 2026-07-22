package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/crew"
	jeffembed "github.com/NeerajG03/JEFF/embed"
	"github.com/NeerajG03/JEFF/hooks"
	"github.com/NeerajG03/JEFF/identity"
	"github.com/NeerajG03/JEFF/memory"
	"github.com/NeerajG03/JEFF/persona"
	"github.com/NeerajG03/JEFF/skill"
	"github.com/NeerajG03/JEFF/workspace"
	"github.com/NeerajG03/gig"
	"github.com/spf13/cobra"
)

func crewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "crew",
		Short: "Manage worker agents via tmux",
		Long:  "Start, monitor, message, and stop worker agents running in tmux windows.",
	}

	cmd.AddCommand(
		crewStartCmd(),
		crewResumeCmd(),
		crewSessionIDCmd(),
		crewListCmd(),
		crewStatusCmd(),
		crewSendCmd(),
		crewAskCmd(),
		crewStopCmd(),
		crewAttachCmd(),
		crewCaptureCmd(),
		crewInboxCmd(),
		crewOrchestratorInboxCmd(),
		crewTouchCmd(),
		crewAckCmd(),
		crewEventsCmd(),
		crewCleanupCmd(),
		crewSignalOrchestratorCmd(),
		crewWorkerStoppedCmd(),
		crewCheckStallsCmd(),
	)

	return cmd
}

func crewStartCmd() *cobra.Command {
	var (
		personaName    string
		repos          []string
		reposReadonly  []string
		orchestratorID string
		modelOverride  string
		promptOverride string
		safeFlag       bool
	)

	cmd := &cobra.Command{
		Use:   "start <gig-id> <prompt>",
		Short: "Claim a task and launch a worker agent in tmux",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			taskID := args[0]

			// Handle prompt (positional vs deprecated flag)
			var inputPrompt string
			if len(args) >= 2 {
				inputPrompt = args[1]
			} else if promptOverride != "" {
				fmt.Fprintln(os.Stderr, "WARNING: the --prompt flag is deprecated. Please use the positional argument: jeff crew start <gig-id> \"<prompt>\"")
				inputPrompt = promptOverride
			} else {
				return fmt.Errorf("missing required prompt. Usage: jeff crew start <gig-id> \"<prompt>\" [flags]")
			}
			if strings.TrimSpace(inputPrompt) == "" {
				return fmt.Errorf("prompt cannot be empty. Usage: jeff crew start <gig-id> \"<prompt>\" [flags]")
			}
			promptOverride = inputPrompt // set it for the rest of the flow

			// Resolve the orchestrator identity from the durable identity file
			// (or env override) if not explicitly passed. Must happen before
			// pickupTask so hooks get the orchestrator ID. A malformed identity
			// file is a hard error here — we must never silently fall through to
			// a shared default with an empty orchestrator_id.
			if orchestratorID == "" {
				id, source, err := detectOrchestratorID()
				if err != nil {
					return err
				}
				orchestratorID = id
				if orchestratorID != "" {
					fmt.Fprintf(os.Stderr, "Orchestrator identity: %s (via %s)\n", orchestratorID, source)
				}
			}

			// Resolve agent: persona default takes priority over global config.
			agentTool := cfg.Agent
			if personaName != "" {
				if pa := persona.RegisteredAgent(cfg.Home, personaName); pa != "" {
					agentTool = jeff.AgentTool(pa)
				}
			}

			// Resolve model: --model flag takes priority, then persona default.
			model := modelOverride
			if model == "" && personaName != "" {
				model = persona.RegisteredModel(cfg.Home, personaName)
			}

			// Auto-route backend from model name when --model is explicitly supplied.
			// Known model families unambiguously pick the backend, overriding persona default.
			if modelOverride != "" {
				if inferred := jeff.InferBackend(modelOverride); inferred != "" {
					agentTool = inferred
				} else if !jeff.IsValidModel(agentTool, modelOverride) {
					return fmt.Errorf("%s", jeff.UnknownModelError(modelOverride))
				}
			}

			// Open crew store early for the pre-flight check.
			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return fmt.Errorf("open crew store: %w", err)
			}
			defer cs.Close()

			// Fail loud on a missing identity BEFORE claiming the task or
			// creating a workspace. This kills the gig-be5c silent-fallthrough
			// class: a worker must never be started with an empty orchestrator_id.
			if orchestratorID == "" {
				return fmt.Errorf(
					"no orchestrator identity found. Run `jeff orchestrator init` in the project directory, or set JEFF_ORCHESTRATOR_ID. See `jeff orchestrator init --help`.",
				)
			}
			// The identity must have a registered orchestrator record so worker
			// scoping and signalling resolve. `jeff orchestrator init` registers
			// it; a stale/unknown id fails loud rather than orphaning the worker.
			if _, gerr := cs.GetOrchestrator(orchestratorID); gerr != nil {
				return fmt.Errorf(
					"orchestrator identity %q has no registered record — run `jeff orchestrator init` here to register it, or set JEFF_ORCHESTRATOR_ID to a registered orchestrator",
					orchestratorID,
				)
			}

			// Pre-flight: refuse to start if DB↔tmux state has drifted.
			if err := crew.PreflightStart(cs, taskID); err != nil {
				return err
			}

			// Run the pickup sequence (claim, workspace, worktrees, hooks, skills).
			taskDir, err := pickupTask(taskID, personaName, repos, reposReadonly, orchestratorID, agentTool)
			if err != nil {
				return err
			}

			var sess *crew.Session

			// Path A: if prompt is too long or contains metacharacters, write to file and substitute.
			if len(promptOverride) > 500 || strings.ContainsAny(promptOverride, "\n\"'\\$") || strings.Contains(promptOverride, "\x60") {
				promptFile := filepath.Join(taskDir, "INITIAL-PROMPT.md")
				if err := os.WriteFile(promptFile, []byte(promptOverride), 0644); err != nil {
					return fmt.Errorf("write INITIAL-PROMPT.md: %w", err)
				}
				promptOverride = "Read INITIAL-PROMPT.md at task root and execute it end to end."
			}
			skip := effectiveSkipPermissions(cfg, safeFlag)
			provider := jeff.GetProvider(agentTool)
			var launchCmd string
			if provider != nil && provider.SupportsInlinePrompt() {
				launchArgs := provider.BuildLaunchArgs(jeff.LaunchOpts{
					Model:           model,
					AgentName:       personaName,
					Prompt:          promptOverride,
					SkipPermissions: skip,
				})
				launchCmd = provider.Command()
				for _, a := range launchArgs {
					// Shell-quote args that contain special characters
					// (the prompt is passed as a positional arg and may
					// contain parens, quotes, etc. that break the shell).
					if needsShellQuoting(a) {
						launchCmd += " " + shellQuote(a)
					} else {
						launchCmd += " " + a
					}
				}
			}
			if safeFlag {
				fmt.Fprintf(os.Stderr, "worker will pause on permission prompts — attach with: jeff crew attach %s\n", taskID)
			}
			opts := crew.StartOpts{
				Persona:         personaName,
				Repos:           repos,
				Agent:           string(agentTool),
				Model:           model,
				Prompt:          promptOverride,
				LaunchCmd:       launchCmd,
				SkipPermissions: skip,
			}
			// Single worker-start path: the identity was validated above, so the
			// worker always binds to a non-empty orchestrator_id. The worker's
			// tmux window is hosted in the orchestrator's session when it has one,
			// or the shared "jeff" session otherwise.
			sess, err = crew.StartWorkerForOrchestrator(cs, orchestratorID, taskID, taskDir, opts)
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Started %s in tmux window %s:%s\n", taskID, sess.TmuxSession, sess.WindowName)
			// Structured output for agent consumption.
			data, _ := json.Marshal(sess)
			fmt.Println(string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&personaName, "persona", "", "Persona template")
	cmd.Flags().StringSliceVar(&repos, "repos", nil, "Repos to set up worktrees for")
	cmd.Flags().StringSliceVar(&reposReadonly, "repos-readonly", nil, "Repos to symlink read-only (no worktree, no post-setup)")
	cmd.Flags().StringVar(&orchestratorID, "orchestrator", "", "Orchestrator ID to attach worker to")
	cmd.Flags().StringVar(&modelOverride, "model", "", "Model name; auto-routes backend (sonnet/opus/haiku/claude-* → claude, pro/flash/flash-lite/auto/gemini-* → gemini)")
	cmd.Flags().StringVar(&promptOverride, "prompt", "", "Custom initial prompt (overrides default)")
	cmd.Flags().MarkDeprecated("prompt", "use the positional argument instead: jeff crew start <gig-id> \"<prompt>\" [flags]")
	cmd.Flags().BoolVar(&safeFlag, "safe", false, `Launch the worker with its permission prompts enabled (pass "--safe" to override skip_permissions)`)
	cmd.ValidArgsFunction = readyTaskCompletion
	cmd.RegisterFlagCompletionFunc("persona", personaCompletion)
	cmd.RegisterFlagCompletionFunc("repos", repoNameCompletion)
	cmd.RegisterFlagCompletionFunc("repos-readonly", repoNameCompletion)
	cmd.RegisterFlagCompletionFunc("orchestrator", orchestratorCompletion)
	return cmd
}

// detectOrchestratorID resolves the durable orchestrator identity for the
// current process via the identity file chain (env override → cwd file → parent
// walk → global default). It is a thin wrapper over identity.Detect.
//
// Returns an empty id with a nil error when no identity is configured, so
// callers decide whether that is fatal (crew start) or tolerable (crew list
// --all). A non-nil error means a genuine I/O failure or a malformed identity
// file — those must propagate and fail loud, never degrade to a shared default.
func detectOrchestratorID() (string, identity.Source, error) {
	return identity.Detect()
}

// currentTmuxSessionName returns the tmux session name owning the given pane, or
// "" on error / outside tmux. Used by `jeff orchestrator init` to name the
// identity and detect an adoptable session.
func currentTmuxSessionName(paneID string) string {
	if paneID == "" {
		return ""
	}
	out, err := exec.Command("tmux", "display-message", "-t", paneID, "-p", "#{session_name}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func crewResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <gig-id>",
		Short: "Resume a worker agent in tmux (workspace must exist)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			taskID := args[0]

			td, err := workspace.Open(cfg.Home, taskID)
			if err != nil {
				return fmt.Errorf("workspace not found for %s: %w", taskID, err)
			}

			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return fmt.Errorf("open crew store: %w", err)
			}
			defer cs.Close()

			// Pre-flight: refuse to resume if DB↔tmux state has drifted.
			if err := crew.PreflightResume(cs, taskID); err != nil {
				return err
			}

			// Detect persona and repos from existing workspace.
			personaName := detectPersona(td.Path)
			repos := detectRepos(td.Path)
			// Look up existing session for resume context.
			var resumeSessionID, orchestratorID string
			agentTool := cfg.Agent
			model := persona.RegisteredModel(cfg.Home, personaName)

			if existing, err := cs.GetSession(taskID); err == nil {
				resumeSessionID = existing.LatestSessionID()
				if resumeSessionID != "" {
					fmt.Fprintf(os.Stderr, "Resuming Claude session %s\n", resumeSessionID)
				}

				orchestratorID = existing.OrchestratorID
				agentTool, model = resolveResumeAgentModel(existing, agentTool, model)

			}

			// Prefer the current identity, fall back to the one stored on the
			// original session. A malformed identity file fails loud.
			detected, _, derr := detectOrchestratorID()
			if derr != nil {
				return derr
			}
			if detected != "" {
				orchestratorID = detected
			}

			// Build launch command via provider. Resume has no --safe flag
			// (per plan); the safety posture always follows current config,
			// not the original session, so it resolves fresh on every resume.
			skip := effectiveSkipPermissions(cfg, false)
			provider := jeff.GetProvider(agentTool)
			var launchCmd string
			if provider != nil {
				launchArgs := provider.BuildLaunchArgs(jeff.LaunchOpts{
					Model:           model,
					ResumeSessionID: resumeSessionID,
					AgentName:       personaName,
					SkipPermissions: skip,
				})
				launchCmd = provider.Command()
				for _, a := range launchArgs {
					launchCmd += " " + a
				}
			}
			opts := crew.StartOpts{
				Persona:         personaName,
				Repos:           repos,
				SkipPermissions: skip,
				Resume:          true,
				Agent:           string(agentTool),
				Model:           model,
				ResumeSessionID: resumeSessionID,
				LaunchCmd:       launchCmd,
			}

			// Resume requires an orchestrator identity just like start: no
			// silent-fallthrough path remains that would rebind the worker with
			// an empty orchestrator_id.
			if orchestratorID == "" {
				return fmt.Errorf(
					"no orchestrator identity found for resume. Run `jeff orchestrator init` in the project directory, or set JEFF_ORCHESTRATOR_ID.",
				)
			}
			fmt.Fprintf(os.Stderr, "Orchestrator identity: %s\n", orchestratorID)
			sess, err := crew.StartWorkerForOrchestrator(cs, orchestratorID, taskID, td.Path, opts)
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Resumed %s in tmux window %s\n", taskID, sess.WindowName)
			return nil
		},
	}
}

func crewSessionIDCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "session-id <gig-id> <session-id>",
		Short: "Record a Claude session ID for a worker (called by SessionStart hook)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := args[0]
			sessionID := args[1]

			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return fmt.Errorf("open crew store: %w", err)
			}
			defer cs.Close()

			if err := cs.AppendSessionID(taskID, sessionID); err != nil {
				return fmt.Errorf("append session ID: %w", err)
			}
			return nil
		},
	}
}

func crewListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Show crew sessions (filtered to current orchestrator by default)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			showAll, _ := cmd.Flags().GetBool("all")
			orchestratorFlag, _ := cmd.Flags().GetString("orchestrator")

			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			// Refresh state from tmux.
			gigStore, _ := openGigStore()
			if gigStore != nil {
				defer gigStore.Close()
			}
			crew.Refresh(cs, func(taskID string) bool {
				if gigStore == nil {
					return false
				}
				t, err := gigStore.Get(taskID)
				return err == nil && t.Status.IsTerminal()
			})

			// --all: no orchestrator filter, show all statuses.
			// --orchestrator <id>: filter to specific orchestrator.
			// default: filter to the current identity (if any), active only.
			orchestratorFilter, ferr := resolveCrewListOrchestratorFilter(showAll, orchestratorFlag)
			if ferr != nil {
				return ferr
			}
			activeOnly := !showAll
			sessions, err := cs.ListSessions(activeOnly, orchestratorFilter)
			if err != nil {
				return err
			}

			// Print a filter header so callers know the scope.
			switch {
			case showAll:
				fmt.Fprintln(os.Stdout, "All crew:")
			case orchestratorFilter != "":
				fmt.Fprintf(os.Stdout, "Crew for orchestrator %s:\n", orchestratorFilter)
			}

			if len(sessions) == 0 {
				if activeOnly {
					fmt.Fprintln(os.Stderr, "(no running sessions — use --all to see all)")
				} else {
					fmt.Fprintln(os.Stderr, "(no crew sessions)")
				}
				return nil
			}

			// Column header.
			fmt.Fprintf(os.Stdout, "%-12s %-10s %-8s %-12s %-12s %s\n",
				"TASK", "PERSONA", "MODEL", "STATUS", "STARTED", "LAST CHECKPOINT")

			for _, sess := range sessions {
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
				// Pad status to 12 visible chars; ANSI escapes don't count toward width.
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

	cmd.Flags().BoolP("all", "a", false, "Show all sessions from all orchestrators, including done/failed/stopped")
	cmd.Flags().String("orchestrator", "", "Show only sessions belonging to this orchestrator ID")
	return cmd
}

// resolveCrewListOrchestratorFilter returns the orchestrator ID to filter by.
// Returns "" (no filter) when showAll is true or when no identity is configured.
// Returns orchestratorFlag when explicitly provided. Otherwise falls back to the
// current durable identity. A malformed identity file surfaces as an error so
// `crew list` fails loud (use --all to bypass identity resolution entirely).
func resolveCrewListOrchestratorFilter(showAll bool, orchestratorFlag string) (string, error) {
	if showAll {
		return "", nil
	}
	if orchestratorFlag != "" {
		return orchestratorFlag, nil
	}
	id, _, err := detectOrchestratorID()
	if err != nil {
		return "", err
	}
	return id, nil
}

func crewStatusCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "status [gig-id]",
		Short: "Detailed status of a worker session",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			if len(args) == 0 {
				// Show summary of all.
				return crewListCmd().RunE(cmd, args)
			}

			taskID := args[0]
			sess, err := cs.GetSession(taskID)
			if err != nil {
				return fmt.Errorf("session not found: %w", err)
			}

			if jsonOut {
				data, _ := json.MarshalIndent(sess, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			// Detailed output.
			gigStore, _ := openGigStore()
			if gigStore != nil {
				defer gigStore.Close()
			}

			fmt.Fprintf(os.Stdout, "Task:       %s\n", sess.TaskID)
			fmt.Fprintf(os.Stdout, "Status:     %s (tmux: %s:%s)\n", sess.Status, sess.TmuxSession, sess.WindowName)
			if sess.Persona != "" {
				fmt.Fprintf(os.Stdout, "Persona:    %s\n", sess.Persona)
			}
			if len(sess.Repos) > 0 {
				fmt.Fprintf(os.Stdout, "Repos:      %s\n", strings.Join(sess.Repos, ", "))
			}
			fmt.Fprintf(os.Stdout, "Started:    %s ago\n", relativeTime(sess.StartedAt))

			// Checkpoint info.
			if gigStore != nil {
				if cp, err := gigStore.LatestCheckpoint(sess.TaskID); err == nil && cp != nil {
					fmt.Fprintf(os.Stdout, "Checkpoint: %q (%s ago)\n", cp.Done, relativeTime(cp.CreatedAt))
					if cp.Next != "" {
						fmt.Fprintf(os.Stdout, "Next:       %s\n", cp.Next)
					}
				}
			}

			// Pending messages.
			count, _ := cs.PendingCount(taskID, "to_worker")
			fmt.Fprintf(os.Stdout, "Inbox:      %d pending\n", count)

			// Pane capture.
			if crew.HasWindow(sess.WindowName) {
				target := crew.SessionTarget(sess.TmuxSession, sess.WindowName)
				if pane, err := crew.CapturePane(target, 5); err == nil && pane != "" {
					fmt.Fprintf(os.Stdout, "Pane (last 5 lines):\n")
					for _, line := range strings.Split(pane, "\n") {
						fmt.Fprintf(os.Stdout, "  > %s\n", line)
					}
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func crewSendCmd() *cobra.Command {
	var interrupt bool

	cmd := &cobra.Command{
		Use:   "send <gig-id> <message>",
		Short: "Send a message to a worker agent",
		Long: `Send a message to a worker agent. The message is stored in the inbox
and delivered directly into the agent's tmux pane.

Use --interrupt to Ctrl-C the agent first, then send the message
(useful when the agent is mid-turn and you need to redirect it).`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, content := args[0], args[1]

			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			msg, err := crew.Send(cs, taskID, content, interrupt)
			if err != nil {
				return err
			}

			if interrupt {
				fmt.Fprintf(os.Stderr, "Interrupted and sent message %s to %s\n", msg.ID, taskID)
			} else {
				fmt.Fprintf(os.Stderr, "Sent message %s to %s\n", msg.ID, taskID)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&interrupt, "interrupt", false, "Interrupt (Ctrl-C) the agent before sending")
	return cmd
}

func crewAskCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ask [gig-id] <message>",
		Short: "Send a message from a worker to its orchestrator",
		Long:  "Worker sends a to_orchestrator message. Looks up orchestrator via orchestrator_id FK and delivers to orchestrator pane.",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var taskID, content string
			if len(args) == 2 {
				taskID, content = args[0], args[1]
			} else {
				// Single arg: resolve task ID from cwd.
				var err error
				taskID, _, err = resolveTaskID(nil)
				if err != nil {
					return fmt.Errorf("provide task ID or run from task dir: %w", err)
				}
				content = args[0]
			}

			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			msg, err := crew.Ask(cs, taskID, content)
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Sent %s to orchestrator (from %s)\n", msg.ID, taskID)
			return nil
		},
	}
}

func crewStopCmd() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "stop [gig-id]",
		Short: "Stop a worker session",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			if all {
				if err := crew.StopAll(cs); err != nil {
					return err
				}
				fmt.Fprintln(os.Stderr, "Stopped all crew sessions")
				return nil
			}

			if len(args) == 0 {
				return fmt.Errorf("provide a task ID or use --all")
			}

			taskID := args[0]

			// Probe window state before stopping so we can give informative output.
			paneAlive := false
			if sess, err := cs.GetSession(taskID); err == nil {
				paneAlive = crew.HasWindowInSession(sess.TmuxSession, sess.WindowName)
			}

			if err := crew.Stop(cs, taskID); err != nil {
				return err
			}

			if paneAlive {
				fmt.Fprintf(os.Stderr, "Stopped %s\n", taskID)
			} else {
				fmt.Fprintf(os.Stderr, "pane was already gone — DB reconciled for %s\n", taskID)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Stop all sessions")
	return cmd
}

func crewAttachCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "attach <gig-id>",
		Short: "Attach to a worker's tmux window",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			sess, err := cs.GetSession(args[0])
			if err != nil {
				return fmt.Errorf("session not found: %w", err)
			}

			fmt.Fprintf(os.Stderr, "Attaching to %s...\n", sess.WindowName)
			return crew.AttachSession(sess.WindowName)
		},
	}
}

func crewCaptureCmd() *cobra.Command {
	var lines int

	cmd := &cobra.Command{
		Use:   "capture <gig-id>",
		Short: "Capture terminal output from a worker's tmux pane",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			sess, err := cs.GetSession(args[0])
			if err != nil {
				return fmt.Errorf("session not found: %w", err)
			}

			target := crew.SessionTarget(sess.TmuxSession, sess.WindowName)
			out, err := crew.CapturePane(target, lines)
			if err != nil {
				return err
			}
			fmt.Println(out)
			return nil
		},
	}

	cmd.Flags().IntVar(&lines, "lines", 50, "Number of lines to capture")
	return cmd
}

func crewInboxCmd() *cobra.Command {
	var (
		countOnly bool
		format    string
		ackAll    bool
	)

	cmd := &cobra.Command{
		Use:   "inbox [gig-id]",
		Short: "Show pending messages for a worker (used by hook)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := ""
			if len(args) > 0 {
				taskID = args[0]
			} else {
				var err error
				taskID, _, err = resolveTaskID(nil)
				if err != nil {
					return fmt.Errorf("provide task ID or run from task dir: %w", err)
				}
			}

			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			if ackAll {
				return cs.AckAll(taskID, "to_worker")
			}

			if countOnly {
				count, err := cs.PendingCount(taskID, "to_worker")
				if err != nil {
					return err
				}
				fmt.Println(count)
				return nil
			}

			msgs, err := cs.PendingMessages(taskID, "to_worker")
			if err != nil {
				return err
			}

			if len(msgs) == 0 {
				if format != "agent" {
					fmt.Println("0")
				}
				return nil
			}

			switch format {
			case "agent":
				// Format for hook injection. Auto-ack since delivery is confirmed.
				fmt.Println("## Orchestrator Messages")
				fmt.Println()
				for _, m := range msgs {
					fmt.Printf("[Orchestrator %s]: %s\n", m.ID, m.Content)
					cs.AckMessage(m.ID, "")
				}
			case "json":
				data, _ := json.MarshalIndent(msgs, "", "  ")
				fmt.Println(string(data))
			default:
				for _, m := range msgs {
					fmt.Printf("[%s] %s: %s\n", m.Type, m.ID, m.Content)
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&countOnly, "count", false, "Only print the count")
	cmd.Flags().StringVar(&format, "format", "text", "Output format: text, json, agent")
	cmd.Flags().BoolVar(&ackAll, "ack", false, "Acknowledge all pending messages")
	return cmd
}

func crewAckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ack <msg-id> [response]",
		Short: "Acknowledge a message from the orchestrator",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			msgID := args[0]
			response := ""
			if len(args) > 1 {
				response = args[1]
			}

			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			if err := cs.AckMessage(msgID, response); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Acked %s\n", msgID)
			return nil
		},
	}
}

func crewTouchCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "touch <gig-id>",
		Short:  "Update last_seen heartbeat for a worker session",
		Args:   cobra.ExactArgs(1),
		Hidden: true, // Used by worker-heartbeat hook.
		RunE: func(cmd *cobra.Command, args []string) error {
			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()
			return cs.TouchSession(args[0])
		},
	}
}

func crewOrchestratorInboxCmd() *cobra.Command {
	var (
		countOnly bool
		format    string
		ackAll    bool
	)

	cmd := &cobra.Command{
		Use:    "orchestrator-inbox <orchestrator-id>",
		Short:  "Show pending messages from workers to an orchestrator",
		Args:   cobra.ExactArgs(1),
		Hidden: true, // Used by orchestrator-inbox hook.
		RunE: func(cmd *cobra.Command, args []string) error {
			orchestratorID := args[0]

			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			if ackAll {
				// Ack all to_orchestrator messages for this orchestrator's workers.
				workers, err := cs.WorkersForOrchestrator(orchestratorID)
				if err != nil {
					return err
				}
				for _, w := range workers {
					_ = cs.AckAll(w.TaskID, "to_orchestrator")
				}
				return nil
			}

			msgs, err := cs.PendingOrchestratorMessages(orchestratorID)
			if err != nil {
				return err
			}

			if countOnly {
				fmt.Println(len(msgs))
				return nil
			}

			if len(msgs) == 0 {
				if format != "agent" {
					fmt.Println("0")
				}
				return nil
			}

			switch format {
			case "agent":
				// Format for hook injection. Auto-ack since delivery is confirmed.
				fmt.Println("## Worker Messages")
				fmt.Println()
				for _, m := range msgs {
					fmt.Printf("[Worker %s]: %s\n", m.TaskID, m.Content)
					cs.AckMessage(m.ID, "")
				}
			case "json":
				data, _ := json.MarshalIndent(msgs, "", "  ")
				fmt.Println(string(data))
			default:
				for _, m := range msgs {
					fmt.Printf("[%s] %s from %s: %s\n", m.Type, m.ID, m.TaskID, m.Content)
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&countOnly, "count", false, "Only print the count")
	cmd.Flags().StringVar(&format, "format", "text", "Output format: text, json, agent")
	cmd.Flags().BoolVar(&ackAll, "ack", false, "Acknowledge all pending messages")
	return cmd
}

func crewEventsCmd() *cobra.Command {
	var (
		since  string
		taskID string
	)

	cmd := &cobra.Command{
		Use:   "events",
		Short: "Poll gig events from active crew sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			gigStore, err := openGigStore()
			if err != nil {
				return err
			}
			defer gigStore.Close()

			duration, err := time.ParseDuration(since)
			if err != nil {
				return fmt.Errorf("invalid --since: %w", err)
			}

			cutoff := time.Now().Add(-duration)
			events, err := gigStore.EventsSince(cutoff)
			if err != nil {
				return err
			}

			// If filtering by task, get active session IDs.
			var filterIDs map[string]bool
			if taskID != "" {
				filterIDs = map[string]bool{taskID: true}
			} else {
				cs, err := crew.Open(cfg.Home)
				if err == nil {
					defer cs.Close()
					sessions, _ := cs.ListSessions(false, "")
					if len(sessions) > 0 {
						filterIDs = make(map[string]bool)
						for _, s := range sessions {
							filterIDs[s.TaskID] = true
						}
					}
				}
			}

			for _, ev := range events {
				if filterIDs != nil && !filterIDs[ev.TaskID] {
					continue
				}
				age := relativeTime(ev.Timestamp)
				summary := ev.NewValue
				if len(summary) > 60 {
					summary = summary[:57] + "..."
				}
				if ev.Type == gig.EventStatusChanged {
					summary = ev.OldValue + " → " + ev.NewValue
				}
				fmt.Fprintf(os.Stdout, "%-8s %-12s %-14s %s\n",
					age, ev.TaskID, ev.Type, summary)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&since, "since", "5m", "Time window (e.g. 5m, 1h, 30m)")
	cmd.Flags().StringVar(&taskID, "task", "", "Filter to a specific task")
	return cmd
}

func crewCleanupCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Reconcile tmux windows with crew DB, remove orphans",
		Long: `Reconcile tmux windows with the crew database:

  1. Kill orphaned tmux windows (no matching DB session)
  2. Mark stale DB sessions as failed (tmux window gone)
  3. Mark stale orchestrators as stopped (tmux session gone)

Use --dry-run to preview what would be cleaned up without making changes.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			result, err := crew.Cleanup(cs, cfg.Home, dryRun)
			if err != nil {
				return err
			}

			if result.IsClean() {
				fmt.Fprintln(os.Stderr, "Nothing to clean up — all in sync.")
				return nil
			}

			prefix := ""
			if dryRun {
				prefix = "[dry-run] "
			}

			if len(result.OrphanedWindows) > 0 {
				fmt.Fprintf(os.Stderr, "%sOrphaned tmux windows:\n", prefix)
				for _, tw := range result.OrphanedWindows {
					action := "killed"
					if dryRun {
						action = "would kill"
					}
					fmt.Fprintf(os.Stderr, "  %s %s:%s\n", action, tw.Session, tw.Window)
				}
			}

			if len(result.StaleSessions) > 0 {
				fmt.Fprintf(os.Stderr, "%sStale DB sessions:\n", prefix)
				for _, taskID := range result.StaleSessions {
					action := "marked failed"
					if dryRun {
						action = "would mark failed"
					}
					fmt.Fprintf(os.Stderr, "  %s %s\n", action, taskID)
				}
			}

			if len(result.StaleOrch) > 0 {
				fmt.Fprintf(os.Stderr, "%sStale orchestrators:\n", prefix)
				for _, id := range result.StaleOrch {
					action := "marked stopped"
					if dryRun {
						action = "would mark stopped"
					}
					fmt.Fprintf(os.Stderr, "  %s %s\n", action, id)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview cleanup without making changes")
	return cmd
}

// pickupTask runs the full pickup sequence and returns the task directory.
// Extracted from pickupCmd for reuse by crew start.
func pickupTask(taskID, personaName string, repos, reposReadonly []string, orchestratorID string, agentOverride jeff.AgentTool) (string, error) {
	store, err := openGigStore()
	if err != nil {
		return "", err
	}
	defer store.Close()

	if err := jeff.EnsureAttrs(store); err != nil {
		return "", fmt.Errorf("ensure attrs: %w", err)
	}

	task, err := store.Get(taskID)
	if err != nil {
		return "", fmt.Errorf("task %s not found: %w", taskID, err)
	}

	claimResult, err := store.Claim(taskID, "jeff")
	if err != nil {
		return "", fmt.Errorf("claim: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Claimed %s: %s\n", taskID, task.Title)
	if claimResult.ParentProgressed {
		fmt.Fprintf(os.Stderr, "Parent %s → in_progress\n", claimResult.ParentID)
	}

	td, err := workspace.Create(cfg.Home, taskID, task.Title)
	if err != nil {
		return "", fmt.Errorf("create workspace: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Workspace: %s\n", td.Path)

	allRepos := append([]string{}, repos...)
	allRepos = append(allRepos, reposReadonly...)
	if len(allRepos) > 0 {
		reposJSON, _ := json.Marshal(allRepos)
		if err := store.SetAttr(taskID, jeff.AttrRepos, string(reposJSON)); err != nil {
			return "", fmt.Errorf("set repos attr: %w", err)
		}
	}
	if personaName != "" {
		if err := store.SetAttr(taskID, jeff.AttrPersona, personaName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: set persona attr: %v\n", err)
		}
	}
	if err := store.SetAttr(taskID, jeff.AttrTeamSize, "1"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: set team_size attr: %v\n", err)
	}

	taskJSON := buildTaskJSON(store, task)
	for _, repoName := range repos {
		rc := cfg.Repos[repoName]
		branch, err := resolveRepoBranch(rc, taskJSON, taskID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: branch name for %s: %v, using %s\n", repoName, err, taskID)
			branch = taskID
		}

		opts := workspace.WorktreeOpts{
			JeffHome: cfg.Home,
			RepoName: repoName,
			Branch:   branch,
			TaskDir:  td.Path,
		}
		if rc != nil {
			opts.BaseBranch = rc.BaseBranch
			opts.PostSetup = rc.PostSetup
		}

		wtDir, err := workspace.WorktreeAdd(opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: worktree for %s: %v\n", repoName, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "Worktree: %s → %s\n", repoName, wtDir)
	}

	for _, repoName := range reposReadonly {
		target, err := workspace.ReadonlyLink(cfg.Home, repoName, td.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: readonly link for %s: %v\n", repoName, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "Readonly: %s → %s\n", repoName, target)
	}

	if personaName != "" {
		if err := memory.EnsurePersonaDir(cfg.Home, personaName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: persona memory: %v\n", err)
		}
	}
	for _, repoName := range allRepos {
		if err := memory.EnsureRepoDir(cfg.Home, repoName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: repo learnings: %v\n", err)
		}
	}

	if err := writeTaskClaudeMD(td.Path, cfg.Home, store, task, personaName, allRepos); err != nil {
		return "", fmt.Errorf("write task CLAUDE.md: %w", err)
	}

	var memScopes []string
	if personaName != "" {
		if content, _ := memory.LoadPersonaMemory(cfg.Home, personaName); content != "" {
			memScopes = append(memScopes, "persona:"+personaName)
		}
	}
	for _, r := range allRepos {
		if content, _ := memory.LoadRepoLearnings(cfg.Home, r); content != "" {
			memScopes = append(memScopes, "repo:"+r)
		}
	}
	if len(memScopes) > 0 {
		if data, err := json.Marshal(memScopes); err == nil {
			if err := store.SetAttr(taskID, jeff.AttrMemoryLoaded, string(data)); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: set memory_loaded attr: %v\n", err)
			}
		}
	}

	// Append a readonly notice so the agent knows which repos it must not modify.
	if len(reposReadonly) > 0 {
		if err := appendReadonlyNote(td.Path, reposReadonly); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: readonly note: %v\n", err)
		}
	}

	// Inject memory addendum + suppress native memory for the active agent.
	// RunSessionStart is idempotent; the bash hook re-runs it on session resume.
	agentKind := string(agentOverride)
	if agentKind == "" {
		agentKind = string(cfg.Agent)
	}
	if err := hooks.RunSessionStart(cfg.Home, td.Path, personaName, taskID, allRepos, agentKind); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: memory session-start: %v\n", err)
	}

	reg := hooks.DefaultRegistry()
	mgr := hooks.NewManager(reg)
	hctx := hooks.HookContext{
		JeffHome:           cfg.Home,
		TargetDir:          td.Path,
		GigHome:            cfg.GigHome,
		TaskID:             taskID,
		OrchestratorID:     orchestratorID,
		CheckpointPatterns: cfg.CheckpointPatterns,
		Persona:            personaName,
		Repos:              allRepos,
	}
	// Install hooks for ALL registered agents so the workspace is ready
	// regardless of which agent launches (same pattern as context aliases).
	taskEnabled := hooks.EnabledForSource(cfg.Hooks, hooks.SourceTask, reg)
	if len(taskEnabled) > 0 {
		for _, agent := range jeff.RegisteredAgents() {
			p := jeff.GetProvider(agent)
			if p == nil {
				continue
			}
			if err := mgr.Sync(td.Path, taskEnabled, p.HookDeliveryKey(), hctx); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: task hooks (%s): %v\n", agent, err)
			}
		}
	}

	// Always alias .gemini/skills → .claude/skills before injecting skills,
	// regardless of whether the gemini agent is registered. Skills should be
	// in sync across agents, so gemini sessions see what claude sessions get.
	if err := jeffembed.EnsureGeminiSkillsAlias(td.Path); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: alias .gemini/skills: %v\n", err)
	}

	// Inject matching skills into ALL registered agent config dirs that support skills.
	// This ensures skills are available regardless of which agent launches in this workspace.
	mctx := &skill.MatchContext{
		Persona: personaName,
		GigType: string(task.Type),
		Labels:  task.Labels,
	}
	var injectedNames []string
	injectedSet := make(map[string]bool)
	for _, agent := range jeff.RegisteredAgents() {
		p := jeff.GetProvider(agent)
		if p == nil || p.SkillsSubdir() == "" {
			continue
		}
		names, err := skill.InjectMatchingTo(cfg.Home, td.Path, p.ConfigDir(), p.SkillsSubdir(), mctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skill injection (%s): %v\n", agent, err)
		}
		if len(injectedNames) == 0 {
			injectedNames = names
		}
		for _, n := range names {
			injectedSet[n] = true
		}
	}
	if len(injectedNames) > 0 {
		fmt.Fprintf(os.Stderr, "Skills: %s\n", strings.Join(injectedNames, ", "))
	}
	if len(injectedSet) > 0 {
		names := make([]string, 0, len(injectedSet))
		for n := range injectedSet {
			names = append(names, n)
		}
		sort.Strings(names)
		if data, err := json.Marshal(names); err == nil {
			if err := store.SetAttr(taskID, jeff.AttrSkillsLoaded, string(data)); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: set skills_loaded attr: %v\n", err)
			}
		}
	}

	return td.Path, nil
}

// relativeTime returns a human-friendly relative time string.
func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func crewSignalOrchestratorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "signal-orchestrator <gig-id> <message>",
		Short: "Send a signal directly to a worker's orchestrator pane",
		Long:  "Delivers a formatted message to the orchestrator's tmux pane. Used by stall watchdogs and completion hooks.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, message := args[0], args[1]

			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			if err := crew.SignalOrchestrator(cs, taskID, message); err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Signaled orchestrator for %s\n", taskID)
			return nil
		},
	}
}

func crewWorkerStoppedCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "worker-stopped <gig-id>",
		Short:  "Signal that a worker's agent turn ended (used by the worker-stop hook)",
		Long:   "Persists a de-duplicated to_orchestrator stop signal and wakes the orchestrator pane. Durable: recovered by the orchestrator-inbox poll even if the pane is dead.",
		Args:   cobra.ExactArgs(1),
		Hidden: true, // Used by the worker-stop hook.
		RunE: func(cmd *cobra.Command, args []string) error {
			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()
			return crew.SignalWorkerStopped(cs, args[0])
		},
	}
}

func crewCheckStallsCmd() *cobra.Command {
	var threshold string

	cmd := &cobra.Command{
		Use:   "check-stalls",
		Short: "Check for stalled workers and signal their orchestrators",
		RunE: func(cmd *cobra.Command, args []string) error {
			dur, err := time.ParseDuration(threshold)
			if err != nil {
				return fmt.Errorf("invalid threshold: %w", err)
			}

			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			n, err := crew.CheckStalls(cs, dur)
			if err != nil {
				return err
			}

			if n > 0 {
				fmt.Fprintf(os.Stderr, "Signaled %d stalled worker(s)\n", n)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&threshold, "threshold", "10m", "Idle time before a worker is considered stalled")
	return cmd
}

// crewStatusLabel returns a colored status string for crew sessions.
func crewStatusLabel(status string) string {
	switch status {
	case "running":
		return colorize(cGreen+cBold, "● running")
	case "starting":
		return colorize(cYellow, "◉ starting")
	case "done":
		return colorize(cDim, "○ done")
	case "failed":
		return colorize(cRed+cBold, "✕ failed")
	case "stopped":
		return colorize(cYellow, "■ stopped")
	default:
		return status
	}
}

// needsShellQuoting returns true if a string contains characters that
// would be interpreted by the shell (parens, quotes, semicolons, etc.).
func needsShellQuoting(s string) bool {
	for _, c := range s {
		switch c {
		case ' ', '(', ')', '\'', '"', '`', '$', '\\', ';', '&', '|', '<', '>', '{', '}', '!', '~', '#', '*', '?', '[', ']':
			return true
		}
	}
	return false
}

// shellQuote wraps a string in single quotes for safe shell passing.
// Single quotes inside the string are handled by ending the quote,
// adding an escaped single quote, and resuming the quote.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func resolveResumeAgentModel(existing *crew.Session, defaultAgent jeff.AgentTool, defaultModel string) (jeff.AgentTool, string) {
	agent := defaultAgent
	model := defaultModel

	if existing != nil {
		if existing.Agent != "" {
			agent = jeff.AgentTool(existing.Agent)
		}
		if existing.Model != "" {
			model = existing.Model
		}
	}

	return agent, model
}
