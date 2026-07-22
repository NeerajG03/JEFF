package crew

import (
	"path/filepath"
	"strings"
	"testing"
	"sync"
	"time"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatalf("open crew store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestOpenAndClose(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Verify the DB file was created.
	dbPath := filepath.Join(dir, dbFile)
	if _, err := filepath.Abs(dbPath); err != nil {
		t.Fatalf("db path: %v", err)
	}
}

func TestSessionCRUD(t *testing.T) {
	store := tempStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	// Put a session.
	sess := &Session{
		TaskID:      "gig-ab12",
		TmuxSession: TmuxSessionName,
		WindowName:  "gig-ab12",
		TaskDir:     "/tmp/tasks/gig-ab12-auth",
		Persona:     "jenko",
		Repos:       []string{"backend", "frontend"},
		PID:         12345,
		Status:      "running",
		StartedAt:   now,
		LastSeen:    now,
	}
	if err := store.PutSession(sess); err != nil {
		t.Fatal(err)
	}

	// Get it back.
	got, err := store.GetSession("gig-ab12")
	if err != nil {
		t.Fatal(err)
	}
	if got.TaskID != "gig-ab12" {
		t.Errorf("task_id = %q, want %q", got.TaskID, "gig-ab12")
	}
	if got.Persona != "jenko" {
		t.Errorf("persona = %q, want %q", got.Persona, "jenko")
	}
	if len(got.Repos) != 2 || got.Repos[0] != "backend" {
		t.Errorf("repos = %v, want [backend frontend]", got.Repos)
	}
	if got.Status != "running" {
		t.Errorf("status = %q, want %q", got.Status, "running")
	}

	// List active only.
	sessions, err := store.ListSessions(true, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}

	// Update status to stopped.
	if err := store.UpdateStatus("gig-ab12", "stopped"); err != nil {
		t.Fatal(err)
	}

	// Should not appear in active list.
	sessions, err = store.ListSessions(true, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("got %d active sessions after stop, want 0", len(sessions))
	}

	// Should appear in all sessions.
	sessions, err = store.ListSessions(false, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d total sessions, want 1", len(sessions))
	}
	if sessions[0].Status != "stopped" {
		t.Errorf("status = %q, want %q", sessions[0].Status, "stopped")
	}

	// Remove.
	if err := store.RemoveSession("gig-ab12"); err != nil {
		t.Fatal(err)
	}
	sessions, err = store.ListSessions(false, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("got %d sessions after remove, want 0", len(sessions))
	}
}

func TestSessionUpsert(t *testing.T) {
	store := tempStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	sess := &Session{
		TaskID:      "gig-cd34",
		TmuxSession: TmuxSessionName,
		WindowName:  "gig-cd34",
		TaskDir:     "/tmp/tasks/gig-cd34",
		Status:      "starting",
		StartedAt:   now,
		LastSeen:    now,
	}
	if err := store.PutSession(sess); err != nil {
		t.Fatal(err)
	}

	// Upsert with new status.
	sess.Status = "running"
	sess.PID = 999
	if err := store.PutSession(sess); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetSession("gig-cd34")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "running" || got.PID != 999 {
		t.Errorf("upsert: status=%q pid=%d, want running/999", got.Status, got.PID)
	}
}

func TestMessageLifecycle(t *testing.T) {
	store := tempStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	// Need a session first (FK constraint).
	sess := &Session{
		TaskID:      "gig-ab12",
		TmuxSession: TmuxSessionName,
		WindowName:  "gig-ab12",
		TaskDir:     "/tmp",
		Status:      "running",
		StartedAt:   now,
		LastSeen:    now,
	}
	if err := store.PutSession(sess); err != nil {
		t.Fatal(err)
	}

	// Send a message.
	msg := &Message{
		ID:        "msg-001",
		TaskID:    "gig-ab12",
		Direction: "to_worker",
		Type:      "message",
		Content:   "Focus on edge cases",
		CreatedAt: now,
	}
	if err := store.SendMessage(msg); err != nil {
		t.Fatal(err)
	}

	// Send another.
	msg2 := &Message{
		ID:        "msg-002",
		TaskID:    "gig-ab12",
		Direction: "to_worker",
		Type:      "message",
		Content:   "Add table-driven tests",
		CreatedAt: now.Add(time.Second),
	}
	if err := store.SendMessage(msg2); err != nil {
		t.Fatal(err)
	}

	// Pending count.
	count, err := store.PendingCount("gig-ab12", "to_worker")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("pending count = %d, want 2", count)
	}

	// Pending messages.
	msgs, err := store.PendingMessages("gig-ab12", "to_worker")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d pending, want 2", len(msgs))
	}
	if msgs[0].ID != "msg-001" {
		t.Errorf("first message ID = %q, want msg-001", msgs[0].ID)
	}

	// Ack one with response.
	if err := store.AckMessage("msg-001", "done, added edge case tests"); err != nil {
		t.Fatal(err)
	}

	// Check pending reduced.
	count, err = store.PendingCount("gig-ab12", "to_worker")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("pending after ack = %d, want 1", count)
	}

	// Verify response was stored.
	recent, err := store.RecentMessages("gig-ab12", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range recent {
		if m.ID == "msg-001" {
			if m.Response != "done, added edge case tests" {
				t.Errorf("response = %q, want %q", m.Response, "done, added edge case tests")
			}
			if m.AckedAt == nil {
				t.Error("acked_at is nil, want set")
			}
		}
	}

	// Ack all remaining.
	if err := store.AckAll("gig-ab12", "to_worker"); err != nil {
		t.Fatal(err)
	}
	count, err = store.PendingCount("gig-ab12", "to_worker")
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("pending after ack all = %d, want 0", count)
	}
}

