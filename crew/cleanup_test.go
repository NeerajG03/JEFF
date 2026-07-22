package crew

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// withCustomFakeTmux installs a fake tmux that prints specific output for list-sessions/list-windows
func withCustomFakeTmux(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "tmux")
	script := `#!/bin/sh
if [ "$1" = "list-sessions" ]; then
	echo "jeff"
	echo "jeff-dm"
elif [ "$1" = "list-windows" ]; then
	if [ "$3" = "jeff" ]; then
		echo "gig-live"
	elif [ "$3" = "jeff-dm" ]; then
		echo "dashboard"
	fi
fi
`
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatalf("create fake tmux binary: %v", err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

func TestCleanup(t *testing.T) {
	withCustomFakeTmux(t) // Provides a mock environment

	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()

	// Seed sessions
	sLive := &Session{
		TaskID:      "gig-live",
		TmuxSession: "jeff",
		WindowName:  "gig-live",
		TaskDir:     "/tmp",
		Status:      "running",
		StartedAt:   now,
		LastSeen:    now,
	}
	sDead := &Session{
		TaskID:      "gig-dead",
		TmuxSession: "jeff",
		WindowName:  "gig-dead",
		TaskDir:     "/tmp",
		Status:      "running",
		StartedAt:   now,
		LastSeen:    now,
	}
	store.PutSession(sLive)
	store.PutSession(sDead)

	// Seed orchestrator
	orchLive := &Orchestrator{
		ID:          "orch-live",
		TmuxSession: "jeff-dm",
		TmuxWindow:  "dashboard",
		Status:      "running",
		StartedAt:   now,
	}
	orchDead := &Orchestrator{
		ID:          "orch-dead",
		TmuxSession: "jeff-dm-dead",
		TmuxWindow:  "dashboard",
		Status:      "running",
		StartedAt:   now,
	}
	store.PutOrchestrator(orchLive)
	store.PutOrchestrator(orchDead)

	// 1. Dry run
	res, err := Cleanup(store, filepath.Dir(dir), true)
	if err != nil {
		t.Fatalf("Cleanup dry-run: %v", err)
	}
	if len(res.OrphanedWindows) != 0 || len(res.StaleSessions) != 1 || len(res.StaleOrch) != 1 {
		t.Errorf("dry-run res unexpected: %+v", res)
	}

	// Verify DB unchanged
	if got, _ := store.GetSession("gig-dead"); got.Status != "running" {
		t.Errorf("dry-run mutated gig-dead")
	}

	// 2. Real run
	_, err = Cleanup(store, filepath.Dir(dir), false)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	if got, _ := store.GetSession("gig-dead"); got.Status != "failed" {
		t.Errorf("gig-dead status = %s, want failed", got.Status)
	}
	if got, _ := store.GetSession("gig-live"); got.Status != "running" {
		t.Errorf("gig-live status = %s, want running", got.Status)
	}

	if got, _ := store.GetOrchestrator("orch-dead"); got.Status != "stopped" {
		t.Errorf("orch-dead status = %s, want stopped", got.Status)
	}
	if got, _ := store.GetOrchestrator("orch-live"); got.Status != "running" {
		t.Errorf("orch-live status = %s, want running", got.Status)
	}
}
