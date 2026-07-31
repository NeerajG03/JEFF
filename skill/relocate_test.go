package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSkillsJSONStoresRelativeLocations pins the on-disk form; absolute paths are
// what made the home non-relocatable (#84).
func TestSkillsJSONStoresRelativeLocations(t *testing.T) {
	home := t.TempDir()
	if err := SeedDefaults(home); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(SkillsPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), home) {
		t.Errorf("skills.json contains the absolute home path %q:\n%s", home, raw)
	}

	var sc SkillConfig
	if err := json.Unmarshal(raw, &sc); err != nil {
		t.Fatal(err)
	}
	if len(sc.Skills) == 0 {
		t.Fatal("no skills seeded")
	}
	for name, entry := range sc.Skills {
		if filepath.IsAbs(entry.Location) {
			t.Errorf("skill %s stored an absolute location %q", name, entry.Location)
		}
	}
}

// TestSkillsSurviveHomeRelocation is the end-to-end proof: move the home, read it
// from the new place, with no repair step.
func TestSkillsSurviveHomeRelocation(t *testing.T) {
	base := t.TempDir()
	oldHome := filepath.Join(base, "old-home")
	newHome := filepath.Join(base, "moved-home")

	if err := os.MkdirAll(oldHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SeedDefaults(oldHome); err != nil {
		t.Fatal(err)
	}
	before, err := List(oldHome)
	if err != nil || len(before) == 0 {
		t.Fatalf("seed produced no skills: %v", err)
	}

	if err := os.Rename(oldHome, newHome); err != nil {
		t.Fatal(err)
	}

	after, err := List(newHome)
	if err != nil {
		t.Fatalf("reading the relocated home failed: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("skill count changed after relocation: %d → %d", len(before), len(after))
	}
	for _, s := range after {
		if strings.HasPrefix(s.Entry.Location, oldHome) {
			t.Errorf("skill %s still points into the OLD home: %s", s.Name, s.Entry.Location)
		}
		if _, err := os.Stat(s.Entry.Location); err != nil {
			t.Errorf("skill %s broken after relocation: %v", s.Name, err)
		}
	}
}

// TestLoadSkillsResolvesAgainstHome confirms callers keep seeing absolute paths.
func TestLoadSkillsResolvesAgainstHome(t *testing.T) {
	home := t.TempDir()
	if err := SeedDefaults(home); err != nil {
		t.Fatal(err)
	}
	sc, err := LoadSkills(home)
	if err != nil {
		t.Fatal(err)
	}
	for name, entry := range sc.Skills {
		if !filepath.IsAbs(entry.Location) {
			t.Errorf("skill %s: caller saw a relative location %q; Load must resolve", name, entry.Location)
		}
	}
}
