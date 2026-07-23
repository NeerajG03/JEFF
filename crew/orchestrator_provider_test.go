package crew

import (
	"testing"
	"time"
)

// TestOrchestratorAgentModelRoundTrip verifies that Agent and Model fields
// persist and round-trip correctly through PutOrchestrator, GetOrchestrator,
// and ListOrchestrators.
func TestOrchestratorAgentModelRoundTrip(t *testing.T) {
	store := tempStore(t)

	orch := &Orchestrator{
		ID:          "orch-oc",
		TmuxSession: "jeff-oc",
		TmuxWindow:  "orchestrator",
		Agent:       "opencode",
		Model:       "opencode/deepseek-v4-flash-free",
		StartedAt:   time.Now().UTC(),
		Status:      "running",
	}
	if err := store.PutOrchestrator(orch); err != nil {
		t.Fatalf("PutOrchestrator: %v", err)
	}

	got, err := store.GetOrchestrator("orch-oc")
	if err != nil {
		t.Fatalf("GetOrchestrator: %v", err)
	}
	if got.Agent != "opencode" {
		t.Errorf("GetOrchestrator Agent = %q, want %q", got.Agent, "opencode")
	}
	if got.Model != "opencode/deepseek-v4-flash-free" {
		t.Errorf("GetOrchestrator Model = %q, want %q", got.Model, "opencode/deepseek-v4-flash-free")
	}

	list, err := store.ListOrchestrators(false)
	if err != nil {
		t.Fatalf("ListOrchestrators: %v", err)
	}
	if len(list) < 1 {
		t.Fatal("ListOrchestrators returned empty, want at least 1")
	}
	var found bool
	for _, o := range list {
		if o.ID == "orch-oc" {
			if o.Agent != "opencode" {
				t.Errorf("ListOrchestrators Agent = %q, want %q", o.Agent, "opencode")
			}
			if o.Model != "opencode/deepseek-v4-flash-free" {
				t.Errorf("ListOrchestrators Model = %q, want %q", o.Model, "opencode/deepseek-v4-flash-free")
			}
			found = true
			break
		}
	}
	if !found {
		t.Errorf("orchestrator orch-oc not found in ListOrchestrators")
	}
}

// TestOrchestratorAgentModelMigration verifies that a pre-migration
// orchestrators row (with only the original 6 columns) scans cleanly,
// with Agent and Model defaulting to empty strings via COALESCE.
func TestOrchestratorAgentModelMigration(t *testing.T) {
	store := tempStore(t)

	_, err := store.DB().Exec(`
		INSERT INTO orchestrators (id, tmux_session, tmux_window, tmux_pane, started_at, status)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"orch-legacy", "jeff-legacy", "", "", time.Now().UTC().Format(timeLayout), "stopped",
	)
	if err != nil {
		t.Fatalf("insert pre-migration row: %v", err)
	}

	got, err := store.GetOrchestrator("orch-legacy")
	if err != nil {
		t.Fatalf("GetOrchestrator for pre-migration row: %v", err)
	}
	if got.Agent != "" {
		t.Errorf("Agent for pre-migration row = %q, want empty string", got.Agent)
	}
	if got.Model != "" {
		t.Errorf("Model for pre-migration row = %q, want empty string", got.Model)
	}
}
