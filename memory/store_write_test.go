package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteCanonical_CreatesFile(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	fm := CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "no-try-catch", Description: "no wrapping async", Type: TypeFeedback},
		Status:      "accepted",
		Scope:       "repo:frontend",
		ValidFrom:   now,
		Source:      Source{Persona: "jenko", Task: "gig-123", Trigger: "user-correction"},
	}

	entry, err := WriteCanonical(home, "repo:frontend", "procedural", fm, "Use error boundaries.\n")
	if err != nil {
		t.Fatalf("WriteCanonical: %v", err)
	}

	if entry.Slug != "no-try-catch" {
		t.Errorf("slug: got %q want %q", entry.Slug, "no-try-catch")
	}
	if entry.Bucket != BucketProcedural {
		t.Errorf("bucket: got %q want %q", entry.Bucket, BucketProcedural)
	}
	if _, err := os.Stat(entry.Path); err != nil {
		t.Errorf("entry file not found: %v", err)
	}

	// Round-trip: ReadEntry should return the same data.
	got, err := ReadEntry(entry.Path)
	if err != nil {
		t.Fatalf("ReadEntry: %v", err)
	}
	if got.FM.Name != "no-try-catch" {
		t.Errorf("round-trip name: got %q", got.FM.Name)
	}
	if got.Body != "Use error boundaries.\n" {
		t.Errorf("round-trip body: got %q", got.Body)
	}
}

func TestWriteCanonical_CoreBucket(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	fm := CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "core", Description: "core preferences", Type: TypeUser},
		Source:      Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
	}

	entry, err := WriteCanonical(home, "persona:jenko", "core", fm, "Prefer British English.\n")
	if err != nil {
		t.Fatalf("WriteCanonical core: %v", err)
	}

	wantPath := filepath.Join(PersonaScopePath(home, "jenko"), "core.md")
	if entry.Path != wantPath {
		t.Errorf("core path: got %q want %q", entry.Path, wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("core.md not found: %v", err)
	}
}

func TestWriteCanonical_SetsDefaults(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	// Pass fm with no Status or ValidFrom — should be filled in.
	fm := CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "my-fact", Description: "a fact", Type: TypeProject},
		Source:      Source{Persona: "eric", Task: "t", Trigger: "self-noted"},
	}

	entry, err := WriteCanonical(home, "repo:backend", "semantic", fm, "The fact.\n")
	if err != nil {
		t.Fatalf("WriteCanonical: %v", err)
	}
	if entry.FM.Status != "accepted" {
		t.Errorf("default status: got %q want accepted", entry.FM.Status)
	}
	if entry.FM.ValidFrom.IsZero() {
		t.Error("ValidFrom should be set")
	}
	if entry.FM.Scope != "repo:backend" {
		t.Errorf("scope: got %q want repo:backend", entry.FM.Scope)
	}
}

func TestWriteCanonical_UnknownScope(t *testing.T) {
	home := t.TempDir()
	fm := CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "x", Description: "d", Type: TypeUser},
		Source:      Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
	}
	_, err := WriteCanonical(home, "badscope:foo", "semantic", fm, "body")
	if err == nil {
		t.Error("expected error for unknown scope")
	}
}

func TestWriteCanonical_DuplicateRefused(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	fm := CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "dup-test", Description: "duplicate test", Type: TypeFeedback},
		Status:      "accepted",
		Scope:       "persona:jenko",
		ValidFrom:   time.Now().UTC(),
		Source:      Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
	}

	// First write must succeed.
	_, err := WriteCanonical(home, "persona:jenko", "procedural", fm, "first body\n")
	if err != nil {
		t.Fatalf("first WriteCanonical: %v", err)
	}

	// Second write with same name must fail.
	_, err = WriteCanonical(home, "persona:jenko", "procedural", fm, "second body\n")
	if err == nil {
		t.Fatal("expected error on duplicate write")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention 'already exists', got: %v", err)
	}
	if !strings.Contains(err.Error(), "--supersede") {
		t.Errorf("error should mention --supersede, got: %v", err)
	}

	// Core bucket should still allow overwrite.
	fm.Name = "core"
	coreFM := CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "core", Description: "core overwrite test", Type: TypeProject},
		Status:      "accepted",
		Scope:       "persona:jenko",
		ValidFrom:   time.Now().UTC(),
		Source:      Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
	}
	if _, err := WriteCanonical(home, "persona:jenko", "core", coreFM, "core body\n"); err != nil {
		t.Fatalf("core bucket overwrite should work: %v", err)
	}
	// Second core write should also work (overwrite).
	if _, err := WriteCanonical(home, "persona:jenko", "core", coreFM, "core body updated\n"); err != nil {
		t.Fatalf("second core bucket overwrite should work: %v", err)
	}
}

