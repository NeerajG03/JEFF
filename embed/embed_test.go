package embed

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureGeminiSkillsAlias_FreshDir(t *testing.T) {
	dir := t.TempDir()

	if err := EnsureGeminiSkillsAlias(dir); err != nil {
		t.Fatalf("EnsureGeminiSkillsAlias: %v", err)
	}

	link := filepath.Join(dir, ".gemini", "skills")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	wantTarget := filepath.Join("..", ".claude", "skills")
	if target != wantTarget {
		t.Errorf("target = %q, want %q", target, wantTarget)
	}

	// .claude/skills should exist as a real directory (the symlink target).
	claudeSkills := filepath.Join(dir, ".claude", "skills")
	fi, err := os.Stat(claudeSkills)
	if err != nil {
		t.Fatalf("stat .claude/skills: %v", err)
	}
	if !fi.IsDir() {
		t.Error(".claude/skills is not a directory")
	}

	// Symlink should resolve to the same path.
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	expectedResolved, _ := filepath.EvalSymlinks(claudeSkills)
	if resolved != expectedResolved {
		t.Errorf("symlink resolves to %q, want %q", resolved, expectedResolved)
	}
}

func TestEnsureGeminiSkillsAlias_Idempotent(t *testing.T) {
	dir := t.TempDir()

	if err := EnsureGeminiSkillsAlias(dir); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := EnsureGeminiSkillsAlias(dir); err != nil {
		t.Fatalf("second call: %v", err)
	}

	// Drop a marker into .claude/skills and run again — must be preserved.
	marker := filepath.Join(dir, ".claude", "skills", "marker")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureGeminiSkillsAlias(dir); err != nil {
		t.Fatalf("third call: %v", err)
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "keep" {
		t.Error("idempotent call should not touch existing skill content")
	}

	// Marker should also be visible via the gemini alias.
	viaAlias := filepath.Join(dir, ".gemini", "skills", "marker")
	if _, err := os.Stat(viaAlias); err != nil {
		t.Errorf("marker not visible via .gemini/skills alias: %v", err)
	}
}

func TestEnsureGeminiSkillsAlias_ReplacesStaleSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gemini"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Plant a stale symlink pointing somewhere else.
	stale := filepath.Join(dir, ".gemini", "skills")
	if err := os.Symlink("/tmp/somewhere-else", stale); err != nil {
		t.Fatal(err)
	}

	if err := EnsureGeminiSkillsAlias(dir); err != nil {
		t.Fatalf("EnsureGeminiSkillsAlias: %v", err)
	}

	target, err := os.Readlink(stale)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	wantTarget := filepath.Join("..", ".claude", "skills")
	if target != wantTarget {
		t.Errorf("stale symlink not replaced: target = %q, want %q", target, wantTarget)
	}
}

func TestEnsureOpenCodeSkillsAlias_FreshDir(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureOpenCodeSkillsAlias(dir); err != nil {
		t.Fatalf("EnsureOpenCodeSkillsAlias: %v", err)
	}
	link := filepath.Join(dir, ".opencode", "skills")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	wantTarget := filepath.Join("..", ".claude", "skills")
	if target != wantTarget {
		t.Errorf("target = %q, want %q", target, wantTarget)
	}
	claudeSkills := filepath.Join(dir, ".claude", "skills")
	if fi, err := os.Stat(claudeSkills); err != nil || !fi.IsDir() {
		t.Error(".claude/skills should exist as a directory")
	}
}

func TestEnsureOpenCodeSkillsAlias_Idempotent(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureOpenCodeSkillsAlias(dir); err != nil {
		t.Fatal(err)
	}
	if err := EnsureOpenCodeSkillsAlias(dir); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, ".claude", "skills", "marker")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureOpenCodeSkillsAlias(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".opencode", "skills", "marker")); err != nil {
		t.Errorf("marker not visible via .opencode/skills alias: %v", err)
	}
}

func TestEnsureGeminiSkillsAlias_ErrorsOnNonSymlinkFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gemini"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Plant a regular file at .gemini/skills.
	conflict := filepath.Join(dir, ".gemini", "skills")
	if err := os.WriteFile(conflict, []byte("not a symlink"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := EnsureGeminiSkillsAlias(dir)
	if err == nil {
		t.Fatal("expected error when .gemini/skills exists as a regular file")
	}
}

func TestEnsureGeminiSkillsAlias_RelativeTargetSurvivesRename(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "home")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := EnsureGeminiSkillsAlias(dir); err != nil {
		t.Fatal(err)
	}

	// Move the parent directory; relative symlink should still resolve.
	moved := filepath.Join(parent, "moved")
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(moved, ".gemini", "skills")
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatalf("EvalSymlinks after move: %v", err)
	}
	wantSuffix := filepath.Join("moved", ".claude", "skills")
	if filepath.Base(filepath.Dir(resolved)) != ".claude" {
		t.Errorf("resolved %q does not contain expected suffix %q", resolved, wantSuffix)
	}
}

