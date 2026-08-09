package hooks

import (
	"os"
	"path/filepath"
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
	// Under Model B every home hook is SessionStart — orchestrator-inbox moved
	// from PostToolUse polling to a SessionStart replay (gig-ddd6).
	for _, h := range builtinHooks() {
		if h.Source != SourceHome {
			continue
		}
		if h.Event != "SessionStart" {
			t.Errorf("%s: event = %q, want SessionStart", h.Name, h.Event)
		}
	}
}

func TestTaskHookSources(t *testing.T) {
	taskHooks := map[string]bool{
		"task-context":     true,
		"task-commands":    true,
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

// TestWorkerHeartbeatMatcherStaysWildcard locks in a PR #106 review reversal:
// gig-1d9d.16.2 originally narrowed the matcher to "Bash|Edit|Write" to cut
// the ~1000x oversampling of a subprocess exec on every tool call. hardy
// traced that crew.Refresh() already independently touches last_seen on every
// run for any session whose pane is alive — hasFreshHeartbeat only matters
// once the pane is confirmed dead or gone — so narrowing the matcher bought
// nothing against that path, while removing the only signal keeping
// last_seen fresh during a long Read/Grep-only stretch with no Refresh call
// in between. That is a narrower reopening of the exact false-`failed` class
// W1/#104 fixed. The debounce alone (buildWorkerHeartbeatScript) already caps
// the exec rate regardless of which tools trigger it, so the matcher must
// stay "*" — do not re-narrow it without re-litigating this tradeoff.
func TestWorkerHeartbeatMatcherStaysWildcard(t *testing.T) {
	h := workerHeartbeatHook()
	if h.Matcher != "*" {
		t.Errorf("worker-heartbeat matcher = %q, want %q (must fire on read-only tools too)", h.Matcher, "*")
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

// --- Worker stop hook durability tests (gig-ddd6) ---
//
// buildWorkerStopScript now routes through `jeff crew worker-stopped <taskID>`,
// which persists a de-duplicated to_orchestrator row AND wakes the orchestrator
// pane in one call. This makes the stop signal durable (recovered by the
// orchestrator-inbox poll even if the pane is dead) and moves the tmux-paste
// mechanics into Go's SendCommand, where the gig-4040/gig-33ab two-invocation
// invariant is enforced by crew/tmux_test.go.

// TestWorkerStopScriptUsesDurableCommand asserts the generated script signals
// the orchestrator via the durable `jeff crew worker-stopped` command rather
// than typing raw tmux keystrokes.
func TestWorkerStopScriptUsesDurableCommand(t *testing.T) {
	script := buildWorkerStopScript("gig-test1", "jeff-1")

	if !strings.Contains(script, "jeff crew worker-stopped 'gig-test1'") {
		t.Errorf("script missing durable `jeff crew worker-stopped` call:\n%s", script)
	}
	// It must NOT type the message directly into a tmux pane anymore — that path
	// was fire-and-forget (no DB row) and lost on a dead orchestrator pane.
	if strings.Contains(script, "tmux send-keys") {
		t.Errorf("worker-stop script should no longer type raw tmux keys:\n%s", script)
	}
}

// TestWorkerStopScriptEmptyArgsIsNoop confirms that a missing taskID or
// orchestratorID produces a no-op script (no signalling at all).
func TestWorkerStopScriptEmptyArgsIsNoop(t *testing.T) {
	cases := []struct{ taskID, orchID string }{
		{"", "jeff-1"},
		{"gig-test1", ""},
		{"", ""},
	}
	for _, tc := range cases {
		script := buildWorkerStopScript(tc.taskID, tc.orchID)
		if strings.Contains(script, "worker-stopped") {
			t.Errorf("buildWorkerStopScript(%q,%q): expected no-op, got signal:\n%s",
				tc.taskID, tc.orchID, script)
		}
	}
}

// TestWorkerStopScriptContainsTaskID verifies the durable command targets the
// correct worker task.
func TestWorkerStopScriptContainsTaskID(t *testing.T) {
	script := buildWorkerStopScript("gig-abc1", "jeff-42")

	if !strings.Contains(script, "gig-abc1") {
		t.Errorf("script missing task ID %q:\n%s", "gig-abc1", script)
	}
}

// --- Model B delivery/replay generation tests (gig-ddd6, Section A) ---

// TestInboxReplayIsSessionStartOnly asserts the inbox surfacing hook fires only
// at SessionStart. Under Model B the pane keystroke is the delivery, so there
// must be no PostToolUse/turn-end inbox hook at all.
func TestInboxReplayIsSessionStartOnly(t *testing.T) {
	h := inboxReplayHook()
	if h.Event != "SessionStart" {
		t.Errorf("inbox-replay event = %q, want SessionStart", h.Event)
	}
	if h.Source != SourceTask {
		t.Errorf("inbox-replay source = %q, want task", h.Source)
	}
}

// TestNoPostToolUseResurfacesInboxContent is the anti-double-delivery guard: no
// builtin hook may re-surface message content mid-session (i.e. run
// `jeff crew inbox ... --format agent`) on PostToolUse or Stop. That behavior was
// the [Orchestrator msg-x] double-delivery; Model B replays only at SessionStart.
func TestNoPostToolUseResurfacesInboxContent(t *testing.T) {
	ctx := HookContext{TaskID: "gig-ab12", OrchestratorID: "jeff-1"}
	for _, h := range builtinHooks() {
		if h.Event == "SessionStart" {
			continue // SessionStart replay is the sanctioned surfacing.
		}
		// memory-propose-nudge intentionally blocks on Stop to ask about memory
		// proposals — known exception to the no-block rule (not inbox-related).
		if h.Name == "memory-propose-nudge" {
			continue
		}
		for key, gen := range h.Scripts {
			script := gen(ctx)
			if strings.Contains(script, "crew inbox") && strings.Contains(script, "--format agent") {
				t.Errorf("hook %q (%s, event=%s) re-surfaces inbox content mid-session — double-delivery risk:\n%s",
					h.Name, key, h.Event, script)
			}
			// The rejected Model-A Stop-drain used a block/continue decision.
			if h.Event == "Stop" && strings.Contains(script, `"block"`) {
				t.Errorf("hook %q emits a Stop block/continue decision — Model A was rejected", h.Name)
			}
		}
	}
}

// TestInboxReplayScriptRepliesAndAcks asserts the SessionStart replay script
// drains via `--format agent` (which frames + acks, so each row replays once).
func TestInboxReplayScriptRepliesAndAcks(t *testing.T) {
	script := buildInboxReplayScript("gig-ab12", "SessionStart")
	if !strings.Contains(script, "jeff crew inbox 'gig-ab12' --format agent") {
		t.Errorf("replay script missing framed+acking drain:\n%s", script)
	}
	if !strings.Contains(script, `hookEventName: $ev`) || !strings.Contains(script, `"SessionStart"`) {
		t.Errorf("replay script must emit SessionStart hookEventName:\n%s", script)
	}
}

// TestGeminiInboxReplayEmitsSessionStart / TestGeminiCheckpointNudgeEmitsAfterTool
// lock in the gig-ddd6 hookEventName fix: the emitted event name must match the
// Gemini settings key (SessionStart is shared; PostToolUse maps to AfterTool).
func TestGeminiInboxReplayEmitsSessionStart(t *testing.T) {
	script := inboxReplayHook().Scripts["gemini"](HookContext{TaskID: "gig-ab12"})
	if !strings.Contains(script, `"SessionStart"`) {
		t.Errorf("gemini inbox-replay must emit SessionStart:\n%s", script)
	}
}

func TestGeminiCheckpointNudgeEmitsAfterTool(t *testing.T) {
	ctx := HookContext{CheckpointPatterns: []string{"git commit"}}
	gemini := checkpointNudgeHook().Scripts["gemini"](ctx)
	if !strings.Contains(gemini, `"AfterTool"`) {
		t.Errorf("gemini checkpoint-nudge must emit AfterTool (matches settings key):\n%s", gemini)
	}
	claude := checkpointNudgeHook().Scripts["claude"](ctx)
	if !strings.Contains(claude, `"PostToolUse"`) {
		t.Errorf("claude checkpoint-nudge must emit PostToolUse:\n%s", claude)
	}
}

// TestOrchestratorInboxIsSessionStartReplay asserts the orchestrator direction
// mirrors the worker one: SessionStart replay, emitting SessionStart, draining
// the framed+acking orchestrator inbox.
func TestOrchestratorInboxIsSessionStartReplay(t *testing.T) {
	h := orchestratorInboxHook()
	if h.Event != "SessionStart" {
		t.Errorf("orchestrator-inbox event = %q, want SessionStart", h.Event)
	}
	script := h.Scripts["claude"](HookContext{OrchestratorID: "jeff-1"})
	if !strings.Contains(script, "orchestrator-inbox") || !strings.Contains(script, "--format agent") {
		t.Errorf("orchestrator-inbox replay must drain framed+acking messages:\n%s", script)
	}
	if !strings.Contains(script, `"SessionStart"`) {
		t.Errorf("orchestrator-inbox must emit SessionStart hookEventName:\n%s", script)
	}
}

// TestSyncRemovesStalePostToolUseRegistration reproduces the live-home drift
// (gig-1d9d.16.1): orchestrator-inbox.sh is registered under SessionStart
// (correct, matching builtin.go) AND under a stale leftover PostToolUse block
// (from an older polling-era registration this hook's own doc comment calls
// "the double-delivery"). Re-syncing must remove the registration the registry
// no longer declares for this hook, leaving only the correct SessionStart entry.
func TestSyncRemovesStalePostToolUseRegistration(t *testing.T) {
	dir := t.TempDir()
	sp := settingsPath(dir)

	staleCommand := scriptPath(dir, "orchestrator-inbox")
	settings := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "*",
					"hooks": []any{
						map[string]any{"type": "command", "command": staleCommand, "timeout": 5},
					},
				},
			},
			"PostToolUse": []any{
				map[string]any{
					"matcher": "*",
					"hooks": []any{
						map[string]any{"type": "command", "command": staleCommand, "timeout": 5},
					},
				},
			},
		},
	}
	if err := writeSettingsFile(sp, settings); err != nil {
		t.Fatal(err)
	}

	reg := DefaultRegistry()
	mgr := NewManager(reg)
	ctx := HookContext{JeffHome: dir, TargetDir: dir, OrchestratorID: "jeff-1"}
	enabled := EnabledForSource(nil, SourceHome, reg)
	if err := mgr.Sync(dir, enabled, "claude", ctx); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	got, err := readSettingsFile(sp)
	if err != nil {
		t.Fatal(err)
	}
	hooksMap, _ := got["hooks"].(map[string]any)
	if postToolUse, ok := hooksMap["PostToolUse"]; ok {
		t.Errorf("stale PostToolUse registration for orchestrator-inbox survived sync-hooks: %v", postToolUse)
	}
	sessionStart, _ := hooksMap["SessionStart"].([]any)
	found := false
	for _, b := range sessionStart {
		if blockContainsScript(b, "orchestrator-inbox.sh") {
			found = true
		}
	}
	if !found {
		t.Error("orchestrator-inbox SessionStart registration missing after sync-hooks")
	}
}

