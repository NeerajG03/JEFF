package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestListEntries_Filtering(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	mkCanonical := func(scope string, bucket Bucket, fm CanonicalFrontmatter) {
		if _, err := WriteCanonical(home, scope, string(bucket), fm, "body\n"); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)

	mkCanonical("persona:jenko", BucketProcedural, CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "rule-a", Description: "d", Type: TypeFeedback},
		Status:      "accepted", Scope: "persona:jenko", ValidFrom: now,
		Source: Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
	})
	mkCanonical("persona:jenko", BucketSemantic, CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "fact-1", Description: "d", Type: TypeReference},
		Status:      "superseded", Scope: "persona:jenko", ValidFrom: now,
		Source: Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
	})
	mkCanonical("persona:jenko", BucketCore, CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "core", Description: "core block", Type: TypeProject},
		Status:      "accepted", Scope: "persona:jenko", ValidFrom: now,
		Source: Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
	})
	mkCanonical("repo:gig", BucketProcedural, CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "use-sdk", Description: "d", Type: TypeFeedback},
		Status:      "accepted", Scope: "repo:gig", ValidFrom: now,
		Source: Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
	})

	// Index files in bucket dirs must be ignored.
	jenScope := PersonaScopePath(home, "jenko")
	if err := os.WriteFile(filepath.Join(jenScope, "procedural", "INDEX.md"), []byte("# index\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// All
	all, err := ListEntries(home, EntryFilter{})
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("want 4 entries, got %d: %+v", len(all), all)
	}

	// Persona filter
	jen, err := ListEntries(home, EntryFilter{Persona: "jenko"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jen) != 3 {
		t.Errorf("persona=jenko: want 3, got %d", len(jen))
	}

	// Bucket filter
	proc, err := ListEntries(home, EntryFilter{Bucket: BucketProcedural})
	if err != nil {
		t.Fatal(err)
	}
	if len(proc) != 2 {
		t.Errorf("bucket=procedural: want 2, got %d", len(proc))
	}

	// Status filter
	acc, err := ListEntries(home, EntryFilter{Status: "accepted"})
	if err != nil {
		t.Fatal(err)
	}
	if len(acc) != 3 {
		t.Errorf("status=accepted: want 3, got %d", len(acc))
	}

	// Combined: persona + bucket
	combined, err := ListEntries(home, EntryFilter{Persona: "jenko", Bucket: BucketSemantic})
	if err != nil {
		t.Fatal(err)
	}
	if len(combined) != 1 || combined[0].Slug != "fact-1" {
		t.Errorf("persona=jenko bucket=semantic: want fact-1, got %+v", combined)
	}
}

func TestReadEntry_StandaloneFile(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	fm := CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "x", Description: "d", Type: TypeUser},
		Status:      "accepted", Scope: "persona:jenko", ValidFrom: now,
		Source: Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
	}
	entry, err := WriteCanonical(home, "persona:jenko", string(BucketSemantic), fm, "body\n")
	if err != nil {
		t.Fatalf("WriteCanonical: %v", err)
	}

	got, err := ReadEntry(entry.Path)
	if err != nil {
		t.Fatalf("ReadEntry: %v", err)
	}
	if got.Slug != "x" || got.FM.Name != "x" || got.Body != "body\n" {
		t.Errorf("ReadEntry mismatch: %+v", got)
	}
}

func TestListEntries_EmptyHome(t *testing.T) {
	home := t.TempDir() // no EnsureLayout
	out, err := ListEntries(home, EntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("want 0, got %d", len(out))
	}
}

func TestListEntries_SkipsCorruptEntry(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	// Write one valid entry.
	now := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	fm := CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "valid", Description: "d", Type: TypeFeedback},
		Status:      "accepted", Scope: "persona:jenko", ValidFrom: now,
		Source: Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
	}
	if _, err := WriteCanonical(home, "persona:jenko", string(BucketProcedural), fm, "body\n"); err != nil {
		t.Fatal(err)
	}

	// Write a corrupt file (no frontmatter fence).
	corruptDir := BucketPath(PersonaScopePath(home, "jenko"), BucketProcedural)
	corruptPath := filepath.Join(corruptDir, "corrupt.md")
	os.WriteFile(corruptPath, []byte("not valid frontmatter\n"), 0o644)

	// ListEntries must skip the corrupt file and return the valid one.
	entries, err := ListEntries(home, EntryFilter{})
	if err != nil {
		t.Fatalf("ListEntries should not error on corrupt file: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Slug == "valid" {
			found = true
		}
		if e.Slug == "corrupt" {
			t.Error("corrupt entry should not appear in results")
		}
	}
	if !found {
		t.Error("valid entry should still be returned despite corrupt file")
	}
}
