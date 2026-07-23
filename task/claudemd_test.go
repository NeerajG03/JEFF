package task

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/memory"
	"github.com/NeerajG03/JEFF/workspace"
	"github.com/NeerajG03/gig"
)

// initGitRepoOnBranch initializes a real git repo at dir checked out on branch,
// with one empty commit, so ListTaskWorktrees can resolve the branch via
// `git rev-parse --abbrev-ref HEAD`.
func initGitRepoOnBranch(t *testing.T, dir, branch string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", branch},
		{"config", "user.email", "t@t"},
		{"config", "user.name", "t"},
		{"commit", "--allow-empty", "-q", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func TestWriteClaudeMD_NoPersona(t *testing.T) {
	dir := t.TempDir()
	task := &gig.Task{
		ID:       "gig-ab12",
		Title:    "Test task",
		Priority: gig.P1,
	}

	if err := WriteClaudeMD(dir, t.TempDir(), nil, task, "", nil); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, "gig-ab12") {
		t.Error("missing task ID")
	}
	if !strings.Contains(content, "Test task") {
		t.Error("missing task title")
	}

	lines := strings.Split(content, "\n")
	if lines[0] != "# Current Task" {
		t.Errorf("expected first line to be task context, got %q", lines[0])
	}
}

func TestWriteClaudeMD_WithPersona(t *testing.T) {
	dir := t.TempDir()
	task := &gig.Task{
		ID:       "gig-cd34",
		Title:    "Persona task",
		Priority: gig.P2,
	}

	if err := WriteClaudeMD(dir, t.TempDir(), nil, task, "jenko", nil); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)

	if !strings.Contains(content, "---") {
		t.Error("missing persona divider")
	}
	if !strings.Contains(content, "gig-cd34") {
		t.Error("missing task ID")
	}
}

func TestWriteClaudeMD_InvalidPersonaSkipped(t *testing.T) {
	dir := t.TempDir()
	task := &gig.Task{
		ID:       "gig-ef56",
		Title:    "Bad persona task",
		Priority: gig.P1,
	}

	if err := WriteClaudeMD(dir, t.TempDir(), nil, task, "nonexistent", nil); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)

	if !strings.Contains(content, "gig-ef56") {
		t.Error("missing task ID")
	}
	lines := strings.Split(content, "\n")
	if lines[0] != "# Current Task" {
		t.Errorf("expected task context first, got %q", lines[0])
	}
}

func TestWriteClaudeMD_WithDescription(t *testing.T) {
	dir := t.TempDir()
	task := &gig.Task{
		ID:          "gig-gh78",
		Title:       "Described task",
		Description: "Some detailed description",
		Priority:    gig.P1,
		ParentID:    "gig-parent",
	}

	if err := WriteClaudeMD(dir, t.TempDir(), nil, task, "", nil); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)

	if !strings.Contains(content, "Some detailed description") {
		t.Error("missing description")
	}
	if !strings.Contains(content, "**Parent:** gig-parent") {
		t.Error("missing parent")
	}
}

func TestWriteClaudeMD_NoDescriptionOmitted(t *testing.T) {
	dir := t.TempDir()
	task := &gig.Task{
		ID:       "gig-ij90",
		Title:    "No desc task",
		Priority: gig.P2,
	}

	if err := WriteClaudeMD(dir, t.TempDir(), nil, task, "", nil); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)

	if strings.Contains(content, "**Description:**") {
		t.Error("description line should be omitted when empty")
	}
	if strings.Contains(content, "**Parent:**") {
		t.Error("parent line should be omitted when empty")
	}
}

func TestWriteClaudeMD_NoWorktrees(t *testing.T) {
	dir := t.TempDir()
	task := &gig.Task{
		ID:       "gig-aa11",
		Title:    "No worktrees",
		Priority: gig.P2,
		Type:     gig.TypeTask,
	}

	if err := WriteClaudeMD(dir, t.TempDir(), nil, task, "", nil); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)

	if strings.Contains(content, "## Workspace") {
		t.Error("workspace section should not appear when there are no worktrees")
	}
}

