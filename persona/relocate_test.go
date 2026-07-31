package persona

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPersonasJSONStoresRelativeLocations pins the on-disk form. The registry used
// to persist absolute paths, which made the home non-relocatable: a plain `mv`
// left every persona pointing at the old directory (#84).
func TestPersonasJSONStoresRelativeLocations(t *testing.T) {
	home := t.TempDir()
	if err := SeedDefaults(home); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(PersonasPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), home) {
		t.Errorf("personas.json contains the absolute home path %q:\n%s", home, raw)
	}

	var pc PersonaConfig
	if err := json.Unmarshal(raw, &pc); err != nil {
		t.Fatal(err)
	}
	if len(pc.Personas) == 0 {
		t.Fatal("no personas seeded")
	}
	for name, entry := range pc.Personas {
		if filepath.IsAbs(entry.Location) {
			t.Errorf("persona %s stored an absolute location %q", name, entry.Location)
		}
	}
}

// TestLoadPersonasResolvesAgainstHome verifies callers still see absolute paths, so
// relativizing storage is invisible to every consumer.
func TestLoadPersonasResolvesAgainstHome(t *testing.T) {
	home := t.TempDir()
	if err := SeedDefaults(home); err != nil {
		t.Fatal(err)
	}

	pc, err := LoadPersonas(home)
	if err != nil {
		t.Fatal(err)
	}
	for name, entry := range pc.Personas {
		if !filepath.IsAbs(entry.Location) {
			t.Errorf("persona %s: caller saw a relative location %q; Load must resolve", name, entry.Location)
		}
		if _, err := os.Stat(filepath.Join(entry.Location, "PERSONA.md")); err != nil {
			t.Errorf("persona %s: resolved location does not hold PERSONA.md: %v", name, err)
		}
	}
}

// TestPersonasSurviveHomeRelocation is the end-to-end proof for #84: seed a home,
// move the whole directory, and read it from the new location. No repair step, no
// rewriting — the registry must simply still be correct.
func TestPersonasSurviveHomeRelocation(t *testing.T) {
	base := t.TempDir()
	oldHome := filepath.Join(base, "old-home")
	newHome := filepath.Join(base, "moved-home")

	if err := os.MkdirAll(oldHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SeedDefaults(oldHome); err != nil {
		t.Fatal(err)
	}
	before, err := ListPersonas(oldHome)
	if err != nil || len(before) == 0 {
		t.Fatalf("seed produced no personas: %v", err)
	}

	// The relocation a user performs with `mv`.
	if err := os.Rename(oldHome, newHome); err != nil {
		t.Fatal(err)
	}

	after, err := ListPersonas(newHome)
	if err != nil {
		t.Fatalf("reading the relocated home failed: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("persona count changed after relocation: %d → %d", len(before), len(after))
	}
	for _, p := range after {
		md := filepath.Join(p.Entry.Location, "PERSONA.md")
		if _, err := os.Stat(md); err != nil {
			t.Errorf("persona %s broken after relocation: %s unreadable (%v)", p.Name, md, err)
		}
		if strings.HasPrefix(p.Entry.Location, oldHome) {
			t.Errorf("persona %s still points into the OLD home: %s", p.Name, p.Entry.Location)
		}
	}

	// Templates must be readable through the registry, not just stat-able.
	if _, err := GetTemplate(newHome, before[0].Name); err != nil {
		t.Errorf("GetTemplate after relocation: %v", err)
	}
}

// TestExternalPersonaStaysAbsolute confirms the escape hatch: a persona registered
// from outside the home is not the home's to relocate, so its path stays absolute.
func TestExternalPersonaStaysAbsolute(t *testing.T) {
	home := t.TempDir()
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(external, "PERSONA.md"), []byte("ext — an external persona. Does things.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := AddPersona(home, external, "ext", true); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(PersonasPath(home))
	if err != nil {
		t.Fatal(err)
	}
	var pc PersonaConfig
	if err := json.Unmarshal(raw, &pc); err != nil {
		t.Fatal(err)
	}
	entry, ok := pc.Personas["ext"]
	if !ok {
		t.Fatal("external persona not registered")
	}
	if !filepath.IsAbs(entry.Location) {
		t.Errorf("external persona location = %q, want it to stay absolute", entry.Location)
	}
}
