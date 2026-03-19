package jeff

import (
	"os"
	"path/filepath"
	"testing"
)

func tempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, ".jeff")
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Agent != AgentClaudeCode {
		t.Errorf("expected claude, got %s", cfg.Agent)
	}
	if cfg.Repos == nil {
		t.Error("repos map should be initialized")
	}
}

func TestLoadConfigMissing(t *testing.T) {
	home := tempHome(t)
	cfg, err := LoadConfig(home)
	if err != nil {
		t.Fatalf("load missing config: %v", err)
	}
	if cfg.Agent != AgentClaudeCode {
		t.Errorf("expected default agent, got %s", cfg.Agent)
	}
	if cfg.Home != home {
		t.Errorf("expected home %s, got %s", home, cfg.Home)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	home := tempHome(t)
	os.MkdirAll(home, 0o755)

	cfg := &Config{
		Agent: AgentOpenCode,
		Repos: map[string]string{"backend": "https://github.com/org/backend.git"},
		Home:  home,
	}

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	loaded, err := LoadConfig(home)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Agent != AgentOpenCode {
		t.Errorf("expected opencode, got %s", loaded.Agent)
	}
	if loaded.Repos["backend"] != "https://github.com/org/backend.git" {
		t.Errorf("expected backend repo URL, got %q", loaded.Repos["backend"])
	}
}

func TestLoadConfigInvalidAgent(t *testing.T) {
	home := tempHome(t)
	os.MkdirAll(home, 0o755)

	os.WriteFile(ConfigPath(home), []byte("agent: badtool\n"), 0o644)

	cfg, err := LoadConfig(home)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Agent != AgentClaudeCode {
		t.Errorf("invalid agent should fallback to claude, got %s", cfg.Agent)
	}
}

func TestResolveHomeEnvVar(t *testing.T) {
	t.Setenv("JEFF_HOME", "/tmp/test-jeff-home")
	home, err := ResolveHome()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if home != "/tmp/test-jeff-home" {
		t.Errorf("expected /tmp/test-jeff-home, got %s", home)
	}
}

func TestWriteAndResolveHomePointer(t *testing.T) {
	// Clear env so pointer file is used.
	t.Setenv("JEFF_HOME", "")

	customHome := t.TempDir()
	if err := WriteHomePointer(customHome); err != nil {
		t.Fatalf("write pointer: %v", err)
	}

	home, err := ResolveHome()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if home != customHome {
		t.Errorf("expected %s, got %s", customHome, home)
	}

	// Cleanup: restore default pointer.
	ptr, _ := globalPointerPath()
	os.Remove(ptr)
}

func TestAgentToolCommand(t *testing.T) {
	tests := []struct {
		tool AgentTool
		cmd  string
	}{
		{AgentClaudeCode, "claude"},
		{AgentOpenCode, "opencode"},
	}
	for _, tt := range tests {
		if got := tt.tool.Command(); got != tt.cmd {
			t.Errorf("Command(%s) = %q, want %q", tt.tool, got, tt.cmd)
		}
	}
}