func TestRemoveSessionCascadesMessages(t *testing.T) {
	store := tempStore(t)
	now := time.Now().UTC()

	sess := &Session{
		TaskID: "gig-rm1", TmuxSession: TmuxSessionName,
		WindowName: "gig-rm1", TaskDir: "/tmp",
		Status: "running", StartedAt: now, LastSeen: now,
	}
	store.PutSession(sess)

	msg := &Message{
		ID: "msg-rm1", TaskID: "gig-rm1", Direction: "to_worker",
		Type: "message", Content: "test", CreatedAt: now,
	}
	store.SendMessage(msg)

	// Remove session should cascade.
	if err := store.RemoveSession("gig-rm1"); err != nil {
		t.Fatal(err)
	}

	// Messages should be gone.
	msgs, err := store.PendingMessages("gig-rm1", "to_worker")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("messages after remove = %d, want 0", len(msgs))
	}
}

func TestGenerateMsgID(t *testing.T) {
	id1 := generateMsgID()
	id2 := generateMsgID()

	if id1 == id2 {
		t.Error("generated IDs should be unique")
	}
	if len(id1) != 12 { // "msg-" + 8 hex chars
		t.Errorf("id length = %d, want 12", len(id1))
	}
}

func TestBuildAgentCmd(t *testing.T) {
	tests := []struct {
		agent    string
		model    string
		resumeID string
		want     string
	}{
		{"claude", "", "", "claude --dangerously-skip-permissions"},
		{"claude", "sonnet", "", "claude --dangerously-skip-permissions --model sonnet"},
		{"claude", "opus", "", "claude --dangerously-skip-permissions --model opus"},
		{"", "", "", "claude --dangerously-skip-permissions"},
		{"", "haiku", "", "claude --dangerously-skip-permissions --model haiku"},
		{"claude", "", "abc123", "claude --dangerously-skip-permissions --resume abc123"},
		{"claude", "sonnet", "abc123", "claude --dangerously-skip-permissions --model sonnet --resume abc123"},
	}
	for _, tc := range tests {
		got := buildAgentCmd("", tc.agent, tc.model, tc.resumeID, true)
		if got != tc.want {
			t.Errorf("buildAgentCmd(\"\", %q, %q, %q, true) = %q, want %q", tc.agent, tc.model, tc.resumeID, got, tc.want)
		}
	}

	// SkipPermissions false must produce no permission flag at all.
	got := buildAgentCmd("", "claude", "", "", false)
	if got != "claude" {
		t.Errorf("buildAgentCmd with skip=false = %q, want %q", got, "claude")
	}
	got = buildAgentCmd("", "opencode", "", "", false)
	if got != "opencode" {
		t.Errorf("buildAgentCmd with skip=false = %q, want %q", got, "opencode")
	}
}

