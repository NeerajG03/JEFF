package crew

// Tests for DB↔tmux pre-flight checks and Stop behaviour (gig-508a).
//
// Pre-flight functions use an injectable windowExistsFunc so tests do not
// need a live tmux binary.  Stop tests use withFakeTmux (from tmux_test.go)
// so tmuxRun calls succeed without a real tmux session.

import (
	"strings"
	"testing"
	"time"
)

func runningSession(t *testing.T, store *Store, taskID, tmuxSession string) {
	t.Helper()
	now := time.Now().UTC()
	if err := store.PutSession(&Session{
		TaskID:      taskID,
		TmuxSession: tmuxSession,
		WindowName:  taskID,
		TaskDir:     "/tmp",
		Status:      "running",
		StartedAt:   now,
		LastSeen:    now,
	}); err != nil {
		t.Fatalf("put session %s: %v", taskID, err)
	}
}

func stoppedSession(t *testing.T, store *Store, taskID, tmuxSession string) {
	t.Helper()
	now := time.Now().UTC()
	if err := store.PutSession(&Session{
		TaskID:      taskID,
		TmuxSession: tmuxSession,
		WindowName:  taskID,
		TaskDir:     "/tmp",
		Status:      "stopped",
		StartedAt:   now,
		LastSeen:    now,
	}); err != nil {
		t.Fatalf("put session %s: %v", taskID, err)
	}
}

// --- PreflightStart ---

// TestPreflightStartWindowAlive is the double-claim scenario:
// DB says running, tmux window is alive → block with clear error.
func TestPreflightStartWindowAlive(t *testing.T) {
	store := tempStore(t)
	runningSession(t, store, "gig-dc1", "jeff-1")

	err := preflightStartCheck(store, "gig-dc1", func(_, _ string) bool { return true })
	if err == nil {
		t.Fatal("expected error for live window, got nil")
	}
	if !strings.Contains(err.Error(), "worker already running") {
		t.Errorf("error = %q, want 'worker already running'", err)
	}
	if !strings.Contains(err.Error(), "jeff crew send") {
		t.Errorf("error missing send hint: %q", err)
	}
	if !strings.Contains(err.Error(), "tmux attach") {
		t.Errorf("error missing attach hint: %q", err)
	}
}

// TestPreflightStartStaleDBRow is the stale-window-name scenario:
// DB says running, but tmux window is gone → block with cleanup hint.
func TestPreflightStartStaleDBRow(t *testing.T) {
	store := tempStore(t)
	runningSession(t, store, "gig-sw1", "jeff-1")

	err := preflightStartCheck(store, "gig-sw1", func(_, _ string) bool { return false })
	if err == nil {
		t.Fatal("expected error for stale DB row, got nil")
	}
	if !strings.Contains(err.Error(), "stale DB row") {
		t.Errorf("error = %q, want 'stale DB row'", err)
	}
	if !strings.Contains(err.Error(), "jeff crew cleanup") {
		t.Errorf("error missing cleanup hint: %q", err)
	}
}

// TestPreflightStartNoSession: no DB record at all → proceed (fresh start).
func TestPreflightStartNoSession(t *testing.T) {
	store := tempStore(t)

	err := preflightStartCheck(store, "gig-new1", func(_, _ string) bool { return false })
	if err != nil {
		t.Errorf("expected nil for no session, got: %v", err)
	}
}

// TestPreflightStartStoppedStatus: DB says stopped → proceed (restart is fine).
func TestPreflightStartStoppedStatus(t *testing.T) {
	store := tempStore(t)
	stoppedSession(t, store, "gig-st1", "jeff-1")

	err := preflightStartCheck(store, "gig-st1", func(_, _ string) bool { return false })
	if err != nil {
		t.Errorf("expected nil for stopped status, got: %v", err)
	}
}

// TestPreflightStartStoppedStatusWindowAlive: DB says stopped but window somehow
// alive (orphan pane) → proceed; start will claim it normally and the orphan
// window will collide at CreateWindow time (handled by tmux, not pre-flight).
func TestPreflightStartStoppedStatusWindowAlive(t *testing.T) {
	store := tempStore(t)
	stoppedSession(t, store, "gig-st2", "jeff-1")

	// Even if window is alive, stopped status means pre-flight allows start.
	err := preflightStartCheck(store, "gig-st2", func(_, _ string) bool { return true })
	if err != nil {
		t.Errorf("expected nil for stopped status (even with orphan pane), got: %v", err)
	}
}

// --- PreflightResume ---

