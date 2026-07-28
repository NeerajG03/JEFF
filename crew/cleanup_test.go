package crew

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeWin describes one tmux window served by the cleanup fake tmux.
type fakeWin struct {
	Name     string
	ID       string // window id, e.g. "@3"
	PaneDead bool
}

// withCleanupFakeTmux installs a fake tmux that serves the given
// session→windows layout for list-sessions / list-windows (both the plain
// name format and the id/pane_dead info format) and records every invocation.
// Sessions listed in failSessions exit non-zero on list-windows, simulating a
// transient tmux probe failure. Returns a reader for the recorded calls.
func withCleanupFakeTmux(t *testing.T, layout map[string][]fakeWin, order []string, failSessions ...string) func() []string {
	t.Helper()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "calls.log")

	var sb strings.Builder
	sb.WriteString("#!/bin/sh\n")
	sb.WriteString(`echo "$*" >> ` + logFile + "\n")
	sb.WriteString(`case "$1" in` + "\n")

	// list-sessions
	sb.WriteString("list-sessions)\n")
	for _, sess := range order {
		sb.WriteString(fmt.Sprintf("  echo '%s'\n", sess))
	}
	sb.WriteString("  ;;\n")

	// list-windows -t <sess> -F <fmt>
	sb.WriteString("list-windows)\n")
	sb.WriteString(`  sess="$3"; fmt="$5"` + "\n")
	fail := make(map[string]bool)
	for _, s := range failSessions {
		fail[s] = true
	}
	sb.WriteString(`  case "$sess" in` + "\n")
	for _, sess := range order {
		sb.WriteString(fmt.Sprintf("  '%s')\n", sess))
		if fail[sess] {
			sb.WriteString("    echo 'server busy' >&2; exit 1\n")
		} else {
			sb.WriteString(`    case "$fmt" in` + "\n")
			sb.WriteString("    *window_id*)\n")
			for _, w := range layout[sess] {
				dead := "0"
				if w.PaneDead {
					dead = "1"
				}
				sb.WriteString(fmt.Sprintf("      echo '%s %s %s'\n", w.ID, dead, w.Name))
			}
			sb.WriteString("      ;;\n")
			sb.WriteString("    *)\n")
			for _, w := range layout[sess] {
				sb.WriteString(fmt.Sprintf("      echo '%s'\n", w.Name))
			}
			sb.WriteString("      ;;\n")
			sb.WriteString("    esac\n")
		}
		sb.WriteString("    ;;\n")
	}
	sb.WriteString("  esac\n  ;;\n")
	sb.WriteString("esac\nexit 0\n")

	bin := filepath.Join(dir, "tmux")
	if err := os.WriteFile(bin, []byte(sb.String()), 0755); err != nil {
		t.Fatalf("create fake tmux binary: %v", err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	return func() []string {
		data, _ := os.ReadFile(logFile)
		var lines []string
		for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if l != "" {
				lines = append(lines, l)
			}
		}
		return lines
	}
}