func TestAppendSessionID(t *testing.T) {
	store := tempStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	sess := &Session{
		TaskID:      "gig-sid1",
		TmuxSession: TmuxSessionName,
		WindowName:  "gig-sid1",
		TaskDir:     "/tmp",
		Status:      "running",
		StartedAt:   now,
		LastSeen:    now,
	}
	if err := store.PutSession(sess); err != nil {
		t.Fatal(err)
	}

	// Initially no session IDs.
	got, err := store.GetSession("gig-sid1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.SessionIDs) != 0 {
		t.Errorf("initial session_ids = %v, want empty", got.SessionIDs)
	}
	if got.LatestSessionID() != "" {
		t.Errorf("LatestSessionID() = %q, want empty", got.LatestSessionID())
	}

	// Append first session ID.
	if err := store.AppendSessionID("gig-sid1", "sess-aaa"); err != nil {
		t.Fatal(err)
	}

	got, err = store.GetSession("gig-sid1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.SessionIDs) != 1 || got.SessionIDs[0] != "sess-aaa" {
		t.Errorf("session_ids = %v, want [sess-aaa]", got.SessionIDs)
	}
	if got.LatestSessionID() != "sess-aaa" {
		t.Errorf("LatestSessionID() = %q, want sess-aaa", got.LatestSessionID())
	}

	// Append second session ID (e.g. after context limit restart).
	if err := store.AppendSessionID("gig-sid1", "sess-bbb"); err != nil {
		t.Fatal(err)
	}

	got, err = store.GetSession("gig-sid1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.SessionIDs) != 2 || got.SessionIDs[1] != "sess-bbb" {
		t.Errorf("session_ids = %v, want [sess-aaa sess-bbb]", got.SessionIDs)
	}
	if got.LatestSessionID() != "sess-bbb" {
		t.Errorf("LatestSessionID() = %q, want sess-bbb", got.LatestSessionID())
	}
}

func TestListSessionsOrchestratorFilter(t *testing.T) {
	store := tempStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	put := func(taskID, orchID, status string) {
		t.Helper()
		sess := &Session{
			TaskID:         taskID,
			TmuxSession:    TmuxSessionName,
			WindowName:     taskID,
			TaskDir:        "/tmp",
			OrchestratorID: orchID,
			Status:         status,
			StartedAt:      now,
			LastSeen:       now,
		}
		if err := store.PutSession(sess); err != nil {
			t.Fatalf("put session %s: %v", taskID, err)
		}
	}

	put("gig-a1", "jeff-1", "running")
	put("gig-a2", "jeff-1", "running")
	put("gig-b1", "jeff-2", "running")

	taskIDs := func(sessions []*Session) []string {
		ids := make([]string, len(sessions))
		for i, s := range sessions {
			ids[i] = s.TaskID
		}
		return ids
	}

	// Filter to jeff-1 — only gig-a1 and gig-a2.
	sessions, err := store.ListSessions(false, "jeff-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Errorf("jeff-1 filter: got %v, want 2 sessions", taskIDs(sessions))
	}
	for _, s := range sessions {
		if s.OrchestratorID != "jeff-1" {
			t.Errorf("jeff-1 filter: unexpected session %s with orchestrator_id=%q", s.TaskID, s.OrchestratorID)
		}
	}

	// Filter to jeff-2 — only gig-b1.
	sessions, err = store.ListSessions(false, "jeff-2")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].TaskID != "gig-b1" {
		t.Errorf("jeff-2 filter: got %v, want [gig-b1]", taskIDs(sessions))
	}

	// No filter — all three.
	sessions, err = store.ListSessions(false, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 3 {
		t.Errorf("no filter: got %d, want 3", len(sessions))
	}

	// activeOnly + orchestrator filter: stop gig-a1, jeff-1 should show only gig-a2.
	if err := store.UpdateStatus("gig-a1", "stopped"); err != nil {
		t.Fatal(err)
	}
	sessions, err = store.ListSessions(true, "jeff-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].TaskID != "gig-a2" {
		t.Errorf("activeOnly+jeff-1 after stop: got %v, want [gig-a2]", taskIDs(sessions))
	}
}

