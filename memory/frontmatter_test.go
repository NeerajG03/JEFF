package memory

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestFrontmatter_RoundTrip(t *testing.T) {
	in := Frontmatter{
		Name:        "no-mock-db",
		Description: "Integration tests must hit a real database, not mocks.",
		Type:        TypeFeedback,
	}
	body := "Why: prior incident where mock/prod divergence masked a broken migration.\n\nHow to apply: never substitute a fake DB in tests under /integration.\n"

	var buf bytes.Buffer
	if err := Write(&buf, in, body); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, gotBody, err := Parse(&buf)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got != in {
		t.Errorf("frontmatter mismatch:\n got: %+v\nwant: %+v", got, in)
	}
	if gotBody != body {
		t.Errorf("body mismatch:\n got: %q\nwant: %q", gotBody, body)
	}
}

func TestFrontmatter_Unicode(t *testing.T) {
	fm := Frontmatter{Name: "i18n-rule", Description: "café — é, 日本語, 🚀", Type: TypeReference}
	body := "Body with unicode: café 日本語 🚀\n"
	var buf bytes.Buffer
	if err := Write(&buf, fm, body); err != nil {
		t.Fatal(err)
	}
	got, gotBody, err := Parse(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got != fm {
		t.Errorf("unicode frontmatter not preserved: %+v", got)
	}
	if gotBody != body {
		t.Errorf("unicode body not preserved: %q", gotBody)
	}
}

func TestFrontmatter_RejectsInvalidType(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, Frontmatter{Name: "x", Description: "y", Type: MemoryType("bogus")}, "body")
	if err == nil {
		t.Fatal("expected invalid-type error")
	}
}

func TestFrontmatter_RejectsMissingName(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, Frontmatter{Description: "y", Type: TypeUser}, "body")
	if err == nil {
		t.Fatal("expected missing-name error")
	}
}

func TestFrontmatter_RejectsMissingFences(t *testing.T) {
	if _, _, err := Parse(strings.NewReader("name: x\n")); err == nil {
		t.Fatal("expected missing-fence error")
	}
	if _, _, err := Parse(strings.NewReader("---\nname: x\nno-closing-fence\n")); err == nil {
		t.Fatal("expected missing-closing-fence error")
	}
}

func TestCanonicalFrontmatter_RoundTrip(t *testing.T) {
	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	in := CanonicalFrontmatter{
		Frontmatter: Frontmatter{
			Name:        "no-mock-db",
			Description: "real DB only",
			Type:        TypeFeedback,
		},
		Status:     "accepted",
		Scope:      "repo:gig",
		GoalServed: "jenko/correctness",
		Importance: 8,
		ValidFrom:  now,
		Supersedes: []string{"old-mock-rule"},
		Provenance: "trusted",
		Source: Source{
			Persona: "jenko",
			Task:    "gig-1d33.1",
			Trigger: "user-correction",
		},
		Verifier: &Verifier{Type: "llm-judge", Result: "pass", RanAt: now},
	}
	body := "Body of the canonical entry.\n"

	var buf bytes.Buffer
	if err := writeCanonicalFile(&buf, in, body); err != nil {
		t.Fatal(err)
	}

	got, gotBody, err := ParseCanonical(&buf)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}
	if got.Name != in.Name || got.Description != in.Description || got.Type != in.Type {
		t.Errorf("inline frontmatter mismatch: %+v", got.Frontmatter)
	}
	if got.Status != in.Status || got.Scope != in.Scope || got.GoalServed != in.GoalServed || got.Importance != in.Importance {
		t.Errorf("status/scope/goal/importance mismatch: %+v", got)
	}
	if !got.ValidFrom.Equal(in.ValidFrom) {
		t.Errorf("ValidFrom mismatch: %v want %v", got.ValidFrom, in.ValidFrom)
	}
	if got.ValidTo != nil {
		t.Errorf("ValidTo should be nil, got %v", got.ValidTo)
	}
	if len(got.Supersedes) != 1 || got.Supersedes[0] != "old-mock-rule" {
		t.Errorf("Supersedes mismatch: %v", got.Supersedes)
	}
	if got.Verifier == nil || got.Verifier.Type != "llm-judge" || got.Verifier.Result != "pass" {
		t.Errorf("Verifier mismatch: %+v", got.Verifier)
	}
	if got.Source != in.Source {
		t.Errorf("Source mismatch: %+v want %+v", got.Source, in.Source)
	}
	if gotBody != body {
		t.Errorf("body mismatch: %q want %q", gotBody, body)
	}
}

func TestCanonicalFrontmatter_OptionalsOmitted(t *testing.T) {
	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	in := CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: "n", Description: "d", Type: TypeUser},
		Status:      "accepted",
		Scope:       "persona:jenko",
		ValidFrom:   now,
		Source:      Source{Persona: "jenko", Task: "t", Trigger: "self-noted"},
	}
	var buf bytes.Buffer
	if err := writeCanonicalFile(&buf, in, ""); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, banned := range []string{"valid_to:", "superseded_by:", "supersedes:", "verifier:"} {
		if strings.Contains(out, banned) {
			t.Errorf("output should omit %q when zero, got:\n%s", banned, out)
		}
	}
}
