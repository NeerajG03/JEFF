package hooks

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryProposeNudgeHookDefinition(t *testing.T) {
	h := memoryProposeNudgeHook()
	if h.Name != "memory-propose-nudge" {
		t.Errorf("name = %q, want 'memory-propose-nudge'", h.Name)
	}
	if h.Source != SourceTask {
		t.Errorf("source = %q, want SourceTask", h.Source)
	}
	if h.Event != "Stop" {
		t.Errorf("event = %q, want 'Stop'", h.Event)
	}
	if h.Scripts["claude"] == nil {
		t.Error("Scripts[\"claude\"] is nil")
	}
	if h.Scripts["gemini"] == nil {
		t.Error("Scripts[\"gemini\"] is nil")
	}
}

func TestMemoryProposeNudgeScriptContent(t *testing.T) {
	ctx := HookContext{TaskID: "gig-test1", Persona: "jenko"}
	h := memoryProposeNudgeHook()
	for _, key := range []string{"claude", "gemini"} {
		script := h.Scripts[key](ctx)
		if !strings.Contains(script, ".nudged") {
			t.Errorf("[%s] script missing sentinel '.nudged'", key)
		}
		if !strings.Contains(script, "decision") {
			t.Errorf("[%s] script missing 'decision' field", key)
		}
		if !strings.Contains(script, "block") {
			t.Errorf("[%s] script missing 'block' value", key)
		}
		if !strings.Contains(script, "jeff memory propose") {
			t.Errorf("[%s] script missing 'jeff memory propose' command", key)
		}
	}
}

// TestMemoryProposeNudgeFirstFire verifies that when no sentinel exists the
// script emits {"decision":"block",...} and creates the .nudged sentinel.
func TestMemoryProposeNudgeFirstFire(t *testing.T) {
	requireBinaries(t, "bash", "jq")

	taskDir := t.TempDir()
	script := memoryProposeNudgeHook().Scripts["claude"](HookContext{})

	scriptFile := filepath.Join(t.TempDir(), "hook.sh")
	if err := os.WriteFile(scriptFile, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	cmd := exec.Command("bash", scriptFile)
	cmd.Dir = taskDir
	cmd.Stdin = strings.NewReader("{}")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("script failed: %v\nstdout: %s", err, out)
	}

	sentinel := filepath.Join(taskDir, ".nudged")
	if _, err := os.Stat(sentinel); err != nil {
		t.Error("sentinel .nudged not created after first fire")
	}

	var resp map[string]any
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("output not valid JSON: %v\noutput: %s", err, out)
	}
	if resp["decision"] != "block" {
		t.Errorf("decision = %v, want 'block'", resp["decision"])
	}
	reason, _ := resp["reason"].(string)
	if reason == "" {
		t.Error("reason is empty")
	}
	if !strings.Contains(reason, "jeff memory propose") {
		t.Errorf("reason does not mention jeff memory propose: %q", reason)
	}
}

// TestMemoryProposeNudgeSubsequentFire verifies that when the sentinel already
// exists the script emits {} (no block) and exits cleanly.
func TestMemoryProposeNudgeSubsequentFire(t *testing.T) {
	requireBinaries(t, "bash")

	taskDir := t.TempDir()
	sentinel := filepath.Join(taskDir, ".nudged")
	if err := os.WriteFile(sentinel, []byte{}, 0o644); err != nil {
		t.Fatalf("create sentinel: %v", err)
	}

	script := memoryProposeNudgeHook().Scripts["claude"](HookContext{})

	scriptFile := filepath.Join(t.TempDir(), "hook.sh")
	if err := os.WriteFile(scriptFile, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	cmd := exec.Command("bash", scriptFile)
	cmd.Dir = taskDir
	cmd.Stdin = strings.NewReader("{}")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("script failed: %v\nstdout: %s", err, out)
	}

	var resp map[string]any
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("output not valid JSON: %v\noutput: %s", err, out)
	}
	if resp["decision"] != nil {
		t.Errorf("expected no 'decision' field when sentinel exists, got: %v", resp["decision"])
	}
}

// requireBinaries skips the test if any of the named binaries are not on PATH.
func requireBinaries(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("%s not available: %v", name, err)
		}
	}
}