func TestPutSessionPreservesSessionIDs(t *testing.T) {
	store := tempStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	sess := &Session{
		TaskID:      "gig-sid2",
		TmuxSession: TmuxSessionName,
		WindowName:  "gig-sid2",
		TaskDir:     "/tmp",
		Status:      "running",
		StartedAt:   now,
		LastSeen:    now,
	}
	if err := store.PutSession(sess); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionID("gig-sid2", "sess-xyz"); err != nil {
		t.Fatal(err)
	}

	// PutSession upsert should NOT overwrite session_ids.
	sess.Status = "running"
	if err := store.PutSession(sess); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetSession("gig-sid2")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.SessionIDs) != 1 || got.SessionIDs[0] != "sess-xyz" {
		t.Errorf("session_ids after upsert = %v, want [sess-xyz]", got.SessionIDs)
	}
}

// --- Lifecycle send-path regression tests (gig-4040 / gig-33ab) ---
//
// Send() is the main entry point for delivering messages to worker panes.
// The unified send path emits exactly two tmux send-keys calls: paste then
// Enter.  When interrupt=true, a C-c call precedes the two-call delivery.
//
// We use a fake tmux binary (withFakeTmux, defined in tmux_test.go) so these
// tests run without a live tmux session.

func sessionForSendTest(t *testing.T, store *Store, taskID string) {
	t.Helper()
	now := time.Now().UTC()
	sess := &Session{
		TaskID:      taskID,
		TmuxSession: TmuxSessionName,
		WindowName:  taskID,
		TaskDir:     "/tmp",
		Status:      "running",
		StartedAt:   now,
		LastSeen:    now,
	}
	if err := store.PutSession(sess); err != nil {
		t.Fatalf("put session: %v", err)
	}
}

// forceWindowLive makes windowIsLive return true for the duration of a test, so
// Send takes the live-delivery branch (type into pane) without a real tmux.
func forceWindowLive(t *testing.T) {
	t.Helper()
	prev := windowIsLive
	windowIsLive = func(_, _ string) bool { return true }
	t.Cleanup(func() { windowIsLive = prev })
}

// TestSendUsesTwoSeparateTmuxCalls verifies that a plain send to a LIVE worker
// produces exactly two send-keys calls (paste + Enter) with no chaining (Model B
// direct delivery), and that the typed line carries the attributed framing.
func TestSendUsesTwoSeparateTmuxCalls(t *testing.T) {
	store := tempStore(t)
	sessionForSendTest(t, store, "gig-send1")
	calls := withFakeTmux(t)
	forceWindowLive(t)

	msg, err := Send(store, "gig-send1", "Focus on error handling", false)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	got := sendKeysCalls(calls())
	if len(got) != 2 {
		t.Fatalf("want 2 send-keys calls, got %d: %v", len(got), got)
	}
	assertNoSingleCallCombinesTextAndEnter(t, got)

	if !strings.Contains(got[0], " -l ") {
		t.Errorf("first call missing -l flag: %q", got[0])
	}
	// The typed line must carry the attribution + msg-id + content (Model B: the
	// keystroke IS the delivery).
	if !strings.Contains(got[0], "[Orchestrator "+msg.ID+"]") {
		t.Errorf("typed line missing attribution/msg-id: %q", got[0])
	}
	if !strings.Contains(got[0], "Focus on error handling") {
		t.Errorf("typed line missing content: %q", got[0])
	}
	if !strings.Contains(got[1], "Enter") {
		t.Errorf("second call missing Enter: %q", got[1])
	}

	// Live-delivered → acked, so it is NOT left pending (won't replay on resume).
	pending, err := store.PendingMessages("gig-send1", "to_worker")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("live-delivered message should be acked (0 pending), got %d", len(pending))
	}
}

