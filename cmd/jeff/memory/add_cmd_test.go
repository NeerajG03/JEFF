package memory

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// runAddCmd builds a fresh cobra root, runs `add <args...>`, and returns any error.
func runAddCmd(t *testing.T, args []string) error {
	t.Helper()
	root := &cobra.Command{Use: "jeff"}
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.AddCommand(newAddCmd())
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"add"}, args...))
	return root.Execute()
}

func TestAddCmd_PermissionDenied(t *testing.T) {
	// JEFF_MEMORY_CAN_ADD must NOT be set (or not "1").
	t.Setenv("JEFF_MEMORY_CAN_ADD", "")

	err := runAddCmd(t, []string{
		"--name", "test-entry",
		"--type", "feedback",
		"--description", "d",
		"--body", "b",
		"--scope", "persona:jenko",
		"--bucket", "semantic",
	})
	if err == nil {
		t.Fatal("expected permission error")
	}
	if !strings.Contains(err.Error(), "marlowe") {
		t.Errorf("expected 'marlowe' in error message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "propose") {
		t.Errorf("expected 'propose' hint in error message, got: %v", err)
	}
}

func TestAddCmd_PermissionDenied_WrongValue(t *testing.T) {
	t.Setenv("JEFF_MEMORY_CAN_ADD", "0")

	err := runAddCmd(t, []string{
		"--name", "test-entry",
		"--type", "feedback",
		"--description", "d",
		"--body", "b",
		"--scope", "persona:jenko",
		"--bucket", "semantic",
	})
	if err == nil {
		t.Fatal("expected permission error when JEFF_MEMORY_CAN_ADD=0")
	}
}

func TestAddCmd_PersonaScope_Semantic(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	t.Setenv("JEFF_MEMORY_CAN_ADD", "1")
	t.Setenv("JEFF_PERSONA", "marlowe")
	t.Setenv("JEFF_TASK_ID", "gig-curate")

	err := runAddCmd(t, []string{
		"--name", "no-mock-db",
		"--type", "feedback",
		"--description", "Use real DB in integration tests",
		"--body", "Why: prior incident where mocks masked a migration bug.",
		"--scope", "persona:jenko",
		"--bucket", "semantic",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(home, "memory", "personas", "jenko", "semantic", "no-mock-db.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected canonical entry at %s: %v", path, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read entry: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "status: accepted") {
		t.Errorf("expected status:accepted in frontmatter, got:\n%s", content)
	}
	if !strings.Contains(content, "provenance: trusted") {
		t.Errorf("expected provenance:trusted in frontmatter, got:\n%s", content)
	}
	if !strings.Contains(content, "scope: persona:jenko") {
		t.Errorf("expected scope in frontmatter, got:\n%s", content)
	}
}

func TestAddCmd_RepoScope_Procedural(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	t.Setenv("JEFF_MEMORY_CAN_ADD", "1")

	err := runAddCmd(t, []string{
		"--name", "branch-naming",
		"--type", "project",
		"--description", "Branch naming convention for jeff repo",
		"--body", "Use jenko/ prefix for all jenko branches.",
		"--scope", "repo:jeff",
		"--bucket", "procedural",
		"--persona", "marlowe",
		"--task", "gig-curate",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(home, "memory", "repos", "jeff", "procedural", "branch-naming.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected entry at %s: %v", path, err)
	}
}

func TestAddCmd_CoreBucket(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	t.Setenv("JEFF_MEMORY_CAN_ADD", "1")

	err := runAddCmd(t, []string{
		"--name", "core",
		"--type", "user",
		"--description", "Core facts about jenko",
		"--body", "jenko is the implementer persona.",
		"--scope", "persona:jenko",
		"--bucket", "core",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Core bucket writes to <scope>/core.md
	path := filepath.Join(home, "memory", "personas", "jenko", "core.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected core.md at %s: %v", path, err)
	}
}

func TestAddCmd_ProjectScope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	t.Setenv("JEFF_MEMORY_CAN_ADD", "1")

	err := runAddCmd(t, []string{
		"--name", "deadline-note",
		"--type", "project",
		"--description", "Project deadline context",
		"--body", "Merge freeze on 2026-05-10.",
		"--scope", "project:memory-epic",
		"--bucket", "episodic",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(home, "memory", "projects", "memory-epic", "episodic", "deadline-note.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected entry at %s: %v", path, err)
	}
}

func TestAddCmd_OrchestratorScope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	t.Setenv("JEFF_MEMORY_CAN_ADD", "1")

	err := runAddCmd(t, []string{
		"--name", "curate-rule",
		"--type", "feedback",
		"--description", "Curation principle",
		"--body", "Prefer persona-scoped over repo-scoped for behavioral patterns.",
		"--scope", "orchestrator",
		"--bucket", "procedural",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(home, "memory", "orchestrator", "procedural", "curate-rule.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected entry at %s: %v", path, err)
	}
}

func TestAddCmd_IndexMdCreated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	t.Setenv("JEFF_MEMORY_CAN_ADD", "1")

	err := runAddCmd(t, []string{
		"--name", "index-test",
		"--type", "feedback",
		"--description", "Testing index creation",
		"--body", "b",
		"--scope", "persona:jenko",
		"--bucket", "semantic",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	indexPath := filepath.Join(home, "memory", "personas", "jenko", "semantic", "INDEX.md")
	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("expected INDEX.md at %s: %v", indexPath, err)
	}
	data, _ := os.ReadFile(indexPath)
	if !strings.Contains(string(data), "index-test") {
		t.Errorf("INDEX.md missing entry, got:\n%s", data)
	}
}

func TestAddCmd_InvalidScope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	t.Setenv("JEFF_MEMORY_CAN_ADD", "1")

	tests := []struct {
		scope   string
		wantErr string
	}{
		{"", "scope"},
		{"badscope", "kind:name"},
		{"unknown:foo", "unknown"},
		{"persona:", "empty"},
	}
	for _, tc := range tests {
		args := []string{
			"--name", "test",
			"--type", "feedback",
			"--description", "d",
			"--body", "b",
			"--scope", tc.scope,
			"--bucket", "semantic",
		}
		err := runAddCmd(t, args)
		if err == nil {
			t.Errorf("scope %q: expected error", tc.scope)
			continue
		}
		if tc.scope != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.wantErr)) {
			t.Errorf("scope %q: expected %q in error, got: %v", tc.scope, tc.wantErr, err)
		}
	}
}

func TestAddCmd_InvalidBucket(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	t.Setenv("JEFF_MEMORY_CAN_ADD", "1")

	err := runAddCmd(t, []string{
		"--name", "test",
		"--type", "feedback",
		"--description", "d",
		"--body", "b",
		"--scope", "persona:jenko",
		"--bucket", "notabucket",
	})
	if err == nil {
		t.Fatal("expected error for invalid bucket")
	}
	if !strings.Contains(err.Error(), "notabucket") {
		t.Errorf("expected bucket name in error, got: %v", err)
	}
}

func TestAddCmd_InvalidName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	t.Setenv("JEFF_MEMORY_CAN_ADD", "1")

	err := runAddCmd(t, []string{
		"--name", "Invalid_Name",
		"--type", "feedback",
		"--description", "d",
		"--body", "b",
		"--scope", "persona:jenko",
		"--bucket", "semantic",
	})
	if err == nil {
		t.Fatal("expected error for invalid name")
	}
}

func TestAddCmd_SourceFromEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	t.Setenv("JEFF_MEMORY_CAN_ADD", "1")
	t.Setenv("JEFF_PERSONA", "marlowe")
	t.Setenv("JEFF_TASK_ID", "gig-curate-run")

	err := runAddCmd(t, []string{
		"--name", "source-check",
		"--type", "user",
		"--description", "source from env",
		"--body", "b",
		"--scope", "persona:jenko",
		"--bucket", "semantic",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(home, "memory", "personas", "jenko", "semantic", "source-check.md")
	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "persona: marlowe") {
		t.Errorf("expected source.persona=marlowe in frontmatter, got:\n%s", content)
	}
	if !strings.Contains(content, "task: gig-curate-run") {
		t.Errorf("expected source.task=gig-curate-run in frontmatter, got:\n%s", content)
	}
}
