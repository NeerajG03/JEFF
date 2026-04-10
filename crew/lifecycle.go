package crew

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// initialPrompt is sent to workers after the agent launches so they begin
// working immediately instead of sitting idle.
const initialPrompt = "Read your CLAUDE.md for task context and begin working on the task."

// StartOpts configures a new crew session.
type StartOpts struct {
	Persona string
	Repos   []string
	Resume  bool   // if true, skip pickup (workspace exists)
	Agent   string // agent command (e.g. "claude"), defaults to "claude"
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
	if agent == "" {
		agent = "claude"
	}
	agentCmd := agent + " --dangerously-skip-permissions"
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

	agent := opts.Agent
	if agent == "" {
		agent = "claude"
	}
	agentCmd := agent + " --dangerously-skip-permissions"
	if err := SendCommand(target, agentCmd); err != nil {
		KillWindow(target)
		return nil, fmt.Errorf("launch agent: %w", err)
	}

	// Send initial prompt so the worker starts immediately.
	time.Sleep(3 * time.Second)
	if err := SendCommand(target, initialPrompt); err != nil {
		KillWindow(target)
		return nil, fmt.Errorf("send initial prompt: %w", err)
	}

	pid, _ := WindowPID(target)
	paneID, _ := PaneID(target)

	now := time.Now().UTC()
	sess := &Session{
		TaskID:         taskID,
		TmuxSession:    orch.TmuxSession,
		WindowName:     windowName,
		TmuxPane:       paneID,
		TaskDir:        taskDir,
		Persona:        opts.Persona,
		Repos:          opts.Repos,
		OrchestratorID: orchestratorID,
		PID:            pid,
		Status:         "running",
		StartedAt:      now,
		LastSeen:       now,
	}

	if err := store.PutSession(sess); err != nil {
		return nil, fmt.Errorf("record session: %w", err)
	}

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

	// Launch agent.
	agent := opts.Agent
	if agent == "" {
		agent = "claude"
	}
	agentCmd := agent + " --dangerously-skip-permissions"
	if err := SendCommand(target, agentCmd); err != nil {
		// Clean up the window if agent launch fails.
		KillWindow(target)
		return nil, fmt.Errorf("launch agent: %w", err)
	}

	// Send initial prompt so the worker starts immediately.
	time.Sleep(3 * time.Second)
	if err := SendCommand(target, initialPrompt); err != nil {
		KillWindow(target)
		return nil, fmt.Errorf("send initial prompt: %w", err)
	}

	// Get PID of the agent process.
	pid, _ := WindowPID(target)

	now := time.Now().UTC()
	sess := &Session{
		TaskID:      taskID,
		TmuxSession: TmuxSessionName,
		WindowName:  windowName,
		TaskDir:     taskDir,
		Persona:     opts.Persona,
		Repos:       opts.Repos,
		PID:         pid,
		Status:      "running",
		StartedAt:   now,
		LastSeen:    now,
	}

	if err := store.PutSession(sess); err != nil {
		return nil, fmt.Errorf("record session: %w", err)
	}

	return sess, nil
}

// Stop gracefully stops a worker session.
func Stop(store *Store, taskID string) error {
	sess, err := store.GetSession(taskID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
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
	sessions, err := store.ListSessions(true)
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

// Refresh syncs session state with tmux.
// Marks sessions as "failed" if their window no longer exists,
// or "done" if the window is gone and the task is closed in gig.
func Refresh(store *Store, isTaskClosed func(taskID string) bool) error {
	sessions, err := store.ListSessions(true)
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
	return nil
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
		if err := SendCommand(target, content); err != nil {
			return nil, fmt.Errorf("send nudge: %w", err)
		}

	case MsgStatus:
		// Send /btw via tmux pane (sidechain, no context pollution).
		if err := store.SendMessage(msg); err != nil {
			return nil, fmt.Errorf("store status msg: %w", err)
		}
		if err := SendCommand(target, "/btw "+content); err != nil {
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
		time.Sleep(2 * time.Second)
		if err := SendCommand(target, content); err != nil {
			return nil, fmt.Errorf("send divert message: %w", err)
		}

	case MsgNormal:
		// Send directly to agent input.
		if err := store.SendMessage(msg); err != nil {
			return nil, fmt.Errorf("store normal msg: %w", err)
		}
		if err := SendCommand(target, content); err != nil {
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

func generateMsgID() string {
	raw := fmt.Sprintf("%s-%d", uuid.New().String(), time.Now().UnixNano())
	sum := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(sum[:])[:8]
	return "msg-" + hash
}