func TestWriteClaudeMD_WithWorktrees(t *testing.T) {
	dir := t.TempDir()

	// Real git repos on branch gig-bb22 so ListTaskWorktrees resolves the branch.
	wtFrontend := filepath.Join(t.TempDir(), "frontend", "gig-bb22")
	initGitRepoOnBranch(t, wtFrontend, "gig-bb22")
	os.Symlink(wtFrontend, filepath.Join(dir, "frontend"))

	wtBackend := filepath.Join(t.TempDir(), "backend", "gig-bb22")
	initGitRepoOnBranch(t, wtBackend, "gig-bb22")
	os.Symlink(wtBackend, filepath.Join(dir, "backend"))

	task := &gig.Task{
		ID:       "gig-bb22",
		Title:    "With worktrees",
		Priority: gig.P1,
		Type:     gig.TypeFeature,
	}

	if err := WriteClaudeMD(dir, t.TempDir(), nil, task, "", nil); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)

	if !strings.Contains(content, "## Workspace") {
		t.Fatal("missing workspace section")
	}
	if !strings.Contains(content, "frontend/") {
		t.Error("missing frontend worktree")
	}
	if !strings.Contains(content, "backend/") {
		t.Error("missing backend worktree")
	}
	if !strings.Contains(content, "(branch: gig-bb22)") {
		t.Error("missing branch name")
	}
}

func TestWriteClaudeMD_WorktreeAddedLater(t *testing.T) {
	dir := t.TempDir()
	task := &gig.Task{
		ID:       "gig-cc33",
		Title:    "Incremental worktrees",
		Priority: gig.P2,
		Type:     gig.TypeTask,
	}

	// First write — no worktrees.
	if err := WriteClaudeMD(dir, t.TempDir(), nil, task, "", nil); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if strings.Contains(string(data), "## Workspace") {
		t.Fatal("workspace section should not exist yet")
	}

	// Simulate adding a worktree symlink (real git repo on branch gig-cc33).
	wtDir := filepath.Join(t.TempDir(), "api", "gig-cc33")
	initGitRepoOnBranch(t, wtDir, "gig-cc33")
	os.Symlink(wtDir, filepath.Join(dir, "api"))

	// Rewrite — should now include workspace.
	if err := WriteClaudeMD(dir, t.TempDir(), nil, task, "", nil); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)

	if !strings.Contains(content, "## Workspace") {
		t.Fatal("missing workspace section after adding worktree")
	}
	if !strings.Contains(content, "api/") {
		t.Error("missing api worktree")
	}
	if !strings.Contains(content, "(branch: gig-cc33)") {
		t.Error("missing branch name")
	}
}

