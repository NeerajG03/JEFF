package main

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/NeerajG03/JEFF/crew"
	"github.com/NeerajG03/JEFF/identity"
)

func runOrchestratorRm(t *testing.T, args ...string) error {
	t.Helper()
	cmd := orchestratorRmCmd()
	cmd.SetArgs(args)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	return cmd.Execute()
}

// TestRefreshDurableOrchestratorStatuses_NoPaneStaysRegistered is the
// display-time counterpart of the init-time fix: even if a stale "running"
// value is somehow already in the DB (e.g. rows written before this fix),
// list/info must never display it as running when nothing is bound.
func TestRefreshDurableOrchestratorStatuses_NoPaneStaysRegistered(t *testing.T) {
	home := initHome(t)
	cs, err := crew.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	orch := &crew.Orchestrator{ID: "jeff-stale", Status: crew.OrchStatusRunning, StartedAt: time.Now().UTC()}
	if err := cs.PutOrchestrator(orch); err != nil {
		t.Fatal(err)
	}

	orchs := []*crew.Orchestrator{orch}
	refreshDurableOrchestratorStatuses(cs, orchs, func(string) (bool, error) {
		t.Fatal("paneIsDead should not be called when TmuxPane is empty")
		return false, nil
	})
	if orchs[0].Status != crew.OrchStatusRegistered {
		t.Errorf("status = %q, want %q", orchs[0].Status, crew.OrchStatusRegistered)
	}

	// And it's persisted, not just mutated in the local slice.
	reread, err := cs.GetOrchestrator("jeff-stale")
	if err != nil {
		t.Fatal(err)
	}
	if reread.Status != crew.OrchStatusRegistered {
		t.Errorf("persisted status = %q, want %q", reread.Status, crew.OrchStatusRegistered)
	}
}

// TestRefreshDurableOrchestratorStatuses_DeadPanePersistsStopped confirms a
// confirmed-dead pane transitions and PERSISTS to stopped (the third state
// in #86's table), not just displays differently once.
func TestRefreshDurableOrchestratorStatuses_DeadPanePersistsStopped(t *testing.T) {
	home := initHome(t)
	cs, err := crew.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	orch := &crew.Orchestrator{ID: "jeff-dead", TmuxPane: "%9", Status: crew.OrchStatusRunning, StartedAt: time.Now().UTC()}
	if err := cs.PutOrchestrator(orch); err != nil {
		t.Fatal(err)
	}

	orchs := []*crew.Orchestrator{orch}
	refreshDurableOrchestratorStatuses(cs, orchs, func(string) (bool, error) { return true, nil })
	if orchs[0].Status != crew.OrchStatusStopped {
		t.Errorf("status = %q, want %q", orchs[0].Status, crew.OrchStatusStopped)
	}

	reread, err := cs.GetOrchestrator("jeff-dead")
	if err != nil {
		t.Fatal(err)
	}
	if reread.Status != crew.OrchStatusStopped {
		t.Errorf("persisted status = %q, want %q", reread.Status, crew.OrchStatusStopped)
	}
}

// TestRefreshDurableOrchestratorStatuses_ProbeErrorNeverPersists confirms a
// transient probe error does not write a wrong status into the DB — the
// "unknown is not death" rule must hold at the persistence layer too, not
// just in the pure derivation function.
func TestRefreshDurableOrchestratorStatuses_ProbeErrorNeverPersists(t *testing.T) {
	home := initHome(t)
	cs, err := crew.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	orch := &crew.Orchestrator{ID: "jeff-flaky", TmuxPane: "%9", Status: crew.OrchStatusRunning, StartedAt: time.Now().UTC()}
	if err := cs.PutOrchestrator(orch); err != nil {
		t.Fatal(err)
	}

	orchs := []*crew.Orchestrator{orch}
	refreshDurableOrchestratorStatuses(cs, orchs, func(string) (bool, error) { return false, errors.New("tmux busy") })
	if orchs[0].Status != crew.OrchStatusRunning {
		t.Errorf("status = %q, want unchanged %q", orchs[0].Status, crew.OrchStatusRunning)
	}
}

