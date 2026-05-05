package memory

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mem "github.com/NeerajG03/JEFF/memory"
)

func seedStatusHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if err := mem.EnsureLayout(home); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	// Seed accepted entries.
	jenScope := mem.PersonaScopePath(home, "jenko")
	bucketDir := mem.BucketPath(jenScope, mem.BucketProcedural)
	if err := os.MkdirAll(bucketDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"rule-a", "rule-b"} {
		fp := filepath.Join(bucketDir, slug+".md")
		f, err := os.Create(fp)
		if err != nil {
			t.Fatal(err)
		}
		fm := mem.CanonicalFrontmatter{
			Frontmatter: mem.Frontmatter{Name: slug, Description: "d", Type: mem.TypeFeedback},
			Status:      "accepted", Scope: "persona:jenko", ValidFrom: now,
			Source: mem.Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
		}
		if err := mem.WriteCanonical(f, fm, "body\n"); err != nil {
			f.Close()
			t.Fatal(err)
		}
		f.Close()
	}

	// Seed a superseded entry.
	supBucketDir := mem.BucketPath(jenScope, mem.BucketSemantic)
	if err := os.MkdirAll(supBucketDir, 0o755); err != nil {
		t.Fatal(err)
	}
	supPath := filepath.Join(supBucketDir, "old-fact.md")
	f, err := os.Create(supPath)
	if err != nil {
		t.Fatal(err)
	}
	fm := mem.CanonicalFrontmatter{
		Frontmatter: mem.Frontmatter{Name: "old-fact", Description: "d", Type: mem.TypeProject},
		Status:      "superseded", Scope: "persona:jenko", ValidFrom: now,
		Source: mem.Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
	}
	if err := mem.WriteCanonical(f, fm, "body\n"); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	// Seed a flagged entry.
	flagBucketDir := mem.BucketPath(mem.RepoScopePath(home, "jeff"), mem.BucketSemantic)
	if err := os.MkdirAll(flagBucketDir, 0o755); err != nil {
		t.Fatal(err)
	}
	flagPath := filepath.Join(flagBucketDir, "needs-review.md")
	ff, err := os.Create(flagPath)
	if err != nil {
		t.Fatal(err)
	}
	ffm := mem.CanonicalFrontmatter{
		Frontmatter: mem.Frontmatter{Name: "needs-review", Description: "d", Type: mem.TypeReference},
		Status:      "accepted", Scope: "repo:jeff", ValidFrom: now,
		Provenance:  "review-required",
		Source:      mem.Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
	}
	if err := mem.WriteCanonical(ff, ffm, "body\n"); err != nil {
		ff.Close()
		t.Fatal(err)
	}
	ff.Close()

	// Seed queue entries.
	for i := 0; i < 3; i++ {
		_, err := mem.WriteQueueEntry(home, mem.SessionQueueEntry{
			Task:    "gig-100",
			Persona: "jenko",
			EndedAt: now.Add(time.Duration(i) * time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Seed a proposal.
	_, err = mem.WriteProposal(home, "jenko", "gig-100",
		mem.Frontmatter{Name: "my-proposal", Description: "d", Type: mem.TypeFeedback},
		"body",
	)
	if err != nil {
		t.Fatal(err)
	}

	return home
}

func TestRunStatus_Human(t *testing.T) {
	home := seedStatusHome(t)
	var buf bytes.Buffer
	if err := runStatus(&buf, home, false); err != nil {
		t.Fatalf("runStatus: %v", err)
	}
	out := buf.String()
	t.Log(out)

	if !strings.Contains(out, "Queue:") {
		t.Error("expected Queue: line")
	}
	if !strings.Contains(out, "3 sessions") {
		t.Errorf("expected 3 queue sessions, got:\n%s", out)
	}
	if !strings.Contains(out, "Canonical:") {
		t.Error("expected Canonical: line")
	}
	// 2 accepted persona entries + 1 accepted repo entry = 3 canonical
	if !strings.Contains(out, "3 entries") {
		t.Errorf("expected 3 canonical entries, got:\n%s", out)
	}
	if !strings.Contains(out, "Superseded:") {
		t.Error("expected Superseded: line")
	}
	if !strings.Contains(out, "Flagged:") {
		t.Error("expected Flagged: line")
	}
}

func TestRunStatus_JSON(t *testing.T) {
	home := seedStatusHome(t)
	var buf bytes.Buffer
	if err := runStatus(&buf, home, true); err != nil {
		t.Fatalf("runStatus JSON: %v", err)
	}
	var r statusResult
	if err := json.Unmarshal(buf.Bytes(), &r); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	if r.QueueDepth != 3 {
		t.Errorf("want queue_depth=3, got %d", r.QueueDepth)
	}
	if r.Proposals != 1 {
		t.Errorf("want proposals_pending=1, got %d", r.Proposals)
	}
	if r.CanonicalTotal != 3 {
		t.Errorf("want canonical_total=3, got %d", r.CanonicalTotal)
	}
	if r.ByScope.Persona != 2 {
		t.Errorf("want by_scope.persona=2, got %d", r.ByScope.Persona)
	}
	if r.ByScope.Repo != 1 {
		t.Errorf("want by_scope.repo=1, got %d", r.ByScope.Repo)
	}
	if r.Superseded != 1 {
		t.Errorf("want superseded=1, got %d", r.Superseded)
	}
	if r.Flagged != 1 {
		t.Errorf("want flagged=1, got %d", r.Flagged)
	}
}

func TestRunStatus_Empty(t *testing.T) {
	home := t.TempDir()
	if err := mem.EnsureLayout(home); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := runStatus(&buf, home, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "0 sessions") {
		t.Errorf("expected empty queue, got:\n%s", out)
	}
}

func TestRunStatus_LastCurated(t *testing.T) {
	home := t.TempDir()
	if err := mem.EnsureLayout(home); err != nil {
		t.Fatal(err)
	}
	// Write a .last-curated marker.
	ts := time.Date(2026, 5, 1, 18, 30, 0, 0, time.UTC)
	markerPath := filepath.Join(mem.MemoryRoot(home), ".last-curated")
	if err := os.WriteFile(markerPath, []byte(ts.Format(time.RFC3339)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := runStatus(&buf, home, true); err != nil {
		t.Fatal(err)
	}
	var r statusResult
	if err := json.Unmarshal(buf.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r.LastCurate == "" {
		t.Error("expected last_curate to be set")
	}
}
