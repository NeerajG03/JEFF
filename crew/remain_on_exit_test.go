package crew

// Tests for gig-6c1c: a worker's tmux window must NEVER be closed or destroyed
// when the worker stops or the agent process exits.
//
// Two guarantees are covered here:
//
//  1. Every worker window is created with the `remain-on-exit` option set to
//     "on", so tmux keeps the pane (in a dead state, showing final output)
//     when the agent CLI process exits — turn complete, crash, or interrupt.
//
//  2. Stop() interrupts the agent but does NOT issue kill-window, so a stopped
//     worker's window stays open for inspection.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withFakeTmuxListing installs a stub tmux that behaves like withFakeTmux but
// additionally makes `list-windows` report the given window names on stdout, so
// HasWindowInSession returns true for them. This lets Stop tests exercise the
// "window is alive" branch without a live tmux server.
func withFakeTmuxListing(t *testing.T, windows ...string) func() []string {
	t.Helper()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "calls.log")
	bin := filepath.Join(dir, "tmux")
	listing := strings.Join(windows, "\n")
	script := "#!/bin/sh\n" +
		"echo \"$*\" >> " + logFile + "\n" +
		"if [ \"$1\" = list-windows ]; then printf '%s\\n' '" + listing + "'; fi\n"
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
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

func hasCallWithPrefix(calls []string, prefix string) bool {
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

// TestCreateWindowSetsRemainOnExit verifies CreateWindow enables remain-on-exit
// on the new window so the pane survives agent process exit.
func TestCreateWindowSetsRemainOnExit(t *testing.T) {
	calls := withFakeTmux(t)

	if _, err := CreateWindow("gig-roe1", "/tmp"); err != nil {
		t.Fatalf("CreateWindow: %v", err)
	}

	got := calls()
	if !hasCallWithPrefix(got, "set-option -w -t jeff:gig-roe1 remain-on-exit on") {
		t.Errorf("CreateWindow did not set remain-on-exit; calls: %v", got)
	}
}

// TestCreateWindowInSessionSetsRemainOnExit verifies the arbitrary-session
// window creation path also enables remain-on-exit.
func TestCreateWindowInSessionSetsRemainOnExit(t *testing.T) {
	calls := withFakeTmux(t)

	if _, err := CreateWindowInSession("jeff-2", "gig-roe2", "/tmp"); err != nil {
		t.Fatalf("CreateWindowInSession: %v", err)
	}

	got := calls()
	if !hasCallWithPrefix(got, "set-option -w -t jeff-2:gig-roe2 remain-on-exit on") {
		t.Errorf("CreateWindowInSession did not set remain-on-exit; calls: %v", got)
	}
}

// TestStopDoesNotKillWindow is the core regression test for gig-6c1c: when a
// worker with a live window is stopped, Stop must send an interrupt (C-c) but
// must NEVER issue kill-window. The window is left open for inspection.
func TestStopDoesNotKillWindow(t *testing.T) {
	store := tempStore(t)
	runningSession(t, store, "gig-stop1", "jeff")
	// list-windows reports the window → HasWindowInSession = true (alive branch).
	calls := withFakeTmuxListing(t, "gig-stop1")

	if err := Stop(store, "gig-stop1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	got := calls()
	if hasCallWithPrefix(got, "kill-window") {
		t.Errorf("Stop killed the window — worker windows must stay open (gig-6c1c); calls: %v", got)
	}
	if !hasCallWithPrefix(got, "send-keys -t jeff:gig-stop1 C-c") {
		t.Errorf("Stop did not interrupt the agent; calls: %v", got)
	}

	sess, err := store.GetSession("gig-stop1")
	if err != nil {
		t.Fatalf("GetSession after stop: %v", err)
	}
	if sess.Status != "stopped" {
		t.Errorf("status = %q, want stopped", sess.Status)
	}
}
