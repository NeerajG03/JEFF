package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func tempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	home := filepath.Join(dir, ".jeff")
	os.MkdirAll(filepath.Join(home, ".skills"), 0o755)
	return home
}

func writeSkill(t *testing.T, dir, name string) string {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: "+name+"\n---\nTest skill.\n"), 0o644)
	return skillDir
}

// --- Load/Save ---

func TestLoadSkills_Empty(t *testing.T) {
	home := tempHome(t)
	sc, err := LoadSkills(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.Skills) != 0 {
		t.Errorf("expected empty skills, got %d", len(sc.Skills))
	}
}

func TestLoadSkills_RoundTrip(t *testing.T) {
	home := tempHome(t)
	sc := &SkillConfig{
		Skills: map[string]*SkillEntry{
			"deploy": {Location: "/tmp/deploy", Personas: []string{"jenko"}},
		},
	}
	if err := SaveSkills(home, sc); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadSkills(home)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Skills["deploy"] == nil {
		t.Fatal("deploy skill missing after round trip")
	}
	if loaded.Skills["deploy"].Location != "/tmp/deploy" {
		t.Errorf("location = %s, want /tmp/deploy", loaded.Skills["deploy"].Location)
	}
	if loaded.Schema == "" {
		t.Error("schema should be set")
	}
}

// --- Add ---

func TestAdd_CopiesLocal(t *testing.T) {
	home := tempHome(t)
	srcDir := writeSkill(t, t.TempDir(), "my-skill")

	entry, err := Add(home, srcDir, "my-skill", false)
	if err != nil {
		t.Fatal(err)
	}

	// Should be copied to .skills/my-skill/
	expected := filepath.Join(DefaultSkillsDir(home), "my-skill")
	if entry.Location != expected {
		t.Errorf("location = %s, want %s", entry.Location, expected)
	}

	// SKILL.md should exist at destination.
	if _, err := os.Stat(filepath.Join(expected, "SKILL.md")); err != nil {
		t.Error("SKILL.md not copied to destination")
	}

	// Should be in registry.
	sc, _ := LoadSkills(home)
	if sc.Skills["my-skill"] == nil {
		t.Error("skill not in registry")
	}
}

func TestAdd_External(t *testing.T) {
	home := tempHome(t)
	srcDir := writeSkill(t, t.TempDir(), "ext-skill")

	entry, err := Add(home, srcDir, "ext-skill", true)
	if err != nil {
		t.Fatal(err)
	}

	// Should point to original location, not copied.
	if entry.Location != srcDir {
		t.Errorf("location = %s, want %s", entry.Location, srcDir)
	}
}

func TestAdd_MissingSKILLMD(t *testing.T) {
	home := tempHome(t)
	emptyDir := t.TempDir()

	_, err := Add(home, emptyDir, "bad", false)
	if err == nil {
		t.Error("expected error for missing SKILL.md")
	}
}

func TestAdd_DuplicateName(t *testing.T) {
	home := tempHome(t)
	srcDir := writeSkill(t, t.TempDir(), "dup")

	if _, err := Add(home, srcDir, "dup", true); err != nil {
		t.Fatal(err)
	}
	_, err := Add(home, srcDir, "dup", true)
	if err == nil {
		t.Error("expected error for duplicate skill name")
	}
}

// --- Remove ---

func TestRemove(t *testing.T) {
	home := tempHome(t)
	srcDir := writeSkill(t, t.TempDir(), "rm-test")
	Add(home, srcDir, "rm-test", true)

	if err := Remove(home, "rm-test", false); err != nil {
		t.Fatal(err)
	}
	sc, _ := LoadSkills(home)
	if sc.Skills["rm-test"] != nil {
		t.Error("skill still in registry after remove")
	}
}

// --- SetTags ---

func TestSetTags(t *testing.T) {
	home := tempHome(t)
	srcDir := writeSkill(t, t.TempDir(), "tag-test")
	Add(home, srcDir, "tag-test", true)

	personas := []string{"jenko", "hardy"}
	types := []string{"bug"}
	tags := []string{"auth"}
	if err := SetTags(home, "tag-test", personas, types, tags); err != nil {
		t.Fatal(err)
	}

	entry, _ := Get(home, "tag-test")
	if len(entry.Personas) != 2 || entry.Personas[0] != "jenko" {
		t.Errorf("personas = %v", entry.Personas)
	}
	if len(entry.GigTypes) != 1 || entry.GigTypes[0] != "bug" {
		t.Errorf("gig_types = %v", entry.GigTypes)
	}
	if len(entry.Tags) != 1 || entry.Tags[0] != "auth" {
		t.Errorf("tags = %v", entry.Tags)
	}
}

// --- Matching ---

