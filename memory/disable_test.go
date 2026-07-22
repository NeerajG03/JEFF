package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDisabled_DefaultFalse(t *testing.T) {
	home := t.TempDir()
	// No jeff.json — not disabled.
	if Disabled(home) {
		t.Error("expected false when no jeff.json exists")
	}
}

func TestDisabled_EnvVar(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_MEMORY_DISABLE", "1")
	if !Disabled(home) {
		t.Error("expected true with JEFF_MEMORY_DISABLE=1")
	}
}

func TestDisabled_EnvVarTrue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_MEMORY_DISABLE", "true")
	if !Disabled(home) {
		t.Error("expected true with JEFF_MEMORY_DISABLE=true")
	}
}

func TestDisabled_EnvVarFalse(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_MEMORY_DISABLE", "0")
	if Disabled(home) {
		t.Error("expected false with JEFF_MEMORY_DISABLE=0")
	}
}

func TestDisabled_ConfigTrue(t *testing.T) {
	home := t.TempDir()
	cfg := map[string]any{
		"memory": map[string]any{
			"disabled": true,
		},
	}
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(home, "jeff.json"), data, 0o644)

	if !Disabled(home) {
		t.Error("expected true with jeff.json memory.disabled=true")
	}
}

func TestDisabled_ConfigFalse(t *testing.T) {
	home := t.TempDir()
	cfg := map[string]any{
		"memory": map[string]any{
			"disabled": false,
		},
	}
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(home, "jeff.json"), data, 0o644)

	if Disabled(home) {
		t.Error("expected false with jeff.json memory.disabled=false")
	}
}

func TestDisabled_EnvOverridesConfig(t *testing.T) {
	home := t.TempDir()
	cfg := map[string]any{
		"memory": map[string]any{
			"disabled": false,
		},
	}
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(home, "jeff.json"), data, 0o644)

	t.Setenv("JEFF_MEMORY_DISABLE", "1")
	if !Disabled(home) {
		t.Error("expected true when env var overrides config")
	}
}
