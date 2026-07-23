package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	jeffembed "github.com/NeerajG03/JEFF/embed"
	"github.com/NeerajG03/JEFF/memory"
	"github.com/NeerajG03/JEFF/skill"
)

func TestInstallLearnCommand_OnPickup(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()

	err := memory.InstallLearnCommand(dir, "gig-lc01", "jenko", home, []string{"backend"})
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, ".claude", "commands", "learn.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("learn.md not created: %v", err)
	}

	content := string(data)
	for _, want := range []string{"gig-lc01", "jenko", "backend", "scratchpad.md"} {
		if !strings.Contains(content, want) {
			t.Errorf("learn.md missing %q", want)
		}
	}
}

// Mirrors the alias-then-inject sequence pickup runs for a fresh task
// workspace, exercising the integration between EnsureGeminiSkillsAlias and
// skill.InjectTo. Ensures gemini sessions see the same skills as claude
// sessions even on a claude-only setup.
func TestTaskWorkspace_GeminiSkillsAliasReflectsClaudeSkills(t *testing.T) {
	taskDir := t.TempDir()

	// Source skill location (would normally live under JEFF_HOME/.skills/<name>).
	skillSrc := filepath.Join(t.TempDir(), "deploy")
	if err := os.MkdirAll(skillSrc, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(skillSrc, "SKILL.md"), []byte("test"), 0o644)

	// Step 1: alias .gemini/skills → .claude/skills (unconditional).
	if err := jeffembed.EnsureGeminiSkillsAlias(taskDir); err != nil {
		t.Fatalf("alias: %v", err)
	}

	// Step 2: inject a skill into the canonical .claude/skills dir.
	if err := skill.InjectTo("deploy", skillSrc, taskDir, ".claude", "skills"); err != nil {
		t.Fatalf("inject: %v", err)
	}

	// The skill must be visible via both .claude/skills and .gemini/skills.
	for _, agentDir := range []string{".claude", ".gemini"} {
		path := filepath.Join(taskDir, agentDir, "skills", "deploy", "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("skill not visible at %s: %v", path, err)
		}
	}

	// Re-running the alias must be idempotent and must not disturb injected skills.
	if err := jeffembed.EnsureGeminiSkillsAlias(taskDir); err != nil {
		t.Fatalf("alias re-run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(taskDir, ".gemini", "skills", "deploy", "SKILL.md")); err != nil {
		t.Errorf("skill no longer visible after idempotent alias call: %v", err)
	}
}
