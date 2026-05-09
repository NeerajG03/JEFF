package crew

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DefaultPrompt is sent to workers after the agent launches so they begin
// working immediately instead of sitting idle.
const DefaultPrompt = "Read your task context using `gig show` for your task ID and begin working."

// isActiveStatus reports whether a session status means the worker may still be running.
func isActiveStatus(status string) bool {
	return status == "running" || status == "starting"
}

// windowExistsFunc checks whether a named window exists in a tmux session.
// It is a variable type so tests can substitute a mock without a real tmux binary.
type windowExistsFunc func(session, window string) bool

// preflightStartCheck is the testable core of PreflightStart.
func preflightStartCheck(store *Store, taskID string, hasWindow windowExistsFunc) error {
	existing, err := store.GetSession(taskID)
	if err != nil {
		return nil // No existing session — safe to start fresh.
	}
	if !isActiveStatus(existing.Status) {
		return nil // Stopped/done/failed — safe to restart.
	}
	if hasWindow(existing.TmuxSession, existing.WindowName) {
		return fmt.Errorf(
			"worker already running for %s in tmux %s:%s — use `jeff crew send %s \"...\"` to talk to it, or `tmux attach -t %s:%s` to view it",
			taskID, existing.TmuxSession, existing.WindowName,
			taskID, existing.TmuxSession, existing.WindowName,
		)
	}
	return fmt.Errorf(
		"stale DB row for %s: DB says running but tmux window %q is gone — run `jeff crew cleanup` to reconcile, then retry",
		taskID, existing.WindowName,
	)
}

// preflightResumeCheck is the testable core of PreflightResume.
func preflightResumeCheck(store *Store, taskID string, hasWindow windowExistsFunc) error {
	existing, err := store.GetSession(taskID)
	if err != nil {
		return fmt.Errorf("no worker record for %s — use `jeff crew start %s` to launch fresh", taskID, taskID)
	}
	if !isActiveStatus(existing.Status) {
		return nil // Stopped/done/failed — safe to relaunch.
	}
	if hasWindow(existing.TmuxSession, existing.WindowName) {
		return fmt.Errorf(
			"worker already running for %s in tmux %s:%s — use `jeff crew send %s \"...\"` to talk to it, or `tmux attach -t %s:%s` to view it",
			taskID, existing.TmuxSession, existing.WindowName,
			taskID, existing.TmuxSession, existing.WindowName,
		)
	}
	return fmt.Errorf(
		"stale DB row for %s: DB says running but tmux window %q is gone — run `jeff crew cleanup` to reconcile, then retry",
		taskID, existing.WindowName,
	)
}

// PreflightStart checks whether starting a new worker for taskID is safe.
// Returns a human-readable error with a next-step hint when DB↔tmux state has drifted.
func PreflightStart(store *Store, taskID string) error {
	return preflightStartCheck(store, taskID, HasWindowInSession)
}

// PreflightResume checks whether resuming a worker for taskID is safe.
// Returns a human-readable error with a next-step hint when DB↔tmux state has drifted.
func PreflightResume(store *Store, taskID string) error {
	return preflightResumeCheck(store, taskID, HasWindowInSession)
}

// buildAgentCmd constructs the agent CLI command using the LaunchCmd if provided,
// or falls back to the legacy "claude --dangerously-skip-permissions" pattern.
func buildAgentCmd(launchCmd, agent, model, resumeSessionID string) string {
	if launchCmd != "" {
		return launchCmd
	}
	// Legacy fallback.
	if agent == "" {
		agent = "claude"
	}
	cmd := agent + " --dangerously-skip-permissions"
	if model != "" {
		cmd += " --model " + model
	}
	if resumeSessionID != "" {
		cmd += " --resume " + resumeSessionID
	}
	return cmd
}

// StartOpts configures a new crew session.
type StartOpts struct {
	Persona         string
	Repos           []string
	Resume          bool   // if true, skip pickup (workspace exists)
	Agent           string // agent command (e.g. "claude"), defaults to "claude"
	Model           string // model override (e.g. "sonnet", "opus", "haiku")
	ResumeSessionID string // session ID to resume via --resume
	Prompt          string // custom initial prompt (overrides DefaultPrompt; empty = default; ignored on resume)
	LaunchCmd       string // full CLI command (built by provider); if set, overrides Agent/Model/ResumeSessionID
}

