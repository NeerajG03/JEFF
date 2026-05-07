package main

import (
	"os"
	"path/filepath"
	"testing"

	jeff "github.com/NeerajG03/JEFF"
)

func setupJeffHome(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	home := filepath.Join(base, ".jeff")
	os.MkdirAll(home, 0o755)
	c := jeff.DefaultConfig()
	c.Home = home
	if err := jeff.SaveConfig(&c); err != nil {
		t.Fatal(err)
	}
	// Redirect HOME so WriteHomePointer doesn't corrupt the real pointer.
	t.Setenv("HOME", base)
	return home
}

func TestEnsureDirs_CreatesAll(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".jeff")
	ensureDirs(home)

	expected := []string{
		"repos", "tasks", "worktrees", "exports",
		"scripts", ".skills", ".claude", ".opencode",
	}
	for _, d := range expected {
		path := filepath.Join(home, d)
		fi, err := os.Stat(path)
		if err != nil {
			t.Errorf("%s not created: %v", d, err)
			continue
		}
		if !fi.IsDir() {
			t.Errorf("%s is not a directory", d)
		}
	}
}

func TestEnsureDirs_Idempotent(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".jeff")
	ensureDirs(home)

	// Create a file inside one of the dirs to verify it's not wiped.
	marker := filepath.Join(home, "repos", "marker.txt")
	os.WriteFile(marker, []byte("keep"), 0o644)

	ensureDirs(home) // second call

	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal("marker file was removed on second ensureDirs call")
	}
	if string(data) != "keep" {
		t.Error("marker file contents changed")
	}
}

func TestWriteDefaults_CreatesFiles(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".jeff")
	ensureDirs(home)
	writeDefaults(home)

	files := []string{
		filepath.Join(home, ".skills", "skills.json"),
		filepath.Join(home, ".claude", "settings.json"),
		filepath.Join(home, ".claude", "settings.local.json"),
		filepath.Join(home, ".opencode", "settings.json"),
		filepath.Join(home, ".opencode", "settings.local.json"),
	}
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("%s not created: %v", filepath.Base(f), err)
		}
	}
}

func TestWriteDefaults_DoesNotOverwrite(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".jeff")
	ensureDirs(home)

	// Write custom content first.
	custom := filepath.Join(home, ".skills", "skills.json")
	os.WriteFile(custom, []byte(`{"custom":true}`), 0o644)

	writeDefaults(home)

	data, err := os.ReadFile(custom)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"custom":true}` {
		t.Error("writeDefaults overwrote existing file")
	}
}

func TestEnsureDirs_AddsNewDirs(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".jeff")

	// Simulate old install: only some dirs exist.
	os.MkdirAll(filepath.Join(home, "repos"), 0o755)
	os.MkdirAll(filepath.Join(home, "tasks"), 0o755)
	// .skills/ is missing (added in newer version)

	ensureDirs(home)

	if _, err := os.Stat(filepath.Join(home, ".skills")); err != nil {
		t.Error(".skills/ not created by ensureDirs on existing home")
	}
}

func TestRunUpdate_NoInit(t *testing.T) {
	// Redirect HOME so we don't read the real pointer.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("JEFF_HOME", filepath.Join(t.TempDir(), "nonexistent"))
	err := runUpdate()
	if err == nil {
		t.Error("expected error when JEFF is not initialized")
	}
}

func TestRunUpdate_SyncsExisting(t *testing.T) {
	home := setupJeffHome(t)

	// Only minimal dirs exist (simulating old version).
	// .skills/ should be missing.
	t.Setenv("JEFF_HOME", home)

	err := runUpdate()
	if err != nil {
		t.Fatalf("runUpdate failed: %v", err)
	}

	// .skills/ should now exist.
	if _, err := os.Stat(filepath.Join(home, ".skills")); err != nil {
		t.Error(".skills/ not created by update")
	}

	// skills.json should exist.
	if _, err := os.Stat(filepath.Join(home, ".skills", "skills.json")); err != nil {
		t.Error("skills.json not created by update")
	}
}

func TestRunUpdate_CreatesGeminiSkillsAlias(t *testing.T) {
	home := setupJeffHome(t)
	t.Setenv("JEFF_HOME", home)

	if err := runUpdate(); err != nil {
		t.Fatalf("runUpdate failed: %v", err)
	}

	link := filepath.Join(home, ".gemini", "skills")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf(".gemini/skills symlink not created: %v", err)
	}
	wantTarget := filepath.Join("..", ".claude", "skills")
	if target != wantTarget {
		t.Errorf("target = %q, want %q", target, wantTarget)
	}

	// Re-running update should be a no-op for the alias.
	if err := runUpdate(); err != nil {
		t.Fatalf("second runUpdate failed: %v", err)
	}
	target2, err := os.Readlink(link)
	if err != nil || target2 != wantTarget {
		t.Errorf("alias broke on second update: target=%q err=%v", target2, err)
	}
}

func TestRunUpdate_PreservesConfig(t *testing.T) {
	home := setupJeffHome(t)

	// Set a custom agent value.
	c, _ := jeff.LoadConfig(home)
	c.Agent = jeff.AgentOpenCode
	jeff.SaveConfig(c)

	t.Setenv("JEFF_HOME", home)

	if err := runUpdate(); err != nil {
		t.Fatalf("runUpdate failed: %v", err)
	}

	// Config should still have custom value.
	reloaded, err := jeff.LoadConfig(home)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Agent != jeff.AgentOpenCode {
		t.Errorf("agent = %s, want opencode (config was overwritten)", reloaded.Agent)
	}
}
