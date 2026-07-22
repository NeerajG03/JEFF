package memory_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NeerajG03/JEFF/memory"
)

func TestUpdateOnPartialState(t *testing.T) {
	home := bootstrapHome(t)

	// Create a partial layout: memory root only, no marlowe goal, no skill.
	if err := os.MkdirAll(filepath.Join(home, "memory"), 0o755); err != nil {
		t.Fatal(err)
	}

	report, err := memory.Update(home)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Marlowe GOAL.md should have been created.
	goalPath := filepath.Join(home, "personas", "marlowe", "GOAL.md")
	assertFile(t, home, "personas/marlowe/GOAL.md")

	// Created list should contain the goal path.
	if !containsPath(report.Created, goalPath) {
		t.Errorf("UpdateReport.Created missing %s; got %v", goalPath, report.Created)
	}

	// Curation skill should have been created.
	assertFile(t, home, ".skills/curation/SKILL.md")
}

func TestUpdateOnCompleteState(t *testing.T) {
	home := bootstrapHome(t)

	// Full initialize first.
	if err := memory.Initialize(home); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	report, err := memory.Update(home)
	if err != nil {
		t.Fatalf("Update after full init: %v", err)
	}

	// Everything should be skipped, nothing created.
	if len(report.Created) != 0 {
		t.Errorf("expected no new creations on complete state; got %v", report.Created)
	}
}

func TestUpdatePreservesUserEditedFiles(t *testing.T) {
	home := bootstrapHome(t)

	// Initialize first so all files exist.
	if err := memory.Initialize(home); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Simulate user editing GOAL.md.
	goalPath := filepath.Join(home, "personas", "marlowe", "GOAL.md")
	userContent := "# My custom marlowe goal\n\nUser-edited content.\n"
	if err := os.WriteFile(goalPath, []byte(userContent), 0o644); err != nil {
		t.Fatalf("write user goal: %v", err)
	}

	// Update should not overwrite user edits.
	if _, err := memory.Update(home); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// GOAL.md should still have user content.
	data, err := os.ReadFile(goalPath)
	if err != nil {
		t.Fatalf("read goal: %v", err)
	}
	if string(data) != userContent {
		t.Errorf("GOAL.md was overwritten; want user content, got %q", string(data))
	}
}

func TestUpdateDetectsOldLayout(t *testing.T) {
	home := bootstrapHome(t)

	// Create legacy layout.
	legacyMemDir := filepath.Join(home, "personas", "jenko", "memory")
	if err := os.MkdirAll(legacyMemDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyMemDir, "MEMORY.md"), []byte("# Jenko Memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := memory.Update(home)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if len(report.Migrations) == 0 {
		t.Error("expected migration hints for old layout; got none")
	}
}

// containsPath reports whether any element of paths has the given suffix.
func containsPath(paths []string, target string) bool {
	for _, p := range paths {
		if p == target {
			return true
		}
	}
	return false
}
