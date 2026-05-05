package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ---- EnvOverrides tests ----

func TestEnvOverrides_ClaudeWorker(t *testing.T) {
	m := EnvOverrides("jenko", "claude")
	if m["CLAUDE_CODE_DISABLE_AUTO_MEMORY"] != "1" {
		t.Error("want CLAUDE_CODE_DISABLE_AUTO_MEMORY=1 for claude")
	}
	if _, ok := m["GEMINI_NO_AUTO_MEMORY"]; ok {
		t.Error("should not set GEMINI var for claude")
	}
	if m["JEFF_MEMORY_CAN_ADD"] != "" {
		t.Errorf("non-marlowe persona should have JEFF_MEMORY_CAN_ADD='', got %q", m["JEFF_MEMORY_CAN_ADD"])
	}
}

func TestEnvOverrides_GeminiWorker(t *testing.T) {
	m := EnvOverrides("eric", "gemini")
	if m["GEMINI_NO_AUTO_MEMORY"] != "1" {
		t.Error("want GEMINI_NO_AUTO_MEMORY=1 for gemini")
	}
	if _, ok := m["CLAUDE_CODE_DISABLE_AUTO_MEMORY"]; ok {
		t.Error("should not set CLAUDE var for gemini")
	}
	if m["JEFF_MEMORY_CAN_ADD"] != "" {
		t.Error("non-marlowe persona should have JEFF_MEMORY_CAN_ADD=''")
	}
}

func TestEnvOverrides_MarloweClaude(t *testing.T) {
	m := EnvOverrides("marlowe", "claude")
	if m["JEFF_MEMORY_CAN_ADD"] != "1" {
		t.Errorf("marlowe should have JEFF_MEMORY_CAN_ADD=1, got %q", m["JEFF_MEMORY_CAN_ADD"])
	}
	if m["CLAUDE_CODE_DISABLE_AUTO_MEMORY"] != "1" {
		t.Error("marlowe still suppresses native Claude memory")
	}
}

func TestEnvOverrides_MarloweGemini(t *testing.T) {
	m := EnvOverrides("marlowe", "gemini")
	if m["JEFF_MEMORY_CAN_ADD"] != "1" {
		t.Errorf("marlowe gemini should have JEFF_MEMORY_CAN_ADD=1, got %q", m["JEFF_MEMORY_CAN_ADD"])
	}
}

func TestEnvOverrides_AllWorkersHaveCanAddKey(t *testing.T) {
	personas := []string{"jenko", "schmidt", "eric", "hardy", "doug", "dickson"}
	for _, p := range personas {
		m := EnvOverrides(p, "claude")
		if _, ok := m["JEFF_MEMORY_CAN_ADD"]; !ok {
			t.Errorf("persona %s: JEFF_MEMORY_CAN_ADD key missing (must be present even when empty)", p)
		}
		if m["JEFF_MEMORY_CAN_ADD"] == "1" {
			t.Errorf("persona %s: JEFF_MEMORY_CAN_ADD should not be 1", p)
		}
	}
}

// ---- ApplySettings tests ----

func TestApplySettings_Claude(t *testing.T) {
	dir := t.TempDir()

	if err := ApplySettings(dir, "claude"); err != nil {
		t.Fatalf("ApplySettings claude: %v", err)
	}

	path := filepath.Join(dir, ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("settings.json not written: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	env, _ := settings["env"].(map[string]any)
	if env == nil {
		t.Fatal("settings.json missing 'env' key")
	}
	if env["CLAUDE_CODE_DISABLE_AUTO_MEMORY"] != "1" {
		t.Errorf("want CLAUDE_CODE_DISABLE_AUTO_MEMORY=1, got %v", env["CLAUDE_CODE_DISABLE_AUTO_MEMORY"])
	}
}

func TestApplySettings_ClaudeIdempotent(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 3; i++ {
		if err := ApplySettings(dir, "claude"); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
	}

	path := filepath.Join(dir, ".claude", "settings.json")
	data, _ := os.ReadFile(path)
	var settings map[string]any
	json.Unmarshal(data, &settings)

	env, _ := settings["env"].(map[string]any)
	if env["CLAUDE_CODE_DISABLE_AUTO_MEMORY"] != "1" {
		t.Error("idempotent apply broke the setting")
	}
}

func TestApplySettings_ClaudeMergesExisting(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0o755)

	// Write an existing settings file with some content.
	existing := `{"$schema":"https://example.com/schema","env":{"MY_VAR":"hello"}}`
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(existing), 0o644)

	if err := ApplySettings(dir, "claude"); err != nil {
		t.Fatalf("ApplySettings: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	var settings map[string]any
	json.Unmarshal(data, &settings)

	env, _ := settings["env"].(map[string]any)
	// Our key is added.
	if env["CLAUDE_CODE_DISABLE_AUTO_MEMORY"] != "1" {
		t.Error("suppress key not merged")
	}
	// Existing key is preserved.
	if env["MY_VAR"] != "hello" {
		t.Error("existing env var was clobbered")
	}
	// Schema is preserved.
	if settings["$schema"] == nil {
		t.Error("$schema was lost during merge")
	}
}

func TestApplySettings_Gemini(t *testing.T) {
	dir := t.TempDir()

	if err := ApplySettings(dir, "gemini"); err != nil {
		t.Fatalf("ApplySettings gemini: %v", err)
	}

	path := filepath.Join(dir, ".gemini", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("gemini settings.json not written: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	mem, _ := settings["memory"].(map[string]any)
	if mem == nil {
		t.Fatal("missing 'memory' key in gemini settings")
	}
	if mem["autoload"] != false {
		t.Errorf("want memory.autoload=false, got %v", mem["autoload"])
	}
}

func TestApplySettings_UnknownAgent(t *testing.T) {
	dir := t.TempDir()
	// Should be a no-op, not an error.
	if err := ApplySettings(dir, "opencode"); err != nil {
		t.Errorf("unexpected error for unknown agent: %v", err)
	}
}