func TestWriteClaudeMD_CheckpointRendered(t *testing.T) {
	dir := t.TempDir()
	store, err := gig.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	task, err := store.Create(gig.CreateParams{Title: "Checkpoint task", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.AddCheckpoint(task.ID, "jenko", gig.CheckpointParams{
		Done:      "Implemented attrs",
		Decisions: "Used a map for dedup",
		Next:      "Write tests",
		Blockers:  "None",
		Files:     []string{"attrs.go", "attrs_test.go"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := WriteClaudeMD(dir, t.TempDir(), store, task, "", nil); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)

	if !strings.Contains(content, "## Resuming: Last Checkpoint") {
		t.Error("missing checkpoint section")
	}
	for _, want := range []string{"Implemented attrs", "Used a map for dedup", "Write tests", "attrs.go, attrs_test.go"} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in checkpoint section", want)
		}
	}
}

func TestWriteClaudeMD_NoCheckpointNoSection(t *testing.T) {
	dir := t.TempDir()
	store, err := gig.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	task, err := store.Create(gig.CreateParams{Title: "No checkpoint task", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}

	if err := WriteClaudeMD(dir, t.TempDir(), store, task, "", nil); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if strings.Contains(string(data), "## Resuming: Last Checkpoint") {
		t.Error("checkpoint section should not appear with no checkpoints")
	}
}

func TestWriteClaudeMD_NilStoreNoPanicNoSection(t *testing.T) {
	dir := t.TempDir()
	task := &gig.Task{ID: "gig-nn01", Title: "Nil store", Priority: gig.P1}

	if err := WriteClaudeMD(dir, t.TempDir(), nil, task, "", nil); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if strings.Contains(string(data), "## Resuming: Last Checkpoint") {
		t.Error("checkpoint section should not appear when store is nil")
	}
}

func TestResolvePersona_PrefersAttrOverDetect(t *testing.T) {
	store, err := gig.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := jeff.EnsureAttrs(store); err != nil {
		t.Fatal(err)
	}

	task, err := store.Create(gig.CreateParams{Title: "Persona attr task", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttr(task.ID, jeff.AttrPersona, "jenko"); err != nil {
		t.Fatal(err)
	}

	// CLAUDE.md on disk says "schmidt" — the attr must win.
	dir := t.TempDir()
	if err := WriteClaudeMD(dir, t.TempDir(), nil, task, "schmidt", nil); err != nil {
		t.Fatal(err)
	}

	if got := ResolvePersona(store, task.ID, dir); got != "jenko" {
		t.Errorf("expected attr persona jenko, got %q", got)
	}
}

func TestResolvePersona_FallsBackToDetect(t *testing.T) {
	store, err := gig.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := jeff.EnsureAttrs(store); err != nil {
		t.Fatal(err)
	}

	task, err := store.Create(gig.CreateParams{Title: "Old workspace task", Type: gig.TypeTask})
	if err != nil {
		t.Fatal(err)
	}
	// No AttrPersona set — mimics a workspace created by an older binary.

	dir := t.TempDir()
	if err := WriteClaudeMD(dir, t.TempDir(), nil, task, "jenko", nil); err != nil {
		t.Fatal(err)
	}

	if got := ResolvePersona(store, task.ID, dir); got != "jenko" {
		t.Errorf("expected fallback DetectPersona jenko, got %q", got)
	}
}

func TestDetectPersona(t *testing.T) {
	// No CLAUDE.md — should return "".
	dir := t.TempDir()
	if got := DetectPersona(dir); got != "" {
		t.Errorf("expected empty, got %q", got)
	}

	// Write with persona, then detect.
	task := &gig.Task{ID: "gig-dd44", Title: "Detect test", Priority: gig.P2, Type: gig.TypeTask}
	if err := WriteClaudeMD(dir, t.TempDir(), nil, task, "jenko", nil); err != nil {
		t.Fatal(err)
	}
	if got := DetectPersona(dir); got != "jenko" {
		t.Errorf("expected jenko, got %q", got)
	}

	// Write without persona, should return "".
	dir2 := t.TempDir()
	if err := WriteClaudeMD(dir2, t.TempDir(), nil, task, "", nil); err != nil {
		t.Fatal(err)
	}
	if got := DetectPersona(dir2); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestWriteClaudeMD_LabelsAndType(t *testing.T) {
	dir := t.TempDir()
	task := &gig.Task{
		ID:       "gig-ee55",
		Title:    "Full fields",
		Priority: gig.P0,
		Type:     gig.TypeBug,
		Labels:   []string{"urgent", "backend"},
		ParentID: "gig-parent",
	}

	if err := WriteClaudeMD(dir, t.TempDir(), nil, task, "", nil); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)

	for _, want := range []string{"bug", "urgent, backend", "gig-parent", "P0"} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in CLAUDE.md", want)
		}
	}
}

func TestWriteClaudeMD_WithPersonaMemory(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()

	// Populate persona memory.
	memory.EnsurePersonaDir(home, "jenko")
	md := "# Jenko Memory\n\n- [style](style.md) — early returns over nested ifs\n"
	os.WriteFile(filepath.Join(memory.PersonaMemoryDir(home, "jenko"), "MEMORY.md"), []byte(md), 0o644)

	task := &gig.Task{ID: "gig-mm01", Title: "Memory test", Priority: gig.P1}
	if err := WriteClaudeMD(dir, home, nil, task, "jenko", nil); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)

	if !strings.Contains(content, "## Persona Memory") {
		t.Error("missing Persona Memory section")
	}
	if !strings.Contains(content, "early returns") {
		t.Error("persona memory content not injected")
	}
	if !strings.Contains(content, "Detail files:") {
		t.Error("missing detail files path")
	}
}

func TestWriteClaudeMD_EmptyPersonaMemory(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()

	// Ensure dir exists but only has seed content.
	memory.EnsurePersonaDir(home, "jenko")

	task := &gig.Task{ID: "gig-mm02", Title: "Empty memory", Priority: gig.P1}
	if err := WriteClaudeMD(dir, home, nil, task, "jenko", nil); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)

	if strings.Contains(content, "## Persona Memory") {
		t.Error("Persona Memory section should not appear for seed-only MEMORY.md")
	}
	if !strings.Contains(content, "## Scratchpad") {
		t.Error("Scratchpad section should appear when persona is set")
	}
}

func TestWriteClaudeMD_WithRepoLearnings(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()

	// Populate repo learnings.
	memory.EnsureRepoDir(home, "backend")
	md := "# backend Learnings\n\n- [testing](testing.md) — run make migrate before tests\n"
	os.WriteFile(filepath.Join(memory.RepoLearningsDir(home, "backend"), "INDEX.md"), []byte(md), 0o644)

	task := &gig.Task{ID: "gig-mm03", Title: "Repo learnings test", Priority: gig.P2}
	if err := WriteClaudeMD(dir, home, nil, task, "", []string{"backend"}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)

	if !strings.Contains(content, "## Repo Learnings: backend") {
		t.Error("missing Repo Learnings section")
	}
	if !strings.Contains(content, "make migrate") {
		t.Error("repo learnings content not injected")
	}
}

func TestWriteClaudeMD_NoPersonaNoRepos_NoScratchpad(t *testing.T) {
	dir := t.TempDir()
	task := &gig.Task{ID: "gig-mm04", Title: "Bare task", Priority: gig.P2}

	if err := WriteClaudeMD(dir, t.TempDir(), nil, task, "", nil); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)

	if strings.Contains(content, "## Scratchpad") {
		t.Error("Scratchpad section should not appear with no persona and no repos")
	}
	if strings.Contains(content, "## Persona Memory") {
		t.Error("Persona Memory should not appear with no persona")
	}
}

