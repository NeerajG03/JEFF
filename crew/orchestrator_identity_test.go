package crew

import (
	"errors"
	"testing"
	"time"
)

// TestDeriveDurableOrchestratorStatus pins the derivation rule for #86: a
// durable orchestrator identity's status must come from probing pane
// liveness, not from whatever was last written to the DB.
func TestDeriveDurableOrchestratorStatus(t *testing.T) {
	alwaysAlive := func(string) (bool, error) { return false, nil }
	alwaysDead := func(string) (bool, error) { return true, nil }
	probeErr := func(string) (bool, error) { return false, errors.New("tmux busy") }

	tests := []struct {
		name       string
		o          *Orchestrator
		paneIsDead func(string) (bool, error)
		want       string
	}{
		{
			name:       "no pane bound is registered, regardless of stored status",
			o:          &Orchestrator{TmuxPane: "", Status: OrchStatusRunning},
			paneIsDead: alwaysAlive,
			want:       OrchStatusRegistered,
		},
		{
			name:       "live pane is running",
			o:          &Orchestrator{TmuxPane: "%7", Status: OrchStatusRegistered},
			paneIsDead: alwaysAlive,
			want:       OrchStatusRunning,
		},
		{
			name:       "dead pane is stopped",
			o:          &Orchestrator{TmuxPane: "%7", Status: OrchStatusRunning},
			paneIsDead: alwaysDead,
			want:       OrchStatusStopped,
		},
		{
			name:       "probe error keeps last known status -- unknown is not death",
			o:          &Orchestrator{TmuxPane: "%7", Status: OrchStatusRunning},
			paneIsDead: probeErr,
			want:       OrchStatusRunning,
		},
		{
			name:       "probe error on a registered-turned-bound identity keeps registered",
			o:          &Orchestrator{TmuxPane: "%7", Status: OrchStatusRegistered},
			paneIsDead: probeErr,
			want:       OrchStatusRegistered,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeriveDurableOrchestratorStatus(tt.o, tt.paneIsDead); got != tt.want {
				t.Errorf("DeriveDurableOrchestratorStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDeleteOrchestrator confirms the row is fully removed.
func TestDeleteOrchestrator(t *testing.T) {
	store := tempStore(t)
	orch := &Orchestrator{ID: "jeff-x1", Status: OrchStatusRegistered, StartedAt: time.Now().UTC()}
	if err := store.PutOrchestrator(orch); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetOrchestrator("jeff-x1"); err != nil {
		t.Fatalf("precondition: orchestrator should exist: %v", err)
	}

	if err := store.DeleteOrchestrator("jeff-x1"); err != nil {
		t.Fatalf("DeleteOrchestrator: %v", err)
	}
	if _, err := store.GetOrchestrator("jeff-x1"); err == nil {
		t.Error("expected orchestrator to be gone after DeleteOrchestrator")
	}
}