func TestInvalidate_SetsValidTo(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	fm := CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "old-rule", Description: "d", Type: TypeFeedback},
		Status:      "accepted",
		Scope:       "persona:jenko",
		ValidFrom:   time.Now().UTC(),
		Source:      Source{Persona: "jenko", Task: "t", Trigger: "user-correction"},
	}
	entry, err := WriteCanonical(home, "persona:jenko", "procedural", fm, "Old body.\n")
	if err != nil {
		t.Fatalf("WriteCanonical: %v", err)
	}

	at := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := Invalidate(entry.Path, "new-rule", at); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}

	// Re-read and verify.
	updated, err := ReadEntry(entry.Path)
	if err != nil {
		t.Fatalf("ReadEntry after Invalidate: %v", err)
	}
	if updated.FM.ValidTo == nil {
		t.Fatal("ValidTo should be set after Invalidate")
	}
	if !updated.FM.ValidTo.Equal(at) {
		t.Errorf("ValidTo: got %v want %v", updated.FM.ValidTo, at)
	}
	if updated.FM.SupersededBy != "new-rule" {
		t.Errorf("SupersededBy: got %q want new-rule", updated.FM.SupersededBy)
	}
	if updated.FM.Status != "superseded" {
		t.Errorf("Status: got %q want superseded", updated.FM.Status)
	}
	// Body must be preserved.
	if updated.Body != "Old body.\n" {
		t.Errorf("body changed after Invalidate: got %q", updated.Body)
	}
}

func TestInvalidate_FileNotDeleted(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	fm := CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "keep-me", Description: "d", Type: TypeFeedback},
		Status:      "accepted",
		Scope:       "repo:gig",
		ValidFrom:   time.Now().UTC(),
		Source:      Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
	}
	entry, err := WriteCanonical(home, "repo:gig", "procedural", fm, "body\n")
	if err != nil {
		t.Fatal(err)
	}

	at := time.Now().UTC()
	if err := Invalidate(entry.Path, "", at); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(entry.Path); err != nil {
		t.Errorf("file must not be deleted: %v", err)
	}
}

func TestSupersede_WritesNewAndInvalidatesOld(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	// Write old entry.
	oldFM := CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "log-format", Description: "logs are JSON", Type: TypeFeedback},
		Status:      "accepted",
		Scope:       "persona:schmidt",
		ValidFrom:   time.Now().UTC(),
		Source:      Source{Persona: "schmidt", Task: "t", Trigger: "self-noted"},
	}
	oldEntry, err := WriteCanonical(home, "persona:schmidt", "procedural", oldFM, "JSON in prod.\n")
	if err != nil {
		t.Fatalf("WriteCanonical old: %v", err)
	}

	// Supersede with new entry.
	newFM := CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "log-format-v2", Description: "logs: JSON prod, plain staging", Type: TypeFeedback},
		Status:      "accepted",
		Scope:       "persona:schmidt",
		ValidFrom:   time.Now().UTC(),
		Source:      Source{Persona: "schmidt", Task: "t2", Trigger: "user-correction"},
	}
	newEntry, err := Supersede(home, oldEntry.Path, newFM, "JSON in prod, plaintext in staging.\n")
	if err != nil {
		t.Fatalf("Supersede: %v", err)
	}

	// New entry must reference old.
	if len(newEntry.FM.Supersedes) == 0 || newEntry.FM.Supersedes[0] != "log-format" {
		t.Errorf("Supersedes: got %v want [log-format]", newEntry.FM.Supersedes)
	}

	// Old entry must be invalidated.
	oldUpdated, err := ReadEntry(oldEntry.Path)
	if err != nil {
		t.Fatalf("ReadEntry old: %v", err)
	}
	if oldUpdated.FM.ValidTo == nil {
		t.Error("old entry ValidTo must be set")
	}
	if oldUpdated.FM.SupersededBy != "log-format-v2" {
		t.Errorf("old SupersededBy: got %q want log-format-v2", oldUpdated.FM.SupersededBy)
	}
	if oldUpdated.FM.Status != "superseded" {
		t.Errorf("old Status: got %q want superseded", oldUpdated.FM.Status)
	}

	// Both files must exist on disk (audit trail).
	if _, err := os.Stat(oldEntry.Path); err != nil {
		t.Error("old entry file must exist (audit trail)")
	}
	if _, err := os.Stat(newEntry.Path); err != nil {
		t.Error("new entry file must exist")
	}
}

func TestUpdateIndex_GeneratesTable(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	for _, name := range []string{"alpha", "beta", "gamma"} {
		fm := CanonicalFrontmatter{
			Frontmatter: Frontmatter{Name: name, Description: "desc " + name, Type: TypeFeedback},
			Status:      "accepted",
			Scope:       "persona:jenko",
			ValidFrom:   now,
			Source:      Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
		}
		if _, err := WriteCanonical(home, "persona:jenko", "procedural", fm, "body\n"); err != nil {
			t.Fatalf("WriteCanonical %s: %v", name, err)
		}
	}

	if err := UpdateIndex(home, "persona:jenko", "procedural"); err != nil {
		t.Fatalf("UpdateIndex: %v", err)
	}

	indexPath := filepath.Join(PersonaScopePath(home, "jenko"), "procedural", "INDEX.md")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read INDEX.md: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "alpha") {
		t.Error("INDEX.md missing alpha")
	}
	if !strings.Contains(content, "beta") {
		t.Error("INDEX.md missing beta")
	}
	if !strings.Contains(content, "gamma") {
		t.Error("INDEX.md missing gamma")
	}
	if !strings.HasPrefix(content, "# INDEX") {
		t.Errorf("INDEX.md should start with '# INDEX', got: %q", content[:50])
	}
}

func TestUpdateIndex_CoreBucketIsNoop(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}
	// UpdateIndex for core should not error.
	if err := UpdateIndex(home, "persona:jenko", "core"); err != nil {
		t.Errorf("UpdateIndex core: %v", err)
	}
}