func TestWriteClaudeMD_ScratchpadHasCorrectPath(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	task := &gig.Task{ID: "gig-mm05", Title: "Scratchpad path", Priority: gig.P1}

	if err := WriteClaudeMD(dir, home, nil, task, "jenko", nil); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)

	expectedPath := filepath.Join(dir, "scratchpad.md")
	if !strings.Contains(content, expectedPath) {
		t.Errorf("scratchpad section should contain path %s", expectedPath)
	}
}

func TestWriteClaudeMD_PersonaSpecificHint(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	task := &gig.Task{ID: "gig-mm06", Title: "Persona hint", Priority: gig.P1}

	// Jenko should get implementer-specific hint.
	if err := WriteClaudeMD(dir, home, nil, task, "jenko", nil); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)
	if !strings.Contains(content, "As jenko, especially capture:") {
		t.Error("missing persona-specific hint for jenko")
	}
	if !strings.Contains(content, "Code style") {
		t.Error("jenko hint should mention code style")
	}

	// Schmidt should get debugger-specific hint.
	dir2 := t.TempDir()
	if err := WriteClaudeMD(dir2, home, nil, task, "schmidt", nil); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(dir2, "CLAUDE.md"))
	content = string(data)
	if !strings.Contains(content, "As schmidt, especially capture:") {
		t.Error("missing persona-specific hint for schmidt")
	}
	if !strings.Contains(content, "Root cause") {
		t.Error("schmidt hint should mention root cause")
	}

	// No persona — no hint.
	dir3 := t.TempDir()
	if err := WriteClaudeMD(dir3, home, nil, task, "", []string{"backend"}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(dir3, "CLAUDE.md"))
	if strings.Contains(string(data), "especially capture:") {
		t.Error("no persona should mean no persona-specific hint")
	}
}

