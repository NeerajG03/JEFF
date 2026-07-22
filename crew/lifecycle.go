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
// or falls back to provider-compatible defaults for older callers.
func buildAgentCmd(launchCmd, agent, model, resumeSessionID string, skipPermissions bool) string {
	if launchCmd != "" {
		return launchCmd
	}
	// Legacy fallback.
	if agent == "" {
		agent = "claude"
	}
	cmd := agent
	if skipPermissions {
		if agent == "opencode" {
			cmd += " --auto"
		} else {
			cmd += " --dangerously-skip-permissions"
		}
	}
	if model != "" {
		cmd += " --model " + model
	}
	if resumeSessionID != "" {
		if agent == "opencode" {
			cmd += " --session " + resumeSessionID
		} else {
			cmd += " --resume " + resumeSessionID
		}
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
	// SkipPermissions is only consulted by the legacy buildAgentCmd fallback
	// (LaunchCmd == ""). It is resolved at launch time by the caller (current
	// config/flag), never persisted — a resumed worker re-resolves fresh.
	SkipPermissions bool
}

// StartOrchestrator creates a new tmux session (jeff-N or jeff-<name>) and launches the
// orchestrator agent in the first window. Records the orchestrator in the DB.
// If name is non-empty, the session is named jeff-<name>; otherwise jeff-N is auto-assigned.
func StartOrchestrator(store *Store, jeffHome string, agent string, name string) (*Orchestrator, error) {
	return StartOrchestratorWithLaunchCmd(store, jeffHome, agent, "", name, "")
}

// StartOrchestratorWithLaunchCmd is the provider-aware orchestrator start path.
// The legacy StartOrchestrator wrapper remains for SDK callers that only know
// an agent command name.
func StartOrchestratorWithLaunchCmd(store *Store, jeffHome string, agent string, model string, name string, launchCmd string) (*Orchestrator, error) {
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

	// Set JEFF_HOME on the tmux session so panes spawned later inherit it.
	_ = tmuxRun("set-environment", "-t", id, "JEFF_HOME", jeffHome)

	// Ensure the self-updating claude install wins over any system install.
	prependLocalBin(target)

	// Bind the orchestrator identity into the pane's live shell BEFORE launching
	// the agent, so the orchestrator's Claude Code process — and every `jeff crew`
	// subprocess it spawns — inherits JEFF_ORCHESTRATOR_SESSION directly.
	//
	// tmux set-environment only affects panes created AFTER the call, so it never
	// reached this already-running shell (gig-be5c RCA: detection had always
	// depended on the fragile session-name regex). Exporting into the shell fixes
	// that; the persisted pane_id → orchestrator_id binding recorded below is the
	// durable fallback for shell/Claude-Code restarts within this same pane.
	exportWorkerEnv(target, "JEFF_ORCHESTRATOR_SESSION", id)

	// Launch agent in the orchestrator window. The legacy fallback (launchCmd
	// == "") defaults to skip=true here since this path has no --safe/config
	// plumbed through it; callers building launchCmd via the provider control
	// SkipPermissions themselves.
	agentCmd := buildAgentCmd(launchCmd, agent, model, "", true)
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
		Agent:       agent,
		Model:       model,
	}

	if err := store.PutOrchestrator(orch); err != nil {
		return nil, fmt.Errorf("record orchestrator: %w", err)
	}

	return orch, nil
}