// StartOrchestrator creates a new tmux session (jeff-N or jeff-<name>) and launches the
// orchestrator agent in the first window. Records the orchestrator in the DB.
// If name is non-empty, the session is named jeff-<name>; otherwise jeff-N is auto-assigned.
func StartOrchestrator(store *Store, jeffHome string, agent string, name string) (*Orchestrator, error) {
	if err := EnsureTmux(); err != nil {
		return nil, err
	}

	var id string
	if name != "" {
		id = "jeff-" + name
		// Verify the ID isn't already in use.
		if existing, err := store.GetOrchestrator(id); err == nil && existing.Status == "running" {
			return nil, fmt.Errorf("orchestrator %q is already running", id)
		}
	} else {
		var err error
		id, err = store.NextOrchestratorID()
		if err != nil {
			return nil, fmt.Errorf("next orchestrator ID: %w", err)
		}
	}

	windowName := "orchestrator"
	target, err := CreateSession(id, windowName, jeffHome)
	if err != nil {
		return nil, err
	}

	// Set JEFF_HOME on the tmux session so all panes inherit it.
	_ = tmuxRun("set-environment", "-t", id, "JEFF_HOME", jeffHome)
	// Set JEFF_ORCHESTRATOR_SESSION so workers can reliably detect which
	// orchestrator they belong to without querying tmux (Fix 2).
	_ = tmuxRun("set-environment", "-t", id, "JEFF_ORCHESTRATOR_SESSION", id)

	// Launch agent in the orchestrator window.
	agentCmd := buildAgentCmd("", agent, "", "")
	if err := SendCommand(target, agentCmd); err != nil {
		return nil, fmt.Errorf("launch orchestrator agent: %w", err)
	}

	paneID, _ := PaneID(target)

	now := time.Now().UTC()
	orch := &Orchestrator{
		ID:          id,
		TmuxSession: id,
		TmuxWindow:  windowName,
		TmuxPane:    paneID,
		StartedAt:   now,
		Status:      "running",
	}

	if err := store.PutOrchestrator(orch); err != nil {
		return nil, fmt.Errorf("record orchestrator: %w", err)
	}

	return orch, nil
}

// StartWorkerForOrchestrator creates a tmux window in the orchestrator's session
// for a worker, launches the agent, and records the session with orchestrator_id set.
func StartWorkerForOrchestrator(store *Store, orchestratorID, taskID, taskDir string, opts StartOpts) (*Session, error) {
	orch, err := store.GetOrchestrator(orchestratorID)
	if err != nil {
		return nil, fmt.Errorf("get orchestrator: %w", err)
	}

	if err := EnsureTmux(); err != nil {
		return nil, err
	}

	windowName := SanitizeWindowName(taskID)
	target, err := CreateWindowInSession(orch.TmuxSession, windowName, taskDir)
	if err != nil {
		return nil, err
	}

	// Export worker-specific env vars into the shell so subprocesses
	// (e.g. "jeff memory propose") route to the correct persona dir.
	exportWorkerEnv(target, "JEFF_PERSONA", opts.Persona)
	exportWorkerEnv(target, "JEFF_TASK_ID", taskID)

	// Record session BEFORE launching agent so the SessionStart hook
	// (which captures session_id) can find the row in the DB.
	now := time.Now().UTC()
	sess := &Session{
		TaskID:         taskID,
		TmuxSession:    orch.TmuxSession,
		WindowName:     windowName,
		TaskDir:        taskDir,
		Persona:        opts.Persona,
		Agent:          opts.Agent,
		Model:          opts.Model,
		Repos:          opts.Repos,
		OrchestratorID: orchestratorID,
		Status:         "running",
		StartedAt:      now,
		LastSeen:       now,
	}
	if err := store.PutSession(sess); err != nil {
		KillWindow(target)
		return nil, fmt.Errorf("record session: %w", err)
	}

	agentCmd := buildAgentCmd(opts.LaunchCmd, opts.Agent, opts.Model, opts.ResumeSessionID)
	if err := SendCommand(target, agentCmd); err != nil {
		KillWindow(target)
		return nil, fmt.Errorf("launch agent: %w", err)
	}

	// If LaunchCmd includes an inline prompt, the agent starts working immediately.
	// Only fall back to sleep+send for agents without inline prompt support.
	if opts.LaunchCmd == "" && opts.ResumeSessionID == "" {
		prompt := opts.Prompt
		if prompt == "" {
			prompt = DefaultPrompt
		}
		time.Sleep(3 * time.Second)
		if err := SendCommand(target, prompt); err != nil {
			KillWindow(target)
			return nil, fmt.Errorf("send initial prompt: %w", err)
		}
	}

	// Update with PID and pane ID now that the agent is running.
	pid, _ := WindowPID(target)
	paneID, _ := PaneID(target)
	sess.PID = pid
	sess.TmuxPane = paneID
	_ = store.PutSession(sess)

	return sess, nil
}

