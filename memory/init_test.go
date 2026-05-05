package memory_test

import (
	"os"
	"path/filepath"
	"testing"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/memory"
)

// bootstrapHome creates a minimal JEFF_HOME for testing: writes jeff.json so
// ensureMemoryConfig can load it without touching real config paths.
func bootstrapHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	cfg := jeff.DefaultConfig()
	cfg.Home = home
	if err := jeff.SaveConfig(&cfg); err != nil {
		t.Fatalf("setup: save config: %v", err)
	}
	return home
}

func TestInitialize(t *testing.T) {
	home := bootstrapHome(t)

	if err := memory.Initialize(home); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Memory layout.
	assertDir(t, home, "memory")
	assertDir(t, home, "memory/personas")
	assertDir(t, home, "memory/repos")
	assertDir(t, home, "proposals")
	assertDir(t, home, "queue/sessions")
	assertDir(t, home, "transcripts")
	assertDir(t, home, "archive")

	// Marlowe GOAL.md.
	assertFile(t, home, "personas/marlowe/GOAL.md")

	// Curation skill.
	assertFile(t, home, ".skills/curation/SKILL.md")

	// Slash commands.
	assertFile(t, home, ".claude/commands/memory.md")
	assertFile(t, home, ".claude/commands/memory-propose.md")

	// jeff.json has memory section.
	cfg, err := jeff.LoadConfig(home)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Memory == nil {
		t.Error("jeff.json: memory section is nil after Initialize")
	}
}

func TestInitializeIdempotent(t *testing.T) {
	home := bootstrapHome(t)

	if err := memory.Initialize(home); err != nil {
		t.Fatalf("first Initialize: %v", err)
	}
	if err := memory.Initialize(home); err != nil {
		t.Fatalf("second Initialize: %v", err)
	}
}

func TestInitializeNoConfig(t *testing.T) {
	// Initialize should work even if jeff.json does not yet exist (creates it).
	home := t.TempDir()
	// Ensure .skills dir exists so SeedCuration can write to it.
	if err := os.MkdirAll(filepath.Join(home, ".skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := memory.Initialize(home); err != nil {
		t.Fatalf("Initialize without prior jeff.json: %v", err)
	}
	assertDir(t, home, "memory/personas")
}

// ---- helpers ----

func assertDir(t *testing.T, base, rel string) {
	t.Helper()
	p := filepath.Join(base, filepath.FromSlash(rel))
	info, err := os.Stat(p)
	if err != nil {
		t.Errorf("expected dir %s: %v", rel, err)
		return
	}
	if !info.IsDir() {
		t.Errorf("expected %s to be a directory", rel)
	}
}

func assertFile(t *testing.T, base, rel string) {
	t.Helper()
	p := filepath.Join(base, filepath.FromSlash(rel))
	info, err := os.Stat(p)
	if err != nil {
		t.Errorf("expected file %s: %v", rel, err)
		return
	}
	if info.IsDir() {
		t.Errorf("expected %s to be a file, got dir", rel)
	}
}
