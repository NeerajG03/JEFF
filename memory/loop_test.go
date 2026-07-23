package memory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// loopTestRunner simulates the marlowe agent: it writes a canonical entry and
// returns a JSON report.
type loopTestRunner struct {
	home   string
	scope  string
	bucket string
	fm     CanonicalFrontmatter
	body   string
}

func (r *loopTestRunner) Run(_ context.Context, _ string, _ []string) (string, error) {
	entry, err := WriteCanonical(r.home, r.scope, r.bucket, r.fm, r.body)
	if err != nil {
		return "", err
	}
	_ = entry
	report := agentReport{Accepted: 1, Skipped: 0, Invalidated: 0, Flagged: []string{}}
	b, _ := json.Marshal(report)
	return string(b), nil
}

func TestLoop_EndToEnd(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	persona := "jenko"
	task := "gig-loop"

	// Step 1: Write a proposal (as 'jeff memory propose' would).
	proposalFM := Frontmatter{
		Name:        "async-boundary",
		Description: "Use top-level error boundaries",
		Type:        TypeFeedback,
	}
	proposal, err := WriteProposal(home, persona, task, proposalFM, "Don't wrap async in try/catch.\n")
	if err != nil {
		t.Fatalf("WriteProposal: %v", err)
	}
	if _, err := os.Stat(proposal.Path); err != nil {
		t.Fatalf("proposal file not found: %v", err)
	}

	// Step 2: Write a queue entry (as RunSessionEnd would).
	qe := SessionQueueEntry{
		Task:      task,
		Persona:   persona,
		Repos:     []string{"jeff"},
		Proposals: []string{"async-boundary"},
		EndedAt:   now,
	}
	qePath, err := WriteQueueEntry(home, qe)
	if err != nil {
		t.Fatalf("WriteQueueEntry: %v", err)
	}
	if _, err := os.Stat(qePath); err != nil {
		t.Fatalf("queue file not found: %v", err)
	}

	// Step 3: Curate using a mock runner that writes a canonical entry
	// (simulating marlowe's 'jeff memory add').
	canonicalFM := CanonicalFrontmatter{
		Frontmatter: Frontmatter{
			Name:        "async-boundary",
			Description: "Use top-level error boundaries",
			Type:        TypeFeedback,
		},
		Status:    "accepted",
		Scope:     "persona:" + persona,
		ValidFrom: now,
		Source:    Source{Persona: "marlowe", Task: task, Trigger: "curator"},
	}
	runner := &loopTestRunner{
		home:   home,
		scope:  "persona:" + persona,
		bucket: "procedural",
		fm:     canonicalFM,
		body:   "Don't wrap async in try/catch.\n",
	}

	report, err := Curate(CurateOptions{
		Home:         home,
		Persona:      persona,
		Runner:       runner,
		SkillContent: "",
	})
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}

	if report.Processed != 1 {
		t.Errorf("processed: got %d want 1", report.Processed)
	}
	if report.Accepted != 1 {
		t.Errorf("accepted: got %d want 1", report.Accepted)
	}

	// Step 4a: Canonical entry exists on disk.
	canonicalPath := filepath.Join(PersonaScopePath(home, persona), "procedural", "async-boundary.md")
	if _, err := os.Stat(canonicalPath); err != nil {
		t.Fatalf("canonical entry not found: %v", err)
	}

	// Step 4b: Queue entry has been archived (moved out of queue/sessions/).
	if _, err := os.Stat(qePath); err == nil {
		t.Error("queue entry should have been archived (removed from queue/sessions/)")
	} else if !os.IsNotExist(err) {
		t.Errorf("stat queue entry: %v", err)
	}

	// Step 4c: Proposal has been archived.
	if _, err := os.Stat(proposal.Path); err == nil {
		t.Error("proposal should have been archived")
	} else if !os.IsNotExist(err) {
		t.Errorf("stat proposal: %v", err)
	}

	// Step 4d: .last-curated stamp exists.
	stampPath := filepath.Join(MemoryRoot(home), ".last-curated")
	if _, err := os.Stat(stampPath); err != nil {
		t.Errorf(".last-curated stamp not found: %v", err)
	}

	// Step 4e: buildMemoryIndex includes the new entry.
	idx := buildMemoryIndex(home, persona, []string{"jeff"})
	if idx == "" {
		t.Fatal("memory index is empty after curation")
	}
	if !strings.Contains(idx, "async-boundary") {
		t.Errorf("memory index missing new entry 'async-boundary':\n%s", idx)
	}
	if !strings.Contains(idx, "Use top-level error boundaries") {
		t.Errorf("memory index missing description:\n%s", idx)
	}
}
