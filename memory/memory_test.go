package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsurePersonaDir(t *testing.T) {
	home := t.TempDir()

	if err := EnsurePersonaDir(home, "jenko"); err != nil {
		t.Fatal(err)
	}

	// Verify directory exists.
	dir := PersonaMemoryDir(home, "jenko")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("memory dir not created: %v", err)
	}

	// Verify seed MEMORY.md.
	data, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
	if err != nil {
		t.Fatalf("MEMORY.md not created: %v", err)
	}
	if !strings.Contains(string(data), "Jenko Memory") {
		t.Errorf("seed does not contain persona name: %s", data)
	}

	// Idempotent — second call doesn't overwrite.
	custom := "# Custom content\n- [style](style.md) — user prefers early returns\n"
	os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte(custom), 0o644)
	if err := EnsurePersonaDir(home, "jenko"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(dir, "MEMORY.md"))
	if string(data) != custom {
		t.Error("EnsurePersonaDir overwrote existing MEMORY.md")
	}
}

func TestEnsureRepoDir(t *testing.T) {
	home := t.TempDir()

	if err := EnsureRepoDir(home, "backend"); err != nil {
		t.Fatal(err)
	}

	dir := RepoLearningsDir(home, "backend")
	data, err := os.ReadFile(filepath.Join(dir, "INDEX.md"))
	if err != nil {
		t.Fatalf("INDEX.md not created: %v", err)
	}
	if !strings.Contains(string(data), "backend Learnings") {
		t.Errorf("seed does not contain repo name: %s", data)
	}
}

func TestLoadPersonaMemoryEmpty(t *testing.T) {
	home := t.TempDir()
	EnsurePersonaDir(home, "jenko")

	content, err := LoadPersonaMemory(home, "jenko")
	if err != nil {
		t.Fatal(err)
	}
	if content != "" {
		t.Errorf("seed-only MEMORY.md should return empty, got: %q", content)
	}
}

func TestLoadPersonaMemoryWithContent(t *testing.T) {
	home := t.TempDir()
	EnsurePersonaDir(home, "jenko")

	md := "# Jock Memory\n\n- [style](style.md) — early returns over nested ifs\n"
	os.WriteFile(filepath.Join(PersonaMemoryDir(home, "jenko"), "MEMORY.md"), []byte(md), 0o644)

	content, err := LoadPersonaMemory(home, "jenko")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "early returns") {
		t.Errorf("expected content, got: %q", content)
	}
}

func TestLoadRepoLearningsEmpty(t *testing.T) {
	home := t.TempDir()
	EnsureRepoDir(home, "backend")

	content, err := LoadRepoLearnings(home, "backend")
	if err != nil {
		t.Fatal(err)
	}
	if content != "" {
		t.Errorf("seed-only INDEX.md should return empty, got: %q", content)
	}
}

func TestLoadRepoLearningsWithContent(t *testing.T) {
	home := t.TempDir()
	EnsureRepoDir(home, "backend")

	md := "# backend Learnings\n\n- [testing](testing.md) — run make migrate before tests\n"
	os.WriteFile(filepath.Join(RepoLearningsDir(home, "backend"), "INDEX.md"), []byte(md), 0o644)

	content, err := LoadRepoLearnings(home, "backend")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "make migrate") {
		t.Errorf("expected content, got: %q", content)
	}
}

func TestLoadMissingDir(t *testing.T) {
	home := t.TempDir()

	content, err := LoadPersonaMemory(home, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if content != "" {
		t.Errorf("missing dir should return empty, got: %q", content)
	}
}

func TestInstallLearnCommand(t *testing.T) {
	taskDir := t.TempDir()
	home := t.TempDir()

	err := InstallLearnCommand(taskDir, "gig-42", "jenko", home, []string{"backend", "frontend"})
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(taskDir, ".claude", "commands", "learn.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("learn.md not created: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "gig-42") {
		t.Error("learn.md missing task ID")
	}
	if !strings.Contains(content, "jenko") {
		t.Error("learn.md missing persona name")
	}
	if !strings.Contains(content, "backend") {
		t.Error("learn.md missing repo name")
	}
	if !strings.Contains(content, "scratchpad.md") {
		t.Error("learn.md missing scratchpad reference")
	}
}

func TestScratchpadPath(t *testing.T) {
	p := ScratchpadPath("/tmp/tasks/gig-42")
	if !strings.HasSuffix(p, "scratchpad.md") {
		t.Errorf("unexpected path: %s", p)
	}
}
