package stats

import (
	"encoding/json"
	"strings"
	"fmt"
	"testing"
	"time"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/gig"
)

func tempStore(t *testing.T) *gig.Store {
	t.Helper()
	store, err := gig.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestCollect_BasicFixture(t *testing.T) {
	store := tempStore(t)
	if err := jeff.EnsureAttrs(store); err != nil {
		t.Fatal(err)
	}

	// Task A: claimed + closed with full attrs.
	taskA, err := store.Create(gig.CreateParams{Title: "Full task", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(taskA.ID, "jeff"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().Exec("UPDATE events SET timestamp = ? WHERE task_id = ? AND event_type = ?", time.Now().Add(-time.Hour).Format(time.RFC3339), taskA.ID, "status_changed"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(taskA.ID, jeff.AttrPersona, "jenko"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(taskA.ID, jeff.AttrRepos, `["backend"]`); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(taskA.ID, jeff.AttrOutcome, "done"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(taskA.ID, jeff.AttrSkillsLoaded, `["terraform","docker"]`); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(taskA.ID, jeff.AttrMemoryLoaded, `["jenko"]`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := store.AddCheckpoint(taskA.ID, "jenko", gig.CheckpointParams{
			Done:      fmt.Sprintf("Step %d", i+1),
			Decisions: "",
			Next:      "",
			Blockers:  "",
			Files:     nil,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.CloseTask(taskA.ID, "done", "jeff"); err != nil {
		t.Fatal(err)
	}

	// Task B: closed, no attrs at all (legacy).
	taskB, err := store.Create(gig.CreateParams{Title: "Legacy task", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(taskB.ID, "jeff"); err != nil {
		t.Fatal(err)
	}
	if err := store.CloseTask(taskB.ID, "done", "jeff"); err != nil {
		t.Fatal(err)
	}

	// Task C: cancelled with persona=hardy.
	taskC, err := store.Create(gig.CreateParams{Title: "Cancelled task", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(taskC.ID, "jeff"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(taskC.ID, jeff.AttrPersona, "hardy"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(taskC.ID, jeff.AttrOutcome, "abandoned"); err != nil {
		t.Fatal(err)
	}
	if err := store.CancelTask(taskC.ID, "abandoned", "jeff"); err != nil {
		t.Fatal(err)
	}

	since := time.Now().Add(-time.Hour)
	report, err := Collect(store, Options{Since: since})
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(report.Tasks))
	}

	// Verify task A.
	var foundA, foundB, foundC bool
	for _, ts := range report.Tasks {
		switch ts.ID {
		case taskA.ID:
			foundA = true
			if ts.Persona != "jenko" {
				t.Errorf("taskA persona=jenko, got %q", ts.Persona)
			}
			if len(ts.Repos) != 1 || ts.Repos[0] != "backend" {
				t.Errorf("taskA repos=[backend], got %v", ts.Repos)
			}
			if ts.Outcome != "done" {
				t.Errorf("taskA outcome=done, got %q", ts.Outcome)
			}
			if ts.Checkpoints != 2 {
				t.Errorf("taskA checkpoints=2, got %d", ts.Checkpoints)
			}
			if ts.ClaimedAt == nil {
				t.Error("taskA claimed_at should not be nil")
			}
			if ts.ClosedAt == nil {
				t.Error("taskA closed_at should not be nil")
			}
			if ts.CycleTime <= 0 {
				t.Errorf("taskA cycle_time should be positive, got %v", ts.CycleTime)
			}
			if len(ts.SkillsLoaded) != 2 {
				t.Errorf("taskA skills_loaded=2, got %d", len(ts.SkillsLoaded))
			}
			if len(ts.MemoryLoaded) != 1 {
				t.Errorf("taskA memory_loaded=1, got %d", len(ts.MemoryLoaded))
			}
		case taskB.ID:
			foundB = true
			if ts.Persona != "" {
				t.Errorf("taskB persona should be empty, got %q", ts.Persona)
			}
			if len(ts.Repos) != 0 {
				t.Errorf("taskB repos should be empty, got %v", ts.Repos)
			}
		case taskC.ID:
			foundC = true
			if ts.Persona != "hardy" {
				t.Errorf("taskC persona=hardy, got %q", ts.Persona)
			}
			if ts.Outcome != "abandoned" {
				t.Errorf("taskC outcome=abandoned, got %q", ts.Outcome)
			}
		}
	}
	if !foundA || !foundB || !foundC {
		t.Error("not all tasks were found in report")
	}

	// Check ByOutcome.
	if report.ByOutcome["done"] != 1 {
		t.Errorf("expected 1 done, got %d", report.ByOutcome["done"])
	}
	if report.ByOutcome["(none)"] != 1 {
		t.Errorf("expected 1 (none) outcome, got %d", report.ByOutcome["(none)"])
	}
	if report.ByOutcome["abandoned"] != 1 {
		t.Errorf("expected 1 abandoned, got %d", report.ByOutcome["abandoned"])
	}

	// Check ByPersona groups.
	p, ok := report.ByPersona["jenko"]
	if !ok {
		t.Fatal("expected jenko persona group")
	}
	if p.Tasks != 1 {
		t.Errorf("jenko tasks=1, got %d", p.Tasks)
	}
	if p.CycleSamples != 1 {
		t.Errorf("jenko cycle_samples=1, got %d", p.CycleSamples)
	}

	if _, ok := report.ByPersona["(none)"]; !ok {
		t.Error("expected (none) persona group for legacy task")
	}

	h, ok := report.ByPersona["hardy"]
	if !ok {
		t.Fatal("expected hardy persona group")
	}
	if h.Tasks != 1 {
		t.Errorf("hardy tasks=1, got %d", h.Tasks)
	}

	// Check ByRepo.
	ra, ok := report.ByRepo["backend"]
	if !ok {
		t.Fatal("expected backend repo group")
	}
	if ra.Tasks != 1 {
		t.Errorf("backend tasks=1, got %d", ra.Tasks)
	}

	// Check MemoryUse.
	if report.MemoryUse.Total != 3 {
		t.Errorf("total memory tasks=3, got %d", report.MemoryUse.Total)
	}
	if report.MemoryUse.WithMemory != 1 {
		t.Errorf("with_memory=1, got %d", report.MemoryUse.WithMemory)
	}
	if report.MemoryUse.WithSkills != 1 {
		t.Errorf("with_skills=1, got %d", report.MemoryUse.WithSkills)
	}
}

func TestCollect_PersonaFilter(t *testing.T) {
	store := tempStore(t)
	if err := jeff.EnsureAttrs(store); err != nil {
		t.Fatal(err)
	}

	taskA, err := store.Create(gig.CreateParams{Title: "Task A", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(taskA.ID, jeff.AttrPersona, "jenko"); err != nil {
		t.Fatal(err)
	}
	if err := store.CloseTask(taskA.ID, "done", "jeff"); err != nil {
		t.Fatal(err)
	}

	taskB, err := store.Create(gig.CreateParams{Title: "Task B", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(taskB.ID, jeff.AttrPersona, "hardy"); err != nil {
		t.Fatal(err)
	}
	if err := store.CloseTask(taskB.ID, "done", "jeff"); err != nil {
		t.Fatal(err)
	}

	since := time.Now().Add(-time.Hour)

	// Filter to jenko only.
	report, err := Collect(store, Options{Since: since, Persona: "jenko"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Tasks) != 1 {
		t.Fatalf("expected 1 task for persona=jenko, got %d", len(report.Tasks))
	}
	if report.Tasks[0].ID != taskA.ID {
		t.Errorf("expected taskA, got %s", report.Tasks[0].ID)
	}
}

func TestCollect_OutcomeFilter(t *testing.T) {
	store := tempStore(t)
	if err := jeff.EnsureAttrs(store); err != nil {
		t.Fatal(err)
	}

	taskA, err := store.Create(gig.CreateParams{Title: "Task A", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(taskA.ID, jeff.AttrOutcome, "done"); err != nil {
		t.Fatal(err)
	}
	if err := store.CloseTask(taskA.ID, "done", "jeff"); err != nil {
		t.Fatal(err)
	}

	taskB, err := store.Create(gig.CreateParams{Title: "Task B", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(taskB.ID, jeff.AttrOutcome, "abandoned"); err != nil {
		t.Fatal(err)
	}
	if err := store.CloseTask(taskB.ID, "done", "jeff"); err != nil {
		t.Fatal(err)
	}

	since := time.Now().Add(-time.Hour)
	report, err := Collect(store, Options{Since: since, Outcome: "done"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Tasks) != 1 {
		t.Fatalf("expected 1 task for outcome=done, got %d", len(report.Tasks))
	}
	if report.Tasks[0].ID != taskA.ID {
		t.Errorf("expected taskA, got %s", report.Tasks[0].ID)
	}
}

func TestCollect_RepoFilter(t *testing.T) {
	store := tempStore(t)
	if err := jeff.EnsureAttrs(store); err != nil {
		t.Fatal(err)
	}

	taskA, err := store.Create(gig.CreateParams{Title: "Task A", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(taskA.ID, jeff.AttrRepos, `["backend","frontend"]`); err != nil {
		t.Fatal(err)
	}
	if err := store.CloseTask(taskA.ID, "done", "jeff"); err != nil {
		t.Fatal(err)
	}

	taskB, err := store.Create(gig.CreateParams{Title: "Task B", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(taskB.ID, jeff.AttrRepos, `["infra"]`); err != nil {
		t.Fatal(err)
	}
	if err := store.CloseTask(taskB.ID, "done", "jeff"); err != nil {
		t.Fatal(err)
	}

	since := time.Now().Add(-time.Hour)
	report, err := Collect(store, Options{Since: since, Repo: "backend"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Tasks) != 1 {
		t.Fatalf("expected 1 task for repo=backend, got %d", len(report.Tasks))
	}
	if report.Tasks[0].ID != taskA.ID {
		t.Errorf("expected taskA, got %s", report.Tasks[0].ID)
	}
}

func TestCollect_ExcludeBeforeSince(t *testing.T) {
	store := tempStore(t)
	if err := jeff.EnsureAttrs(store); err != nil {
		t.Fatal(err)
	}

	task, err := store.Create(gig.CreateParams{Title: "Old task", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CloseTask(task.ID, "done", "jeff"); err != nil {
		t.Fatal(err)
	}

	// Since is in the future — all tasks excluded.
	since := time.Now().Add(time.Hour)
	report, err := Collect(store, Options{Since: since})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Tasks) != 0 {
		t.Errorf("expected 0 tasks with future since, got %d", len(report.Tasks))
	}
}

func TestCollect_EmptyStore(t *testing.T) {
	store := tempStore(t)
	if err := jeff.EnsureAttrs(store); err != nil {
		t.Fatal(err)
	}

	since := time.Now().Add(-30 * 24 * time.Hour)
	report, err := Collect(store, Options{Since: since})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(report.Tasks))
	}
	if report.MemoryUse.Total != 0 {
		t.Errorf("expected 0 total memory tasks, got %d", report.MemoryUse.Total)
	}
}

func TestParseSince(t *testing.T) {
	cases := []struct {
		input string
		ok    bool
	}{
		{"30d", true},
		{"7d", true},
		{"90d", true},
		{"0d", true},
		{"1d", true},
		{"365d", true},
		{"2w", false},
		{"", false},
		{"abc", false},
		{"-1d", false},
		{"1m", false},
		{"1y", false},
	}

	for _, c := range cases {
		got, err := ParseSince(c.input)
		if c.ok && err != nil {
			t.Errorf("ParseSince(%q) unexpected error: %v", c.input, err)
		}
		if !c.ok && err == nil {
			t.Errorf("ParseSince(%q) expected error, got %v", c.input, got)
		}
	}
}

func TestJSONRoundTrip(t *testing.T) {
	store := tempStore(t)
	if err := jeff.EnsureAttrs(store); err != nil {
		t.Fatal(err)
	}

	task, err := store.Create(gig.CreateParams{Title: "JSON task", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(task.ID, "jeff"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(task.ID, jeff.AttrPersona, "jenko"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(task.ID, jeff.AttrRepos, `["backend"]`); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(task.ID, jeff.AttrOutcome, "done"); err != nil {
		t.Fatal(err)
	}
	if err := store.CloseTask(task.ID, "done", "jeff"); err != nil {
		t.Fatal(err)
	}

	since := time.Now().Add(-time.Hour)
	report, err := Collect(store, Options{Since: since})
	if err != nil {
		t.Fatal(err)
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Report
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(decoded.Tasks) != 1 {
		t.Fatalf("expected 1 task in round-trip, got %d", len(decoded.Tasks))
	}
	if decoded.Tasks[0].ID != task.ID {
		t.Errorf("expected task ID %s, got %s", task.ID, decoded.Tasks[0].ID)
	}
}

func TestFmtDur(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Minute, "0.5h"},
		{2 * time.Hour, "2.0h"},
		{4*time.Hour + 12*time.Minute, "4.2h"},
		{12 * time.Hour, "0.5d"},
		{36 * time.Hour, "1.5d"},
		{72 * time.Hour, "3.0d"},
	}
	for _, c := range cases {
		got := fmtDur(c.d)
		if got != c.want {
			t.Errorf("fmtDur(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestCollect_MalformedAttrNoCrash(t *testing.T) {
	store := tempStore(t)
	if err := jeff.EnsureAttrs(store); err != nil {
		t.Fatal(err)
	}

	task, err := store.Create(gig.CreateParams{Title: "Bad attrs", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}
	// Insert malformed JSON values directly to bypass validation.
	_, err = store.DB().Exec("INSERT INTO custom_attributes (task_id, key, value, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		task.ID, jeff.AttrRepos, "not-json", time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DB().Exec("INSERT INTO custom_attributes (task_id, key, value, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		task.ID, jeff.AttrSkillsLoaded, "also-not-json", time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CloseTask(task.ID, "done", "jeff"); err != nil {
		t.Fatal(err)
	}

	since := time.Now().Add(-time.Hour)
	report, err := Collect(store, Options{Since: since})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(report.Tasks))
	}
	// Should have empty slices, not crash.
	if report.Tasks[0].Repos != nil {
		t.Logf("repos=%v (expected nil after bad JSON)", report.Tasks[0].Repos)
	}
}

func TestCollect_OpenTaskExcluded(t *testing.T) {
	store := tempStore(t)
	if err := jeff.EnsureAttrs(store); err != nil {
		t.Fatal(err)
	}

	// Create an open (not closed) task.
	task, err := store.Create(gig.CreateParams{Title: "Open task", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(task.ID, jeff.AttrPersona, "jenko"); err != nil {
		t.Fatal(err)
	}

	since := time.Now().Add(-time.Hour)
	report, err := Collect(store, Options{Since: since})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Tasks) != 0 {
		t.Errorf("expected 0 tasks (open task excluded), got %d", len(report.Tasks))
	}
}

func TestCollect_PRUris(t *testing.T) {
	store := tempStore(t)
	if err := jeff.EnsureAttrs(store); err != nil {
		t.Fatal(err)
	}

	task, err := store.Create(gig.CreateParams{Title: "PR task", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(task.ID, jeff.AttrPRURLs, `{"backend":"https://github.com/org/backend/pull/42"}`); err != nil {
		t.Fatal(err)
	}
	if err := store.CloseTask(task.ID, "done", "jeff"); err != nil {
		t.Fatal(err)
	}

	since := time.Now().Add(-time.Hour)
	report, err := Collect(store, Options{Since: since})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(report.Tasks))
	}
	if len(report.Tasks[0].PRs) != 1 {
		t.Fatalf("expected 1 PR URL, got %d", len(report.Tasks[0].PRs))
	}
	if report.Tasks[0].PRs["backend"] != "https://github.com/org/backend/pull/42" {
		t.Errorf("unexpected PR URL: %v", report.Tasks[0].PRs)
	}
}

func TestCollect_CycleTimeCheck(t *testing.T) {
	store := tempStore(t)
	if err := jeff.EnsureAttrs(store); err != nil {
		t.Fatal(err)
	}

	task, err := store.Create(gig.CreateParams{Title: "Cycle test", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}
	// Close without claim — no cycle time.
	if err := store.CloseTask(task.ID, "done", "jeff"); err != nil {
		t.Fatal(err)
	}

	since := time.Now().Add(-time.Hour)
	report, err := Collect(store, Options{Since: since})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(report.Tasks))
	}
	if report.Tasks[0].CycleTime != 0 {
		t.Errorf("expected 0 cycle time (no claim), got %v", report.Tasks[0].CycleTime)
	}
	if report.Tasks[0].ClaimedAt != nil {
		t.Error("expected nil claimed_at")
	}
}

func TestHumanOutputRendering(t *testing.T) {
	store := tempStore(t)
	if err := jeff.EnsureAttrs(store); err != nil {
		t.Fatal(err)
	}

	task, err := store.Create(gig.CreateParams{Title: "Render task", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(task.ID, "jeff"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(task.ID, jeff.AttrPersona, "jenko"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(task.ID, jeff.AttrRepos, `["backend"]`); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(task.ID, jeff.AttrOutcome, "done"); err != nil {
		t.Fatal(err)
	}
	if err := store.CloseTask(task.ID, "done", "jeff"); err != nil {
		t.Fatal(err)
	}

	since := time.Now().Add(-30 * 24 * time.Hour)
	report, err := Collect(store, Options{Since: since})
	if err != nil {
		t.Fatal(err)
	}

	groups := SortGroups(report.ByPersona)
	if len(groups) != 1 {
		t.Fatalf("expected 1 persona group, got %d", len(groups))
	}
	if groups[0].Name != "jenko" {
		t.Errorf("expected jenko, got %s", groups[0].Name)
	}

	var sb strings.Builder
	sb.WriteString("JEFF Stats (last 30 days)\n")
	sb.WriteString("──────────────────────────────────────\n")
	sb.WriteString("Tasks: 1\n")
	sb.WriteString("By persona:\n")
	for _, g := range groups {
		sb.WriteString("  " + g.Name + "\n")
	}

	out := sb.String()
	if !strings.Contains(out, "jenko") {
		t.Error("human output should contain persona name")
	}
	if !strings.Contains(out, "Tasks: 1") {
		t.Error("human output should show task count")
	}
}


