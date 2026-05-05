package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryType_Valid(t *testing.T) {
	cases := []struct {
		in   MemoryType
		want bool
	}{
		{TypeUser, true},
		{TypeFeedback, true},
		{TypeProject, true},
		{TypeReference, true},
		{MemoryType("bogus"), false},
		{MemoryType(""), false},
	}
	for _, tc := range cases {
		if got := tc.in.Valid(); got != tc.want {
			t.Errorf("MemoryType(%q).Valid() = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseMemoryType(t *testing.T) {
	if _, err := ParseMemoryType("user"); err != nil {
		t.Fatalf("user: %v", err)
	}
	if _, err := ParseMemoryType("nope"); err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestScopeAndBucketValid(t *testing.T) {
	if !ScopePersona.Valid() || !ScopeRepo.Valid() || !ScopeProject.Valid() || !ScopeOrchestrator.Valid() {
		t.Fatal("expected canonical scopes to validate")
	}
	if ScopeKind("nope").Valid() {
		t.Fatal("nope should be invalid scope")
	}
	for _, b := range Buckets {
		if !b.Valid() {
			t.Errorf("bucket %q should validate", b)
		}
	}
	if Bucket("nope").Valid() {
		t.Fatal("nope should be invalid bucket")
	}
}

func TestPaths(t *testing.T) {
	home := "/tmp/J"
	if got, want := MemoryRoot(home), "/tmp/J/memory"; got != want {
		t.Errorf("MemoryRoot=%q want %q", got, want)
	}
	if got, want := PersonaScopePath(home, "jenko"), "/tmp/J/memory/personas/jenko"; got != want {
		t.Errorf("PersonaScopePath=%q want %q", got, want)
	}
	if got, want := RepoScopePath(home, "gig"), "/tmp/J/memory/repos/gig"; got != want {
		t.Errorf("RepoScopePath=%q want %q", got, want)
	}
	if got, want := ProjectScopePath(home, "alpha"), "/tmp/J/memory/projects/alpha"; got != want {
		t.Errorf("ProjectScopePath=%q want %q", got, want)
	}
	scope := PersonaScopePath(home, "jenko")
	if got, want := BucketPath(scope, BucketProcedural), "/tmp/J/memory/personas/jenko/procedural"; got != want {
		t.Errorf("BucketPath=%q want %q", got, want)
	}
	if got, want := QueueSessionsRoot(home), "/tmp/J/queue/sessions"; got != want {
		t.Errorf("QueueSessionsRoot=%q want %q", got, want)
	}
	if got, want := ProposalsTaskPath(home, "jenko", "g-1"), "/tmp/J/proposals/jenko/g-1"; got != want {
		t.Errorf("ProposalsTaskPath=%q want %q", got, want)
	}
}

func TestEnsureLayout_Idempotent(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	wants := []string{
		MemoryRoot(home),
		filepath.Join(MemoryRoot(home), "personas"),
		filepath.Join(MemoryRoot(home), "repos"),
		filepath.Join(MemoryRoot(home), "projects"),
		OrchestratorScopePath(home),
		ProposalsRoot(home),
		QueueRoot(home),
		QueueSessionsRoot(home),
		TranscriptsRoot(home),
		ArchiveRoot(home),
	}
	for _, d := range wants {
		info, err := os.Stat(d)
		if err != nil {
			t.Fatalf("%s missing: %v", d, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s not a directory", d)
		}
	}

	// Second call must succeed (idempotency).
	if err := EnsureLayout(home); err != nil {
		t.Fatalf("second EnsureLayout: %v", err)
	}

	// Pre-existing user content must be preserved.
	userFile := filepath.Join(MemoryRoot(home), "personas", "marker.txt")
	if err := os.WriteFile(userFile, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(userFile)
	if err != nil || string(data) != "keep" {
		t.Fatalf("EnsureLayout clobbered user content: %v %q", err, data)
	}
}

func TestEnsureLayout_RejectsEmptyHome(t *testing.T) {
	if err := EnsureLayout(""); err == nil {
		t.Fatal("expected error for empty jeffHome")
	}
}
