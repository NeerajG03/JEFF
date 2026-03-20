package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NeerajG03/gig"
)

func TestWriteTaskClaudeMD_NoPersona(t *testing.T) {
	dir := t.TempDir()
	task := &gig.Task{
		ID:       "gig-ab12",
		Title:    "Test task",
		Priority: gig.P1,
	}

	if err := writeTaskClaudeMD(dir, task, ""); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Should have task context.
	if !strings.Contains(content, "gig-ab12") {
		t.Error("missing task ID")
	}
	if !strings.Contains(content, "Test task") {
		t.Error("missing task title")
	}

	// Should NOT have persona section — first line should be task context.
	lines := strings.Split(content, "\n")
	if lines[0] != "# Current Task" {
		t.Errorf("expected first line to be task context, got %q", lines[0])
	}
}

func TestWriteTaskClaudeMD_WithPersona(t *testing.T) {
	dir := t.TempDir()
	task := &gig.Task{
		ID:       "gig-cd34",
		Title:    "Persona task",
		Priority: gig.P2,
	}

	if err := writeTaskClaudeMD(dir, task, "jock"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)

	// Should have persona content followed by divider.
	if !strings.Contains(content, "---") {
		t.Error("missing persona divider")
	}
	// Should have task context after persona.
	if !strings.Contains(content, "gig-cd34") {
		t.Error("missing task ID")
	}
}

func TestWriteTaskClaudeMD_InvalidPersonaSkipped(t *testing.T) {
	dir := t.TempDir()
	task := &gig.Task{
		ID:       "gig-ef56",
		Title:    "Bad persona task",
		Priority: gig.P1,
	}

	if err := writeTaskClaudeMD(dir, task, "nonexistent"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)

	// Should still have task context, just no persona.
	if !strings.Contains(content, "gig-ef56") {
		t.Error("missing task ID")
	}
	// No divider since persona was invalid.
	lines := strings.Split(content, "\n")
	if lines[0] != "# Current Task" {
		t.Errorf("expected task context first, got %q", lines[0])
	}
}

func TestWriteTaskClaudeMD_WithDescription(t *testing.T) {
	dir := t.TempDir()
	task := &gig.Task{
		ID:          "gig-gh78",
		Title:       "Described task",
		Description: "Some detailed description",
		Priority:    gig.P1,
		ParentID:    "gig-parent",
	}

	if err := writeTaskClaudeMD(dir, task, ""); err != nil {
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

func TestWriteTaskClaudeMD_NoDescriptionOmitted(t *testing.T) {
	dir := t.TempDir()
	task := &gig.Task{
		ID:       "gig-ij90",
		Title:    "No desc task",
		Priority: gig.P2,
	}

	if err := writeTaskClaudeMD(dir, task, ""); err != nil {
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
