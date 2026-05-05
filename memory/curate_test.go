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

// mockRunner is an AgentRunner that returns a pre-configured response.
type mockRunner struct {
	output     string
	err        error
	lastPrompt string
	lastEnv    []string
}

func (m *mockRunner) Run(_ context.Context, prompt string, env []string) (string, error) {
	m.lastPrompt = prompt
	m.lastEnv = env
	return m.output, m.err
}

// mockReportJSON returns a JSON curation report string as would be emitted by marlowe.
func mockReportJSON(accepted, skipped, invalidated int, flagged []string) string {
	r := agentReport{
		Accepted:    accepted,
		Skipped:     skipped,
		Invalidated: invalidated,
		Flagged:     flagged,
	}
	if flagged == nil {
		r.Flagged = []string{}
	}
	b, _ := json.Marshal(r)
	return string(b)
}

func TestCurate_EmptyQueue(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	runner := &mockRunner{output: ""}
	report, err := Curate(CurateOptions{Home: home, Runner: runner})
	if err != nil {
		t.Fatalf("Curate empty queue: %v", err)
	}
	if report.Processed != 0 {
		t.Errorf("processed: got %d want 0", report.Processed)
	}
}

func TestCurate_ProcessesQueueEntry(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	qe := SessionQueueEntry{
		Task:    "gig-test-1",
		Persona: "jenko",
		Repos:   []string{"backend"},
		EndedAt: time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC),
	}
	if _, err := WriteQueueEntry(home, qe); err != nil {
		t.Fatalf("WriteQueueEntry: %v", err)
	}

	proposalFM := Frontmatter{
		Name:        "no-try-catch",
		Description: "don't wrap async in try/catch",
		Type:        TypeFeedback,
	}
	if _, err := WriteProposal(home, "jenko", "gig-test-1", proposalFM, "Use error boundaries.\n"); err != nil {
		t.Fatalf("WriteProposal: %v", err)
	}

	runner := &mockRunner{
		output: "Processing done.\n" + mockReportJSON(1, 0, 0, nil),
	}

	report, err := Curate(CurateOptions{
		Home:   home,
		Runner: runner,
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
	if len(report.Errors) > 0 {
		t.Errorf("unexpected errors: %v", report.Errors)
	}
}

func TestCurate_ArchivesQueueAndProposals(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	qe := SessionQueueEntry{
		Task:    "gig-arch-1",
		Persona: "eric",
		EndedAt: time.Now().UTC(),
	}
	qPath, err := WriteQueueEntry(home, qe)
	if err != nil {
		t.Fatalf("WriteQueueEntry: %v", err)
	}

	proposalFM := Frontmatter{Name: "auth-loc", Description: "auth location", Type: TypeProject}
	proposal, err := WriteProposal(home, "eric", "gig-arch-1", proposalFM, "middleware/auth.go\n")
	if err != nil {
		t.Fatalf("WriteProposal: %v", err)
	}

	runner := &mockRunner{output: mockReportJSON(1, 0, 0, nil)}
	if _, err := Curate(CurateOptions{Home: home, Runner: runner}); err != nil {
		t.Fatalf("Curate: %v", err)
	}

	// Queue file must be gone (moved to archive).
	if _, err := os.Stat(qPath); err == nil {
		t.Error("queue entry should have been archived (moved), not still in place")
	}

	// Proposal file must be gone (moved to archive).
	if _, err := os.Stat(proposal.Path); err == nil {
		t.Error("proposal should have been archived (moved), not still in place")
	}

	// Archive dir must contain them.
	archiveRoot := ArchiveRoot(home)
	var qFound, pFound bool
	filepath.WalkDir(archiveRoot, func(path string, d os.DirEntry, _ error) error { //nolint:errcheck
		base := filepath.Base(path)
		if strings.HasPrefix(base, "gig-arch-1") && strings.HasSuffix(base, ".json") {
			qFound = true
		}
		if base == "auth-loc.md" {
			pFound = true
		}
		return nil
	})
	if !qFound {
		t.Error("queue entry not found in archive")
	}
	if !pFound {
		t.Error("proposal not found in archive")
	}
}

func TestCurate_PersonaFilter(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	for _, persona := range []string{"jenko", "schmidt"} {
		qe := SessionQueueEntry{
			Task:    "gig-" + persona,
			Persona: persona,
			EndedAt: time.Now().UTC(),
		}
		if _, err := WriteQueueEntry(home, qe); err != nil {
			t.Fatalf("WriteQueueEntry %s: %v", persona, err)
		}
	}

	runner := &mockRunner{output: mockReportJSON(0, 0, 0, nil)}

	report, err := Curate(CurateOptions{Home: home, Persona: "jenko", Runner: runner})
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}
	if report.Processed != 1 {
		t.Errorf("with persona filter: processed %d want 1", report.Processed)
	}

	// schmidt's queue entry must still be in queue.
	remaining, err := ListQueueEntries(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].Persona != "schmidt" {
		t.Errorf("schmidt entry should remain; got: %+v", remaining)
	}
}

