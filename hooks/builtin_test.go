package hooks

import (
	"strings"
	"testing"
)

func TestBuiltinHooksGenerateContent(t *testing.T) {
	ctx := HookContext{
		JeffHome:           "/tmp/test-jeff",
		TargetDir:          "/tmp/test-jeff",
		GigHome:            "/tmp/test-gig",
		TaskID:             "gig-ab12",
		CheckpointPatterns: []string{"git commit", "go test.*PASS"},
	}

	for _, h := range builtinHooks() {
		t.Run(h.Name+"/claude", func(t *testing.T) {
			if h.ClaudeScript == nil {
				t.Fatal("ClaudeScript is nil")
			}
			content := h.ClaudeScript(ctx)
			if content == "" {
				t.Fatal("ClaudeScript returned empty")
			}
			if len(content) < 20 {
				t.Fatalf("ClaudeScript suspiciously short: %q", content)
			}
		})

		t.Run(h.Name+"/opencode", func(t *testing.T) {
			if h.OpenCodeSnippet == nil {
				t.Fatal("OpenCodeSnippet is nil")
			}
			// Some task hooks return empty for OpenCode (e.g. PostToolUse hooks).
			// Just verify no panic.
			h.OpenCodeSnippet(ctx)
		})
	}
}

func TestHomeHooksAreSessionStart(t *testing.T) {
	// Some home hooks are PostToolUse (e.g. orchestrator-inbox).
	postToolUseHome := map[string]bool{
		"orchestrator-inbox": true,
	}
	for _, h := range builtinHooks() {
		if h.Source != SourceHome {
			continue
		}
		if postToolUseHome[h.Name] {
			if h.Event != "PostToolUse" {
				t.Errorf("%s: event = %q, want PostToolUse", h.Name, h.Event)
			}
			continue
		}
		if h.Event != "SessionStart" {
			t.Errorf("%s: event = %q, want SessionStart", h.Name, h.Event)
		}
	}
}

func TestTaskHookSources(t *testing.T) {
	taskHooks := map[string]bool{
		"task-context":    true,
		"task-commands":   true,
		"checkpoint-nudge": true,
	}
	for _, h := range builtinHooks() {
		if taskHooks[h.Name] {
			if h.Source != SourceTask {
				t.Errorf("%s: source = %q, want %q", h.Name, h.Source, SourceTask)
			}
		}
	}
}

func TestTaskContextHookIncludesTaskID(t *testing.T) {
	ctx := HookContext{TaskID: "gig-ab12"}
	h := taskContextHook()
	script := h.ClaudeScript(ctx)
	if !strings.Contains(script, "gig-ab12") {
		t.Error("task-context script does not contain task ID")
	}
}

func TestCheckpointNudgeWithPatterns(t *testing.T) {
	ctx := HookContext{CheckpointPatterns: []string{"git commit", "go test.*PASS"}}
	h := checkpointNudgeHook()
	script := h.ClaudeScript(ctx)
	if !strings.Contains(script, "git commit") {
		t.Error("checkpoint-nudge script missing pattern 'git commit'")
	}
	if !strings.Contains(script, "go test.*PASS") {
		t.Error("checkpoint-nudge script missing pattern 'go test.*PASS'")
	}
	if !strings.Contains(script, "grep -qE") {
		t.Error("checkpoint-nudge script missing grep check")
	}
}

func TestCheckpointNudgeWithoutPatterns(t *testing.T) {
	ctx := HookContext{CheckpointPatterns: nil}
	h := checkpointNudgeHook()
	script := h.ClaudeScript(ctx)
	// Should be a no-op script (no grep, just exits).
	if strings.Contains(script, "grep") {
		t.Error("checkpoint-nudge with no patterns should not grep")
	}
}

func TestTimeoutOrDefault(t *testing.T) {
	h := &Hook{Timeout: 0}
	if h.TimeoutOrDefault() != 10 {
		t.Fatalf("got %d, want 10", h.TimeoutOrDefault())
	}
	h.Timeout = 5
	if h.TimeoutOrDefault() != 5 {
		t.Fatalf("got %d, want 5", h.TimeoutOrDefault())
	}
}
