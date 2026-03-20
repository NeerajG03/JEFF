package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadWriteSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0o755)

	original := map[string]any{
		"$schema": "https://json.schemastore.org/claude-code-settings.json",
		"foo":     "bar",
	}
	if err := writeSettings(dir, original); err != nil {
		t.Fatal(err)
	}

	got, err := readSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got["foo"] != "bar" {
		t.Fatalf("got foo=%v, want bar", got["foo"])
	}
}

func TestReadSettingsNotExist(t *testing.T) {
	dir := t.TempDir()
	settings, err := readSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if settings["$schema"] == nil {
		t.Fatal("expected $schema in fresh settings")
	}
}

func TestAddHookToSettings(t *testing.T) {
	settings := map[string]any{}
	addHookToSettings(settings, "SessionStart", "*", "/path/to/test-hook.sh", 10)

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		t.Fatal("expected hooks map")
	}
	blocks, _ := hooks["SessionStart"].([]any)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}

	block := blocks[0].(map[string]any)
	if block["matcher"] != "*" {
		t.Fatalf("got matcher %v, want *", block["matcher"])
	}
}

func TestAddHookToSettingsIdempotent(t *testing.T) {
	settings := map[string]any{}
	addHookToSettings(settings, "SessionStart", "*", "/path/to/test-hook.sh", 10)
	addHookToSettings(settings, "SessionStart", "*", "/path/to/test-hook.sh", 10)

	hooks := settings["hooks"].(map[string]any)
	blocks := hooks["SessionStart"].([]any)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks after double add, want 1", len(blocks))
	}
}

func TestRemoveHookFromSettings(t *testing.T) {
	settings := map[string]any{}
	addHookToSettings(settings, "SessionStart", "*", "/path/to/keep.sh", 10)
	addHookToSettings(settings, "SessionStart", "*", "/path/to/remove-me.sh", 10)

	removeHookFromSettings(settings, "remove-me.sh")

	hooks := settings["hooks"].(map[string]any)
	blocks := hooks["SessionStart"].([]any)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks after remove, want 1", len(blocks))
	}

	// Verify the remaining one is keep.sh
	block := blocks[0].(map[string]any)
	hookList := block["hooks"].([]any)
	hk := hookList[0].(map[string]any)
	if cmd, _ := hk["command"].(string); cmd != "/path/to/keep.sh" {
		t.Fatalf("remaining hook command = %q, want keep.sh", cmd)
	}
}

func TestRemoveHookCleansUpEmptyEvent(t *testing.T) {
	settings := map[string]any{}
	addHookToSettings(settings, "SessionStart", "*", "/path/to/only.sh", 10)
	removeHookFromSettings(settings, "only.sh")

	if _, exists := settings["hooks"]; exists {
		t.Fatal("expected hooks key to be removed when empty")
	}
}

func TestInstallClaude(t *testing.T) {
	dir := t.TempDir()

	h := &Hook{
		Name:    "test-hook",
		Source:  SourceHome,
		Event:   "SessionStart",
		Matcher: "*",
		Timeout: 5,
		ClaudeScript: func(ctx HookContext) string {
			return "#!/bin/bash\necho hello"
		},
	}

	ctx := HookContext{JeffHome: dir, TargetDir: dir}
	if err := installClaude(h, dir, ctx); err != nil {
		t.Fatal(err)
	}

	// Check script was written.
	sp := scriptPath(dir, "test-hook")
	data, err := os.ReadFile(sp)
	if err != nil {
		t.Fatalf("script not written: %v", err)
	}
	if string(data) != "#!/bin/bash\necho hello" {
		t.Fatalf("unexpected script content: %s", data)
	}

	// Check settings.json was updated.
	settings, _ := readSettings(dir)
	hooks, _ := settings["hooks"].(map[string]any)
	blocks, _ := hooks["SessionStart"].([]any)
	if len(blocks) != 1 {
		t.Fatalf("got %d settings blocks, want 1", len(blocks))
	}
}

func TestInstallClaudeIdempotent(t *testing.T) {
	dir := t.TempDir()

	h := &Hook{
		Name:    "test-hook",
		Source:  SourceHome,
		Event:   "SessionStart",
		Matcher: "*",
		ClaudeScript: func(ctx HookContext) string {
			return "#!/bin/bash\necho hello"
		},
	}

	ctx := HookContext{JeffHome: dir, TargetDir: dir}
	installClaude(h, dir, ctx)
	installClaude(h, dir, ctx)

	settings, _ := readSettings(dir)
	hooks, _ := settings["hooks"].(map[string]any)
	blocks, _ := hooks["SessionStart"].([]any)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks after double install, want 1", len(blocks))
	}
}

func TestUninstallClaude(t *testing.T) {
	dir := t.TempDir()

	h := &Hook{
		Name:    "test-hook",
		Source:  SourceHome,
		Event:   "SessionStart",
		Matcher: "*",
		ClaudeScript: func(ctx HookContext) string {
			return "#!/bin/bash\necho hello"
		},
	}

	ctx := HookContext{JeffHome: dir, TargetDir: dir}
	installClaude(h, dir, ctx)

	if err := uninstallClaude("test-hook", dir); err != nil {
		t.Fatal(err)
	}

	// Script removed.
	if _, err := os.Stat(scriptPath(dir, "test-hook")); !os.IsNotExist(err) {
		t.Fatal("expected script to be removed")
	}

	// Settings cleaned up.
	settings, _ := readSettings(dir)
	if _, exists := settings["hooks"]; exists {
		t.Fatal("expected hooks key to be removed")
	}
}

func TestUninstallPreservesOtherHooks(t *testing.T) {
	dir := t.TempDir()

	// Simulate a pre-existing non-jeff hook in settings.json.
	settings := map[string]any{
		"$schema": "https://json.schemastore.org/claude-code-settings.json",
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "*",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "/other/tool/hook.sh",
							"timeout": 5,
						},
					},
				},
			},
		},
	}
	writeSettings(dir, settings)

	// Install jeff hook.
	h := &Hook{
		Name:    "jeff-test",
		Source:  SourceHome,
		Event:   "SessionStart",
		Matcher: "*",
		ClaudeScript: func(ctx HookContext) string {
			return "#!/bin/bash\necho jeff"
		},
	}
	ctx := HookContext{JeffHome: dir, TargetDir: dir}
	installClaude(h, dir, ctx)

	// Uninstall jeff hook — other hook should remain.
	uninstallClaude("jeff-test", dir)

	settings, _ = readSettings(dir)
	hooks, _ := settings["hooks"].(map[string]any)
	blocks, _ := hooks["SessionStart"].([]any)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1 (the other tool's hook)", len(blocks))
	}

	// Verify it's the other tool's hook.
	data, _ := json.Marshal(blocks[0])
	if got := string(data); !contains(got, "/other/tool/hook.sh") {
		t.Fatalf("remaining hook should be other tool's, got: %s", got)
	}
}

func TestInstalledClaudeHooks(t *testing.T) {
	dir := t.TempDir()
	hdir := hooksDir(dir)
	os.MkdirAll(hdir, 0o755)

	os.WriteFile(filepath.Join(hdir, "gig-instructions.sh"), []byte("#!/bin/bash"), 0o755)
	os.WriteFile(filepath.Join(hdir, "jeff-repos.sh"), []byte("#!/bin/bash"), 0o755)
	os.WriteFile(filepath.Join(hdir, "not-a-hook.txt"), []byte("ignore"), 0o644)

	names := installedClaudeHooks(dir)
	if len(names) != 2 {
		t.Fatalf("got %d installed, want 2: %v", len(names), names)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