// TestPreflightResumeNoSession: no DB record → error pointing to crew start.
func TestPreflightResumeNoSession(t *testing.T) {
	store := tempStore(t)

	err := preflightResumeCheck(store, "gig-nr1", func(_, _ string) bool { return false })
	if err == nil {
		t.Fatal("expected error for no session, got nil")
	}
	if !strings.Contains(err.Error(), "jeff crew start") {
		t.Errorf("error missing start hint: %q", err)
	}
}

// TestPreflightResumeWindowAlive: DB says running, window alive → error.
func TestPreflightResumeWindowAlive(t *testing.T) {
	store := tempStore(t)
	runningSession(t, store, "gig-rwa1", "jeff-1")

	err := preflightResumeCheck(store, "gig-rwa1", func(_, _ string) bool { return true })
	if err == nil {
		t.Fatal("expected error for live window, got nil")
	}
	if !strings.Contains(err.Error(), "worker already running") {
		t.Errorf("error = %q, want 'worker already running'", err)
	}
}

// TestPreflightResumeStaleDBRow: DB says running, window dead → stale error.
func TestPreflightResumeStaleDBRow(t *testing.T) {
	store := tempStore(t)
	runningSession(t, store, "gig-rsw1", "jeff-1")

	err := preflightResumeCheck(store, "gig-rsw1", func(_, _ string) bool { return false })
	if err == nil {
		t.Fatal("expected error for stale DB row, got nil")
	}
	if !strings.Contains(err.Error(), "stale DB row") {
		t.Errorf("error = %q, want 'stale DB row'", err)
	}
	if !strings.Contains(err.Error(), "jeff crew cleanup") {
		t.Errorf("error missing cleanup hint: %q", err)
	}
}

// TestPreflightResumeStoppedStatus: DB says stopped → proceed (normal resume).
func TestPreflightResumeStoppedStatus(t *testing.T) {
	store := tempStore(t)
	stoppedSession(t, store, "gig-rst1", "jeff-1")

	err := preflightResumeCheck(store, "gig-rst1", func(_, _ string) bool { return false })
	if err != nil {
		t.Errorf("expected nil for stopped status, got: %v", err)
	}
}

// --- Stop behaviour ---

// TestStopNoSession: no DB record → clear error, no tmux calls.
func TestStopNoSession(t *testing.T) {
	store := tempStore(t)
	calls := withFakeTmux(t)

	err := Stop(store, "gig-ns1")
	if err == nil {
		t.Fatal("expected error for missing session, got nil")
	}
	if !strings.Contains(err.Error(), "no worker") {
		t.Errorf("error = %q, want 'no worker'", err)
	}
	if got := calls(); len(got) != 0 {
		t.Errorf("expected no tmux calls, got: %v", got)
	}
}

// TestStopWindowAlreadyGone: DB record exists but window is dead.
// Stop must update DB to stopped and return nil (idempotent; pane already gone).
func TestStopWindowAlreadyGone(t *testing.T) {
	store := tempStore(t)
	runningSession(t, store, "gig-wag1", "jeff")
	_ = withFakeTmux(t) // list-windows returns nothing → HasWindowInSession = false

	if err := Stop(store, "gig-wag1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	sess, err := store.GetSession("gig-wag1")
	if err != nil {
		t.Fatalf("GetSession after stop: %v", err)
	}
	if sess.Status != "stopped" {
		t.Errorf("status = %q, want stopped", sess.Status)
	}
}

// TestStopOrphanPane: DB already says stopped but window somehow alive.
// Stop must still reconcile DB (idempotent) without error.
// We verify the stop path runs cleanly using the fake tmux.
func TestStopOrphanPane(t *testing.T) {
	store := tempStore(t)
	stoppedSession(t, store, "gig-op1", "jeff")
	_ = withFakeTmux(t)

	// Second stop call must not error even though DB already says stopped.
	if err := Stop(store, "gig-op1"); err != nil {
		t.Fatalf("idempotent Stop: %v", err)
	}

	sess, _ := store.GetSession("gig-op1")
	if sess.Status != "stopped" {
		t.Errorf("status after idempotent stop = %q, want stopped", sess.Status)
	}
}

// TestStopDBReconciledWhenPaneGone: Stop with dead pane updates status
// correctly for a session that was in "starting" state.
func TestStopDBReconciledWhenPaneGone(t *testing.T) {
	store := tempStore(t)
	now := time.Now().UTC()
	store.PutSession(&Session{
		TaskID: "gig-rec1", TmuxSession: "jeff", WindowName: "gig-rec1",
		TaskDir: "/tmp", Status: "starting", StartedAt: now, LastSeen: now,
	})
	_ = withFakeTmux(t)

	if err := Stop(store, "gig-rec1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	sess, _ := store.GetSession("gig-rec1")
	if sess.Status != "stopped" {
		t.Errorf("status = %q, want stopped", sess.Status)
	}
}
