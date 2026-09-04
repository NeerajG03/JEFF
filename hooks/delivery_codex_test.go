package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCodexEventNameMapping(t *testing.T) {
	d := &codexDelivery{}
	events := []string{"SessionStart", "PostToolUse", "Stop", "SessionEnd", "PreCompact", "PreToolUse"}
	for _, ev := range events {
		h := &Hook{Event: ev}
		if got := d.EventName(h); got != ev {
			t.Errorf("EventName(%q) = %q, want %q", ev, got, ev)
		}
	}
}

func TestCodexDeliveryInstallAndUninstall(t *testing.T) {
	dir := t.TempDir()
	d := &codexDelivery{}

	h := &Hook{
		Name:    "test-codex-hook",
		Source:  SourceTask,
		Event:   "SessionStart",
		Matcher: "startup|resume",
		Timeout: 15,
		Scripts: map[string]func(ctx HookContext) string{
			"codex": func(ctx HookContext) string {
				return "#!/bin/bash\necho '{\"status\": \"ok\"}'"
			},
		},
	}

	ctx := HookContext{JeffHome: dir, TargetDir: dir}
	if err := d.Install(h, dir, ctx); err != nil {
		t.Fatalf("Install: %v", err)
	}

	hooksPath := codexHooksPath(dir)
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("read hooks.json: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse hooks.json: %v", err)
	}

	hooksMap, ok := parsed["hooks"].(map[string]any)
	if !ok {
		t.Fatal("hooks.json missing hooks key")
	}

	blocks, ok := hooksMap["SessionStart"].([]any)
	if !ok || len(blocks) != 1 {
		t.Fatalf("expected 1 SessionStart block, got %v", hooksMap["SessionStart"])
	}

	block := blocks[0].(map[string]any)
	if block["matcher"] != "startup|resume" {
		t.Errorf("matcher = %v, want 'startup|resume'", block["matcher"])
	}

	scriptFile := filepath.Join(dir, "hooks", "test-codex-hook.sh")
	if _, err := os.Stat(scriptFile); err != nil {
		t.Fatalf("script file not created: %v", err)
	}

	if !d.IsManaged(dir, "test-codex-hook") {
		t.Error("expected hook to be marked managed")
	}

	// Test uninstall
	if err := d.Uninstall("test-codex-hook", dir); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	if _, err := os.Stat(scriptFile); !os.IsNotExist(err) {
		t.Errorf("expected script file to be deleted, got err: %v", err)
	}

	dataAfter, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("read hooks.json after uninstall: %v", err)
	}
	var parsedAfter map[string]any
	if err := json.Unmarshal(dataAfter, &parsedAfter); err != nil {
		t.Fatalf("parse hooks.json after uninstall: %v", err)
	}
	hooksMapAfter, _ := parsedAfter["hooks"].(map[string]any)
	if len(hooksMapAfter) > 0 {
		t.Errorf("expected hooks map to be empty or nil after uninstalling all hooks, got %v", hooksMapAfter)
	}
}
