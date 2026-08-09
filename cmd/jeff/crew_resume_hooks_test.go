package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/crew"
	"github.com/NeerajG03/JEFF/hooks"
)

// TestCrewResumeResyncsStaleTaskHooks is the Part E repro (gig-1d9d.16 rule 3):
// `jeff crew resume` must re-sync a task dir's hooks, so a hook removed from
// the registry since the dir was created (e.g. the old inbox-check.sh poll)
// is actually cleaned up rather than staying armed forever. `jeff work` already
// did this on its resume path; `jeff crew resume` did not.
//
// PATH is deliberately stripped of tmux so the run fails at the real
// tmux-touching step (crew.StartWorkerForOrchestrator) rather than risk ever
// invoking a real tmux server — this test only cares about what happens
// BEFORE that point, which is exactly where the re-sync call lives.
func TestCrewResumeResyncsStaleTaskHooks(t *testing.T) {
	home := t.TempDir()
	cfg = &jeff.Config{Home: home, Agent: "claude"}
	t.Cleanup(func() { cfg = nil })

	taskID := "gig-e2e1"
	taskDir := filepath.Join(home, "tasks", taskID+"-resync-test")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Seed a stale hook the registry no longer declares — install it for real
	// so it carries jeff's version marker, matching how an actually-removed
	// hook would be sitting in an existing task dir today.
	legacyReg := hooks.NewRegistry()
	legacyReg.Register(&hooks.Hook{
		Name: "legacy-inbox-check", Source: hooks.SourceTask, Event: "PostToolUse", Matcher: "*",
		Scripts: map[string]func(hooks.HookContext) string{
			"claude": func(ctx hooks.HookContext) string {
				return "#!/bin/bash\nset -euo pipefail\ncat > /dev/null\necho '{}'\n"
			},
		},
	})
	legacyCtx := hooks.HookContext{JeffHome: home, TargetDir: taskDir}
	if err := hooks.NewManager(legacyReg).Install("legacy-inbox-check", taskDir, "claude", legacyCtx); err != nil {
		t.Fatalf("seed legacy hook: %v", err)
	}
	legacyScript := filepath.Join(taskDir, "hooks", "legacy-inbox-check.sh")
	if _, err := os.Stat(legacyScript); err != nil {
		t.Fatalf("legacy hook script not seeded: %v", err)
	}

	cs, err := crew.Open(home)
	if err != nil {
		t.Fatalf("open crew store: %v", err)
	}
	defer cs.Close()
	now := time.Now().UTC()
	if err := cs.PutSession(&crew.Session{
		TaskID:         taskID,
		TmuxSession:    crew.TmuxSessionName,
		WindowName:     taskID,
		TaskDir:        taskDir,
		OrchestratorID: "jeff-test-1",
		Status:         "stopped", // inactive: PreflightResume passes without touching tmux
		StartedAt:      now,
		LastSeen:       now,
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := cs.PutOrchestrator(&crew.Orchestrator{
		ID:          "jeff-test-1",
		TmuxSession: "jeff-test-orch-session",
		TmuxWindow:  "jeff-test-orch-session:0",
		Status:      "running",
		StartedAt:   now,
	}); err != nil {
		t.Fatalf("seed orchestrator: %v", err)
	}

	// Deterministic orchestrator identity, independent of ambient files.
	t.Setenv("JEFF_ORCHESTRATOR_ID", "jeff-test-1")
	// No tmux on PATH: StartWorkerForOrchestrator must fail here, safely, well
	// after the hook re-sync this test is checking for.
	noToolsDir := t.TempDir()
	t.Setenv("PATH", noToolsDir)

	cmd := crewResumeCmd()
	cmd.SetArgs([]string{taskID})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected an error (no tmux on PATH) — got nil, resume should not be able to succeed in this test")
	}
	if !strings.Contains(err.Error(), "tmux") && !strings.Contains(err.Error(), "executable file not found") {
		t.Fatalf("expected a tmux-not-found style error confirming we reached the launch step, got: %v", err)
	}

	if _, err := os.Stat(legacyScript); !os.IsNotExist(err) {
		t.Error("legacy-inbox-check.sh should have been removed by resume's hook re-sync")
	}
	if _, err := os.Stat(filepath.Join(taskDir, "hooks", "worker-heartbeat.sh")); err != nil {
		t.Errorf("worker-heartbeat.sh should have been (re)installed by resume's hook re-sync: %v", err)
	}
}
