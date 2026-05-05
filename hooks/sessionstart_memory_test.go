package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSessionStart_FullFlow(t *testing.T) {
	jeffHome := t.TempDir()
	taskDir := t.TempDir()

	// Seed a minimal CLAUDE.md for injection.
	os.WriteFile(filepath.Join(taskDir, "CLAUDE.md"), []byte("# Task\n"), 0o644)

	if err := RunSessionStart(jeffHome, taskDir, "jenko", "gig-ss1", []string{"jeff"}, "claude"); err != nil {
		t.Fatalf("RunSessionStart: %v", err)
	}

	// Addendum present in CLAUDE.md.
	data, _ := os.ReadFile(filepath.Join(taskDir, "CLAUDE.md"))
	if !strings.Contains(string(data), "<!-- jeff-memory-addendum -->") {
		t.Error("addendum not injected into CLAUDE.md")
	}
	if !strings.Contains(string(data), "jenko") {
		t.Error("persona not substituted in addendum")
	}

	// .claude/settings.json has disable flag.
	settingsPath := filepath.Join(taskDir, ".claude", "settings.json")
	settingsData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf(".claude/settings.json not written: %v", err)
	}
	var settings map[string]any
	json.Unmarshal(settingsData, &settings)
	env, _ := settings["env"].(map[string]any)
	if env == nil || env["CLAUDE_CODE_DISABLE_AUTO_MEMORY"] != "1" {
		t.Error("CLAUDE_CODE_DISABLE_AUTO_MEMORY not set in .claude/settings.json")
	}

	// Layout was created.
	queueDir := filepath.Join(jeffHome, "queue", "sessions")
	if _, err := os.Stat(queueDir); err != nil {
		t.Errorf("queue/sessions dir not created: %v", err)
	}

	// Start log was written.
	entries, _ := os.ReadDir(queueDir)
	var logFound bool
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "-start.log") {
			logFound = true
		}
	}
	if !logFound {
		t.Error("start log not written to queue/sessions/")
	}
}

func TestRunSessionStart_Idempotent(t *testing.T) {
	jeffHome := t.TempDir()
	taskDir := t.TempDir()
	os.WriteFile(filepath.Join(taskDir, "CLAUDE.md"), []byte("# Task\n"), 0o644)

	for i := 0; i < 3; i++ {
		if err := RunSessionStart(jeffHome, taskDir, "jenko", "gig-ss2", []string{"jeff"}, "claude"); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
	}

	data, _ := os.ReadFile(filepath.Join(taskDir, "CLAUDE.md"))
	n := strings.Count(string(data), "<!-- jeff-memory-addendum -->")
	if n != 1 {
		t.Errorf("expected 1 start sentinel after 3 runs, got %d", n)
	}
}

func TestSessionStartMemoryHookDefinition(t *testing.T) {
	h := sessionStartMemoryHook()
	if h.Name != "memory-session-start" {
		t.Errorf("hook name = %q, want 'memory-session-start'", h.Name)
	}
	if h.Source != SourceTask {
		t.Errorf("source = %q, want SourceTask", h.Source)
	}
	if h.Event != "SessionStart" {
		t.Errorf("event = %q, want 'SessionStart'", h.Event)
	}
}

func TestSessionStartMemoryHookScriptContent(t *testing.T) {
	ctx := HookContext{
		TaskID:  "gig-test1",
		Persona: "jenko",
		Repos:   []string{"jeff"},
	}
	h := sessionStartMemoryHook()

	for _, key := range []string{"claude", "gemini"} {
		gen := h.Scripts[key]
		if gen == nil {
			t.Fatalf("Scripts[%q] is nil", key)
		}
		script := gen(ctx)
		if !strings.Contains(script, "jeff memory session-start") {
			t.Errorf("[%s] script missing 'jeff memory session-start'", key)
		}
		if !strings.Contains(script, "gig-test1") {
			t.Errorf("[%s] script missing task ID", key)
		}
		if !strings.Contains(script, "jenko") {
			t.Errorf("[%s] script missing persona", key)
		}
	}
}

func TestSessionStartMemoryHookRegistered(t *testing.T) {
	reg := DefaultRegistry()
	if reg.Get("memory-session-start") == nil {
		t.Error("memory-session-start not in DefaultRegistry")
	}
}