// StartWorkerForOrchestrator launches a worker agent bound to orchestratorID and
// records the session with a non-empty orchestrator_id. This is the ONLY worker
// start path — the former shared-default crew.Start (which hard-coded an empty
// orchestrator_id) was deleted so no code path can strand a worker without an
// orchestrator (the gig-be5c silent-fallthrough class).
//
// Where the worker's tmux window is hosted depends on whether the orchestrator
// is itself in tmux:
//   - Orchestrator running in its own tmux session (`jeff orchestrator start`, or
//     an adopted session): the worker is a tab in that session, so the
//     orchestrator sees it alongside its other workers and pane notifications
//     route directly.
//   - Durable identity with no live tmux session (Cursor, VS Code, plain
//     terminal, CI — the reason this package exists): the worker is hosted in the
//     shared "jeff" session instead. It still binds to orchestrator_id for
//     scoping and DB-backed signalling; real-time pane notifications simply
//     aren't available and delivery falls back to the events poll.
func StartWorkerForOrchestrator(store *Store, orchestratorID, taskID, taskDir string, opts StartOpts) (*Session, error) {
	orch, err := store.GetOrchestrator(orchestratorID)
	if err != nil {
		return nil, fmt.Errorf("get orchestrator: %w", err)
	}

	if err := EnsureTmux(); err != nil {
		return nil, err
	}

	windowName := SanitizeWindowName(taskID)

	// Pick the tmux session that will host the worker window.
	hostSession := orch.TmuxSession
	var target string
	if hostSession != "" && HasSession(hostSession) {
		target, err = CreateWindowInSession(hostSession, windowName, taskDir)
	} else {
		// No live orchestrator session — host in the shared "jeff" session.
		if err = EnsureSession(); err != nil {
			return nil, fmt.Errorf("ensure tmux session: %w", err)
		}
		hostSession = TmuxSessionName
		target, err = CreateWindow(windowName, taskDir)
	}
	if err != nil {
		return nil, err
	}

	// Export worker-specific env vars into the shell so subprocesses
	// (e.g. "jeff memory propose") route to the correct persona dir.
	exportWorkerEnv(target, "JEFF_PERSONA", opts.Persona)
	exportWorkerEnv(target, "JEFF_TASK_ID", taskID)
	memoryCanAdd := ""
	if opts.Persona == "marlowe" {
		memoryCanAdd = "1"
	}
	exportWorkerEnv(target, "JEFF_MEMORY_CAN_ADD", memoryCanAdd)

	// Record session BEFORE launching agent so the SessionStart hook
	// (which captures session_id) can find the row in the DB.
	now := time.Now().UTC()
	sess := &Session{
		TaskID:         taskID,
		TmuxSession:    hostSession,
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

	// Ensure the self-updating claude install wins over any system install.
	prependLocalBin(target)

	agentCmd := buildAgentCmd(opts.LaunchCmd, opts.Agent, opts.Model, opts.ResumeSessionID, opts.SkipPermissions)
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
		if err := sendCommandForSession(target, prompt, opts.Agent); err != nil {
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

// Stop gracefully stops a worker session.
func Stop(store *Store, taskID string) error {
	sess, err := store.GetSession(taskID)
	if err != nil {
		return fmt.Errorf("no worker for %s — nothing to stop", taskID)
	}

	target := SessionTarget(sess.TmuxSession, sess.WindowName)

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
		// A durable identity registered outside tmux has no session to probe;
		// its liveness is not tied to tmux, so never mark it stopped here.
		if o.TmuxSession == "" {
			continue
		}
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

// sendCommandForSession sends a command to a tmux target, routing through the
// appropriate primitive for the agent. Gemini uses SendCommandViaBuffer (atomic
// bracketed-paste + Enter) so Ink's usePaste hook receives the message as a
// single block. All other agents use the standard SendCommand path unchanged.
func sendCommandForSession(target, command, agent string) error {
	if agent == "gemini" || agent == "claude" {
		return SendCommandViaBuffer(target, command)
	}
	if agent == "opencode" {
		return SendCommand(target, command)
	}
	return SendCommand(target, command)
}

// windowIsLive reports whether a worker's tmux window is currently alive. It is
// a package var so tests can force a deterministic answer without a real tmux.
var windowIsLive = HasWindowInSession

// FrameToWorker renders the attributed line typed into a worker's pane (and
// replayed from the log). The msg-id is carried in the frame so the worker can
// still `jeff crew ack <msg-id>`. This is the single source of the
// `[Orchestrator <msg-id>]: <content>` framing — the inbox replay path
// (`jeff crew inbox --format agent`) produces the identical string.
func FrameToWorker(msgID, content string) string {
	return fmt.Sprintf("[Orchestrator %s]: %s", msgID, content)
}

// Send delivers a message to a worker under the Model B (direct-delivery +
// inbox-as-log) contract: it writes ONE durable log row and types the ATTRIBUTED
// content straight into the worker's pane. The pane keystroke IS the delivery;
// the log row exists only for crash/restart recovery (replayed once at the
// worker's next SessionStart).
//
// The row is acked as soon as the pane delivery succeeds, so a live-delivered
// message is never replayed. If the pane is not live (worker stopped/restarting)
// the type fails and the row is left unacked — the SessionStart replay hook
// surfaces it exactly once on resume. `jeff crew send` therefore succeeds even
// when the worker is down: the message is durably queued.
func Send(store *Store, taskID, content string, interrupt bool) (*Message, error) {
	sess, err := store.GetSession(taskID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	target := SessionTarget(sess.TmuxSession, sess.WindowName)

	msg := &Message{
		ID:        generateMsgID(),
		TaskID:    taskID,
		Direction: "to_worker",
		Type:      "message",
		Content:   content,
		CreatedAt: time.Now().UTC(),
	}

	// Durable log row (unacked). Its role is recovery, not live delivery.
	if err := store.SendMessage(msg); err != nil {
		return nil, fmt.Errorf("store message: %w", err)
	}

	// Only interrupt a live worker; a Ctrl-C to a dead pane is pointless.
	live := windowIsLive(sess.TmuxSession, sess.WindowName)
	if interrupt && live {
		if err := SendInterrupt(target); err != nil {
			return nil, fmt.Errorf("interrupt: %w", err)
		}
		time.Sleep(interruptSettleDelay(sess.Agent))
	}

	// The delivery: type the attributed content into the pane. On success, ack
	// the row so it is not replayed on the next SessionStart. On failure (pane
	// not live), leave it unacked for SessionStart recovery — this is not an
	// error, the message is durably logged.
	framed := FrameToWorker(msg.ID, content)
	if live {
		if err := sendCommandForSession(target, framed, sess.Agent); err == nil {
			_ = store.AckMessage(msg.ID, "")
		}
	}

	return msg, nil
}

// FrameToOrchestrator renders the attributed line typed into an orchestrator's
// pane (and replayed from the log). It mirrors FrameToWorker for the reverse
// direction. `jeff crew orchestrator-inbox --format agent` produces the identical
// string on the SessionStart replay path.
func FrameToOrchestrator(taskID, message string) string {
	return fmt.Sprintf("[Worker %s]: %s", taskID, message)
}

// SignalOrchestrator delivers a worker→orchestrator signal under the same Model B
// contract as Send, in the reverse direction: type the framed content into the
// orchestrator pane AND write a durable to_orchestrator log row. On successful
// live delivery the row is acked; otherwise it is left unacked and replayed once
// on the orchestrator's next SessionStart.
func SignalOrchestrator(store *Store, taskID, message string) error {
	return signalOrchestrator(store, taskID, message, "signal", false)
}

// SignalWorkerStopped records a durable, de-duplicated worker-stopped signal and
// delivers it to the orchestrator pane. De-duplication collapses repeated
// turn-end stop signals (e.g. Claude's per-turn Stop) while one is still unacked,
// so the orchestrator is not spammed with "[Worker stopped]" lines.
func SignalWorkerStopped(store *Store, taskID string) error {
	message := "Agent has stopped working — the tmux window is still active."
	return signalOrchestrator(store, taskID, message, "worker-stop", true)
}

// signalOrchestrator is the shared implementation for durable worker→orchestrator
// signals. msgType tags the stored row; when dedupe is true, no new row is stored
// (and no duplicate pane line typed) if an unacked to_orchestrator row of the
// same type already exists for the task.
func signalOrchestrator(store *Store, taskID, message, msgType string, dedupe bool) error {
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

	// De-dup: while an equivalent signal is still unacked (undelivered/unseen),
	// don't queue another row or re-type the pane line. This is the cross-turn
	// debounce for Claude's per-turn Stop.
	if dedupe {
		if n, err := store.PendingCountByType(taskID, "to_orchestrator", msgType); err == nil && n > 0 {
			return nil
		}
	}

	// Durable log row first (unacked), so the signal survives a dead orchestrator
	// pane and is replayed on the orchestrator's next SessionStart.
	msg := &Message{
		ID:        generateMsgID(),
		TaskID:    taskID,
		Direction: "to_orchestrator",
		Type:      msgType,
		Content:   message,
		CreatedAt: time.Now().UTC(),
	}
	_ = store.SendMessage(msg)

	target := orchestratorSignalTarget(orch)
	if target == "" {
		// Durable identity with no live pane — the stored row is the delivery
		// path; the orchestrator replays it on its next SessionStart.
		return nil
	}

	// Type the framed content into the orchestrator pane. On success, ack the row
	// so it is not replayed; on failure leave it unacked for SessionStart recovery.
	if err := SendCommand(target, FrameToOrchestrator(taskID, message)); err == nil {
		_ = store.AckMessage(msg.ID, "")
	}

	return nil
}

// orchestratorSignalTarget returns the tmux target to push real-time signals to,
// or "" when the orchestrator has no live tmux binding (a non-tmux durable
// identity). Prefers the pane id (survives window renames); falls back to
// session:window only when a session name is recorded.
func orchestratorSignalTarget(orch *Orchestrator) string {
	if orch.TmuxPane != "" {
		return orch.TmuxPane
	}
	if orch.TmuxSession != "" {
		return SessionTarget(orch.TmuxSession, orch.TmuxWindow)
	}
	return ""
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
		Type:      "message",
		Content:   content,
		CreatedAt: time.Now().UTC(),
	}

	if err := store.SendMessage(msg); err != nil {
		return nil, fmt.Errorf("store ask message: %w", err)
	}

	// Direct delivery to the orchestrator pane (Model B). The row above is the
	// durable log; a non-tmux orchestrator (no live pane) recovers the message on
	// its next SessionStart replay, so we don't fail the ask.
	target := orchestratorSignalTarget(orch)
	if target == "" {
		return msg, nil
	}

	// On successful live delivery, ack so the row is not replayed on the
	// orchestrator's next SessionStart; on failure leave it unacked for recovery.
	if err := SendCommand(target, FrameToOrchestrator(taskID, content)); err == nil {
		_ = store.AckMessage(msg.ID, "")
	}

	return msg, nil
}

// exportWorkerEnv sends "export KEY=value" to the worker's shell so that
// subprocesses (e.g. "jeff memory propose") inherit the correct persona and
// task context without requiring explicit flags.
func exportWorkerEnv(target, key, value string) {
	safe := "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
	_ = SendCommand(target, "export "+key+"="+safe)
}

// prependLocalBin ensures $HOME/.local/bin is at the front of PATH in the
// target pane. Self-updating tool installs (e.g. Claude Code) live there and
// must take precedence over system package manager installs (e.g. brew).
// tmux new-session may start a non-login shell or inherit a stale PATH — this
// call makes the correct binary win regardless of session origin.
func prependLocalBin(target string) {
	_ = SendCommand(target, `export PATH="$HOME/.local/bin:$PATH"`)
}

func generateMsgID() string {
	raw := fmt.Sprintf("%s-%d", uuid.New().String(), time.Now().UnixNano())
	sum := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(sum[:])[:8]
	return "msg-" + hash
}
