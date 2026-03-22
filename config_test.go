package jeff

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/NeerajG03/JEFF/internal/testutil"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Agent != AgentClaudeCode {
		t.Errorf("expected claude, got %s", cfg.Agent)
	}
	if cfg.Repos == nil {
		t.Error("repos map should be initialized")
	}
	if cfg.Schema == "" {
		t.Error("schema should be set")
	}
}

func TestLoadConfigMissing(t *testing.T) {
	home := testutil.TempHome(t)
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
	home := testutil.TempHome(t)
	os.MkdirAll(home, 0o755)

	cfg := &Config{
		Agent: AgentOpenCode,
		Repos: map[string]*RepoConfig{
			"backend": {URL: "https://github.com/org/backend.git", PostSetup: "./setup.sh"},
		},
		Home: home,
	}

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	// Verify it's JSON.
	data, _ := os.ReadFile(ConfigPath(home))
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("saved file is not valid JSON: %v", err)
	}
	if raw["$schema"] == nil {
		t.Error("$schema missing from saved config")
	}

	loaded, err := LoadConfig(home)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Agent != AgentOpenCode {
		t.Errorf("expected opencode, got %s", loaded.Agent)
	}
	if loaded.Repos["backend"] == nil || loaded.Repos["backend"].URL != "https://github.com/org/backend.git" {
		t.Errorf("expected backend repo URL")
	}
	if loaded.Repos["backend"].PostSetup != "./setup.sh" {
		t.Errorf("expected post_setup, got %q", loaded.Repos["backend"].PostSetup)
	}
}

func TestLoadConfigInvalidAgent(t *testing.T) {
	home := testutil.TempHome(t)
	os.MkdirAll(home, 0o755)

	os.WriteFile(ConfigPath(home), []byte(`{"agent":"badtool"}`+"\n"), 0o644)

	cfg, err := LoadConfig(home)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Agent != AgentClaudeCode {
		t.Errorf("invalid agent should fallback to claude, got %s", cfg.Agent)
	}
}

func TestMigrateFromYAML(t *testing.T) {
	home := testutil.TempHome(t)
	os.MkdirAll(home, 0o755)

	// Write a legacy jeff.yaml.
	yamlContent := `agent: claude
ide: windsurf
gig_home: ""
repos:
  backend:
    url: https://github.com/org/backend.git
    base_branch: origin/develop
hooks:
  gig-ready-tasks: true
`
	os.WriteFile(legacyConfigPath(home), []byte(yamlContent), 0o644)

	// LoadConfig should auto-migrate.
	cfg, err := LoadConfig(home)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Agent != AgentClaudeCode {
		t.Errorf("agent = %s, want claude", cfg.Agent)
	}
	if cfg.IDE != IDEWindsurf {
		t.Errorf("ide = %s, want windsurf", cfg.IDE)
	}
	if cfg.Repos["backend"] == nil {
		t.Fatal("backend repo missing after migration")
	}
	if cfg.Repos["backend"].BaseBranch != "origin/develop" {
		t.Errorf("base_branch = %s, want origin/develop", cfg.Repos["backend"].BaseBranch)
	}
	if !cfg.Hooks["gig-ready-tasks"] {
		t.Error("hooks not migrated")
	}

	// jeff.json should exist now.
	if _, err := os.Stat(ConfigPath(home)); err != nil {
		t.Error("jeff.json not created after migration")
	}

	// jeff.yaml should be removed.
	if _, err := os.Stat(legacyConfigPath(home)); err == nil {
		t.Error("jeff.yaml not removed after migration")
	}

	// Verify the JSON file has $schema.
	data, _ := os.ReadFile(ConfigPath(home))
	var raw map[string]any
	json.Unmarshal(data, &raw)
	if raw["$schema"] == nil {
		t.Error("$schema missing from migrated config")
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
