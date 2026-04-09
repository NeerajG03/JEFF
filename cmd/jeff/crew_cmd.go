package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/crew"
	"github.com/NeerajG03/JEFF/hooks"
	"github.com/NeerajG03/JEFF/memory"
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
		crewCheckStallsCmd(),
	)

	return cmd
}

func crewStartCmd() *cobra.Command {
	var (
		personaName    string
		repos          []string
		orchestratorID string
	)

	cmd := &cobra.Command{
		Use:   "start <gig-id>",
		Short: "Claim a task and launch a worker agent in tmux",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := args[0]

			// Auto-detect orchestrator from tmux session name if not set.
			// Must happen before pickupTask so hooks get the orchestrator ID.
			if orchestratorID == "" {
				if os.Getenv("TMUX") != "" {
					out, err := exec.Command("tmux", "display-message", "-p", "#{session_name}").Output()
					if err == nil {
						name := strings.TrimSpace(string(out))
						if matched, _ := regexp.MatchString(`^jeff-\d+$`, name); matched {
							orchestratorID = name
							fmt.Fprintf(os.Stderr, "Auto-detected orchestrator: %s\n", orchestratorID)
						}
					}
				}
			}

			// Run the pickup sequence (claim, workspace, worktrees, hooks, skills).
			taskDir, err := pickupTask(taskID, personaName, repos, orchestratorID)
			if err != nil {
				return err
			}

			// Open crew store and start tmux session.
			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return fmt.Errorf("open crew store: %w", err)
			}
			defer cs.Close()

			var sess *crew.Session
			if orchestratorID != "" {
				// Launch as a tab in the orchestrator's tmux session.
				sess, err = crew.StartWorkerForOrchestrator(cs, orchestratorID, taskID, taskDir, crew.StartOpts{
					Persona: personaName,
					Repos:   repos,
					Agent:   string(cfg.Agent),
				})
			} else {
				sess, err = crew.Start(cs, taskID, taskDir, crew.StartOpts{
					Persona: personaName,
					Repos:   repos,
					Agent:   string(cfg.Agent),
				})
			}
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
	cmd.Flags().StringVar(&orchestratorID, "orchestrator", "", "Orchestrator ID to attach worker to")
	cmd.ValidArgsFunction = readyTaskCompletion
	cmd.RegisterFlagCompletionFunc("persona", personaCompletion)
	cmd.RegisterFlagCompletionFunc("repos", repoNameCompletion)
	cmd.RegisterFlagCompletionFunc("orchestrator", orchestratorCompletion)
	return cmd
}

func crewResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <gig-id>",
		Short: "Resume a worker agent in tmux (workspace must exist)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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

			// Detect persona and repos from existing workspace.
			personaName := detectPersona(td.Path)
			repos := detectRepos(td.Path)

			sess, err := crew.Start(cs, taskID, td.Path, crew.StartOpts{
				Persona: personaName,
				Repos:   repos,
				Resume:  true,
				Agent:   string(cfg.Agent),
			})
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Resumed %s in tmux window %s\n", taskID, sess.WindowName)
			return nil
		},
	}
}

func crewListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show all crew sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
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

			sessions, err := cs.ListSessions(false)
			if err != nil {
				return err
			}

			if len(sessions) == 0 {
				fmt.Fprintln(os.Stderr, "(no crew sessions)")
				return nil
			}

			// Header.
			fmt.Fprintf(os.Stdout, "%-12s %-10s %-10s %-12s %s\n",
				"TASK", "PERSONA", "STATUS", "STARTED", "LAST CHECKPOINT")

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

				status := crewStatusLabel(sess.Status)
				fmt.Fprintf(os.Stdout, "%-12s %-10s %-10s %-12s %s\n",
					sess.TaskID, sess.Persona, status, started, ckpt)
			}
			return nil
		},
	}
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
	var msgType string

	cmd := &cobra.Command{
		Use:   "send <gig-id> <message>",
		Short: "Send a message to a worker agent",
		Long: `Send a message to a worker agent. Message types:

  nudge   — one-way instruction via hook (low context pollution)
  status  — asks via /btw (sidechain, reads response from pane)
  divert  — interrupts agent, then sends message (heavy)
  normal  — types directly into agent input (full context impact)`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, content := args[0], args[1]

			cs, err := crew.Open(cfg.Home)
			if err != nil {
				return err
			}
			defer cs.Close()

			mt := crew.MessageType(msgType)
			msg, err := crew.Send(cs, taskID, mt, content)
			if err != nil {
				return err
			}

			switch mt {
			case crew.MsgNudge:
				fmt.Fprintf(os.Stderr, "Sent nudge %s to %s (will appear at next tool use)\n", msg.ID, taskID)
			case crew.MsgStatus:
				fmt.Fprintf(os.Stderr, "Sent /btw to %s tmux pane. Capturing response in 10s...\n", taskID)
				time.Sleep(10 * time.Second)
				sess, _ := cs.GetSession(taskID)
				if sess != nil && crew.HasWindow(sess.WindowName) {
					target := crew.SessionTarget(sess.TmuxSession, sess.WindowName)
					if pane, err := crew.CapturePane(target, 15); err == nil {
						fmt.Fprintf(os.Stdout, "Response:\n%s\n", pane)
					}
				}
			case crew.MsgDivert:
				fmt.Fprintf(os.Stderr, "Diverted %s: %q\n", taskID, content)
			case crew.MsgNormal:
				fmt.Fprintf(os.Stderr, "Sent message to %s\n", taskID)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&msgType, "type", "nudge", "Message type: nudge, status, divert, normal")
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

			if err := crew.Stop(cs, args[0]); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Stopped %s\n", args[0])
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
					sessions, _ := cs.ListSessions(false)
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
func pickupTask(taskID, personaName string, repos []string, orchestratorID string) (string, error) {
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

	if len(repos) > 0 {
		reposJSON, _ := json.Marshal(repos)
		if err := store.SetAttr(taskID, jeff.AttrRepos, string(reposJSON)); err != nil {
			return "", fmt.Errorf("set repos attr: %w", err)
		}
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

	if personaName != "" {
		if err := memory.EnsurePersonaDir(cfg.Home, personaName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: persona memory: %v\n", err)
		}
	}
	for _, repoName := range repos {
		if err := memory.EnsureRepoDir(cfg.Home, repoName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: repo learnings: %v\n", err)
		}
	}

	if err := writeTaskClaudeMD(td.Path, cfg.Home, task, personaName, repos); err != nil {
		return "", fmt.Errorf("write task CLAUDE.md: %w", err)
	}

	if personaName != "" || len(repos) > 0 {
		if err := memory.InstallLearnCommand(td.Path, taskID, personaName, cfg.Home, repos); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: learn command: %v\n", err)
		}
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
	}
	taskEnabled := hooks.EnabledForSource(cfg.Hooks, hooks.SourceTask, reg)
	if len(taskEnabled) > 0 {
		agent := hooks.AgentTool(cfg.Agent)
		if err := mgr.Sync(td.Path, taskEnabled, agent, hctx); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: task hooks: %v\n", err)
		}
	}

	mctx := &skill.MatchContext{
		Persona: personaName,
		GigType: string(task.Type),
		Labels:  task.Labels,
	}
	injected, err := skill.InjectMatching(cfg.Home, td.Path, mctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: skill injection: %v\n", err)
	} else if len(injected) > 0 {
		fmt.Fprintf(os.Stderr, "Skills: %s\n", strings.Join(injected, ", "))
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
