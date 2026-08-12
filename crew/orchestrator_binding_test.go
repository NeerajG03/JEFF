package crew

// Tests for gig-9c92 Option A: durable pane_id → orchestrator_id binding and the
// end-to-end payoff (a bound worker can signal its orchestrator).

import (
	"strings"
	"testing"
	"time"
)

// durableOrchestrator registers a non-tmux durable identity: a running
// orchestrator row with no tmux session, window, or pane.
func durableOrchestrator(t *testing.T, store *Store, id string) {
	t.Helper()
	if err := store.PutOrchestrator(&Orchestrator{
		ID:          id,
		TmuxSession: "",
		TmuxWindow:  "",
		TmuxPane:    "",
		StartedAt:   time.Now().UTC(),
		Status:      "running",
	}); err != nil {
		t.Fatalf("put durable orchestrator %s: %v", id, err)
	}
}

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

// TestSignalOrchestratorNoopForDurableIdentity confirms a worker bound to a
// durable identity that has NO tmux pane/session (the non-tmux case this change
// enables) does not error on signal — there is no live pane, so the real-time
// push is dropped and the orchestrator picks the state up on its next poll.
func TestSignalOrchestratorNoopForDurableIdentity(t *testing.T) {
	calls := withFakeTmux(t)
	store := tempStore(t)

	// Durable identity: registered, but no tmux pane and no tmux session.
	durableOrchestrator(t, store, "orch-durable")
	now := time.Now().UTC()
	if err := store.PutSession(&Session{
		TaskID:         "gig-nd",
		TmuxSession:    "jeff",
		WindowName:     "gig-nd",
		TaskDir:        "/tmp",
		OrchestratorID: "orch-durable",
		Status:         "running",
		StartedAt:      now,
		LastSeen:       now,
	}); err != nil {
		t.Fatalf("put bound session: %v", err)
	}

	if err := SignalOrchestrator(store, "gig-nd", "done"); err != nil {
		t.Fatalf("SignalOrchestrator (durable, no pane) = %v, want nil", err)
	}
	// No signal should have been pushed anywhere (no pane target exists).
	for _, c := range sendKeysCalls(calls()) {
		if strings.Contains(c, "[Worker gig-nd]") {
			t.Errorf("unexpected real-time push for durable identity: %q", c)
		}
	}
}

// TestAskStoresMessageForDurableIdentity confirms a worker under a non-tmux
// durable identity can still `ask` — the message is persisted (for polling)
// even though there is no pane to push it to live.
func TestAskStoresMessageForDurableIdentity(t *testing.T) {
	withFakeTmux(t)
	store := tempStore(t)

	// A truly non-tmux durable identity: no session, no pane, so there is no live
	// target. Under Model B the ask row is left unacked (queued) for the
	// orchestrator's SessionStart replay rather than acked on live delivery.
	durableOrchestrator(t, store, "orch-durable")
	now := time.Now().UTC()
	if err := store.PutSession(&Session{
		TaskID:         "gig-ask",
		TmuxSession:    "jeff",
		WindowName:     "gig-ask",
		TaskDir:        "/tmp",
		OrchestratorID: "orch-durable",
		Status:         "running",
		StartedAt:      now,
		LastSeen:       now,
	}); err != nil {
		t.Fatalf("put bound session: %v", err)
	}

	msg, err := Ask(store, "gig-ask", "need review")
	if err != nil {
		t.Fatalf("Ask (durable) = %v, want nil", err)
	}
	pending, err := store.PendingOrchestratorMessages("orch-durable")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != msg.ID {
		t.Errorf("expected the asked message to be queued for polling; got %d messages", len(pending))
	}
}

// TestSignalWorkerStoppedDurableAndDeduped covers gig-ddd6 item 3 + the
// no-[Worker stopped]-spam bar: to a durable orchestrator with no live pane, the
// stop signal is persisted as an unacked to_orchestrator row (recovered on the
// orchestrator's SessionStart), and repeated stop signals collapse to ONE
// unacked row (the cross-turn debounce for Claude's per-turn Stop).
func TestSignalWorkerStoppedDurableAndDeduped(t *testing.T) {
	withFakeTmux(t)
	store := tempStore(t)

	durableOrchestrator(t, store, "orch-durable")
	now := time.Now().UTC()
	if err := store.PutSession(&Session{
		TaskID:         "gig-ws",
		TmuxSession:    "jeff",
		WindowName:     "gig-ws",
		TaskDir:        "/tmp",
		OrchestratorID: "orch-durable",
		Status:         "running",
		StartedAt:      now,
		LastSeen:       now,
	}); err != nil {
		t.Fatalf("put bound session: %v", err)
	}

	// Simulate Claude firing Stop three times in a row.
	for i := 0; i < 3; i++ {
		if err := SignalWorkerStopped(store, "gig-ws"); err != nil {
			t.Fatalf("SignalWorkerStopped: %v", err)
		}
	}

	pending, err := store.PendingOrchestratorMessages("orch-durable")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("want exactly 1 de-duplicated worker-stop row, got %d", len(pending))
	}
	if pending[0].Type != "worker-stop" {
		t.Errorf("row type = %q, want worker-stop", pending[0].Type)
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

// TestSignalOrchestratorNoopForMissingSession covers a task that was never
// registered as a crew worker at all — `jeff pickup` writes no sessions row, so
// GetSession returns sql.ErrNoRows. That is not a failure: there is simply no
// orchestrator to signal, exactly as for a session with an empty
// OrchestratorID. Before this was handled, Teardown's best-effort signal made
// every solo `jeff done` print
// "Warning: signal orchestrator: get session: sql: no rows in result set".
func TestSignalOrchestratorNoopForMissingSession(t *testing.T) {
	withFakeTmux(t)
	store := tempStore(t) // empty — no session row for this task at all

	if err := SignalOrchestrator(store, "gig-never-registered", "done"); err != nil {
		t.Errorf("SignalOrchestrator (no session row) = %v, want nil (no-op)", err)
	}
}

// TestSignalOrchestratorNoopForMissingOrchestrator covers a task bound to an
// orchestrator ID that does not exist in the orchestrators table (gig-1cf3).
func TestSignalOrchestratorNoopForMissingOrchestrator(t *testing.T) {
	withFakeTmux(t)
	store := tempStore(t)
	now := time.Now().UTC()

	if err := store.PutSession(&Session{
		TaskID:         "gig-stale-orch",
		TmuxSession:    "jeff",
		WindowName:     "gig-stale-orch",
		TaskDir:        "/tmp",
		OrchestratorID: "orch-does-not-exist",
		Status:         "running",
		StartedAt:      now,
		LastSeen:       now,
	}); err != nil {
		t.Fatalf("put session: %v", err)
	}

	if err := SignalOrchestrator(store, "gig-stale-orch", "done"); err != nil {
		t.Errorf("SignalOrchestrator (missing orchestrator row) = %v, want nil (no-op)", err)
	}
}
