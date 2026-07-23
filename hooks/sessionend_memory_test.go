package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NeerajG03/JEFF/memory"
)

func TestRunSessionEnd_DisableGateSkipsAll(t *testing.T) {
	jeffHome := t.TempDir()
	t.Setenv("JEFF_MEMORY_DISABLE", "1")

	if err := RunSessionEnd(jeffHome, "gig-ds1", "jenko", []string{"jeff"}, "claude", "", "user"); err != nil {
		t.Fatalf("RunSessionEnd with disabled memory: %v", err)
	}

	// No queue entry should be written.
	entries, err := memory.ListQueueEntries(jeffHome)
	if err != nil {
		t.Fatalf("ListQueueEntries: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 queue entries when memory disabled, got %d", len(entries))
	}
}

func TestRunSessionEnd_QueueWrite(t *testing.T) {
	jeffHome := t.TempDir()

	// EnsureLayout so queue dir exists.
	if err := memory.EnsureLayout(jeffHome); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}

	if err := RunSessionEnd(jeffHome, "gig-se1", "jenko", []string{"jeff"}, "claude", "", "user"); err != nil {
		t.Fatalf("RunSessionEnd: %v", err)
	}

	entries, err := memory.ListQueueEntries(jeffHome)
	if err != nil {
		t.Fatalf("ListQueueEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 queue entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Task != "gig-se1" {
		t.Errorf("task = %q, want gig-se1", e.Task)
	}
	if e.Persona != "jenko" {
		t.Errorf("persona = %q, want jenko", e.Persona)
	}
	if len(e.Repos) != 1 || e.Repos[0] != "jeff" {
		t.Errorf("repos = %v, want [jeff]", e.Repos)
	}
	if e.Reason != "user" {
		t.Errorf("reason = %q, want user", e.Reason)
	}
}

func TestRunSessionEnd_TranscriptCopy(t *testing.T) {
	jeffHome := t.TempDir()
	memory.EnsureLayout(jeffHome)

	// Create a fake transcript.
	src := filepath.Join(t.TempDir(), "session.jsonl")
	os.WriteFile(src, []byte(`{"role":"user","content":"hello"}`+"\n"), 0o644)

	if err := RunSessionEnd(jeffHome, "gig-se2", "jenko", nil, "claude", src, "unknown"); err != nil {
		t.Fatalf("RunSessionEnd with transcript: %v", err)
	}

	// Transcript was copied into transcripts/gig-se2/.
	transcriptsDir := filepath.Join(jeffHome, "transcripts", "gig-se2")
	files, err := os.ReadDir(transcriptsDir)
	if err != nil {
		t.Fatalf("transcripts dir not created: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 transcript copy, got %d", len(files))
	}

	// Confirm queue entry records the copied path.
	entries, _ := memory.ListQueueEntries(jeffHome)
	if len(entries) == 0 {
		t.Fatal("no queue entry written")
	}
	if entries[0].TranscriptPath == "" {
		t.Error("queue entry transcript_path is empty")
	}
}

func TestRunSessionEnd_Idempotent(t *testing.T) {
	jeffHome := t.TempDir()
	memory.EnsureLayout(jeffHome)

	// Each call creates a new queue entry (different timestamp).
	for i := 0; i < 2; i++ {
		if err := RunSessionEnd(jeffHome, "gig-se3", "jenko", nil, "claude", "", "user"); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
	}

	entries, _ := memory.ListQueueEntries(jeffHome)
	if len(entries) != 2 {
		t.Errorf("expected 2 queue entries (one per call), got %d", len(entries))
	}
}

func TestRunSessionEnd_WithProposals(t *testing.T) {
	jeffHome := t.TempDir()
	memory.EnsureLayout(jeffHome)

	// Write a proposal for this persona+task.
	fm := memory.Frontmatter{Name: "test-proposal", Description: "desc", Type: memory.TypeFeedback}
	memory.WriteProposal(jeffHome, "jenko", "gig-se4", fm, "body text")

	if err := RunSessionEnd(jeffHome, "gig-se4", "jenko", nil, "claude", "", "unknown"); err != nil {
		t.Fatalf("RunSessionEnd: %v", err)
	}

	entries, _ := memory.ListQueueEntries(jeffHome)
	if len(entries) == 0 {
		t.Fatal("no queue entry")
	}
	if len(entries[0].Proposals) == 0 {
		t.Error("queue entry missing proposals list")
	}
	if entries[0].Proposals[0] != "test-proposal" {
		t.Errorf("proposal slug = %q, want test-proposal", entries[0].Proposals[0])
	}
}

