package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndReadProposal(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	fm := Frontmatter{Name: "no-mock-db", Description: "real db", Type: TypeFeedback}
	body := "Why: prior incident.\n"

	p, err := WriteProposal(home, "jenko", "gig-1d33.1", fm, body)
	if err != nil {
		t.Fatalf("WriteProposal: %v", err)
	}
	if p.Slug != "no-mock-db" || p.Persona != "jenko" || p.Task != "gig-1d33.1" {
		t.Errorf("proposal fields: %+v", p)
	}
	if _, err := os.Stat(p.Path); err != nil {
		t.Fatalf("proposal not on disk: %v", err)
	}

	read, err := ReadProposal(p.Path)
	if err != nil {
		t.Fatalf("ReadProposal: %v", err)
	}
	if read.FM != fm || read.Body != body {
		t.Errorf("round-trip mismatch:\n got fm=%+v body=%q\nwant fm=%+v body=%q", read.FM, read.Body, fm, body)
	}
	if read.Persona != "jenko" || read.Task != "gig-1d33.1" {
		t.Errorf("persona/task not derived from path: %+v", read)
	}
}

func TestListProposals_Filtering(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	mk := func(persona, task, name string) {
		fm := Frontmatter{Name: name, Description: "d", Type: TypeUser}
		if _, err := WriteProposal(home, persona, task, fm, "body\n"); err != nil {
			t.Fatal(err)
		}
	}
	mk("jenko", "g-1", "rule-a")
	mk("jenko", "g-1", "rule-b")
	mk("jenko", "g-2", "rule-c")
	mk("schmidt", "g-1", "rule-d")

	// All
	all, err := ListProposals(home, "", "")
	if err != nil || len(all) != 4 {
		t.Fatalf("all: got %d entries err=%v", len(all), err)
	}
	// By persona
	jen, err := ListProposals(home, "jenko", "")
	if err != nil || len(jen) != 3 {
		t.Fatalf("jenko: got %d entries err=%v", len(jen), err)
	}
	// By persona+task
	jen1, err := ListProposals(home, "jenko", "g-1")
	if err != nil || len(jen1) != 2 {
		t.Fatalf("jenko g-1: got %d entries err=%v", len(jen1), err)
	}
}

func TestListProposals_MissingDirOK(t *testing.T) {
	home := t.TempDir() // EnsureLayout NOT called
	out, err := ListProposals(home, "", "")
	if err != nil {
		t.Fatalf("missing dir should not error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("missing dir should produce no entries, got %d", len(out))
	}
}

func TestWriteProposal_Validation(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteProposal(home, "", "t", Frontmatter{Name: "x", Type: TypeUser}, ""); err == nil {
		t.Error("missing persona should error")
	}
	if _, err := WriteProposal(home, "p", "", Frontmatter{Name: "x", Type: TypeUser}, ""); err == nil {
		t.Error("missing task should error")
	}
	if _, err := WriteProposal(home, "p", "t", Frontmatter{Type: TypeUser}, ""); err == nil {
		t.Error("missing name should error")
	}
	if _, err := WriteProposal(home, "p", "t", Frontmatter{Name: "x", Type: MemoryType("bad")}, ""); err == nil {
		t.Error("bad type should error")
	}

	// Sanity: file lives at the expected path
	fm := Frontmatter{Name: "rule", Description: "d", Type: TypeProject}
	if _, err := WriteProposal(home, "p", "t", fm, "body\n"); err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(ProposalsTaskPath(home, "p", "t"), "rule.md")
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected file at %s: %v", expected, err)
	}
}