// Start creates a tmux window for a task and launches the agent.
// The caller is responsible for running pickup (workspace setup) beforehand.
// Start only handles tmux window creation + agent launch + session recording.
func Start(store *Store, taskID, taskDir string, opts StartOpts) (*Session, error) {
	if err := EnsureTmux(); err != nil {
		return nil, err
	}
	if err := EnsureSession(); err != nil {
		return nil, fmt.Errorf("ensure tmux session: %w", err)
	}

	// Use task ID as window name (short, unique).
	windowName := taskID
	target, err := CreateWindow(windowName, taskDir)
	if err != nil {
		return nil, err
	}

	// Export worker-specific env vars into the shell so subprocesses
	// (e.g. "jeff memory propose") route to the correct persona dir.
	exportWorkerEnv(target, "JEFF_PERSONA", opts.Persona)
	exportWorkerEnv(target, "JEFF_TASK_ID", taskID)

	// Record session BEFORE launching agent so the SessionStart hook
	// (which captures session_id) can find the row in the DB.
	now := time.Now().UTC()
	sess := &Session{
		TaskID:      taskID,
		TmuxSession: TmuxSessionName,
		WindowName:  windowName,
		TaskDir:     taskDir,
		Persona:     opts.Persona,
		Agent:       opts.Agent,
		Model:       opts.Model,
		Repos:       opts.Repos,
		Status:      "running",
		StartedAt:   now,
		LastSeen:    now,
	}
	if err := store.PutSession(sess); err != nil {
		KillWindow(target)
		return nil, fmt.Errorf("record session: %w", err)
	}

	// Launch agent.
	agentCmd := buildAgentCmd(opts.LaunchCmd, opts.Agent, opts.Model, opts.ResumeSessionID)
	if err := SendCommand(target, agentCmd); err != nil {
		KillWindow(target)
		return nil, fmt.Errorf("launch agent: %w", err)
	}

	// If LaunchCmd includes an inline prompt, the agent starts working immediately.
	// Only fall back to sleep+send for agents without inline prompt support.
	if opts.LaunchCmd == "" && opts.ResumeSessionID == "" {
		prompt := opts.Prompt
		if prompt == "" {
			prompt = DefaultPrompt
		}
		time.Sleep(3 * time.Second)
		if err := SendCommand(target, prompt); err != nil {
			KillWindow(target)
			return nil, fmt.Errorf("send initial prompt: %w", err)
		}
	}

	// Update with PID now that the agent is running.
	pid, _ := WindowPID(target)
	sess.PID = pid
	_ = store.PutSession(sess)

	return sess, nil
}

// Stop gracefully stops a worker session.
func Stop(store *Store, taskID string) error {
	sess, err := store.GetSession(taskID)
	if err != nil {
		return fmt.Errorf("no worker for %s — nothing to stop", taskID)
	}

	target := sess.TmuxSession + ":" + sess.WindowName

	// Send interrupt to stop the agent.
	if HasWindowInSession(sess.TmuxSession, sess.WindowName) {
		_ = SendInterrupt(target)
		// Give the agent a moment to exit gracefully.
		time.Sleep(3 * time.Second)
		if HasWindowInSession(sess.TmuxSession, sess.WindowName) {
			_ = KillWindow(target)
		}
	}

	return store.UpdateStatus(taskID, "stopped")
}