// TestCrossEventPurgeNeverDeletesForeignHook is hardy's PR #106 review probe,
// promoted to a permanent regression test. removeScriptFromOtherEvents (added
// to fix the drift above) originally matched by basename alone — a user's own
// hook hand-registered on any event, sharing a basename with one of jeff's
// (e.g. their own "jeff-instructions.sh"), was silently deleted the next time
// jeff synced a hook with that name on a different event. That is worse than
// the #95 rewrite-by-basename bug this same file already guards against for
// refreshHookCommand (deletion vs. rewrite), and TestSyncRemovesStale... above
// cannot catch it — it only ever duplicates the SAME script path across
// events, never a foreign one sharing a basename.
func TestCrossEventPurgeNeverDeletesForeignHook(t *testing.T) {
	dir := t.TempDir()
	sp := settingsPath(dir)

	// A live, hand-authored hook living OUTSIDE jeff's hooks/ layout, whose
	// basename collides with jeff's own "jeff-instructions" hook (SessionStart).
	foreignDir := t.TempDir()
	foreignScript := filepath.Join(foreignDir, "jeff-instructions.sh")
	if err := os.WriteFile(foreignScript, []byte("#!/bin/bash\necho mine\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	settings := map[string]any{
		"hooks": map[string]any{
			"Notification": []any{
				map[string]any{
					"matcher": "*",
					"hooks": []any{
						map[string]any{"type": "command", "command": foreignScript, "timeout": 5},
					},
				},
			},
		},
	}
	if err := writeSettingsFile(sp, settings); err != nil {
		t.Fatal(err)
	}

	reg := DefaultRegistry()
	mgr := NewManager(reg)
	ctx := HookContext{JeffHome: dir, TargetDir: dir}
	enabled := EnabledForSource(nil, SourceHome, reg)
	if err := mgr.Sync(dir, enabled, "claude", ctx); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	got, err := readSettingsFile(sp)
	if err != nil {
		t.Fatal(err)
	}
	hooksMap, _ := got["hooks"].(map[string]any)
	notification, ok := hooksMap["Notification"]
	if !ok {
		t.Fatal("user's Notification hook was deleted entirely by the cross-event purge")
	}
	arr, _ := notification.([]any)
	if len(arr) != 1 || !blockContainsScript(arr[0], "jeff-instructions.sh") {
		t.Errorf("user's Notification registration was altered: %v", notification)
	}
	if _, err := os.Stat(foreignScript); err != nil {
		t.Errorf("user's foreign script was removed from disk: %v", err)
	}
}