func callsWithPrefix(calls []string, prefix string) []string {
	var out []string
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

func seedSession(t *testing.T, store *Store, taskID, tmuxSession, window, status string) {
	t.Helper()
	now := time.Now().UTC()
	if err := store.PutSession(&Session{
		TaskID:      taskID,
		TmuxSession: tmuxSession,
		WindowName:  window,
		TaskDir:     "/tmp",
		Status:      status,
		StartedAt:   now,
		LastSeen:    now,
	}); err != nil {
		t.Fatalf("PutSession(%s): %v", taskID, err)
	}
}

func TestCleanup(t *testing.T) {
	calls := withCleanupFakeTmux(t, map[string][]fakeWin{
		"jeff":    {{Name: "gig-live", ID: "@1", PaneDead: false}},
		"jeff-dm": {{Name: "dashboard", ID: "@2", PaneDead: false}},
	}, []string{"jeff", "jeff-dm"})

	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	seedSession(t, store, "gig-live", "jeff", "gig-live", "running")
	seedSession(t, store, "gig-dead", "jeff", "gig-dead", "running")

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

	if kills := callsWithPrefix(calls(), "kill-"); len(kills) != 0 {
		t.Errorf("no windows should be killed, got: %v", kills)
	}
}

// TestCleanupNeverKillsLiveOrphanWindows is the core regression test for the
// "opencode agent stops → every worker window closes" incident: windows whose
// pane is ALIVE must never be killed, even when the DB has no row for them
// (renamed window, diverged JEFF_HOME, a start race). They are reported as
// LiveOrphans instead.
func TestCleanupNeverKillsLiveOrphanWindows(t *testing.T) {
	calls := withCleanupFakeTmux(t, map[string][]fakeWin{
		"jeff-1": {
			{Name: "orchestrator", ID: "@1", PaneDead: false},
			{Name: "gig-known", ID: "@2", PaneDead: false},   // has a DB row
			{Name: "gig-unknown", ID: "@3", PaneDead: false}, // live pane, no DB row
			{Name: "opencode", ID: "@4", PaneDead: false},    // renamed worker window, no DB row
		},
	}, []string{"jeff-1"})

	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	seedSession(t, store, "gig-known", "jeff-1", "gig-known", "running")

	res, err := Cleanup(store, t.TempDir(), false)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	if kills := callsWithPrefix(calls(), "kill-"); len(kills) != 0 {
		t.Fatalf("live windows were killed: %v", kills)
	}
	if len(res.OrphanedWindows) != 0 {
		t.Errorf("live windows reported as killed orphans: %+v", res.OrphanedWindows)
	}
	if len(res.LiveOrphans) != 2 {
		t.Errorf("want 2 live orphans reported, got %+v", res.LiveOrphans)
	}
	if res.IsClean() {
		t.Errorf("result with live orphans must not be IsClean")
	}
}

// TestCleanupKillsDeadOrphanWindows: a window with no DB row whose pane has
// exited is a remain-on-exit leftover — cleanup kills it by window ID.
func TestCleanupKillsDeadOrphanWindows(t *testing.T) {
	calls := withCleanupFakeTmux(t, map[string][]fakeWin{
		"jeff-1": {
			{Name: "orchestrator", ID: "@1", PaneDead: false},
			{Name: "gig-leftover", ID: "@7", PaneDead: true}, // dead pane, no DB row
			{Name: "gig-known", ID: "@2", PaneDead: true},    // dead pane BUT has a DB row — Refresh's job, not cleanup's
		},
	}, []string{"jeff-1"})

	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	seedSession(t, store, "gig-known", "jeff-1", "gig-known", "running")

	res, err := Cleanup(store, t.TempDir(), false)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	kills := callsWithPrefix(calls(), "kill-window")
	if len(kills) != 1 {
		t.Fatalf("want exactly 1 kill-window, got: %v", kills)
	}
	// Killed by stable window ID, not by (possibly renamed) name target.
	if !strings.Contains(kills[0], "@7") {
		t.Errorf("kill-window must target the window ID @7, got: %q", kills[0])
	}
	if len(res.OrphanedWindows) != 1 || res.OrphanedWindows[0].Window != "gig-leftover" {
		t.Errorf("OrphanedWindows = %+v, want gig-leftover", res.OrphanedWindows)
	}
}

// TestCleanupDryRunKillsNothing: dry-run reports dead orphans without killing.
func TestCleanupDryRunKillsNothing(t *testing.T) {
	calls := withCleanupFakeTmux(t, map[string][]fakeWin{
		"jeff-1": {
			{Name: "orchestrator", ID: "@1", PaneDead: false},
			{Name: "gig-leftover", ID: "@7", PaneDead: true},
		},
	}, []string{"jeff-1"})

	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	seedSession(t, store, "gig-other", "jeff-1", "gig-other", "done")

	res, err := Cleanup(store, t.TempDir(), true)
	if err != nil {
		t.Fatalf("Cleanup dry-run: %v", err)
	}
	if len(res.OrphanedWindows) != 1 {
		t.Errorf("dry-run should report the dead orphan, got %+v", res.OrphanedWindows)
	}
	if kills := callsWithPrefix(calls(), "kill-"); len(kills) != 0 {
		t.Errorf("dry-run must not kill, got: %v", kills)
	}
}

// TestCleanupSkipsKillsWhenStoreHasNoState: a crew DB with zero sessions AND
// zero orchestrators cannot arbitrate orphans — it is almost certainly the
// wrong jeff.db (JEFF_HOME divergence). Nothing may be killed, even dead panes.
func TestCleanupSkipsKillsWhenStoreHasNoState(t *testing.T) {
	calls := withCleanupFakeTmux(t, map[string][]fakeWin{
		"jeff-1": {
			{Name: "orchestrator", ID: "@1", PaneDead: false},
			{Name: "gig-aaaa", ID: "@2", PaneDead: false},
			{Name: "gig-bbbb", ID: "@3", PaneDead: true},
		},
	}, []string{"jeff-1"})

	store, err := Open(t.TempDir()) // empty store
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	res, err := Cleanup(store, t.TempDir(), false)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if kills := callsWithPrefix(calls(), "kill-"); len(kills) != 0 {
		t.Fatalf("empty-DB cleanup must not kill anything, got: %v", kills)
	}
	if !res.SkippedNoState {
		t.Errorf("want SkippedNoState=true, got %+v", res)
	}
	if len(res.OrphanedWindows) != 0 {
		t.Errorf("no window may be reported as killed: %+v", res.OrphanedWindows)
	}
	if len(res.LiveOrphans) != 2 {
		t.Errorf("both worker windows should be reported (kept), got %+v", res.LiveOrphans)
	}
}

// TestCleanupListErrorDoesNotFailSessionsOrKill: when list-windows errors for
// a session, its workers must be left untouched (no stale-marking, no kills) —
// the old code killed whole sessions on a transient list error.
func TestCleanupListErrorDoesNotFailSessionsOrKill(t *testing.T) {
	calls := withCleanupFakeTmux(t, map[string][]fakeWin{
		"jeff":   {{Name: "dashboard", ID: "@1", PaneDead: false}},
		"jeff-1": {{Name: "orchestrator", ID: "@2", PaneDead: false}, {Name: "gig-w1", ID: "@3", PaneDead: false}},
	}, []string{"jeff", "jeff-1"}, "jeff-1") // jeff-1 listing fails

	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	seedSession(t, store, "gig-w1", "jeff-1", "gig-w1", "running")

	res, err := Cleanup(store, t.TempDir(), false)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	if kills := callsWithPrefix(calls(), "kill-"); len(kills) != 0 {
		t.Fatalf("list error must not lead to kills, got: %v", kills)
	}
	if len(res.StaleSessions) != 0 {
		t.Errorf("workers in an unlistable session must not be marked stale: %+v", res.StaleSessions)
	}
	if got, _ := store.GetSession("gig-w1"); got.Status != "running" {
		t.Errorf("gig-w1 status = %s, want running", got.Status)
	}
}
