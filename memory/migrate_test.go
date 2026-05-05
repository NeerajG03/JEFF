package memory_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NeerajG03/JEFF/memory"
)

func setupLegacyLayout(t *testing.T, home string) {
	t.Helper()

	// Legacy persona memory: personas/jenko/memory/MEMORY.md + detail file.
	legacyMem := filepath.Join(home, "personas", "jenko", "memory")
	if err := os.MkdirAll(legacyMem, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyMem, "MEMORY.md"), []byte("# Jenko Memory\n\n- [async-style](async-style.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyMem, "async-style.md"), []byte("---\nname: async-style\ndescription: Use top-level boundaries\ntype: feedback\n---\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Legacy repo learnings: learnings/jeff/INDEX.md + detail file.
	legacyLearn := filepath.Join(home, "learnings", "jeff")
	if err := os.MkdirAll(legacyLearn, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyLearn, "INDEX.md"), []byte("# Jeff Learnings\n\n- [tmux-quirk](tmux-quirk.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyLearn, "tmux-quirk.md"), []byte("Dots in task IDs break tmux targets.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateDryRun(t *testing.T) {
	home := bootstrapHome(t)
	setupLegacyLayout(t, home)

	report, err := memory.Migrate(home, true)
	if err != nil {
		t.Fatalf("Migrate dry-run: %v", err)
	}

	if len(report.Moved) == 0 {
		t.Error("dry-run: expected Moved entries in report; got none")
	}
	if len(report.Errors) != 0 {
		t.Errorf("dry-run: unexpected errors: %v", report.Errors)
	}

	// Dry-run must NOT have written any files.
	newPath := filepath.Join(home, "memory", "personas", "jenko", "semantic", "INDEX.md")
	if _, err := os.Stat(newPath); err == nil {
		t.Errorf("dry-run: wrote %s but should not have", newPath)
	}

	// Dry-run must NOT have moved/deleted original files.
	origPath := filepath.Join(home, "personas", "jenko", "memory", "MEMORY.md")
	if _, err := os.Stat(origPath); err != nil {
		t.Errorf("dry-run: original file %s was removed", origPath)
	}
}

func TestMigratePersonaMemory(t *testing.T) {
	home := bootstrapHome(t)
	// Ensure memory layout exists so destination dirs can be created.
	if err := memory.EnsureLayout(home); err != nil {
		t.Fatal(err)
	}
	setupLegacyLayout(t, home)

	report, err := memory.Migrate(home, false)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(report.Errors) != 0 {
		t.Errorf("Migrate errors: %v", report.Errors)
	}

	// MEMORY.md → INDEX.md in new semantic bucket.
	assertFile(t, home, "memory/personas/jenko/semantic/INDEX.md")
	// Detail file preserved.
	assertFile(t, home, "memory/personas/jenko/semantic/async-style.md")

	// Original files moved to archive.
	archiveGlob := filepath.Join(home, "archive", "migration-*", "personas", "jenko", "memory", "MEMORY.md")
	matches, _ := filepath.Glob(archiveGlob)
	if len(matches) == 0 {
		t.Errorf("MEMORY.md not archived; expected match for %s", archiveGlob)
	}

	// Original location should no longer have MEMORY.md.
	origPath := filepath.Join(home, "personas", "jenko", "memory", "MEMORY.md")
	if _, err := os.Stat(origPath); err == nil {
		t.Errorf("original file %s still exists after migration", origPath)
	}
}

func TestMigrateRepoLearnings(t *testing.T) {
	home := bootstrapHome(t)
	if err := memory.EnsureLayout(home); err != nil {
		t.Fatal(err)
	}
	setupLegacyLayout(t, home)

	report, err := memory.Migrate(home, false)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(report.Errors) != 0 {
		t.Errorf("Migrate errors: %v", report.Errors)
	}

	// INDEX.md stays INDEX.md in new tree.
	assertFile(t, home, "memory/repos/jeff/semantic/INDEX.md")
	// Detail file (no frontmatter) gets wrapped.
	newDetail := filepath.Join(home, "memory", "repos", "jeff", "semantic", "tmux-quirk.md")
	data, err := os.ReadFile(newDetail)
	if err != nil {
		t.Fatalf("read migrated detail: %v", err)
	}
	if string(data[:3]) != "---" {
		t.Errorf("migrated detail missing frontmatter; content starts: %q", string(data[:20]))
	}
}

func TestMigrateReport(t *testing.T) {
	home := bootstrapHome(t)
	if err := memory.EnsureLayout(home); err != nil {
		t.Fatal(err)
	}
	setupLegacyLayout(t, home)

	report, err := memory.Migrate(home, false)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Should have at least 4 moved entries: MEMORY.md, async-style.md, INDEX.md, tmux-quirk.md
	if len(report.Moved) < 4 {
		t.Errorf("expected at least 4 Moved entries; got %d: %v", len(report.Moved), report.Moved)
	}
}

func TestMigrateEmptyHome(t *testing.T) {
	home := bootstrapHome(t)

	// No old layout — should be a no-op.
	report, err := memory.Migrate(home, false)
	if err != nil {
		t.Fatalf("Migrate on empty home: %v", err)
	}
	if len(report.Moved) != 0 {
		t.Errorf("expected empty Moved for home with no old layout; got %v", report.Moved)
	}
	if len(report.Errors) != 0 {
		t.Errorf("unexpected errors: %v", report.Errors)
	}
}