func TestCurate_PromptContainsSkillAndProposals(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	qe := SessionQueueEntry{
		Task:    "gig-prompt-test",
		Persona: "jenko",
		EndedAt: time.Now().UTC(),
	}
	if _, err := WriteQueueEntry(home, qe); err != nil {
		t.Fatal(err)
	}

	fm := Frontmatter{Name: "my-prop", Description: "test proposal", Type: TypeFeedback}
	if _, err := WriteProposal(home, "jenko", "gig-prompt-test", fm, "The body.\n"); err != nil {
		t.Fatal(err)
	}

	runner := &mockRunner{output: mockReportJSON(0, 0, 0, nil)}
	if _, err := Curate(CurateOptions{
		Home:         home,
		Runner:       runner,
		SkillContent: "## SKILL CONTENT HERE",
	}); err != nil {
		t.Fatalf("Curate: %v", err)
	}

	if !strings.Contains(runner.lastPrompt, "SKILL CONTENT HERE") {
		t.Error("prompt should contain skill content")
	}
	if !strings.Contains(runner.lastPrompt, "gig-prompt-test") {
		t.Error("prompt should contain task ID")
	}
	if !strings.Contains(runner.lastPrompt, "my-prop") {
		t.Error("prompt should contain proposal name")
	}
	if !strings.Contains(runner.lastPrompt, "The body.") {
		t.Error("prompt should contain proposal body")
	}
	if !sliceContains(runner.lastEnv, "JEFF_MEMORY_CAN_ADD=1") {
		t.Errorf("env must contain JEFF_MEMORY_CAN_ADD=1; got: %v", runner.lastEnv)
	}
}

func TestCurate_RequiresRunner(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}
	_, err := Curate(CurateOptions{Home: home, Runner: nil})
	if err == nil {
		t.Error("expected error when Runner is nil")
	}
}

func TestCurate_FlaggedReport(t *testing.T) {
	home := t.TempDir()
	if err := EnsureLayout(home); err != nil {
		t.Fatal(err)
	}

	qe := SessionQueueEntry{Task: "gig-flag", Persona: "hardy", EndedAt: time.Now().UTC()}
	if _, err := WriteQueueEntry(home, qe); err != nil {
		t.Fatal(err)
	}

	runner := &mockRunner{
		output: mockReportJSON(0, 0, 0, []string{"conflict-entry-a", "conflict-entry-b"}),
	}
	report, err := Curate(CurateOptions{Home: home, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Flagged) != 2 {
		t.Errorf("flagged: got %d want 2", len(report.Flagged))
	}
}

func TestParseAgentReport(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   agentReport
	}{
		{
			name:   "clean JSON at end",
			output: `Some text.` + "\n" + `{"accepted":3,"skipped":1,"invalidated":0,"flagged":[]}`,
			want:   agentReport{Accepted: 3, Skipped: 1},
		},
		{
			name:   "JSON with flagged",
			output: `{"accepted":0,"skipped":0,"invalidated":1,"flagged":["entry-a"]}`,
			want:   agentReport{Invalidated: 1, Flagged: []string{"entry-a"}},
		},
		{
			name:   "no JSON",
			output: "No report here.",
			want:   agentReport{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseAgentReport(tc.output)
			if got.Accepted != tc.want.Accepted {
				t.Errorf("accepted: got %d want %d", got.Accepted, tc.want.Accepted)
			}
			if got.Skipped != tc.want.Skipped {
				t.Errorf("skipped: got %d want %d", got.Skipped, tc.want.Skipped)
			}
			if got.Invalidated != tc.want.Invalidated {
				t.Errorf("invalidated: got %d want %d", got.Invalidated, tc.want.Invalidated)
			}
			if len(got.Flagged) != len(tc.want.Flagged) {
				t.Errorf("flagged len: got %d want %d", len(got.Flagged), len(tc.want.Flagged))
			}
		})
	}
}

func sliceContains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