func TestDetectRepos(t *testing.T) {
	dir := t.TempDir()

	// No symlinks — empty.
	repos := DetectRepos(dir)
	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}

	// Add worktree symlinks.
	for _, name := range []string{"backend", "frontend"} {
		wtDir := filepath.Join(t.TempDir(), name, "gig-rr01")
		os.MkdirAll(wtDir, 0o755)
		os.Symlink(wtDir, filepath.Join(dir, name))
	}

	repos = DetectRepos(dir)
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
}

func TestWriteWorkspaceLayout_TreeFormat(t *testing.T) {
	dir := t.TempDir()

	// Create 3 worktrees to verify tree connectors.
	for _, name := range []string{"frontend", "backend", "infra"} {
		wtDir := filepath.Join(t.TempDir(), name, "gig-ff66")
		os.MkdirAll(wtDir, 0o755)
		os.Symlink(wtDir, filepath.Join(dir, name))
	}

	var sb strings.Builder
	writeWorkspaceLayout(&sb, dir)
	content := sb.String()

	// Last entry should use └── connector.
	lines := strings.Split(strings.TrimSpace(content), "\n")
	lastTreeLine := ""
	for _, l := range lines {
		if strings.Contains(l, "/") && (strings.Contains(l, "├") || strings.Contains(l, "└")) {
			lastTreeLine = l
		}
	}
	if !strings.HasPrefix(lastTreeLine, "└── ") {
		t.Errorf("last worktree should use └── connector, got %q", lastTreeLine)
	}

	// Non-last entries should use ├──.
	midCount := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "├── ") {
			midCount++
		}
	}
	if midCount != 2 {
		t.Errorf("expected 2 ├── entries, got %d", midCount)
	}

	fmt.Println("Generated layout:\n" + content)
}

func TestBuildTaskJSON_IncludesAttrs(t *testing.T) {
	store, err := gig.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	task, err := store.Create(gig.CreateParams{Title: "Test attrs", Type: gig.TypeFeature})
	if err != nil {
		t.Fatal(err)
	}

	// Define and set a custom attribute.
	store.DefineAttr("branch_prefix", gig.AttrString, "custom branch prefix")
	store.SetAttr(task.ID, "branch_prefix", "neeraj")

	data := buildTaskJSON(store, task)

	var parsed gig.Task
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.ID != task.ID {
		t.Errorf("expected ID %s, got %s", task.ID, parsed.ID)
	}
	if parsed.Type != gig.TypeFeature {
		t.Errorf("expected type feature, got %s", parsed.Type)
	}
	if parsed.Attrs["branch_prefix"] != "neeraj" {
		t.Errorf("expected attr branch_prefix=neeraj, got %q", parsed.Attrs["branch_prefix"])
	}

	fmt.Printf("Task JSON: %s\n", string(data))
}

func TestBuildTaskJSON_NoAttrs(t *testing.T) {
	store, err := gig.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	task, err := store.Create(gig.CreateParams{Title: "No attrs", Type: gig.TypeBug})
	if err != nil {
		t.Fatal(err)
	}

	data := buildTaskJSON(store, task)

	var parsed gig.Task
	json.Unmarshal(data, &parsed)

	if len(parsed.Attrs) != 0 {
		t.Errorf("expected empty attrs, got %v", parsed.Attrs)
	}
}

func TestResolveRepoBranch_NoConfig(t *testing.T) {
	got, err := resolveRepoBranch(nil, nil, "gig-ab12")
	if err != nil {
		t.Fatal(err)
	}
	if got != "gig-ab12" {
		t.Errorf("expected gig-ab12, got %q", got)
	}
}

func TestResolveRepoBranch_NoScript(t *testing.T) {
	rc := &jeff.RepoConfig{URL: "https://example.com"}
	got, err := resolveRepoBranch(rc, nil, "gig-cd34")
	if err != nil {
		t.Fatal(err)
	}
	if got != "gig-cd34" {
		t.Errorf("expected gig-cd34, got %q", got)
	}
}