func TestRunSessionEnd_NoLLM(t *testing.T) {
	// Static check: verify the sessionend file does not reference any LLM/HTTP client.
	// This is enforced by code review, but we add a basic string check to flag regressions.
	src, err := os.ReadFile("sessionend_memory.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	forbidden := []string{"anthropic", "openai", "http.Client", "llm.Call"}
	for _, f := range forbidden {
		if strings.Contains(string(src), f) {
			t.Errorf("sessionend_memory.go references %q — no LLM calls allowed", f)
		}
	}
}

func TestSessionEndMemoryHookDefinition(t *testing.T) {
	h := sessionEndMemoryHook()
	if h.Name != "memory-session-end" {
		t.Errorf("hook name = %q, want 'memory-session-end'", h.Name)
	}
	if h.Source != SourceTask {
		t.Errorf("source = %q, want SourceTask", h.Source)
	}
	if h.Event != "Stop" {
		t.Errorf("event = %q, want 'Stop'", h.Event)
	}
}

func TestSessionEndMemoryHookScriptContent(t *testing.T) {
	ctx := HookContext{
		TaskID:  "gig-test1",
		Persona: "jenko",
		Repos:   []string{"jeff"},
	}
	h := sessionEndMemoryHook()
	for _, key := range []string{"claude", "gemini"} {
		gen := h.Scripts[key]
		if gen == nil {
			t.Fatalf("Scripts[%q] is nil", key)
		}
		script := gen(ctx)
		if !strings.Contains(script, "jeff memory session-end") {
			t.Errorf("[%s] script missing 'jeff memory session-end'", key)
		}
		if !strings.Contains(script, "gig-test1") {
			t.Errorf("[%s] script missing task ID", key)
		}
	}
}

func TestSessionEndMemoryHookRegistered(t *testing.T) {
	reg := DefaultRegistry()
	if reg.Get("memory-session-end") == nil {
		t.Error("memory-session-end not in DefaultRegistry")
	}
}

// Ensure queue entry JSON can be round-tripped.
func TestQueueEntryRoundTrip(t *testing.T) {
	jeffHome := t.TempDir()
	memory.EnsureLayout(jeffHome)

	if err := RunSessionEnd(jeffHome, "gig-rt1", "jenko", []string{"jeff", "gig"}, "claude", "", "user"); err != nil {
		t.Fatalf("RunSessionEnd: %v", err)
	}

	dir := memory.QueueSessionsRoot(jeffHome)
	files, _ := os.ReadDir(dir)
	var jsonFile string
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".json") {
			jsonFile = filepath.Join(dir, f.Name())
		}
	}
	if jsonFile == "" {
		t.Fatal("no queue JSON file written")
	}

	data, _ := os.ReadFile(jsonFile)
	var entry memory.SessionQueueEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("cannot unmarshal queue entry: %v", err)
	}
	if entry.Task != "gig-rt1" {
		t.Errorf("task = %q", entry.Task)
	}
	if len(entry.Repos) != 2 {
		t.Errorf("repos = %v", entry.Repos)
	}
}

// TestRunSessionEnd_Dedupe verifies that two calls with the same source
// transcript produce exactly one queue file (idempotent deduplication).
func TestRunSessionEnd_Dedupe(t *testing.T) {
	jeffHome := t.TempDir()
	homeDir := t.TempDir()

	// Create a stable source transcript.
	srcDir := filepath.Join(homeDir, "session-data")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	srcTranscript := filepath.Join(srcDir, "transcript.jsonl")
	if err := os.WriteFile(srcTranscript, []byte(`{"msg":"hello"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// First call.
	if err := RunSessionEnd(jeffHome, "gig-dd1", "jenko", nil, "claude", srcTranscript, "done"); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Second call with same source transcript.
	if err := RunSessionEnd(jeffHome, "gig-dd1", "jenko", nil, "claude", srcTranscript, "done"); err != nil {
		t.Fatalf("second call: %v", err)
	}

	// Verify: exactly one queue file.
	dir := memory.QueueSessionsRoot(jeffHome)
	files, _ := os.ReadDir(dir)
	var count int
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".json") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 queue file after two calls with same source, got %d", count)
	}
}
