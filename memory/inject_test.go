package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyToTask_FreshFile(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	claudePath := filepath.Join(dir, "CLAUDE.md")
	os.WriteFile(claudePath, []byte("# Existing content\n"), 0o644)

	if err := ApplyToTask(home, dir, "jenko", "gig-test1", []string{"jeff"}, "claude"); err != nil {
		t.Fatalf("ApplyToTask: %v", err)
	}

	data, _ := os.ReadFile(claudePath)
	content := string(data)

	if !strings.Contains(content, addendumStartSentinel) {
		t.Error("missing start sentinel after ApplyToTask")
	}
	if !strings.Contains(content, addendumEndSentinel) {
		t.Error("missing end sentinel after ApplyToTask")
	}
	if !strings.Contains(content, "jenko") {
		t.Error("persona not substituted")
	}
	if !strings.Contains(content, "gig-test1") {
		t.Error("task_id not substituted")
	}
	if !strings.Contains(content, "jeff") {
		t.Error("repos not substituted")
	}
	// Original content is preserved.
	if !strings.Contains(content, "# Existing content") {
		t.Error("original content was lost")
	}
}

func TestApplyToTask_EmptyFile(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	claudePath := filepath.Join(dir, "CLAUDE.md")

	if err := ApplyToTask(home, dir, "schmidt", "gig-abc2", nil, "claude"); err != nil {
		t.Fatalf("ApplyToTask on empty target: %v", err)
	}

	data, _ := os.ReadFile(claudePath)
	content := string(data)
	if !strings.Contains(content, addendumStartSentinel) {
		t.Error("missing sentinel in newly created file")
	}
	if !strings.Contains(content, "(none)") {
		t.Error("empty repos should render as (none)")
	}
}

func TestApplyToTask_Idempotent(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	claudePath := filepath.Join(dir, "CLAUDE.md")
	os.WriteFile(claudePath, []byte("## Header\n"), 0o644)

	// First application.
	if err := ApplyToTask(home, dir, "jenko", "gig-1", []string{"jeff"}, "claude"); err != nil {
		t.Fatalf("first ApplyToTask: %v", err)
	}
	first, _ := os.ReadFile(claudePath)

	// Second application — must not duplicate.
	if err := ApplyToTask(home, dir, "jenko", "gig-1", []string{"jeff"}, "claude"); err != nil {
		t.Fatalf("second ApplyToTask: %v", err)
	}
	second, _ := os.ReadFile(claudePath)

	count := func(s, sub string) int {
		return strings.Count(s, sub)
	}
	if n := count(string(second), addendumStartSentinel); n != 1 {
		t.Errorf("expected 1 start sentinel, got %d", n)
	}
	if n := count(string(second), addendumEndSentinel); n != 1 {
		t.Errorf("expected 1 end sentinel, got %d", n)
	}
	_ = first
}

func TestApplyToTask_SentinelReplacement(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	claudePath := filepath.Join(dir, "CLAUDE.md")
	os.WriteFile(claudePath, []byte("## Header\n"), 0o644)

	// Apply with jenko.
	if err := ApplyToTask(home, dir, "jenko", "gig-1", []string{"jeff"}, "claude"); err != nil {
		t.Fatalf("first apply: %v", err)
	}

	// Apply with different persona — should replace.
	if err := ApplyToTask(home, dir, "schmidt", "gig-1", []string{"jeff"}, "claude"); err != nil {
		t.Fatalf("second apply (different persona): %v", err)
	}

	data, _ := os.ReadFile(claudePath)
	content := string(data)

	if strings.Contains(content, "jenko") {
		t.Error("old persona 'jenko' still present after replacement")
	}
	if !strings.Contains(content, "schmidt") {
		t.Error("new persona 'schmidt' not found after replacement")
	}
	// Header is preserved.
	if !strings.Contains(content, "## Header") {
		t.Error("existing header was lost during replacement")
	}
}

func TestApplyToTask_GeminiUsesGeminiFile(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()

	// Create GEMINI.md directly (no symlink needed for test).
	geminiPath := filepath.Join(dir, "GEMINI.md")
	os.WriteFile(geminiPath, []byte("# Gemini context\n"), 0o644)

	if err := ApplyToTask(home, dir, "jenko", "gig-g1", []string{"jeff"}, "gemini"); err != nil {
		t.Fatalf("ApplyToTask for gemini: %v", err)
	}

	data, _ := os.ReadFile(geminiPath)
	if !strings.Contains(string(data), addendumStartSentinel) {
		t.Error("gemini addendum not written to GEMINI.md")
	}
}

func TestRenderAddendum_Substitutions(t *testing.T) {
	tmpl := "persona={{persona}} task={{task_id}} repos={{repos}} index={{memory_index}}"
	got := renderAddendum(tmpl, "jenko", "gig-x1", []string{"jeff", "gig"}, "IDX\n")
	want := "persona=jenko task=gig-x1 repos=jeff, gig index=IDX\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderAddendum_EmptyRepos(t *testing.T) {
	tmpl := "repos={{repos}}"
	got := renderAddendum(tmpl, "jenko", "gig-x1", nil, "")
	if !strings.Contains(got, "(none)") {
		t.Errorf("empty repos should produce (none), got %q", got)
	}
}

func TestBuildMemoryIndex_Empty(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	got := buildMemoryIndex(home, "jenko", []string{"jeff"})
	if got != "" {
		t.Errorf("expected empty index with no canonical entries, got %q", got)
	}
}

func TestBuildMemoryIndex_PopulatedAndScoped(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}

	mustWrite := func(scope, bucket, name, desc string) {
		fm := CanonicalFrontmatter{Frontmatter: Frontmatter{Name: name, Type: TypeProject, Description: desc}}
		if _, err := WriteCanonical(home, scope, bucket, fm, "body"); err != nil {
			t.Fatalf("WriteCanonical(%s/%s/%s): %v", scope, bucket, name, err)
		}
	}

	mustWrite("persona:jenko", "semantic", "jenko-fact", "A jenko-scoped fact")
	mustWrite("persona:schmidt", "semantic", "schmidt-fact", "Should not appear for jenko")
	mustWrite("repo:jeff", "semantic", "jeff-fact", "A jeff-repo fact")
	mustWrite("repo:other-repo", "semantic", "other-fact", "Out of scope repo, should not appear")
	mustWrite("orchestrator", "semantic", "global-rule", "A global rule")

	got := buildMemoryIndex(home, "jenko", []string{"jeff"})

	for _, want := range []string{
		"## Persona memory (jenko)",
		"`jenko-fact` — A jenko-scoped fact",
		"## Repo memory (jeff)",
		"`jeff-fact` — A jeff-repo fact",
		"## Orchestrator memory (global rules)",
		"`global-rule` — A global rule",
		"Read full body with `jeff memory show <name>` when the topic is relevant.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("index missing %q\nfull index:\n%s", want, got)
		}
	}

	for _, unwanted := range []string{"schmidt-fact", "other-fact", "Repo memory (other-repo)"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("index leaked out-of-scope entry %q\nfull index:\n%s", unwanted, got)
		}
	}
}
