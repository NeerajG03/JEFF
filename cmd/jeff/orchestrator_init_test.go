package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/crew"
	"github.com/NeerajG03/JEFF/identity"
)

// initHome sets up a temp JEFF home, points cfg at it, and clears identity env.
func initHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	cfg = &jeff.Config{Home: home}
	// Keep OS home hermetic so --global and the parent-walk boundary never touch
	// the real ~/.jeff.
	t.Setenv("HOME", home)
	t.Setenv(identity.EnvVar, "")
	t.Setenv(identity.EnvVarLegacy, "")
	t.Setenv("TMUX", "") // ensure no tmux pane is recorded
	t.Setenv("TMUX_PANE", "")
	t.Cleanup(func() { cfg = nil })
	return home
}

func runOrchestratorInit(t *testing.T, args ...string) error {
	t.Helper()
	cmd := orchestratorInitCmd()
	cmd.SetArgs(args)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	return cmd.Execute()
}

func TestOrchestratorInit_WritesShapeAndRegistersRow(t *testing.T) {
	home := initHome(t)
	project := t.TempDir()
	defer chdir(t, project)()

	if err := runOrchestratorInit(t); err != nil {
		t.Fatalf("orchestrator init: %v", err)
	}

	// File written with the right shape.
	id, err := identity.Read(identity.ProjectFilePath(project))
	if err != nil {
		t.Fatalf("read identity: %v", err)
	}
	if id.ID == "" {
		t.Error("id is empty")
	}
	if id.Name != filepath.Base(project) {
		t.Errorf("name = %q, want %q (basename)", id.Name, filepath.Base(project))
	}
	if id.CreatedAt == "" {
		t.Error("created_at is empty")
	}
	if id.TmuxPane != "" {
		t.Errorf("tmux_pane = %q, want empty (not in tmux)", id.TmuxPane)
	}

	// Bridge: an orchestrators DB row must exist so worker start resolves.
	cs, err := crew.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	orch, err := cs.GetOrchestrator(id.ID)
	if err != nil {
		t.Fatalf("orchestrator row not registered: %v", err)
	}
	if orch.TmuxSession != "" {
		t.Errorf("durable identity TmuxSession = %q, want empty", orch.TmuxSession)
	}
	if orch.Status != "running" {
		t.Errorf("status = %q, want running", orch.Status)
	}
}

func TestOrchestratorInit_RefusesExistingThenForce(t *testing.T) {
	initHome(t)
	project := t.TempDir()
	defer chdir(t, project)()

	if err := runOrchestratorInit(t); err != nil {
		t.Fatalf("first init: %v", err)
	}
	first, _ := identity.Read(identity.ProjectFilePath(project))

	// Second init without --force must refuse.
	if err := runOrchestratorInit(t); err == nil {
		t.Fatal("second init without --force should refuse")
	}
	// File must be unchanged.
	after, _ := identity.Read(identity.ProjectFilePath(project))
	if after.ID != first.ID {
		t.Error("refused init still mutated the file")
	}

	// With --force it overwrites (new id).
	if err := runOrchestratorInit(t, "--force"); err != nil {
		t.Fatalf("init --force: %v", err)
	}
	forced, _ := identity.Read(identity.ProjectFilePath(project))
	if forced.ID == first.ID {
		t.Error("--force did not overwrite the id")
	}
}

func TestOrchestratorInit_CustomName(t *testing.T) {
	initHome(t)
	project := t.TempDir()
	defer chdir(t, project)()

	if err := runOrchestratorInit(t, "--name", "jeff-DM20"); err != nil {
		t.Fatalf("init --name: %v", err)
	}
	id, _ := identity.Read(identity.ProjectFilePath(project))
	if id.Name != "jeff-DM20" {
		t.Errorf("name = %q, want jeff-DM20", id.Name)
	}
}

func TestOrchestratorInit_Global(t *testing.T) {
	home := initHome(t)
	// cwd irrelevant for --global.
	project := t.TempDir()
	defer chdir(t, project)()

	if err := runOrchestratorInit(t, "--global"); err != nil {
		t.Fatalf("init --global: %v", err)
	}
	// Global file written under home, not the project dir.
	if _, err := identity.Read(identity.GlobalFilePath(home)); err != nil {
		t.Fatalf("global identity not written: %v", err)
	}
	if _, err := os.Stat(identity.ProjectFilePath(project)); !os.IsNotExist(err) {
		t.Error("--global should not write a per-project file")
	}
}

// TestFindAdoptableOrchestrator covers both the pane binding and the
// session-name match used by the adopt flow.
func TestFindAdoptableOrchestrator(t *testing.T) {
	home := initHome(t)
	cs, err := crew.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	now := time.Now().UTC()
	if err := cs.PutOrchestrator(&crew.Orchestrator{
		ID: "jeff-DM20", TmuxSession: "jeff-DM20", TmuxWindow: "orchestrator",
		TmuxPane: "%42", StartedAt: now, Status: "running",
	}); err != nil {
		t.Fatal(err)
	}

	// By pane.
	if o := findAdoptableOrchestrator(cs, "%42", "whatever"); o == nil || o.ID != "jeff-DM20" {
		t.Errorf("by pane: got %+v, want jeff-DM20", o)
	}
	// By session name (no pane match).
	if o := findAdoptableOrchestrator(cs, "%none", "jeff-DM20"); o == nil || o.ID != "jeff-DM20" {
		t.Errorf("by session: got %+v, want jeff-DM20", o)
	}
	// No match.
	if o := findAdoptableOrchestrator(cs, "%none", "unrelated"); o != nil {
		t.Errorf("no match: got %+v, want nil", o)
	}
}

func TestPromptAdopt(t *testing.T) {
	cases := map[string]bool{
		"\n":    true, // default yes
		"y\n":   true,
		"Y\n":   true,
		"yes\n": true,
		"n\n":   false,
		"no\n":  false,
	}
	for in, want := range cases {
		cmd := orchestratorInitCmd()
		cmd.SetIn(bytes.NewBufferString(in))
		cmd.SetOut(&bytes.Buffer{})
		if got := promptAdopt(cmd, "jeff-DM20"); got != want {
			t.Errorf("promptAdopt(%q) = %v, want %v", in, got, want)
		}
	}
}
