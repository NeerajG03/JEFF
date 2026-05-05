package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteAndListQueueEntry(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	e := SessionQueueEntry{
		Task:           "gig-1d33.1",
		Persona:        "jenko",
		Repos:          []string{"jeff"},
		TranscriptPath: "/tmp/x.jsonl",
		Reason:         "session-end",
		Proposals:      []string{"a.md", "b.md"},
		EndedAt:        time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC),
	}
	path, err := WriteQueueEntry(home, e)
	if err != nil {
		t.Fatalf("WriteQueueEntry: %v", err)
	}
	if filepath.Dir(path) != QueueSessionsRoot(home) {
		t.Errorf("entry not in queue/sessions: %s", path)
	}

	got, err := ListQueueEntries(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 entry, got %d", len(got))
	}
	if got[0].Task != e.Task || got[0].Persona != e.Persona || !got[0].EndedAt.Equal(e.EndedAt) {
		t.Errorf("round-trip mismatch: %+v", got[0])
	}
	if len(got[0].Proposals) != 2 || got[0].Proposals[1] != "b.md" {
		t.Errorf("proposals not preserved: %+v", got[0].Proposals)
	}
}

func TestWriteQueueEntry_AutoEndedAt(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}
	before := time.Now().UTC().Add(-time.Second)
	path, err := WriteQueueEntry(home, SessionQueueEntry{Task: "t"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ListQueueEntries(home)
	if err != nil || len(got) != 1 {
		t.Fatalf("list: %v %d", err, len(got))
	}
	if got[0].EndedAt.Before(before) {
		t.Errorf("EndedAt not auto-set: %v", got[0].EndedAt)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file: %v", err)
	}
}

func TestArchiveQueueEntry(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}
	path, err := WriteQueueEntry(home, SessionQueueEntry{
		Task:    "g-1",
		EndedAt: time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ArchiveQueueEntry(home, path); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("source still present: %v", err)
	}
	// Archive bucket exists somewhere under archive/
	matches, err := filepath.Glob(filepath.Join(ArchiveRoot(home), "*", "queue", filepath.Base(path)))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 archived file, got %v", matches)
	}
}

func TestWriteQueueEntry_RequiresTask(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteQueueEntry(home, SessionQueueEntry{}); err == nil {
		t.Fatal("expected error for missing task")
	}
}

func TestListQueueEntries_MissingDirOK(t *testing.T) {
	home := t.TempDir()
	out, err := ListQueueEntries(home)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(out))
	}
}