func TestOrchestratorRm_RemovesProjectFileAndRow(t *testing.T) {
	home := initHome(t)
	project := t.TempDir()
	defer chdir(t, project)()

	if err := runOrchestratorInit(t); err != nil {
		t.Fatalf("orchestrator init: %v", err)
	}
	id, err := identity.Read(identity.ProjectFilePath(project))
	if err != nil {
		t.Fatal(err)
	}

	if err := runOrchestratorRm(t, id.ID); err != nil {
		t.Fatalf("orchestrator rm: %v", err)
	}

	if _, err := os.Stat(identity.ProjectFilePath(project)); !os.IsNotExist(err) {
		t.Error("expected identity file to be removed")
	}
	cs, err := crew.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	if _, err := cs.GetOrchestrator(id.ID); err == nil {
		t.Error("expected orchestrator row to be removed")
	}
}

// TestOrchestratorRm_RefusesWithLiveWorkersUnlessForce is the repro for #86
// defect 2's core safety requirement: deregistering an orchestrator that
// workers still point at would orphan them.
func TestOrchestratorRm_RefusesWithLiveWorkersUnlessForce(t *testing.T) {
	home := initHome(t)
	project := t.TempDir()
	defer chdir(t, project)()

	if err := runOrchestratorInit(t); err != nil {
		t.Fatalf("orchestrator init: %v", err)
	}
	id, err := identity.Read(identity.ProjectFilePath(project))
	if err != nil {
		t.Fatal(err)
	}

	cs, err := crew.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	if err := cs.PutSession(&crew.Session{
		TaskID: "gig-live1", OrchestratorID: id.ID, Status: "running",
		TmuxSession: "jeff", WindowName: "gig-live1", StartedAt: time.Now().UTC(), LastSeen: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	if err := runOrchestratorRm(t, id.ID); err == nil {
		t.Fatal("expected rm to refuse with a live worker bound")
	}

	// Still there after the refusal.
	if _, err := os.Stat(identity.ProjectFilePath(project)); err != nil {
		t.Error("identity file should survive a refused rm")
	}
	if _, err := cs.GetOrchestrator(id.ID); err != nil {
		t.Error("orchestrator row should survive a refused rm")
	}

	// --force removes it anyway.
	if err := runOrchestratorRm(t, id.ID, "--force"); err != nil {
		t.Fatalf("orchestrator rm --force: %v", err)
	}
	if _, err := cs.GetOrchestrator(id.ID); err == nil {
		t.Error("expected orchestrator row to be removed after --force")
	}
}

// TestOrchestratorRm_GlobalIdentity_HomeApartFromOSHome exercises rm against
// a --global identity with JEFF_HOME != $HOME (the configuration #96 flagged
// as hiding bugs when the two are equal in tests) — the file lives under the
// resolved JEFF home, not under $HOME, and identityFileForOrchestrator must
// resolve it via cfg.Home, not an ambient $HOME lookup.
func TestOrchestratorRm_GlobalIdentity_HomeApartFromOSHome(t *testing.T) {
	jeffHome, _ := initHomeApartFromOSHome(t)
	project := t.TempDir()
	defer chdir(t, project)()

	if err := runOrchestratorInit(t, "--global"); err != nil {
		t.Fatalf("orchestrator init --global: %v", err)
	}
	id, err := identity.Read(identity.GlobalFilePathIn(jeffHome))
	if err != nil {
		t.Fatal(err)
	}

	if err := runOrchestratorRm(t, id.ID); err != nil {
		t.Fatalf("orchestrator rm: %v", err)
	}
	if _, err := os.Stat(identity.GlobalFilePathIn(jeffHome)); !os.IsNotExist(err) {
		t.Error("expected global identity file to be removed")
	}
}

func TestOrchestratorRm_UnknownID(t *testing.T) {
	initHome(t)
	if err := runOrchestratorRm(t, "jeff-does-not-exist"); err == nil {
		t.Fatal("expected error for unknown orchestrator id")
	}
}
