package skill

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed templates/crew-orchestrator/*.md
//go:embed templates/curation/SKILL.md
//go:embed templates/go-testing/SKILL.md
//go:embed templates/pr-review/SKILL.md
//go:embed templates/root-cause/SKILL.md
var templateFS embed.FS

// SeedDefaults seeds the built-in crew-orchestrator skill into jeffHome.
//
// First run: writes SKILL.md + reference.md, creates memory/ dir, and
// registers the skill in skills.json with no persona/tag injection (orchestrator-only).
//
// Subsequent runs (jeff init --update): refreshes SKILL.md + reference.md from
// the embedded templates, ensures memory/ exists, and clears any persona tags
// that may have been set previously.
func SeedDefaults(jeffHome string) error {
	const name = "crew-orchestrator"
	destDir := filepath.Join(DefaultSkillsDir(jeffHome), name)

	// Write (or refresh) embedded template files.
	if err := writeEmbeddedSkill(templateFS, "templates/crew-orchestrator", destDir); err != nil {
		return fmt.Errorf("seed %s: %w", name, err)
	}

	// Ensure memory/ dir exists (never overwrite).
	memDir := filepath.Join(destDir, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		return fmt.Errorf("create memory dir: %w", err)
	}

	// Register (or update) in skills.json — no persona/tag injection.
	sc, err := LoadSkills(jeffHome)
	if err != nil {
		return err
	}

	entry, exists := sc.Skills[name]
	if !exists {
		sc.Skills[name] = &SkillEntry{Location: destDir}
	} else {
		entry.Location = destDir
		entry.Personas = nil
		entry.Tags = nil
		entry.GigTypes = nil
	}

	return SaveSkills(jeffHome, sc)
}

// SeedCuration seeds the built-in curation skill into jeffHome.
// The curation skill is marlowe's operating contract — persona shapes,
// routing matrix, rubrics, and worked examples.
//
// Called by Worker E (jeff init / --update). Safe to call repeatedly.
func SeedCuration(jeffHome string) error {
	const name = "curation"
	destDir := filepath.Join(DefaultSkillsDir(jeffHome), name)

	if err := writeEmbeddedSkill(templateFS, "templates/curation", destDir); err != nil {
		return fmt.Errorf("seed %s: %w", name, err)
	}

	sc, err := LoadSkills(jeffHome)
	if err != nil {
		return err
	}

	entry, exists := sc.Skills[name]
	if !exists {
		sc.Skills[name] = &SkillEntry{Location: destDir}
	} else {
		entry.Location = destDir
		// Curation skill is marlowe-only — keep persona tags if user set them.
	}

	return SaveSkills(jeffHome, sc)
}

// SeedPersonaSkills seeds 3 persona-tagged embedded skills: go-testing→jenko,
// pr-review→hardy, root-cause→schmidt. Each is a genuine, self-contained
// SKILL.md that documents common workflows for that persona.
//
// Called by Worker E (jeff init / --update). Safe to call repeatedly.
// Tags are set so these skills auto-inject when the matching persona picks
// up a task (via jeff pickup --persona <name>).
func SeedPersonaSkills(jeffHome string) error {
	skills := []struct {
		Name    string
		Persona string
	}{
		{"go-testing", "jenko"},
		{"pr-review", "hardy"},
		{"root-cause", "schmidt"},
	}

	for _, s := range skills {
		destDir := filepath.Join(DefaultSkillsDir(jeffHome), s.Name)
		srcDir := "templates/" + s.Name

		if err := writeEmbeddedSkill(templateFS, srcDir, destDir); err != nil {
			return fmt.Errorf("seed %s: %w", s.Name, err)
		}

		sc, err := LoadSkills(jeffHome)
		if err != nil {
			return err
		}

		if entry, exists := sc.Skills[s.Name]; exists {
			entry.Location = destDir
			entry.Personas = []string{s.Persona}
		} else {
			sc.Skills[s.Name] = &SkillEntry{
				Location: destDir,
				Personas: []string{s.Persona},
			}
		}

		if err := SaveSkills(jeffHome, sc); err != nil {
			return fmt.Errorf("save skills after seeding %s: %w", s.Name, err)
		}
	}

	return nil
}

// CurationSkillContent returns the embedded curation SKILL.md content.
// Used by memory.Curate when the skill has not been installed to JEFF_HOME yet.
func CurationSkillContent() (string, error) {
	data, err := templateFS.ReadFile("templates/curation/SKILL.md")
	if err != nil {
		return "", fmt.Errorf("read embedded curation skill: %w", err)
	}
	return string(data), nil
}

// writeEmbeddedSkill writes embedded skill template files to destDir.
// Overwrites existing files so updates pick up the latest embedded content.
func writeEmbeddedSkill(fsys embed.FS, srcDir, destDir string) error {
	return fs.WalkDir(fsys, srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fsys.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
