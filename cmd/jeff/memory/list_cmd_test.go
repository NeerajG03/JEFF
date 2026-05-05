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

func mkCanonicalEntry(t *testing.T, scopePath string, bucket mem.Bucket, slug string, fm mem.CanonicalFrontmatter) {
	t.Helper()
	var dir, fp string
	if bucket == mem.BucketCore {
		fp = filepath.Join(scopePath, "core.md")
		dir = scopePath
	} else {
		dir = mem.BucketPath(scopePath, bucket)
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
	if err := mem.WriteCanonical(f, fm, "test body\n"); err != nil {
		t.Fatal(err)
	}
}

func seedListHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if err := mem.EnsureLayout(home); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	jenScope := mem.PersonaScopePath(home, "jenko")
	mkCanonicalEntry(t, jenScope, mem.BucketProcedural, "async-error-handling", mem.CanonicalFrontmatter{
		Frontmatter: mem.Frontmatter{Name: "async-error-handling", Description: "Don't wrap async in try/catch", Type: mem.TypeFeedback},
		Status:      "accepted", Scope: "persona:jenko", ValidFrom: now,
		Source: mem.Source{Persona: "jenko", Task: "t1", Trigger: "user-correction"},
	})
	mkCanonicalEntry(t, jenScope, mem.BucketSemantic, "auth-middleware-location", mem.CanonicalFrontmatter{
		Frontmatter: mem.Frontmatter{Name: "auth-middleware-location", Description: "Auth middleware in security/ not middleware/", Type: mem.TypeReference},
		Status:      "accepted", Scope: "persona:jenko", ValidFrom: now,
		Source: mem.Source{Persona: "jenko", Task: "t1", Trigger: "self-noted"},
	})
	mkCanonicalEntry(t, jenScope, mem.BucketSemantic, "old-fact", mem.CanonicalFrontmatter{
		Frontmatter: mem.Frontmatter{Name: "old-fact", Description: "An old superseded fact", Type: mem.TypeProject},
		Status:      "superseded", Scope: "persona:jenko", ValidFrom: now,
		Source: mem.Source{Persona: "jenko", Task: "t0", Trigger: "self-noted"},
	})

	repoScope := mem.RepoScopePath(home, "jeff")
	mkCanonicalEntry(t, repoScope, mem.BucketProcedural, "use-sdk", mem.CanonicalFrontmatter{
		Frontmatter: mem.Frontmatter{Name: "use-sdk", Description: "Use the gig SDK not CLI shellouts", Type: mem.TypeFeedback},
		Status:      "accepted", Scope: "repo:jeff", ValidFrom: now,
		Source: mem.Source{Persona: "jenko", Task: "t2", Trigger: "user-correction"},
	})

	return home
}

func TestRunList_Default(t *testing.T) {
	home := seedListHome(t)
	var buf bytes.Buffer
	opts := listOpts{status: "accepted", limit: 50}
	if err := runList(&buf, home, opts); err != nil {
		t.Fatalf("runList: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "async-error-handling") {
		t.Error("expected async-error-handling in output")
	}
	if !strings.Contains(out, "use-sdk") {
		t.Error("expected use-sdk in output")
	}
	// Superseded entry must not appear in accepted-only listing.
	if strings.Contains(out, "old-fact") {
		t.Error("superseded entry old-fact should not appear")
	}
}

func TestRunList_PersonaFilter(t *testing.T) {
	home := seedListHome(t)
	var buf bytes.Buffer
	opts := listOpts{persona: "jenko", status: "accepted", limit: 50}
	if err := runList(&buf, home, opts); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "async-error-handling") {
		t.Error("expected jenko entry")
	}
	if strings.Contains(out, "use-sdk") {
		t.Error("use-sdk is repo:jeff, should be excluded by --persona jenko")
	}
}

func TestRunList_BucketFilter(t *testing.T) {
	home := seedListHome(t)
	var buf bytes.Buffer
	opts := listOpts{bucket: "procedural", status: "accepted", limit: 50}
	if err := runList(&buf, home, opts); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "async-error-handling") {
		t.Error("expected procedural entry")
	}
	if strings.Contains(out, "auth-middleware-location") {
		t.Error("auth-middleware-location is semantic, should be excluded")
	}
}

func TestRunList_ScopeFilter(t *testing.T) {
	home := seedListHome(t)
	var buf bytes.Buffer
	opts := listOpts{scope: "repo:jeff", status: "accepted", limit: 50}
	if err := runList(&buf, home, opts); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "use-sdk") {
		t.Error("expected use-sdk")
	}
	if strings.Contains(out, "async-error-handling") {
		t.Error("async-error-handling is persona:jenko, should be excluded")
	}
}

func TestRunList_SupersededFilter(t *testing.T) {
	home := seedListHome(t)
	var buf bytes.Buffer
	opts := listOpts{status: "superseded", limit: 50}
	if err := runList(&buf, home, opts); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "old-fact") {
		t.Error("expected old-fact in superseded listing")
	}
	if strings.Contains(out, "async-error-handling") {
		t.Error("accepted entry should not appear in superseded listing")
	}
}

func TestRunList_JSONOutput(t *testing.T) {
	home := seedListHome(t)
	var buf bytes.Buffer
	opts := listOpts{status: "accepted", limit: 50, asJSON: true}
	if err := runList(&buf, home, opts); err != nil {
		t.Fatal(err)
	}
	var rows []listJSONEntry
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	if len(rows) == 0 {
		t.Fatal("expected at least one JSON entry")
	}
	for _, r := range rows {
		if r.Scope == "" || r.Name == "" || r.Bucket == "" {
			t.Errorf("incomplete JSON entry: %+v", r)
		}
	}
}

func TestRunList_Limit(t *testing.T) {
	home := seedListHome(t)
	var buf bytes.Buffer
	opts := listOpts{status: "accepted", limit: 1}
	if err := runList(&buf, home, opts); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "3 entries") {
		t.Error("limit=1 should truncate to 1")
	}
}

func TestRunList_InvalidScope(t *testing.T) {
	home := t.TempDir()
	var buf bytes.Buffer
	opts := listOpts{scope: "badformat", status: "accepted", limit: 50}
	err := runList(&buf, home, opts)
	if err == nil {
		t.Error("expected error for invalid scope format")
	}
}

func TestRunList_Empty(t *testing.T) {
	home := t.TempDir()
	if err := mem.EnsureLayout(home); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	opts := listOpts{status: "accepted", limit: 50}
	if err := runList(&buf, home, opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No entries") {
		t.Error("expected 'No entries found' message")
	}
}
