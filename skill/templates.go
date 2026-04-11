package skill

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed templates/crew-orchestrator/*.md
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
	if err := writeEmbedded(destDir); err != nil {
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
		// First seed — register fresh.
		sc.Skills[name] = &SkillEntry{Location: destDir}
	} else {
		// Update run — update location and strip any persona tags.
		entry.Location = destDir
		entry.Personas = nil
		entry.Tags = nil
		entry.GigTypes = nil
	}

	return SaveSkills(jeffHome, sc)
}

// writeEmbedded writes the embedded crew-orchestrator template files to destDir.
// Overwrites existing files so updates pick up the latest embedded content.
func writeEmbedded(destDir string) error {
	return fs.WalkDir(templateFS, "templates/crew-orchestrator", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel("templates/crew-orchestrator", path)
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := templateFS.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
