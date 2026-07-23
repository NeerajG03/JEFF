package crew

// Tests for gig-9c92 Option D: window_name sanitization on the default worker
// path + the one-time migration for pre-existing dotted rows.

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestStartSanitizesWindowName verifies the worker start path stores a
// window_name with dots replaced by hyphens, so the DB row matches the real tmux
// window and SessionTarget() builds a resolvable "session:window" target.
// Regression for the gig-be5c §5 dot bug. Exercises the non-tmux durable
// identity path (orchestrator with empty TmuxSession → worker hosted in the
// shared "jeff" session).
func TestStartSanitizesWindowName(t *testing.T) {
	withFakeTmux(t)
	store := tempStore(t)

	// Durable identity: registered orchestrator with no live tmux session.
	runningOrchestrator(t, store, "orch-durable", "", "running")

	// LaunchCmd set so start skips the 3s sleep + initial-prompt path.
	sess, err := StartWorkerForOrchestrator(store, "orch-durable", "gig-f4e8.2", "/tmp/task", StartOpts{
		Agent:     "claude",
		LaunchCmd: "claude --dangerously-skip-permissions",
	})
	if err != nil {
		t.Fatalf("StartWorkerForOrchestrator: %v", err)
	}

	if sess.OrchestratorID != "orch-durable" {
		t.Errorf("orchestrator_id = %q, want orch-durable (must never be empty)", sess.OrchestratorID)
	}

	if strings.Contains(sess.WindowName, ".") {
		t.Errorf("window_name %q still contains a dot", sess.WindowName)
	}
	if sess.WindowName != "gig-f4e8-2" {
		t.Errorf("window_name = %q, want %q", sess.WindowName, "gig-f4e8-2")
	}

	// The stored row must round-trip to the same sanitized name.
	got, err := store.GetSession("gig-f4e8.2")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.WindowName != "gig-f4e8-2" {
		t.Errorf("stored window_name = %q, want %q", got.WindowName, "gig-f4e8-2")
	}

	// Target building must produce a tmux-resolvable "session:window".
	target := SessionTarget(got.TmuxSession, got.WindowName)
	if target != "jeff:gig-f4e8-2" {
		t.Errorf("SessionTarget = %q, want %q", target, "jeff:gig-f4e8-2")
	}
}

// TestMigrationSanitizesDottedWindowNames verifies the one-time migration
// rewrites pre-existing dotted window_name rows in place, is idempotent, and is
// safe on rows that were already clean.
func TestMigrationSanitizesDottedWindowNames(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "jeff.db")

	// Pre-create the database with v1 schema and user_version = 0 (or 1)
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer db.Close()

	if err := migrateV1(db); err != nil {
		t.Fatalf("migrate v1: %v", err)
	}
	// explicitly set user_version to 1 so V2 runs
	if _, err := db.Exec("PRAGMA user_version = 1"); err != nil {
		t.Fatalf("set user_version: %v", err)
	}

	now := time.Now().UTC().Format(timeLayout)
	_, err = db.Exec(`INSERT INTO sessions (task_id, tmux_session, window_name, tmux_pane, task_dir, status, started_at, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"gig-e117.2.1", "jeff", "gig-e117.2.1", "", "/tmp", "running", now, now)
	if err != nil {
		t.Fatalf("insert dotted session: %v", err)
	}

	_, err = db.Exec(`INSERT INTO sessions (task_id, tmux_session, window_name, tmux_pane, task_dir, status, started_at, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"gig-abcd", "jeff", "gig-abcd", "", "/tmp", "running", now, now)
	if err != nil {
		t.Fatalf("insert clean session: %v", err)
	}
	db.Close()

	// Reopen twice: the first Open() runs migrateV2 and sanitizes; the second is a no-op.
	for i := 0; i < 2; i++ {
		store, err := Open(dir)
		if err != nil {
			t.Fatalf("reopen %d: %v", i, err)
		}
		got, err := store.GetSession("gig-e117.2.1")
		if err != nil {
			t.Fatalf("get dotted session (pass %d): %v", i, err)
		}
		if got.WindowName != "gig-e117-2-1" {
			t.Errorf("pass %d: window_name = %q, want %q", i, got.WindowName, "gig-e117-2-1")
		}
		gotClean, err := store.GetSession("gig-abcd")
		if err != nil {
			t.Fatalf("get clean session (pass %d): %v", i, err)
		}
		if gotClean.WindowName != "gig-abcd" {
			t.Errorf("pass %d: clean window_name mutated to %q", i, gotClean.WindowName)
		}
		store.Close()
	}
}

// TestMigrationSafeOnEmptyDB confirms Open (which runs the migration) succeeds
// on a fresh, empty database.
func TestMigrationSafeOnEmptyDB(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatalf("open empty db: %v", err)
	}
	defer store.Close()
	sessions, err := store.ListSessions(false, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("fresh db has %d sessions, want 0", len(sessions))
	}
}