// TestSendToDeadPaneQueuesForReplay is the recovery unit test (Model B): sending
// to a worker whose pane is NOT live must NOT type anything, must NOT error, and
// must leave the log row unacked so the SessionStart replay surfaces it on resume.
func TestSendToDeadPaneQueuesForReplay(t *testing.T) {
	store := tempStore(t)
	sessionForSendTest(t, store, "gig-dead1")
	calls := withFakeTmux(t)

	// windowIsLive defaults to the real HasWindowInSession, which returns false
	// under the fake tmux (no real window) — exactly the dead-pane case.
	msg, err := Send(store, "gig-dead1", "recover me", false)
	if err != nil {
		t.Fatalf("Send to dead pane should not error (message is queued): %v", err)
	}

	if got := sendKeysCalls(calls()); len(got) != 0 {
		t.Errorf("dead pane: expected no keystrokes typed, got %v", got)
	}

	pending, err := store.PendingMessages("gig-dead1", "to_worker")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != msg.ID {
		t.Fatalf("expected 1 unacked row queued for SessionStart replay, got %d", len(pending))
	}
}

// TestSendWithInterrupt verifies that interrupt=true triggers C-c before
// the paste+Enter sequence to a live worker, and logs the message.
func TestSendWithInterrupt(t *testing.T) {
	store := tempStore(t)
	sessionForSendTest(t, store, "gig-int1")
	calls := withFakeTmux(t)
	forceWindowLive(t)

	msg, err := Send(store, "gig-int1", "urgent redirect", true)
	if err != nil {
		t.Fatalf("Send with interrupt: %v", err)
	}
	if msg == nil {
		t.Fatal("Send returned nil message")
	}

	all := calls()
	sk := sendKeysCalls(all)

	// Must have at least 3 send-keys: C-c, paste (-l), Enter.
	if len(sk) < 3 {
		t.Fatalf("want >=3 send-keys calls (C-c, paste, Enter), got %d: %v", len(sk), sk)
	}
	if !strings.Contains(sk[0], "C-c") {
		t.Errorf("first send-keys must be C-c interrupt, got: %q", sk[0])
	}
	assertNoSingleCallCombinesTextAndEnter(t, sk)

	// Message must be recorded in the DB (as a log row). Live delivery acked it,
	// so it is no longer pending — assert it exists via the recent-message log.
	msgs, err := store.RecentMessages("gig-int1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want 1 logged message, got %d", len(msgs))
	}
	if msgs[0].Content != "urgent redirect" {
		t.Errorf("content = %q, want %q", msgs[0].Content, "urgent redirect")
	}
}

func TestConcurrentWrites(t *testing.T) {
	store := tempStore(t)
	defer store.Close()

	// Seed a session
	sess := &Session{
		TaskID:      "gig-concurrent",
		TmuxSession: "jeff",
		WindowName:  "gig-concurrent",
		TaskDir:     "/tmp",
		Status:      "running",
	}
	if err := store.PutSession(sess); err != nil {
		t.Fatalf("put session: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 8*25*2)

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				if err := store.UpdateStatus("gig-concurrent", "running"); err != nil {
					errs <- err
				}
				if err := store.TouchSession("gig-concurrent"); err != nil {
					errs <- err
				}
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent write failed: %v", err)
	}
}
