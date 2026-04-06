package crew

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// StartOpts configures a new crew session.
type StartOpts struct {
	Persona string
	Repos   []string
	Resume  bool   // if true, skip pickup (workspace exists)
	Agent   string // agent command (e.g. "claude"), defaults to "claude"
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
	if HasWindow(sess.WindowName) {
		_ = SendInterrupt(target)
		// Give the agent a moment to exit gracefully.
		time.Sleep(3 * time.Second)
		if HasWindow(sess.WindowName) {
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
		if HasWindow(sess.WindowName) {
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
		// Write to DB only; hook picks it up at next tool use.
		if err := store.SendMessage(msg); err != nil {
			return nil, fmt.Errorf("store nudge: %w", err)
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

func generateMsgID() string {
	raw := fmt.Sprintf("%s-%d", uuid.New().String(), time.Now().UnixNano())
	sum := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(sum[:])[:8]
	return "msg-" + hash
}
