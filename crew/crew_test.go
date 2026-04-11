package crew

import (
	"path/filepath"
	"testing"
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
	sessions, err := store.ListSessions(true)
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
	sessions, err = store.ListSessions(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("got %d active sessions after stop, want 0", len(sessions))
	}

	// Should appear in all sessions.
	sessions, err = store.ListSessions(false)
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
	sessions, err = store.ListSessions(false)
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

	// Send a nudge.
	msg := &Message{
		ID:        "msg-001",
		TaskID:    "gig-ab12",
		Direction: "to_worker",
		Type:      MsgNudge,
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
		Type:      MsgNudge,
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
		Type: MsgNudge, Content: "test", CreatedAt: now,
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
		agent string
		model string
		want  string
	}{
		{"claude", "", "claude --dangerously-skip-permissions"},
		{"claude", "sonnet", "claude --dangerously-skip-permissions --model sonnet"},
		{"claude", "opus", "claude --dangerously-skip-permissions --model opus"},
		{"", "", "claude --dangerously-skip-permissions"},
		{"", "haiku", "claude --dangerously-skip-permissions --model haiku"},
	}
	for _, tc := range tests {
		got := buildAgentCmd(tc.agent, tc.model)
		if got != tc.want {
			t.Errorf("buildAgentCmd(%q, %q) = %q, want %q", tc.agent, tc.model, got, tc.want)
		}
	}
}
