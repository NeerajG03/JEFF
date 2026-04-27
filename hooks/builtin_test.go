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
			gen := h.Scripts["claude"]
			if gen == nil {
				t.Fatal("Scripts[\"claude\"] is nil")
			}
			content := gen(ctx)
			if content == "" {
				t.Fatal("Scripts[\"claude\"] returned empty")
			}
			if len(content) < 20 {
				t.Fatalf("Scripts[\"claude\"] suspiciously short: %q", content)
			}
		})

		t.Run(h.Name+"/scripts", func(t *testing.T) {
			if len(h.Scripts) == 0 {
				t.Fatal("Scripts map is empty")
			}
			// Verify all script generators don't panic.
			for key, gen := range h.Scripts {
				_ = gen(ctx)
				_ = key
			}
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
	script := h.Scripts["claude"](ctx)
	if !strings.Contains(script, "gig-ab12") {
		t.Error("task-context script does not contain task ID")
	}
}

func TestCheckpointNudgeWithPatterns(t *testing.T) {
	ctx := HookContext{CheckpointPatterns: []string{"git commit", "go test.*PASS"}}
	h := checkpointNudgeHook()
	script := h.Scripts["claude"](ctx)
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
	script := h.Scripts["claude"](ctx)
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

// --- Worker stop hook tmux regression tests (gig-33ab) ---
//
// buildWorkerStopScript generates a shell script that signals the orchestrator
// pane when a worker session ends.  It calls tmux send-keys directly (it
// cannot use Go's SendCommand because it is a shell script).
//
// gig-33ab: the original script used tmux's \; chaining to combine paste and
// Enter in one invocation — the same pattern that caused gig-4040 in Go code.
// Text was delivered to the orchestrator pane but Enter was never sent, so the
// orchestrator never processed the stop signal.
//
// These tests assert the invariant structurally: inspect the generated script
// for the broken pattern and for the correct two-invocation pattern.

// TestWorkerStopScriptUsesTwoSeparateSendKeys is the primary regression test
// for gig-33ab.  The generated script must contain two separate tmux send-keys
// lines: one that pastes text (with -l) and one that sends Enter (without -l).
func TestWorkerStopScriptUsesTwoSeparateSendKeys(t *testing.T) {
	script := buildWorkerStopScript("gig-test1", "jeff-1")

	// Count distinct tmux send-keys lines.
	var pasteLines, enterLines int
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "tmux send-keys") {
			continue
		}
		if strings.Contains(line, " -l ") {
			pasteLines++
		}
		if strings.Contains(line, "Enter") && !strings.Contains(line, " -l ") {
			enterLines++
		}
	}

	if pasteLines != 1 {
		t.Errorf("want exactly 1 send-keys line with -l (paste), got %d\nscript:\n%s", pasteLines, script)
	}
	if enterLines != 1 {
		t.Errorf("want exactly 1 send-keys line with Enter (no -l), got %d\nscript:\n%s", enterLines, script)
	}
}

// TestWorkerStopScriptNoSemicolonChaining is the explicit guard against the
// exact broken pattern from gig-33ab: using tmux's \; command chaining to
// combine paste and Enter in a single invocation.
func TestWorkerStopScriptNoSemicolonChaining(t *testing.T) {
	script := buildWorkerStopScript("gig-test1", "jeff-1")

	// The broken pattern: a single line with both -l and Enter chained via \;
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "tmux") {
			continue
		}
		// Detect the chained form: send-keys -l ... \; send-keys ... Enter
		// (with or without whitespace around \;)
		hasSemichain := strings.Contains(line, `\;`) || strings.Contains(line, ` ; `)
		if hasSemichain && strings.Contains(line, " -l ") && strings.Contains(line, "Enter") {
			t.Errorf(
				"regression (gig-33ab): single tmux line combines -l paste and Enter via chaining:\n  %q\n"+
					"Fix: two separate tmux send-keys invocations — paste first, Enter second.",
				line,
			)
		}
	}
}

// TestWorkerStopScriptEmptyArgsIsNoop confirms that a missing taskID or
// orchestratorID produces a no-op script (no tmux calls at all).
// This prevents partial delivery (text without Enter) on misconfiguration.
func TestWorkerStopScriptEmptyArgsIsNoop(t *testing.T) {
	cases := []struct{ taskID, orchID string }{
		{"", "jeff-1"},
		{"gig-test1", ""},
		{"", ""},
	}
	for _, tc := range cases {
		script := buildWorkerStopScript(tc.taskID, tc.orchID)
		if strings.Contains(script, "tmux send-keys") {
			t.Errorf("buildWorkerStopScript(%q,%q): expected no-op, got tmux calls:\n%s",
				tc.taskID, tc.orchID, script)
		}
	}
}

// TestWorkerStopScriptContainsTaskIDAndOrchestratorTarget verifies the script
// sends to the correct tmux target (orchestratorID:orchestrator) and includes
// the worker task ID in the message — both are required for the orchestrator
// to identify which worker stopped.
func TestWorkerStopScriptContainsTaskIDAndOrchestratorTarget(t *testing.T) {
	script := buildWorkerStopScript("gig-abc1", "jeff-42")

	if !strings.Contains(script, "jeff-42:orchestrator") {
		t.Errorf("script missing orchestrator target %q:\n%s", "jeff-42:orchestrator", script)
	}
	if !strings.Contains(script, "gig-abc1") {
		t.Errorf("script missing task ID %q:\n%s", "gig-abc1", script)
	}
}
