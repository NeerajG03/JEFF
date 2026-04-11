package persona

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".personas"), 0o755)
	return home
}

func writePersona(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	os.MkdirAll(p, 0o755)
	content := "You are " + strings.ToUpper(name[:1]) + name[1:] + " — a test persona.\n\n## Role\n- Do test things\n"
	os.WriteFile(filepath.Join(p, "PERSONA.md"), []byte(content), 0o644)
	return p
}

func TestLoadSaveRoundtrip(t *testing.T) {
	home := tempHome(t)

	pc := &PersonaConfig{
		Personas: map[string]*PersonaEntry{
			"test": {Location: "/tmp/test", Description: "a tester"},
		},
	}
	if err := SavePersonas(home, pc); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadPersonas(home)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Personas["test"].Description != "a tester" {
		t.Errorf("description = %q, want 'a tester'", loaded.Personas["test"].Description)
	}
	if loaded.Schema == "" {
		t.Error("schema should be set")
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	home := tempHome(t)
	pc, err := LoadPersonas(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(pc.Personas) != 0 {
		t.Errorf("expected empty, got %d personas", len(pc.Personas))
	}
}

func TestAddPersonaCopy(t *testing.T) {
	home := tempHome(t)
	srcDir := t.TempDir()
	writePersona(t, srcDir, "my-coder")

	entry, err := AddPersona(home, filepath.Join(srcDir, "my-coder"), "", false)
	if err != nil {
		t.Fatal(err)
	}

	// Should be copied into .personas/my-coder/.
	if !strings.Contains(entry.Location, ".personas") {
		t.Errorf("expected location under .personas, got %s", entry.Location)
	}

	// PERSONA.md should exist at new location.
	if _, err := os.Stat(filepath.Join(entry.Location, "PERSONA.md")); err != nil {
		t.Error("PERSONA.md not copied")
	}

	// Description should be extracted.
	if entry.Description == "" {
		t.Error("description should be extracted from PERSONA.md")
	}
}

func TestAddPersonaExternal(t *testing.T) {
	home := tempHome(t)
	srcDir := t.TempDir()
	path := writePersona(t, srcDir, "external")

	entry, err := AddPersona(home, path, "", true)
	if err != nil {
		t.Fatal(err)
	}

	if entry.Location != path {
		t.Errorf("external location should be original path, got %s", entry.Location)
	}
}

func TestAddPersonaDuplicate(t *testing.T) {
	home := tempHome(t)
	srcDir := t.TempDir()
	writePersona(t, srcDir, "dup")

	AddPersona(home, filepath.Join(srcDir, "dup"), "", true)
	_, err := AddPersona(home, filepath.Join(srcDir, "dup"), "", true)
	if err == nil {
		t.Error("expected error for duplicate")
	}
}

func TestAddPersonaMissingTemplate(t *testing.T) {
	home := tempHome(t)
	emptyDir := t.TempDir()

	_, err := AddPersona(home, emptyDir, "notemplate", false)
	if err == nil {
		t.Error("expected error for missing PERSONA.md")
	}
}

func TestRemovePersona(t *testing.T) {
	home := tempHome(t)
	srcDir := t.TempDir()
	writePersona(t, srcDir, "removeme")
	AddPersona(home, filepath.Join(srcDir, "removeme"), "", false)

	if err := RemovePersona(home, "removeme", true); err != nil {
		t.Fatal(err)
	}

	pc, _ := LoadPersonas(home)
	if _, exists := pc.Personas["removeme"]; exists {
		t.Error("persona should be removed")
	}
}

func TestUpdatePersona(t *testing.T) {
	home := tempHome(t)
	srcDir := t.TempDir()
	writePersona(t, srcDir, "updatable")
	AddPersona(home, filepath.Join(srcDir, "updatable"), "", true)

	if err := UpdatePersona(home, "updatable", "new desc", "capture bugs", ""); err != nil {
		t.Fatal(err)
	}

	entry, _ := GetPersona(home, "updatable")
	if entry.Description != "new desc" {
		t.Errorf("description = %q, want 'new desc'", entry.Description)
	}
	if entry.MemoryHint != "capture bugs" {
		t.Errorf("memory_hint = %q, want 'capture bugs'", entry.MemoryHint)
	}
}

func TestUpdatePersonaPartial(t *testing.T) {
	home := tempHome(t)
	srcDir := t.TempDir()
	writePersona(t, srcDir, "partial")
	AddPersona(home, filepath.Join(srcDir, "partial"), "", true)
	UpdatePersona(home, "partial", "original", "original hint", "sonnet")

	// Update only description.
	UpdatePersona(home, "partial", "new desc", "", "")
	entry, _ := GetPersona(home, "partial")
	if entry.Description != "new desc" {
		t.Errorf("description = %q", entry.Description)
	}
	if entry.MemoryHint != "original hint" {
		t.Errorf("memory_hint should be unchanged, got %q", entry.MemoryHint)
	}
	if entry.Model != "sonnet" {
		t.Errorf("model should be unchanged, got %q", entry.Model)
	}

	// Update only model.
	UpdatePersona(home, "partial", "", "", "opus")
	entry, _ = GetPersona(home, "partial")
	if entry.Model != "opus" {
		t.Errorf("model = %q, want opus", entry.Model)
	}
	if entry.Description != "new desc" {
		t.Errorf("description should be unchanged after model update, got %q", entry.Description)
	}
}

func TestListPersonas(t *testing.T) {
	home := tempHome(t)
	srcDir := t.TempDir()
	writePersona(t, srcDir, "alpha")
	writePersona(t, srcDir, "beta")
	AddPersona(home, filepath.Join(srcDir, "alpha"), "", true)
	AddPersona(home, filepath.Join(srcDir, "beta"), "", true)

	list, err := ListPersonas(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
	if list[0].Name != "alpha" {
		t.Error("should be sorted, first should be alpha")
	}
}

func TestGetTemplate(t *testing.T) {
	home := tempHome(t)
	srcDir := t.TempDir()
	writePersona(t, srcDir, "reader")
	AddPersona(home, filepath.Join(srcDir, "reader"), "", false)

	content, err := GetTemplate(home, "reader")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "Reader") {
		t.Error("template should contain persona name")
	}
}

func TestSeedDefaults(t *testing.T) {
	home := tempHome(t)

	if err := SeedDefaults(home); err != nil {
		t.Fatal(err)
	}

	pc, _ := LoadPersonas(home)
	expectedNames := Names() // embedded persona names
	for _, name := range expectedNames {
		entry, exists := pc.Personas[name]
		if !exists {
			t.Errorf("missing seeded persona: %s", name)
			continue
		}
		// Should have PERSONA.md on disk.
		if _, err := os.Stat(filepath.Join(entry.Location, "PERSONA.md")); err != nil {
			t.Errorf("PERSONA.md not written for %s", name)
		}
		if entry.Description == "" {
			t.Errorf("%s missing description", name)
		}
	}

	// Model should be populated for built-in personas.
	if m := pc.Personas["jenko"].Model; m != "opus" {
		t.Errorf("jenko model = %q, want opus", m)
	}
	if m := pc.Personas["eric"].Model; m != "sonnet" {
		t.Errorf("eric model = %q, want sonnet", m)
	}

	// Idempotent — second call doesn't duplicate.
	if err := SeedDefaults(home); err != nil {
		t.Fatal(err)
	}
	pc2, _ := LoadPersonas(home)
	if len(pc2.Personas) != len(pc.Personas) {
		t.Error("seed should be idempotent")
	}
}

func TestRegisteredNamesWithDescriptions(t *testing.T) {
	home := tempHome(t)
	SeedDefaults(home)

	results := RegisteredNamesWithDescriptions(home)
	if len(results) < 5 {
		t.Fatalf("expected at least 5, got %d", len(results))
	}
	for _, r := range results {
		if !strings.Contains(r, "\t") {
			t.Errorf("expected tab separator, got %q", r)
		}
	}
}