func TestResolveRepoBranch_WithScript(t *testing.T) {
	// Create a branch naming script that uses type + custom attr.
	script := filepath.Join(t.TempDir(), "branch.sh")
	os.WriteFile(script, []byte(`#!/bin/bash
TASK=$(cat)
PREFIX=$(echo "$TASK" | jq -r '.attrs.branch_prefix // empty')
if [ -z "$PREFIX" ]; then
  PREFIX=$(echo "$TASK" | jq -r '.type // "task"')
fi
ID=$(echo "$TASK" | jq -r '.id')
echo "${PREFIX}/${ID}"
`), 0o755)

	rc := &jeff.RepoConfig{BranchName: script}

	// Test with branch_prefix attr.
	taskJSON := []byte(`{"id":"gig-ab12","type":"feature","attrs":{"branch_prefix":"neeraj"}}`)
	got, err := resolveRepoBranch(rc, taskJSON, "fallback")
	if err != nil {
		t.Fatal(err)
	}
	if got != "neeraj/gig-ab12" {
		t.Errorf("expected neeraj/gig-ab12, got %q", got)
	}

	// Test without branch_prefix attr — falls back to type.
	taskJSON = []byte(`{"id":"gig-cd34","type":"bug","attrs":{}}`)
	got, err = resolveRepoBranch(rc, taskJSON, "fallback")
	if err != nil {
		t.Fatal(err)
	}
	if got != "bug/gig-cd34" {
		t.Errorf("expected bug/gig-cd34, got %q", got)
	}
}

func TestResolveRepoBranch_E2E_WithGigStore(t *testing.T) {
	// Full end-to-end: create task in gig, set custom attr, run branch script.
	store, err := gig.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	task, err := store.Create(gig.CreateParams{Title: "Auth refactor", Type: gig.TypeFeature})
	if err != nil {
		t.Fatal(err)
	}

	store.DefineAttr("branch_prefix", gig.AttrString, "custom branch prefix")
	store.SetAttr(task.ID, "branch_prefix", "neeraj")

	// Build task JSON (same as pickup does).
	taskJSON := buildTaskJSON(store, task)

	// Create branch naming script.
	script := filepath.Join(t.TempDir(), "branch.sh")
	os.WriteFile(script, []byte(`#!/bin/bash
TASK=$(cat)
PREFIX=$(echo "$TASK" | jq -r '.attrs.branch_prefix // empty')
if [ -z "$PREFIX" ]; then
  PREFIX=$(echo "$TASK" | jq -r '.type // "task"')
fi
ID=$(echo "$TASK" | jq -r '.id')
echo "${PREFIX}/${ID}"
`), 0o755)

	rc := &jeff.RepoConfig{BranchName: script, BaseBranch: "origin/develop"}

	// Resolve branch name.
	branch, err := resolveRepoBranch(rc, taskJSON, task.ID)
	if err != nil {
		t.Fatal(err)
	}

	if branch != "neeraj/"+task.ID {
		t.Errorf("expected neeraj/%s, got %q", task.ID, branch)
	}

	// Verify base branch would come from config.
	if rc.BaseBranch != "origin/develop" {
		t.Errorf("expected origin/develop, got %q", rc.BaseBranch)
	}

	// Verify ResolveBranchName directly with the JSON to confirm attrs are in there.
	var parsed map[string]any
	json.Unmarshal(taskJSON, &parsed)
	attrs := parsed["attrs"].(map[string]any)
	if attrs["branch_prefix"] != "neeraj" {
		t.Errorf("task JSON missing branch_prefix attr")
	}

	fmt.Printf("Branch: %s (base: %s)\n", branch, rc.BaseBranch)

	// Now test without attr — should fall back to type.
	store.DeleteAttr(task.ID, "branch_prefix")
	taskJSON2 := buildTaskJSON(store, task)
	branch2, err := workspace.ResolveBranchName(script, taskJSON2, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if branch2 != "feature/"+task.ID {
		t.Errorf("expected feature/%s without attr, got %q", task.ID, branch2)
	}
}
