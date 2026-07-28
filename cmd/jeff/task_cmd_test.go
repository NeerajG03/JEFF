package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/gig"
)

// --- resolveGigHome precedence (D2) ---

func TestResolveGigHome_GIG_HOMEEnvWins(t *testing.T) {
	gh := t.TempDir()
	t.Setenv("GIG_HOME", gh)

	cfg := &jeff.Config{GigHome: "/some/jeff-configured-path"}
	got := resolveGigHome(cfg)
	if got != gh {
		t.Errorf("expected GIG_HOME env (%s), got %s", gh, got)
	}
}

func TestResolveGigHome_ConfigSecond(t *testing.T) {
	gh := t.TempDir()
	cfg := &jeff.Config{GigHome: gh}
	got := resolveGigHome(cfg)
	if got != gh {
		t.Errorf("expected cfg.GigHome (%s), got %s", gh, got)
	}
}

func TestResolveGigHome_GigDefaultFallback(t *testing.T) {
	cfg := &jeff.Config{}
	got := resolveGigHome(cfg)
	want := gig.DefaultGigHome()
	if got != want {
		t.Errorf("expected gig default (%s), got %s", want, got)
	}
}

func TestResolveGigHome_NilCfgFallback(t *testing.T) {
	got := resolveGigHome(nil)
	want := gig.DefaultGigHome()
	if got != want {
		t.Errorf("expected gig default (%s) for nil cfg, got %s", want, got)
	}
}

func TestGigHomePrecedence_HooksAndCLIIdentical(t *testing.T) {
	cases := []struct {
		name     string
		setup    func(t *testing.T) *jeff.Config
		wantHome string
	}{
		{
			name: "GIG_HOME env",
			setup: func(t *testing.T) *jeff.Config {
				gh := t.TempDir()
				t.Setenv("GIG_HOME", gh)
				return &jeff.Config{GigHome: "/ignored/when/env/set"}
			},
		},
		{
			name: "jeff.json gig_home",
			setup: func(t *testing.T) *jeff.Config {
				gh := t.TempDir()
				return &jeff.Config{GigHome: gh}
			},
		},
		{
			name: "gig default",
			setup: func(t *testing.T) *jeff.Config {
				return &jeff.Config{}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.setup(t)
			cliResolved := resolveGigHome(cfg)
			hookResolved := resolveGigHome(cfg)
			if cliResolved != hookResolved {
				t.Errorf("CLI resolved %q but hooks resolved %q", cliResolved, hookResolved)
			}
			if tc.name == "GIG_HOME env" {
				if cliResolved == "/ignored/when/env/set" {
					t.Error("GIG_HOME env was ignored in favour of jeff.json gig_home")
				}
			}
			t.Logf("resolved gig home: %s", cliResolved)
		})
	}
}

// --- isGigStoreInitialized (D4) ---

func TestIsGigStoreInitialized_True(t *testing.T) {
	gh := t.TempDir()
	cfg := &jeff.Config{GigHome: gh}
	if err := os.WriteFile(filepath.Join(gh, "gig.yaml"), []byte("prefix: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isGigStoreInitialized(cfg) {
		t.Error("expected true for dir with gig.yaml")
	}
}

func TestIsGigStoreInitialized_False(t *testing.T) {
	gh := t.TempDir()
	cfg := &jeff.Config{GigHome: gh}
	if isGigStoreInitialized(cfg) {
		t.Error("expected false for empty dir")
	}
}

// --- task new (D1 + D4) ---

func TestTaskNew_PrintsIDOnOwnLine(t *testing.T) {
	gh := setupTestGigStore(t, "jefftest")
	t.Setenv("GIG_HOME", gh)
	setupTestConfig(t, gh)

	cmd := taskNewCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"test-task"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task new: %v", err)
	}

	id := strings.TrimSpace(buf.String())
	if !strings.HasPrefix(id, "jefftest-") {
		t.Errorf("expected id with 'jefftest-' prefix, got %q", id)
	}
}

func TestTaskNew_JSONShape(t *testing.T) {
	gh := setupTestGigStore(t, "gig")
	t.Setenv("GIG_HOME", gh)
	setupTestConfig(t, gh)

	cmd := taskNewCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"json-task", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task new --json: %v", err)
	}

	var task gig.Task
	if err := json.Unmarshal(buf.Bytes(), &task); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if task.Title != "json-task" {
		t.Errorf("title = %q, want json-task", task.Title)
	}
	if !strings.HasPrefix(task.ID, "gig-") {
		t.Errorf("id = %q, want gig- prefix", task.ID)
	}
	if task.Type != gig.TypeTask {
		t.Errorf("type = %q, want task (default)", task.Type)
	}
}

func TestTaskNew_UninitializedStore(t *testing.T) {
	gh := t.TempDir()
	cfg = &jeff.Config{GigHome: gh}
	t.Cleanup(func() { cfg = nil })

	cmd := taskNewCmd()
	cmd.SetArgs([]string{"should-fail"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for uninitialized store")
	}
	if !strings.Contains(err.Error(), "gig store not initialized") {
		t.Errorf("error message missing expected text: %v", err)
	}
}

func TestTaskNew_WithFlags(t *testing.T) {
	gh := setupTestGigStore(t, "gig")
	t.Setenv("GIG_HOME", gh)
	setupTestConfig(t, gh)

	cmd := taskNewCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"feature-task", "--type", "feature", "--priority", "1", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task new with flags: %v", err)
	}

	var task gig.Task
	if err := json.Unmarshal(buf.Bytes(), &task); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if task.Type != gig.TypeFeature {
		t.Errorf("type = %q, want feature", task.Type)
	}
	if task.Priority != gig.P1 {
		t.Errorf("priority = %d, want P1(1)", task.Priority)
	}
	if task.Title != "feature-task" {
		t.Errorf("title = %q, want feature-task", task.Title)
	}
}