func TestMatch(t *testing.T) {
	tests := []struct {
		name  string
		entry SkillEntry
		ctx   MatchContext
		want  bool
	}{
		{
			name:  "persona match",
			entry: SkillEntry{Personas: []string{"jenko"}},
			ctx:   MatchContext{Persona: "jenko"},
			want:  true,
		},
		{
			name:  "persona mismatch",
			entry: SkillEntry{Personas: []string{"dickson"}},
			ctx:   MatchContext{Persona: "jenko"},
			want:  false,
		},
		{
			name:  "gig_type match",
			entry: SkillEntry{GigTypes: []string{"bug", "feature"}},
			ctx:   MatchContext{GigType: "bug"},
			want:  true,
		},
		{
			name:  "gig_type mismatch",
			entry: SkillEntry{GigTypes: []string{"feature"}},
			ctx:   MatchContext{GigType: "bug"},
			want:  false,
		},
		{
			name:  "tag intersection",
			entry: SkillEntry{Tags: []string{"deploy", "ci"}},
			ctx:   MatchContext{Labels: []string{"deploy", "auth"}},
			want:  true,
		},
		{
			name:  "no tag intersection",
			entry: SkillEntry{Tags: []string{"deploy"}},
			ctx:   MatchContext{Labels: []string{"auth"}},
			want:  false,
		},
		{
			name:  "all empty = manual only",
			entry: SkillEntry{},
			ctx:   MatchContext{Persona: "jenko", GigType: "bug", Labels: []string{"auth"}},
			want:  false,
		},
		{
			name:  "any dimension: persona matches, type doesnt",
			entry: SkillEntry{Personas: []string{"jenko"}, GigTypes: []string{"feature"}},
			ctx:   MatchContext{Persona: "jenko", GigType: "bug"},
			want:  true,
		},
		{
			name:  "any dimension: type matches, persona doesnt",
			entry: SkillEntry{Personas: []string{"dickson"}, GigTypes: []string{"bug"}},
			ctx:   MatchContext{Persona: "jenko", GigType: "bug"},
			want:  true,
		},
		{
			name:  "empty context",
			entry: SkillEntry{Personas: []string{"jenko"}},
			ctx:   MatchContext{},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Match(&tt.entry, &tt.ctx)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchAll(t *testing.T) {
	sc := &SkillConfig{
		Skills: map[string]*SkillEntry{
			"deploy":  {Personas: []string{"jenko"}, GigTypes: []string{"chore"}},
			"review":  {Personas: []string{"hardy"}},
			"aws-cli": {Tags: []string{"aws"}},
			"manual":  {}, // no dimensions
		},
	}

	ctx := &MatchContext{Persona: "jenko", GigType: "bug", Labels: []string{"aws"}}
	names := MatchAll(sc, ctx)

	if len(names) != 2 {
		t.Fatalf("MatchAll returned %d skills, want 2: %v", len(names), names)
	}
	// Should be sorted: aws-cli, deploy
	if names[0] != "aws-cli" || names[1] != "deploy" {
		t.Errorf("names = %v, want [aws-cli deploy]", names)
	}
}

// --- Inject/Eject ---

func TestInject_CreatesSymlink(t *testing.T) {
	taskDir := t.TempDir()
	skillDir := writeSkill(t, t.TempDir(), "test-skill")

	if err := Inject("test-skill", skillDir, taskDir); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(taskDir, ".claude", "skills", "test-skill")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("symlink not created: %v", err)
	}
	if target != skillDir {
		t.Errorf("symlink target = %s, want %s", target, skillDir)
	}
}

func TestInject_Idempotent(t *testing.T) {
	taskDir := t.TempDir()
	skillDir := writeSkill(t, t.TempDir(), "idem")

	Inject("idem", skillDir, taskDir)
	if err := Inject("idem", skillDir, taskDir); err != nil {
		t.Fatalf("second inject should be no-op: %v", err)
	}
}

func TestEject_RemovesSymlink(t *testing.T) {
	taskDir := t.TempDir()
	skillDir := writeSkill(t, t.TempDir(), "eject-me")
	Inject("eject-me", skillDir, taskDir)

	if err := Eject("eject-me", taskDir); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(taskDir, ".claude", "skills", "eject-me")
	if _, err := os.Lstat(link); err == nil {
		t.Error("symlink still exists after eject")
	}
}

func TestEject_Idempotent(t *testing.T) {
	taskDir := t.TempDir()
	if err := Eject("nonexistent", taskDir); err != nil {
		t.Fatalf("eject nonexistent should be no-op: %v", err)
	}
}

func TestInjected_Lists(t *testing.T) {
	taskDir := t.TempDir()
	s1 := writeSkill(t, t.TempDir(), "s1")
	s2 := writeSkill(t, t.TempDir(), "s2")
	Inject("s1", s1, taskDir)
	Inject("s2", s2, taskDir)

	names, err := Injected(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Errorf("got %d injected, want 2", len(names))
	}
}

func TestInjectMatching_Integration(t *testing.T) {
	home := tempHome(t)
	taskDir := t.TempDir()

	// Register two skills.
	s1 := writeSkill(t, t.TempDir(), "deploy")
	s2 := writeSkill(t, t.TempDir(), "review")
	Add(home, s1, "deploy", true)
	Add(home, s2, "review", true)
	SetTags(home, "deploy", []string{"jenko"}, []string{"feature"}, nil)
	SetTags(home, "review", []string{"hardy"}, nil, nil)

	ctx := &MatchContext{Persona: "jenko", GigType: "bug"}
	injected, err := InjectMatching(home, taskDir, ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Only deploy should match (persona=jock).
	if len(injected) != 1 || injected[0] != "deploy" {
		t.Errorf("injected = %v, want [deploy]", injected)
	}

	// Verify symlink exists.
	link := filepath.Join(taskDir, ".claude", "skills", "deploy")
	if _, err := os.Readlink(link); err != nil {
		t.Error("deploy symlink not created")
	}
}