func TestEnsureCodexSkillsAlias_FreshDir(t *testing.T) {
	dir := t.TempDir()

	if err := EnsureCodexSkillsAlias(dir); err != nil {
		t.Fatalf("EnsureCodexSkillsAlias: %v", err)
	}

	link := filepath.Join(dir, ".agents", "skills")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	wantTarget := filepath.Join("..", ".claude", "skills")
	if target != wantTarget {
		t.Errorf("target = %q, want %q", target, wantTarget)
	}

	claudeSkills := filepath.Join(dir, ".claude", "skills")
	fi, err := os.Stat(claudeSkills)
	if err != nil {
		t.Fatalf("stat .claude/skills: %v", err)
	}
	if !fi.IsDir() {
		t.Error(".claude/skills is not a directory")
	}

	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	expectedResolved, _ := filepath.EvalSymlinks(claudeSkills)
	if resolved != expectedResolved {
		t.Errorf("symlink resolves to %q, want %q", resolved, expectedResolved)
	}
}

func TestEnsureCodexSkillsAlias_Idempotent(t *testing.T) {
	dir := t.TempDir()

	if err := EnsureCodexSkillsAlias(dir); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := EnsureCodexSkillsAlias(dir); err != nil {
		t.Fatalf("second call: %v", err)
	}

	link := filepath.Join(dir, ".agents", "skills")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	wantTarget := filepath.Join("..", ".claude", "skills")
	if target != wantTarget {
		t.Errorf("target = %q, want %q", target, wantTarget)
	}
}

func TestEnsureCodexSkillsAlias_ReplacesEmptyDir(t *testing.T) {
	dir := t.TempDir()
	emptyDir := filepath.Join(dir, ".agents", "skills")
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := EnsureCodexSkillsAlias(dir); err != nil {
		t.Fatalf("EnsureCodexSkillsAlias: %v", err)
	}

	link := filepath.Join(dir, ".agents", "skills")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("not a symlink: %v", err)
	}
	wantTarget := filepath.Join("..", ".claude", "skills")
	if target != wantTarget {
		t.Errorf("target = %q, want %q", target, wantTarget)
	}
}

func TestCreateContextAliases_PreservesExistingRegularFile(t *testing.T) {
	dir := t.TempDir()
	agentsFile := filepath.Join(dir, "AGENTS.md")
	userContent := "# User-authored Codex instructions\nDo not overwrite me.\n"
	if err := os.WriteFile(agentsFile, []byte(userContent), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CreateContextAliases(dir, []string{"AGENTS.md"}); err != nil {
		t.Fatalf("CreateContextAliases failed: %v", err)
	}

	// 1. Backup file must exist and contain user's exact content.
	bakFile := filepath.Join(dir, "AGENTS.md.bak")
	bakData, err := os.ReadFile(bakFile)
	if err != nil {
		t.Fatalf("backup file not created: %v", err)
	}
	if string(bakData) != userContent {
		t.Errorf("backup content = %q, want %q", string(bakData), userContent)
	}

	// 2. AGENTS.md must now be a symlink to CLAUDE.md.
	target, err := os.Readlink(agentsFile)
	if err != nil {
		t.Fatalf("AGENTS.md is not a symlink: %v", err)
	}
	if target != "CLAUDE.md" {
		t.Errorf("AGENTS.md target = %q, want CLAUDE.md", target)
	}

	// 3. Second run must be a clean no-op and not alter the backup.
	if err := CreateContextAliases(dir, []string{"AGENTS.md"}); err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	bakDataAfter, err := os.ReadFile(bakFile)
	if err != nil || string(bakDataAfter) != userContent {
		t.Errorf("backup altered on second call")
	}
}

func TestCreateContextAliases_RefusesWhenBackupExists(t *testing.T) {
	dir := t.TempDir()
	agentsFile := filepath.Join(dir, "AGENTS.md")
	bakFile := filepath.Join(dir, "AGENTS.md.bak")
	os.WriteFile(agentsFile, []byte("active content"), 0o644)
	os.WriteFile(bakFile, []byte("existing backup"), 0o644)

	err := CreateContextAliases(dir, []string{"AGENTS.md"})
	if err == nil {
		t.Fatal("expected error when both regular file and backup exist")
	}

	// Active content must not have been overwritten or deleted.
	data, _ := os.ReadFile(agentsFile)
	if string(data) != "active content" {
		t.Errorf("active file content = %q, want 'active content'", string(data))
	}
}

func TestCreateContextAliases_RefusesDirectory(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "AGENTS.md")
	os.MkdirAll(agentsDir, 0o755)

	err := CreateContextAliases(dir, []string{"AGENTS.md"})
	if err == nil {
		t.Fatal("expected error when alias is a directory")
	}
}
