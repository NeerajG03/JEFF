package hooks

import (
	"encoding/json"
	"os"
	"testing"
)

func TestGeminiEventNameMapping(t *testing.T) {
	tests := []struct {
		claude string
		gemini string
	}{
		{"PostToolUse", "AfterTool"},
		{"Stop", "SessionEnd"},
		{"PreCompact", "PreCompress"},
		{"SessionStart", "SessionStart"}, // unchanged
		{"Unknown", "Unknown"},           // passthrough
	}

	for _, tt := range tests {
		got := geminiEventName(tt.claude)
		if got != tt.gemini {
			t.Errorf("geminiEventName(%q) = %q, want %q", tt.claude, got, tt.gemini)
		}
	}
}

func TestGeminiDeliveryInstallWritesCorrectEvent(t *testing.T) {
	dir := t.TempDir()
	d := &geminiDelivery{}

	h := &Hook{
		Name:    "test-hook",
		Source:  SourceTask,
		Event:   "PostToolUse", // Claude event name
		Matcher: "Bash",
		Scripts: map[string]func(ctx HookContext) string{
			"gemini": func(ctx HookContext) string {
				return "#!/bin/bash\necho test"
			},
		},
	}

	ctx := HookContext{JeffHome: dir, TargetDir: dir}
	if err := d.Install(h, dir, ctx); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Read the Gemini settings.json and verify the event key is "AfterTool".
	data, err := os.ReadFile(geminiSettingsPath(dir))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse settings: %v", err)
	}

	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatal("settings missing hooks key")
	}

	// Should have "AfterTool", NOT "PostToolUse".
	if _, ok := hooksMap["AfterTool"]; !ok {
		t.Errorf("settings.json hooks keys = %v, want AfterTool", keysOf(hooksMap))
	}
	if _, ok := hooksMap["PostToolUse"]; ok {
		t.Error("settings.json should NOT contain PostToolUse for Gemini")
	}
}

func TestGeminiDeliveryInstallStopEvent(t *testing.T) {
	dir := t.TempDir()
	d := &geminiDelivery{}

	h := &Hook{
		Name:    "worker-stop",
		Source:  SourceTask,
		Event:   "Stop",
		Matcher: "*",
		Scripts: map[string]func(ctx HookContext) string{
			"gemini": func(ctx HookContext) string {
				return "#!/bin/bash\necho stop"
			},
		},
	}

	ctx := HookContext{JeffHome: dir, TargetDir: dir}
	if err := d.Install(h, dir, ctx); err != nil {
		t.Fatalf("Install: %v", err)
	}

	data, err := os.ReadFile(geminiSettingsPath(dir))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}

	var settings map[string]any
	json.Unmarshal(data, &settings)
	hooksMap := settings["hooks"].(map[string]any)

	if _, ok := hooksMap["SessionEnd"]; !ok {
		t.Errorf("settings.json hooks keys = %v, want SessionEnd", keysOf(hooksMap))
	}
}

func TestGeminiDeliverySessionStartUnchanged(t *testing.T) {
	dir := t.TempDir()
	d := &geminiDelivery{}

	h := &Hook{
		Name:    "gig-instructions",
		Source:  SourceHome,
		Event:   "SessionStart",
		Matcher: "*",
		Scripts: map[string]func(ctx HookContext) string{
			"gemini": func(ctx HookContext) string {
				return "#!/bin/bash\necho start"
			},
		},
	}

	ctx := HookContext{JeffHome: dir, TargetDir: dir}
	if err := d.Install(h, dir, ctx); err != nil {
		t.Fatalf("Install: %v", err)
	}

	data, _ := os.ReadFile(geminiSettingsPath(dir))
	var settings map[string]any
	json.Unmarshal(data, &settings)
	hooksMap := settings["hooks"].(map[string]any)

	if _, ok := hooksMap["SessionStart"]; !ok {
		t.Errorf("settings.json hooks keys = %v, want SessionStart", keysOf(hooksMap))
	}
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