// StopAll stops all active sessions.
func StopAll(store *Store) error {
	sessions, err := store.ListSessions(true, "")
	if err != nil {
		return err
	}
	var lastErr error
	for _, sess := range sessions {
		if err := Stop(store, sess.TaskID); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Refresh syncs session and orchestrator state with tmux.
// Marks sessions as "failed" if their window no longer exists,
// or "done" if the window is gone and the task is closed in gig.
// Marks orchestrators as "stopped" if their tmux session is gone.
func Refresh(store *Store, isTaskClosed func(taskID string) bool) error {
	sessions, err := store.ListSessions(true, "")
	if err != nil {
		return err
	}

	for _, sess := range sessions {
		if HasWindowInSession(sess.TmuxSession, sess.WindowName) {
			_ = store.TouchSession(sess.TaskID)
			continue
		}

		// Window is gone — determine why.
		if isTaskClosed != nil && isTaskClosed(sess.TaskID) {
			_ = store.UpdateStatus(sess.TaskID, "done")
		} else {
			_ = store.UpdateStatus(sess.TaskID, "failed")
		}
	}

	// Sync orchestrator state: mark stopped if tmux session is gone.
	orchs, err := store.ListOrchestrators(true)
	if err != nil {
		return nil // non-fatal: worker refresh already succeeded
	}
	for _, o := range orchs {
		if !HasSession(o.TmuxSession) {
			_ = store.UpdateOrchestratorStatus(o.ID, "stopped")
		}
	}

	return nil
}

// StopOrchestrator gracefully stops an orchestrator and all its workers.
// It first stops all workers belonging to the orchestrator, then kills the
// tmux session, and updates the orchestrator status in the DB.
func StopOrchestrator(store *Store, orchestratorID string) error {
	orch, err := store.GetOrchestrator(orchestratorID)
	if err != nil {
		return fmt.Errorf("get orchestrator: %w", err)
	}

	// Stop all workers in this orchestrator's session first.
	workers, err := store.WorkersForOrchestrator(orchestratorID)
	if err != nil {
		return fmt.Errorf("list workers: %w", err)
	}
	for _, w := range workers {
		if w.Status == "done" || w.Status == "failed" || w.Status == "stopped" {
			continue
		}
		_ = Stop(store, w.TaskID)
	}

	// Kill the orchestrator's tmux session.
	if HasSession(orch.TmuxSession) {
		_ = KillSession(orch.TmuxSession)
	}

	return store.UpdateOrchestratorStatus(orchestratorID, "stopped")
}

// geminiSendDelay is the paste-to-Enter delay for Gemini CLI workers.
// Gemini's Ink/React TUI processes keyboard input asynchronously; 100 ms (the
// default in SendCommand) is not enough for the pasted text to be committed to
// the input buffer before the Enter keystroke arrives, causing the Enter to be
// dropped.  500 ms is reliably sufficient.  See gig-906c.
const geminiSendDelay = 500 * time.Millisecond

// geminiInterruptSettleDelay is the post-C-c settle time for Gemini CLI workers.
// Gemini's Ink/React TUI takes longer than 2 s to fully transition out of the
// interrupted state.  During that window, the input field accepts text but routes
// it to the "Queued" buffer rather than live input — so the divert message never
// starts a fresh turn.  4 s is reliably sufficient for the TUI to reach an idle
// input state before the divert message is pasted.  See gig-c6dd.
const geminiInterruptSettleDelay = 4 * time.Second

// defaultInterruptSettleDelay is the post-C-c settle time for non-Gemini agents.
const defaultInterruptSettleDelay = 2 * time.Second

// interruptSettleDelay returns the post-C-c settle duration for the given agent.
func interruptSettleDelay(agent string) time.Duration {
	if agent == "gemini" {
		return geminiInterruptSettleDelay
	}
	return defaultInterruptSettleDelay
}

// sendCommandForSession sends a command to a tmux target, using an agent-aware
// paste-to-Enter delay derived from the session's Agent field.
func sendCommandForSession(target, command, agent string) error {
	if agent == "gemini" {
		return SendCommandWithDelay(target, command, geminiSendDelay)
	}
	return SendCommand(target, command)
}

// Send delivers a message to a worker based on its type.
func Send(store *Store, taskID string, msgType MessageType, content string) (*Message, error) {
	sess, err := store.GetSession(taskID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	target := sess.TmuxSession + ":" + sess.WindowName

	msg := &Message{
		ID:        generateMsgID(),
		TaskID:    taskID,
		Direction: "to_worker",
		Type:      msgType,
		Content:   content,
		CreatedAt: time.Now().UTC(),
	}

	switch msgType {
	case MsgNudge:
		// Store and send to pane, same as normal message.
		if err := store.SendMessage(msg); err != nil {
			return nil, fmt.Errorf("store nudge: %w", err)
		}
		if err := sendCommandForSession(target, content, sess.Agent); err != nil {
			return nil, fmt.Errorf("send nudge: %w", err)
		}

	case MsgStatus:
		// Send /btw via tmux pane (sidechain, no context pollution).
		if err := store.SendMessage(msg); err != nil {
			return nil, fmt.Errorf("store status msg: %w", err)
		}
		if err := sendCommandForSession(target, "/btw "+content, sess.Agent); err != nil {
			return nil, fmt.Errorf("send /btw: %w", err)
		}

	case MsgDivert:
		// Interrupt, then send as fresh input.
		if err := store.SendMessage(msg); err != nil {
			return nil, fmt.Errorf("store divert msg: %w", err)
		}
		if err := SendInterrupt(target); err != nil {
			return nil, fmt.Errorf("interrupt: %w", err)
		}
		time.Sleep(interruptSettleDelay(sess.Agent))
		if err := sendCommandForSession(target, content, sess.Agent); err != nil {
			return nil, fmt.Errorf("send divert message: %w", err)
		}

	case MsgNormal:
		// Send directly to agent input.
		if err := store.SendMessage(msg); err != nil {
			return nil, fmt.Errorf("store normal msg: %w", err)
		}
		if err := sendCommandForSession(target, content, sess.Agent); err != nil {
			return nil, fmt.Errorf("send message: %w", err)
		}

	default:
		return nil, fmt.Errorf("unknown message type: %s", msgType)
	}

	return msg, nil
}

// SignalOrchestrator sends a formatted message directly to a worker's
// orchestrator tmux pane. Unlike Ask, it does not store a DB message — it's
// a fire-and-forget notification for completion/stall signals that the
// orchestrator receives as user input immediately, even when idle.
func SignalOrchestrator(store *Store, taskID, message string) error {
	sess, err := store.GetSession(taskID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	if sess.OrchestratorID == "" {
		// Not part of a crew — nothing to signal.
		return nil
	}

	orch, err := store.GetOrchestrator(sess.OrchestratorID)
	if err != nil {
		return fmt.Errorf("get orchestrator %s: %w", sess.OrchestratorID, err)
	}

	target := orch.TmuxPane
	if target == "" {
		target = orch.TmuxSession + ":" + orch.TmuxWindow
	}

	formatted := fmt.Sprintf("[Worker %s]: %s", taskID, message)
	if err := SendCommand(target, formatted); err != nil {
		return fmt.Errorf("signal orchestrator: %w", err)
	}

	return nil
}

// CheckStalls iterates running workers and signals their orchestrators
// for any worker whose last_seen exceeds the given threshold.
func CheckStalls(store *Store, threshold time.Duration) (int, error) {
	rows, err := store.db.Query(`
		SELECT task_id, last_seen, orchestrator_id
		FROM sessions
		WHERE status IN ('running', 'starting') AND orchestrator_id != ''`)
	if err != nil {
		return 0, fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	signaled := 0
	now := time.Now().UTC()
	for rows.Next() {
		var taskID, lastSeenStr, orchID string
		if err := rows.Scan(&taskID, &lastSeenStr, &orchID); err != nil {
			continue
		}
		lastSeen, err := time.Parse(timeLayout, lastSeenStr)
		if err != nil {
			continue
		}
		idle := now.Sub(lastSeen)
		if idle < threshold {
			continue
		}

		msg := fmt.Sprintf("stalled — no activity for %d minutes", int(idle.Minutes()))
		if err := SignalOrchestrator(store, taskID, msg); err != nil {
			continue
		}
		signaled++
	}
	return signaled, rows.Err()
}

// Ask sends a to_orchestrator message from a worker. It looks up the worker's
// orchestrator_id, stores the message, and delivers it to the orchestrator's pane.
func Ask(store *Store, taskID, content string) (*Message, error) {
	sess, err := store.GetSession(taskID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	if sess.OrchestratorID == "" {
		return nil, fmt.Errorf("worker %s has no orchestrator", taskID)
	}

	orch, err := store.GetOrchestrator(sess.OrchestratorID)
	if err != nil {
		return nil, fmt.Errorf("get orchestrator %s: %w", sess.OrchestratorID, err)
	}

	msg := &Message{
		ID:        generateMsgID(),
		TaskID:    taskID,
		Direction: "to_orchestrator",
		Type:      MsgNormal,
		Content:   content,
		CreatedAt: time.Now().UTC(),
	}

	if err := store.SendMessage(msg); err != nil {
		return nil, fmt.Errorf("store ask message: %w", err)
	}

	// Deliver to orchestrator pane. Use pane ID if available, else session:window.
	target := orch.TmuxPane
	if target == "" {
		target = orch.TmuxSession + ":" + orch.TmuxWindow
	}

	// Format: "[worker <task-id>]: <content>"
	formatted := fmt.Sprintf("[worker %s]: %s", taskID, content)
	if err := SendCommand(target, formatted); err != nil {
		return nil, fmt.Errorf("deliver to orchestrator: %w", err)
	}

	return msg, nil
}

// exportWorkerEnv sends "export KEY=value" to the worker's shell so that
// subprocesses (e.g. "jeff memory propose") inherit the correct persona and
// task context without requiring explicit flags.
func exportWorkerEnv(target, key, value string) {
	if value == "" {
		return
	}
	safe := "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
	_ = SendCommand(target, "export "+key+"="+safe)
}

func generateMsgID() string {
	raw := fmt.Sprintf("%s-%d", uuid.New().String(), time.Now().UnixNano())
	sum := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(sum[:])[:8]
	return "msg-" + hash
}
