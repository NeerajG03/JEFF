package crew

// Tests for gig-9c92 Option A: durable pane_id → orchestrator_id binding and the
// end-to-end payoff (a bound worker can signal its orchestrator).

import (
	"strings"
	"testing"
	"time"
)

func runningOrchestrator(t *testing.T, store *Store, id, pane, status string) {
	t.Helper()
	if err := store.PutOrchestrator(&Orchestrator{
		ID:          id,
		TmuxSession: id,
		TmuxWindow:  "orchestrator",
		TmuxPane:    pane,
		StartedAt:   time.Now().UTC(),
		Status:      status,
	}); err != nil {
		t.Fatalf("put orchestrator %s: %v", id, err)
	}
}

// TestStartOrchestratorExportsIdentityBeforeAgentLaunch locks in the Option A
// env-wiring fix: JEFF_ORCHESTRATOR_SESSION must be exported into the pane's live
// shell (via send-keys) BEFORE the agent command is launched, so the orchestrator's
// own Claude Code inherits it. The old broken attempt used `tmux set-environment`
// after the pane had already spawned — that dead path must be gone.
func TestStartOrchestratorExportsIdentityBeforeAgentLaunch(t *testing.T) {
	calls := withFakeTmux(t)
	store := tempStore(t)

	if _, err := StartOrchestrator(store, "/tmp/jeffhome", "claude", "smoke"); err != nil {
		t.Fatalf("StartOrchestrator: %v", err)
	}

	all := calls()

	// The dead set-environment binding for the identity must not be reintroduced.
	for _, c := range all {
		if strings.HasPrefix(c, "set-environment") && strings.Contains(c, "JEFF_ORCHESTRATOR_SESSION") {
			t.Errorf("set-environment must not be used for JEFF_ORCHESTRATOR_SESSION (it never reached the running pane shell): %q", c)
		}
	}

	// Find the export (into the shell) and the agent launch; export must come first.
	exportIdx, agentIdx := -1, -1
	for i, c := range all {
		if !strings.HasPrefix(c, "send-keys ") {
			continue
		}
		if strings.Contains(c, "export JEFF_ORCHESTRATOR_SESSION=") {
			exportIdx = i
		}
		if strings.Contains(c, "claude") && agentIdx == -1 {
			agentIdx = i
		}
	}
	if exportIdx == -1 {
		t.Fatalf("no send-keys exporting JEFF_ORCHESTRATOR_SESSION into the pane shell; calls: %v", all)
	}
	if agentIdx == -1 {
		t.Fatalf("no send-keys launching the agent; calls: %v", all)
	}
	if exportIdx >= agentIdx {
		t.Errorf("identity export (idx %d) must precede agent launch (idx %d) so the agent inherits it", exportIdx, agentIdx)
	}
}

// TestOrchestratorByPaneReturnsPersistedBinding is the core Option A guarantee:
// once an orchestrator is bound to a tmux pane, a lookup by that pane returns its
// ID — this is what makes identity survive shell / Claude Code restarts in the
// same pane, since $TMUX_PANE is stable while the pane exists.
func TestOrchestratorByPaneReturnsPersistedBinding(t *testing.T) {
	store := tempStore(t)
	runningOrchestrator(t, store, "jeff-DM20", "%42", "running")

	got, err := store.OrchestratorByPane("%42")
	if err != nil {
		t.Fatalf("OrchestratorByPane: %v", err)
	}
	if got != "jeff-DM20" {
		t.Errorf("OrchestratorByPane(%%42) = %q, want %q", got, "jeff-DM20")
	}
}

// TestOrchestratorByPaneUnknownPane confirms a non-matching pane yields "" (not
// an error), so detection cleanly falls through to the session-name regex.
func TestOrchestratorByPaneUnknownPane(t *testing.T) {
	store := tempStore(t)
	runningOrchestrator(t, store, "jeff-DM20", "%42", "running")

	got, err := store.OrchestratorByPane("%99")
	if err != nil {
		t.Fatalf("OrchestratorByPane: %v", err)
	}
	if got != "" {
		t.Errorf("OrchestratorByPane(%%99) = %q, want \"\"", got)
	}
}

