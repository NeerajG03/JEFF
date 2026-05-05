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

	mkCanonical := func(scopePath string, bucket Bucket, slug string, fm CanonicalFrontmatter) {
		var dir string
		var fp string
		if bucket == BucketCore {
			fp = filepath.Join(scopePath, "core.md")
			dir = scopePath
		} else {
			dir = BucketPath(scopePath, bucket)
			fp = filepath.Join(dir, slug+".md")
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(fp)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := WriteCanonical(f, fm, "body\n"); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)

	jenScope := PersonaScopePath(home, "jenko")
	mkCanonical(jenScope, BucketProcedural, "rule-a", CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "rule-a", Description: "d", Type: TypeFeedback},
		Status:      "accepted", Scope: "persona:jenko", ValidFrom: now,
		Source: Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
	})
	mkCanonical(jenScope, BucketSemantic, "fact-1", CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "fact-1", Description: "d", Type: TypeReference},
		Status:      "superseded", Scope: "persona:jenko", ValidFrom: now,
		Source: Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
	})
	mkCanonical(jenScope, BucketCore, "core", CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "core", Description: "core block", Type: TypeProject},
		Status:      "accepted", Scope: "persona:jenko", ValidFrom: now,
		Source: Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
	})

	repoScope := RepoScopePath(home, "gig")
	mkCanonical(repoScope, BucketProcedural, "use-sdk", CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "use-sdk", Description: "d", Type: TypeFeedback},
		Status:      "accepted", Scope: "repo:gig", ValidFrom: now,
		Source: Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
	})

	// Index files in bucket dirs must be ignored.
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
	scope := PersonaScopePath(home, "jenko")
	if err := os.MkdirAll(BucketPath(scope, BucketSemantic), 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	fm := CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "x", Description: "d", Type: TypeUser},
		Status:      "accepted", Scope: "persona:jenko", ValidFrom: now,
		Source: Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
	}
	path := filepath.Join(BucketPath(scope, BucketSemantic), "x.md")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCanonical(f, fm, "body\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	got, err := ReadEntry(path)
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
