package memory

import (
	"bytes"
	"strings"
	"testing"
	"time"

	mem "github.com/NeerajG03/JEFF/memory"
)

// seedDiffHome creates a supersedes chain:
//
//	v1 (superseded, superseded_by=v2) → v2 (accepted, supersedes=[v1])
func seedDiffHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if err := mem.EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	v1Time := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	v2Time := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)

	// v1: superseded
	validTo := v2Time
	fm1 := mem.CanonicalFrontmatter{
		Frontmatter:  mem.Frontmatter{Name: "async-error-handling-v1", Description: "Don't use try/catch in async code", Type: mem.TypeFeedback},
		Status:       "superseded",
		Scope:        "persona:jenko",
		ValidFrom:    v1Time,
		ValidTo:      &validTo,
		SupersededBy: "async-error-handling",
		Source:       mem.Source{Persona: "jenko", Task: "t1", Trigger: "user-correction"},
	}
	if _, err := mem.WriteCanonical(home, "persona:jenko", "procedural", fm1, "Original rule — scope too broad.\n"); err != nil {
		t.Fatal(err)
	}

	// v2: accepted
	fm2 := mem.CanonicalFrontmatter{
		Frontmatter: mem.Frontmatter{Name: "async-error-handling", Description: "Don't wrap async in try/catch — repo uses top-level boundaries", Type: mem.TypeFeedback},
		Status:      "accepted",
		Scope:       "persona:jenko",
		ValidFrom:   v2Time,
		Supersedes:  []string{"async-error-handling-v1"},
		Source:      mem.Source{Persona: "jenko", Task: "t2", Trigger: "user-correction"},
	}
	if _, err := mem.WriteCanonical(home, "persona:jenko", "procedural", fm2, "Refined rule with correct scope.\n"); err != nil {
		t.Fatal(err)
	}

	return home
}

func TestRunDiff_Chain(t *testing.T) {
	home := seedDiffHome(t)
	var buf bytes.Buffer
	if err := runDiff(&buf, home, "async-error-handling"); err != nil {
		t.Fatalf("runDiff: %v", err)
	}
	out := buf.String()
	t.Log(out)

	// Both versions should appear.
	if !strings.Contains(out, "v1") {
		t.Error("expected v1 in diff output")
	}
	if !strings.Contains(out, "v2") {
		t.Error("expected v2 in diff output")
	}
	// v1 should appear before v2 (sorted by valid_from).
	v1Pos := strings.Index(out, "v1")
	v2Pos := strings.Index(out, "v2")
	if v1Pos > v2Pos {
		t.Error("v1 should appear before v2 in timeline")
	}
	// v2 should be marked as current.
	if !strings.Contains(out, "current") {
		t.Error("expected current marker on accepted entry")
	}
}

func TestRunDiff_StartFromOldVersion(t *testing.T) {
	home := seedDiffHome(t)
	var buf bytes.Buffer
	// Starting from the old slug should find the full chain.
	if err := runDiff(&buf, home, "async-error-handling-v1"); err != nil {
		t.Fatalf("runDiff from v1: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "v1") || !strings.Contains(out, "v2") {
		t.Error("expected full chain when starting from old version")
	}
}

func TestRunDiff_NotFound(t *testing.T) {
	home := seedDiffHome(t)
	var buf bytes.Buffer
	err := runDiff(&buf, home, "no-such-entry")
	if err == nil {
		t.Error("expected error for nonexistent entry")
	}
	if !strings.Contains(err.Error(), "no entry found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildDiffChain_SingleEntry(t *testing.T) {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	entries := []mem.Entry{
		{
			Slug: "standalone",
			FM: mem.CanonicalFrontmatter{
				Frontmatter: mem.Frontmatter{Name: "standalone"},
				Status:      "accepted",
				ValidFrom:   now,
			},
		},
	}
	chain := buildDiffChain(entries, "standalone")
	if len(chain) != 1 {
		t.Errorf("want 1 entry in chain, got %d", len(chain))
	}
}

func TestBuildDiffChain_Order(t *testing.T) {
	t1 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	entries := []mem.Entry{
		{
			Slug: "new",
			FM: mem.CanonicalFrontmatter{
				Frontmatter: mem.Frontmatter{Name: "new"},
				Status:      "accepted",
				ValidFrom:   t2,
				Supersedes:  []string{"old"},
			},
		},
		{
			Slug: "old",
			FM: mem.CanonicalFrontmatter{
				Frontmatter:  mem.Frontmatter{Name: "old"},
				Status:       "superseded",
				ValidFrom:    t1,
				SupersededBy: "new",
			},
		},
	}
	chain := buildDiffChain(entries, "new")
	if len(chain) != 2 {
		t.Fatalf("want 2 entries, got %d", len(chain))
	}
	if chain[0].Slug != "old" || chain[1].Slug != "new" {
		t.Errorf("expected [old, new] order, got [%s, %s]", chain[0].Slug, chain[1].Slug)
	}
}