// --- task list (D1) ---

func TestTaskList_Empty(t *testing.T) {
	gh := setupTestGigStore(t, "gig")
	t.Setenv("GIG_HOME", gh)
	setupTestConfig(t, gh)

	cmd := taskListCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list: %v", err)
	}

	if !strings.Contains(buf.String(), "No tasks found") {
		t.Errorf("expected 'No tasks found.', got %q", buf.String())
	}
}

func TestTaskList_ShowsTasks(t *testing.T) {
	gh := setupTestGigStore(t, "gig")
	t.Setenv("GIG_HOME", gh)
	setupTestConfig(t, gh)

	store, err := openGigStore(cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	task, err := store.Create(gig.CreateParams{Title: "list-test"})
	if err != nil {
		store.Close()
		t.Fatalf("create task: %v", err)
	}
	store.Close()

	cmd := taskListCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, task.ID) {
		t.Errorf("output missing task ID %s: %s", task.ID, out)
	}
	if !strings.Contains(out, "list-test") {
		t.Errorf("output missing task title: %s", out)
	}
}

func TestTaskList_Ready(t *testing.T) {
	gh := setupTestGigStore(t, "gig")
	t.Setenv("GIG_HOME", gh)
	setupTestConfig(t, gh)

	store, err := openGigStore(cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	task, err := store.Create(gig.CreateParams{Title: "ready-task"})
	if err != nil {
		store.Close()
		t.Fatalf("create task: %v", err)
	}
	store.Close()

	cmd := taskListCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--ready"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --ready: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, task.ID) {
		t.Errorf("--ready output missing task ID %s: %s", task.ID, out)
	}
}

func TestTaskList_JSON(t *testing.T) {
	gh := setupTestGigStore(t, "gig")
	t.Setenv("GIG_HOME", gh)
	setupTestConfig(t, gh)

	store, err := openGigStore(cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	_, err = store.Create(gig.CreateParams{Title: "json-list-test"})
	if err != nil {
		store.Close()
		t.Fatalf("create task: %v", err)
	}
	store.Close()

	cmd := taskListCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --json: %v", err)
	}

	var tasks []*gig.Task
	if err := json.Unmarshal(buf.Bytes(), &tasks); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if len(tasks) == 0 {
		t.Error("expected at least 1 task in JSON output")
	}
}

// --- task show (D1) ---

func TestTaskShow_Text(t *testing.T) {
	gh := setupTestGigStore(t, "gig")
	t.Setenv("GIG_HOME", gh)
	setupTestConfig(t, gh)

	store, err := openGigStore(cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	task, err := store.Create(gig.CreateParams{
		Title:       "show-test",
		Description: "a test task for show",
		Priority:    gig.P0,
	})
	if err != nil {
		store.Close()
		t.Fatalf("create task: %v", err)
	}
	store.Close()

	cmd := taskShowCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{task.ID})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task show: %v", err)
	}

	out := buf.String()
	for _, want := range []string{task.ID, "show-test", "a test task for show"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %s", want, out)
		}
	}
}

func TestTaskShow_JSON(t *testing.T) {
	gh := setupTestGigStore(t, "gig")
	t.Setenv("GIG_HOME", gh)
	setupTestConfig(t, gh)

	store, err := openGigStore(cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	task, err := store.Create(gig.CreateParams{Title: "json-show-test"})
	if err != nil {
		store.Close()
		t.Fatalf("create task: %v", err)
	}
	store.Close()

	cmd := taskShowCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{task.ID, "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task show --json: %v", err)
	}

	var taskOut gig.Task
	if err := json.Unmarshal(buf.Bytes(), &taskOut); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if taskOut.ID != task.ID {
		t.Errorf("id = %q, want %q", taskOut.ID, task.ID)
	}
	if taskOut.Title != "json-show-test" {
		t.Errorf("title = %q, want json-show-test", taskOut.Title)
	}
}

func TestTaskShow_NotFound(t *testing.T) {
	gh := setupTestGigStore(t, "gig")
	t.Setenv("GIG_HOME", gh)
	setupTestConfig(t, gh)

	cmd := taskShowCmd()
	cmd.SetArgs([]string{"gig-nonexistent"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

// --- helpers ---

func setupTestGigStore(t *testing.T, prefix string) string {
	t.Helper()
	gh := t.TempDir()
	dbPath := filepath.Join(gh, "gig.db")
	content := "prefix: " + prefix + "\ndb_path: " + dbPath + "\nhash_length: 4\n"
	if err := os.WriteFile(filepath.Join(gh, "gig.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return gh
}

func setupTestConfig(t *testing.T, gh string) {
	t.Helper()
	cfg = &jeff.Config{
		Home:    t.TempDir(),
		GigHome: gh,
	}
	jeff.SetOpenCodeModelAliases(nil)
	t.Cleanup(func() { cfg = nil })
}