// TestOrchestratorByPaneIgnoresStopped ensures a stopped orchestrator's stale
// pane binding is not returned — only running orchestrators own a pane. This
// matters because panes (e.g. %0) get reused across orchestrator lifetimes.
func TestOrchestratorByPaneIgnoresStopped(t *testing.T) {
	store := tempStore(t)
	runningOrchestrator(t, store, "jeff-old", "%7", "stopped")

	got, err := store.OrchestratorByPane("%7")
	if err != nil {
		t.Fatalf("OrchestratorByPane: %v", err)
	}
	if got != "" {
		t.Errorf("OrchestratorByPane(%%7) = %q, want \"\" (stopped orch must not bind)", got)
	}
}

// TestOrchestratorByPaneEmpty confirms an empty pane id is a clean no-match.
func TestOrchestratorByPaneEmpty(t *testing.T) {
	store := tempStore(t)
	got, err := store.OrchestratorByPane("")
	if err != nil || got != "" {
		t.Errorf("OrchestratorByPane(\"\") = (%q, %v), want (\"\", nil)", got, err)
	}
}

// TestSignalOrchestratorDeliversForBoundWorker demonstrates the end-to-end payoff
// of Option A: a worker carrying an orchestrator_id (as inherited from the
// persisted binding) signals its orchestrator's pane. Contrast with the empty
// case, which stays a no-op.
func TestSignalOrchestratorDeliversForBoundWorker(t *testing.T) {
	calls := withFakeTmux(t)
	store := tempStore(t)

	runningOrchestrator(t, store, "jeff-DM20", "%42", "running")
	now := time.Now().UTC()
	if err := store.PutSession(&Session{
		TaskID:         "gig-w1",
		TmuxSession:    "jeff-DM20",
		WindowName:     "gig-w1",
		TaskDir:        "/tmp",
		OrchestratorID: "jeff-DM20",
		Status:         "running",
		StartedAt:      now,
		LastSeen:       now,
	}); err != nil {
		t.Fatalf("put bound session: %v", err)
	}

	if err := SignalOrchestrator(store, "gig-w1", "done"); err != nil {
		t.Fatalf("SignalOrchestrator (bound): %v", err)
	}
	// A signal must have been delivered to the orchestrator's pane (%42).
	delivered := false
	for _, c := range sendKeysCalls(calls()) {
		if strings.Contains(c, "%42") {
			delivered = true
			break
		}
	}
	if !delivered {
		t.Errorf("expected a send-keys to orchestrator pane %%42; calls: %v", calls())
	}
}

// TestSignalOrchestratorNoopForUnboundWorker confirms the existing no-op behavior
// for a worker with empty orchestrator_id is preserved (nothing to signal).
func TestSignalOrchestratorNoopForUnboundWorker(t *testing.T) {
	withFakeTmux(t)
	store := tempStore(t)
	now := time.Now().UTC()
	if err := store.PutSession(&Session{
		TaskID:      "gig-orphan",
		TmuxSession: "jeff",
		WindowName:  "gig-orphan",
		TaskDir:     "/tmp",
		Status:      "running",
		StartedAt:   now,
		LastSeen:    now,
	}); err != nil {
		t.Fatalf("put unbound session: %v", err)
	}
	if err := SignalOrchestrator(store, "gig-orphan", "done"); err != nil {
		t.Errorf("SignalOrchestrator (unbound) = %v, want nil (no-op)", err)
	}
}

// TestAskErrorsForUnboundWorker confirms a worker with no orchestrator_id still
// errors when it tries to message up — the existing guardrail is unchanged.
func TestAskErrorsForUnboundWorker(t *testing.T) {
	withFakeTmux(t)
	store := tempStore(t)
	now := time.Now().UTC()
	if err := store.PutSession(&Session{
		TaskID:      "gig-orphan",
		TmuxSession: "jeff",
		WindowName:  "gig-orphan",
		TaskDir:     "/tmp",
		Status:      "running",
		StartedAt:   now,
		LastSeen:    now,
	}); err != nil {
		t.Fatalf("put unbound session: %v", err)
	}
	_, err := Ask(store, "gig-orphan", "help")
	if err == nil {
		t.Fatal("Ask for unbound worker returned nil, want error")
	}
	if !strings.Contains(err.Error(), "no orchestrator") {
		t.Errorf("Ask error = %q, want 'no orchestrator'", err)
	}
}
