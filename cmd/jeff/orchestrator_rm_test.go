package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
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

// withFakeTmuxPaneDead puts a tmux on PATH that reports every pane as dead.
// crew.PaneIsDead shells out to `tmux display-message -p '#{pane_dead}'`, so this
// simulates a worker whose pane has genuinely exited while its DB row still says
// "running".
func withFakeTmuxPaneDead(t *testing.T, dead string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = display-message ]; then printf '%s\\n' '" + dead + "'; exit 0; fi\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatalf("create fake tmux: %v", err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

// TestOrchestratorRm_StaleRunningRowDoesNotBlockForever is the repro for
// gig-1d9d.20 (found by review of #110).
//
// The live-worker guard trusts sess.Status from the DB. Nothing re-probes tmux
// before it runs, and the reconcilers that would fix a stale row (jeff crew list
// / cleanup / the TUI / jeff done) are all separate, human-triggered actions. So
// a worker whose pane died can sit at "running" indefinitely, and rm refuses
// forever — reporting a "live worker" that has not been alive in hours. That
// partially re-creates #86's own "cannot be removed" complaint.
func TestOrchestratorRm_StaleRunningRowDoesNotBlockForever(t *testing.T) {
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
	// A row that still SAYS running, with a recorded pane that is actually dead.
	if err := cs.PutSession(&crew.Session{
		TaskID: "gig-stale1", OrchestratorID: id.ID, Status: "running",
		TmuxSession: "jeff", WindowName: "gig-stale1", TmuxPane: "%99",
		StartedAt: time.Now().UTC().Add(-3 * time.Hour),
		LastSeen:  time.Now().UTC().Add(-3 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	cs.Close()

	withFakeTmuxPaneDead(t, "1") // every pane reports dead

	if err := runOrchestratorRm(t, id.ID); err != nil {
		t.Fatalf("rm refused because of a STALE running row whose pane is dead: %v\n"+
			"The guard must re-probe rather than trust sess.Status, or rm can never succeed "+
			"until someone happens to run an unrelated reconciling command.", err)
	}
}

// TestOrchestratorRm_GenuinelyLiveWorkerStillRefuses is the other direction: the
// re-probe must not weaken the guard. A row that says running whose pane really
// is alive must still block rm.
func TestOrchestratorRm_GenuinelyLiveWorkerStillRefuses(t *testing.T) {
	home := initHome(t)
	project := t.TempDir()
	defer chdir(t, project)()

	if err := runOrchestratorInit(t); err != nil {
		t.Fatalf("orchestrator init: %v", err)
	}
	id, _ := identity.Read(identity.ProjectFilePath(project))

	cs, err := crew.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.PutSession(&crew.Session{
		TaskID: "gig-live2", OrchestratorID: id.ID, Status: "running",
		TmuxSession: "jeff", WindowName: "gig-live2", TmuxPane: "%42",
		StartedAt: time.Now().UTC(), LastSeen: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	cs.Close()

	withFakeTmuxPaneDead(t, "0") // pane alive

	if err := runOrchestratorRm(t, id.ID); err == nil {
		t.Fatal("rm removed an orchestrator with a GENUINELY live worker — the re-probe must not weaken the guard")
	}
}
