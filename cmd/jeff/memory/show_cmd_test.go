package memory

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mem "github.com/NeerajG03/JEFF/memory"
)

func seedShowHome(t *testing.T) (home, entryPath string) {
	t.Helper()
	home = t.TempDir()
	if err := mem.EnsureLayout(home); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	scopePath := mem.PersonaScopePath(home, "jenko")
	bucketDir := mem.BucketPath(scopePath, mem.BucketProcedural)
	if err := os.MkdirAll(bucketDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entryPath = filepath.Join(bucketDir, "my-rule.md")
	f, err := os.Create(entryPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	fm := mem.CanonicalFrontmatter{
		Frontmatter: mem.Frontmatter{Name: "my-rule", Description: "A useful rule", Type: mem.TypeFeedback},
		Status:      "accepted", Scope: "persona:jenko", ValidFrom: now,
		Source: mem.Source{Persona: "jenko", Task: "t1", Trigger: "user-correction"},
	}
	if err := mem.WriteCanonical(f, fm, "Rule body goes here.\n"); err != nil {
		t.Fatal(err)
	}
	return home, entryPath
}

func TestRunShow_ByName(t *testing.T) {
	home, _ := seedShowHome(t)
	var buf bytes.Buffer
	if err := runShow(&buf, home, "my-rule"); err != nil {
		t.Fatalf("runShow by name: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "my-rule") {
		t.Error("expected entry name in output")
	}
	if !strings.Contains(out, "Rule body goes here") {
		t.Error("expected body in output")
	}
}

func TestRunShow_ByPath(t *testing.T) {
	home, entryPath := seedShowHome(t)
	_ = home
	var buf bytes.Buffer
	if err := runShow(&buf, home, entryPath); err != nil {
		t.Fatalf("runShow by path: %v", err)
	}
	if !strings.Contains(buf.String(), "my-rule") {
		t.Error("expected entry name in output")
	}
}

func TestRunShow_NotFound(t *testing.T) {
	home, _ := seedShowHome(t)
	var buf bytes.Buffer
	err := runShow(&buf, home, "nonexistent-name")
	if err == nil {
		t.Error("expected error for nonexistent name")
	}
	if !strings.Contains(err.Error(), "no entry found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunShow_BadPath(t *testing.T) {
	home := t.TempDir()
	var buf bytes.Buffer
	err := runShow(&buf, home, "/tmp/does/not/exist.md")
	if err == nil {
		t.Error("expected error for missing file path")
	}
}

func TestRunShow_AmbiguousName(t *testing.T) {
	home := t.TempDir()
	if err := mem.EnsureLayout(home); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	// Create the same slug in two different scopes.
	for _, persona := range []string{"jenko", "schmidt"} {
		scopePath := mem.PersonaScopePath(home, persona)
		bucketDir := mem.BucketPath(scopePath, mem.BucketSemantic)
		if err := os.MkdirAll(bucketDir, 0o755); err != nil {
			t.Fatal(err)
		}
		fp := filepath.Join(bucketDir, "shared-name.md")
		f, err := os.Create(fp)
		if err != nil {
			t.Fatal(err)
		}
		fm := mem.CanonicalFrontmatter{
			Frontmatter: mem.Frontmatter{Name: "shared-name", Description: "desc", Type: mem.TypeUser},
			Status:      "accepted", Scope: "persona:" + persona, ValidFrom: now,
			Source: mem.Source{Persona: persona, Task: "t", Trigger: "self-noted"},
		}
		if err := mem.WriteCanonical(f, fm, "body\n"); err != nil {
			f.Close()
			t.Fatal(err)
		}
		f.Close()
	}

	var buf bytes.Buffer
	err := runShow(&buf, home, "shared-name")
	if err == nil {
		t.Error("expected error for ambiguous name")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("expected ambiguous error, got: %v", err)
	}
	// Output should list candidates.
	if !strings.Contains(buf.String(), "jeff memory show") {
		t.Error("expected candidate paths in output")
	}
}
