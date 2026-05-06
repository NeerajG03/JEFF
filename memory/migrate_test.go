package memory_test

import (
	"os"
	"path/filepath"
	"strings"
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

// TestMigrateLegacyFrontmatter exercises the regression where legacy entries
// had `source: <string>` and `updated: <date>` — valid YAML, but conflicting
// with the canonical schema (where `source` is a struct). After migration,
// ListEntries (which reads canonical frontmatter) must succeed.
func TestMigrateLegacyFrontmatter(t *testing.T) {
	home := bootstrapHome(t)
	if err := memory.EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	// Legacy persona memory with the bug-inducing frontmatter shape.
	legacyMem := filepath.Join(home, "personas", "hardy", "memory")
	if err := os.MkdirAll(legacyMem, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyMem, "MEMORY.md"),
		[]byte("# Hardy Memory\n\n- [pr-review-workflow](pr-review-workflow.md) — Use `gh pr review` directly\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Detail file with the legacy schema that broke the previous migration.
	legacyBody := `---
source: gig-2028
updated: 2026-04-07
---
When reviewing PRs, use ` + "`gh pr review`" + ` directly.
`
	if err := os.WriteFile(filepath.Join(legacyMem, "pr-review-workflow.md"), []byte(legacyBody), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := memory.Migrate(home, false)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(report.Errors) != 0 {
		t.Fatalf("Migrate errors: %v", report.Errors)
	}

	// The migrated file must be readable as a canonical entry — this is the
	// behavior that was broken before the fix.
	entries, err := memory.ListEntries(home, memory.EntryFilter{Persona: "hardy"})
	if err != nil {
		t.Fatalf("ListEntries failed (regression — legacy frontmatter not rewritten): %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected ≥1 migrated entry under persona:hardy; got none")
	}

	// Find the migrated entry and assert canonical fields are populated.
	var found *memory.Entry
	for i := range entries {
		if entries[i].Slug == "pr-review-workflow" {
			found = &entries[i]
			break
		}
	}
	if found == nil {
		t.Fatal("pr-review-workflow not found in ListEntries result")
	}
	if found.FM.Scope != "persona:hardy" {
		t.Errorf("scope: want persona:hardy, got %q", found.FM.Scope)
	}
	if found.FM.Status != "accepted" {
		t.Errorf("status: want accepted, got %q", found.FM.Status)
	}
	if found.FM.Provenance != "review-required" {
		t.Errorf("provenance: want review-required, got %q", found.FM.Provenance)
	}
	if found.FM.Source.Persona != "hardy" {
		t.Errorf("source.persona: want hardy, got %q", found.FM.Source.Persona)
	}
	if found.FM.Source.Task != "gig-2028" {
		t.Errorf("source.task: want gig-2028 (mapped from legacy `source` scalar), got %q", found.FM.Source.Task)
	}
	if found.FM.Source.Trigger != "migration" {
		t.Errorf("source.trigger: want migration, got %q", found.FM.Source.Trigger)
	}
	// Description should come from the INDEX.md entry line.
	if !strings.Contains(found.FM.Description, "gh pr review") {
		t.Errorf("description should be lifted from INDEX.md; got %q", found.FM.Description)
	}
	// valid_from should be parsed from legacy `updated`, not "now".
	wantYear := 2026
	if found.FM.ValidFrom.Year() != wantYear || found.FM.ValidFrom.Month() != 4 {
		t.Errorf("valid_from should map from legacy `updated: 2026-04-07`; got %v", found.FM.ValidFrom)
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
