package crew

// Tests for gig-9c92 Option D: window_name sanitization on the default worker
// path + the one-time migration for pre-existing dotted rows.

import (
	"strings"
	"testing"
	"time"
)

// TestStartSanitizesWindowName verifies crew.Start stores a window_name with
// dots replaced by hyphens, so the DB row matches the real tmux window and
// SessionTarget() builds a resolvable "session:window" target. Regression for
// the gig-be5c §5 dot bug on the default (non-orchestrated) path.
func TestStartSanitizesWindowName(t *testing.T) {
	withFakeTmux(t)
	store := tempStore(t)

	// LaunchCmd set so Start skips the 3s sleep + initial-prompt path.
	sess, err := Start(store, "gig-f4e8.2", "/tmp/task", StartOpts{
		Agent:     "claude",
		LaunchCmd: "claude --dangerously-skip-permissions",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
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
	store, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// Insert a legacy dotted row directly (simulating a pre-fix write).
	now := time.Now().UTC()
	dotted := &Session{
		TaskID:      "gig-e117.2.1",
		TmuxSession: "jeff",
		WindowName:  "gig-e117.2.1", // unsanitized, as older code stored it
		TaskDir:     "/tmp",
		Status:      "running",
		StartedAt:   now,
		LastSeen:    now,
	}
	if err := store.PutSession(dotted); err != nil {
		t.Fatalf("put dotted session: %v", err)
	}
	// A clean row must survive untouched.
	clean := &Session{
		TaskID:      "gig-abcd",
		TmuxSession: "jeff",
		WindowName:  "gig-abcd",
		TaskDir:     "/tmp",
		Status:      "running",
		StartedAt:   now,
		LastSeen:    now,
	}
	if err := store.PutSession(clean); err != nil {
		t.Fatalf("put clean session: %v", err)
	}
	store.Close()

	// Reopen twice: the first migrate() should sanitize; the second is a no-op.
	for i := 0; i < 2; i++ {
		store, err = Open(dir)
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
