// init.go — Worker E: idempotent v1 layout provisioning and legacy migration.
// Initialize: greenfield setup. Update: additive only. Migrate: legacy → v1 tree.
package memory

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	jeff "github.com/NeerajG03/JEFF"
	jeffembed "github.com/NeerajG03/JEFF/embed"
	"github.com/NeerajG03/JEFF/skill"
)

// Initialize sets up the memory subsystem in a fresh JEFF_HOME.
// All steps are individually idempotent: running Initialize twice is safe.
func Initialize(home string) error {
	if err := EnsureLayout(home); err != nil {
		return fmt.Errorf("ensure layout: %w", err)
	}
	if err := installMarloweGoal(home); err != nil {
		return fmt.Errorf("install marlowe goal: %w", err)
	}
	if err := skill.SeedCuration(home); err != nil {
		return fmt.Errorf("seed curation skill: %w", err)
	}
	if err := installSlashCommands(home, false); err != nil {
		return fmt.Errorf("install slash commands: %w", err)
	}
	return ensureMemoryConfig(home)
}

// UpdateReport describes what Update changed.
type UpdateReport struct {
	Created    []string // paths newly created
	Skipped    []string // existed already, not touched
	Migrations []string // hints about old layout needing migration
}

// Update is additive: adds anything missing without clobbering user customizations.
// Does NOT overwrite user-edited GOAL.md, SKILL.md, slash commands, or jeff.json
// fields the user has set.
func Update(home string) (UpdateReport, error) {
	var r UpdateReport

	// Memory tree dirs — EnsureLayout is idempotent; track whether root was new.
	memRoot := MemoryRoot(home)
	memNew := !exists(memRoot)
	if err := EnsureLayout(home); err != nil {
		return r, fmt.Errorf("ensure layout: %w", err)
	}
	if memNew {
		r.Created = append(r.Created, memRoot)
	} else {
		r.Skipped = append(r.Skipped, memRoot)
	}

	// Marlowe GOAL.md — install only if missing.
	goalPath := marloweGoalPath(home)
	if !exists(goalPath) {
		if err := installMarloweGoal(home); err != nil {
			return r, fmt.Errorf("install marlowe goal: %w", err)
		}
		r.Created = append(r.Created, goalPath)
	} else {
		r.Skipped = append(r.Skipped, goalPath)
	}

	// Curation SKILL.md — install only if missing.
	skillPath := filepath.Join(home, ".skills", "curation", "SKILL.md")
	if !exists(skillPath) {
		if err := skill.SeedCuration(home); err != nil {
			return r, fmt.Errorf("seed curation skill: %w", err)
		}
		r.Created = append(r.Created, skillPath)
	} else {
		r.Skipped = append(r.Skipped, skillPath)
	}

	// Slash commands — additive: install only missing ones.
	created, skipped, err := syncSlashCommands(home)
	if err != nil {
		return r, fmt.Errorf("sync slash commands: %w", err)
	}
	r.Created = append(r.Created, created...)
	r.Skipped = append(r.Skipped, skipped...)

	// Old layout detection — emit hints but do not migrate automatically.
	r.Migrations = detectOldLayout(home)

	return r, nil
}

// MigrateReport describes what Migrate changed (or would change on dry-run).
type MigrateReport struct {
	Moved   []string // "old → new" entries
	Skipped []string // files that didn't match known patterns
	Errors  []error
}

// Migrate moves the legacy memory layout into the v1 tree.
//
// Old layout:
//
//	JEFF_HOME/personas/<x>/memory/MEMORY.md    → memory/personas/<x>/semantic/INDEX.md
//	JEFF_HOME/personas/<x>/memory/<detail>.md  → memory/personas/<x>/semantic/<detail>.md
//	JEFF_HOME/learnings/<repo>/INDEX.md        → memory/repos/<repo>/semantic/INDEX.md
//	JEFF_HOME/learnings/<repo>/<detail>.md     → memory/repos/<repo>/semantic/<detail>.md
//
// Old files are moved to archive/migration-YYYYMMDD/ (not deleted).
// Pass dryRun=true to preview changes without writing anything.
func Migrate(home string, dryRun bool) (MigrateReport, error) {
	var r MigrateReport
	stamp := time.Now().Format("20060102")
	archiveBase := filepath.Join(ArchiveRoot(home), "migration-"+stamp)

	// Legacy personas: personas/<x>/memory/ → memory/personas/<x>/semantic/
	legacyPersonasDir := filepath.Join(home, "personas")
	if entries, err := os.ReadDir(legacyPersonasDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			personaName := e.Name()
			srcDir := filepath.Join(legacyPersonasDir, personaName, "memory")
			if !exists(srcDir) {
				continue
			}
			destDir := filepath.Join(PersonaScopePath(home, personaName), string(BucketSemantic))
			archiveDir := filepath.Join(archiveBase, "personas", personaName, "memory")
			migrateDir(srcDir, destDir, archiveDir, dryRun, &r)
		}
	}

	// Legacy learnings: learnings/<repo>/ → memory/repos/<repo>/semantic/
	learningsDir := filepath.Join(home, "learnings")
	if entries, err := os.ReadDir(learningsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			repoName := e.Name()
			srcDir := filepath.Join(learningsDir, repoName)
			destDir := filepath.Join(RepoScopePath(home, repoName), string(BucketSemantic))
			archiveDir := filepath.Join(archiveBase, "learnings", repoName)
			migrateDir(srcDir, destDir, archiveDir, dryRun, &r)
		}
	}

	return r, nil
}

// ---- helpers ----

// marloweGoalPath returns the path where marlowe's GOAL.md lives at runtime.
func marloweGoalPath(home string) string {
	return filepath.Join(home, "personas", "marlowe", "GOAL.md")
}

// installMarloweGoal writes the embedded marlowe GOAL.md to <home>/personas/marlowe/GOAL.md.
// Idempotent: overwrites so upgrades propagate. Update() guards against overwriting
// user-edited files by checking existence before calling this.
func installMarloweGoal(home string) error {
	path := marloweGoalPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create personas/marlowe dir: %w", err)
	}
	return os.WriteFile(path, []byte(jeffembed.MarloweGoalMD), 0o644)
}

// installSlashCommands copies embedded slash-commands/*.md to <home>/.claude/commands/.
// If skipExisting=true, files that already exist are not overwritten.
func installSlashCommands(home string, skipExisting bool) error {
	destDir := filepath.Join(home, ".claude", "commands")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create commands dir: %w", err)
	}

	return fs.WalkDir(jeffembed.SlashCommandsFS, "slash-commands", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		dest := filepath.Join(destDir, d.Name())
		if skipExisting && exists(dest) {
			return nil
		}
		data, err := jeffembed.SlashCommandsFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", path, err)
		}
		return os.WriteFile(dest, data, 0o644)
	})
}

// syncSlashCommands installs missing slash commands and reports what was created/skipped.
func syncSlashCommands(home string) (created, skipped []string, err error) {
	destDir := filepath.Join(home, ".claude", "commands")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create commands dir: %w", err)
	}

	walkErr := fs.WalkDir(jeffembed.SlashCommandsFS, "slash-commands", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		dest := filepath.Join(destDir, d.Name())
		if exists(dest) {
			skipped = append(skipped, dest)
			return nil
		}
		data, readErr := jeffembed.SlashCommandsFS.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read embedded %s: %w", path, readErr)
		}
		if writeErr := os.WriteFile(dest, data, 0o644); writeErr != nil {
			return writeErr
		}
		created = append(created, dest)
		return nil
	})
	return created, skipped, walkErr
}

// ensureMemoryConfig ensures jeff.json has a memory section (if not explicitly disabled).
// Does not overwrite an existing memory.disabled=true setting.
func ensureMemoryConfig(home string) error {
	cfg, err := jeff.LoadConfig(home)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.Memory != nil {
		return nil // already present; don't overwrite user settings
	}
	cfg.Memory = &jeff.MemoryConfig{}
	return jeff.SaveConfig(cfg)
}

// detectOldLayout scans for legacy memory/learnings directories and returns
// migration hint strings for each one found.
func detectOldLayout(home string) []string {
	var hints []string

	// personas/<x>/memory/MEMORY.md
	personasDir := filepath.Join(home, "personas")
	if entries, err := os.ReadDir(personasDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			memFile := filepath.Join(personasDir, e.Name(), "memory", "MEMORY.md")
			if exists(memFile) {
				hints = append(hints, fmt.Sprintf("personas/%s/memory/ → memory/personas/%s/semantic/ (run `jeff memory migrate`)", e.Name(), e.Name()))
			}
		}
	}

	// learnings/<repo>/INDEX.md
	learningsDir := filepath.Join(home, "learnings")
	if entries, err := os.ReadDir(learningsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			indexFile := filepath.Join(learningsDir, e.Name(), "INDEX.md")
			if exists(indexFile) {
				hints = append(hints, fmt.Sprintf("learnings/%s/ → memory/repos/%s/semantic/ (run `jeff memory migrate`)", e.Name(), e.Name()))
			}
		}
	}

	return hints
}

// migrateDir walks srcDir and migrates .md files to destDir, archiving originals.
func migrateDir(srcDir, destDir, archiveDir string, dryRun bool, r *MigrateReport) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		r.Errors = append(r.Errors, fmt.Errorf("read dir %s: %w", srcDir, err))
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			r.Skipped = append(r.Skipped, filepath.Join(srcDir, name))
			continue
		}

		// MEMORY.md and INDEX.md both map to INDEX.md in the new tree.
		destName := name
		if name == "MEMORY.md" || name == "INDEX.md" {
			destName = "INDEX.md"
		}

		srcPath := filepath.Join(srcDir, name)
		destPath := filepath.Join(destDir, destName)
		archivePath := filepath.Join(archiveDir, name)

		r.Moved = append(r.Moved, srcPath+" → "+destPath)

		if dryRun {
			continue
		}

		data, err := os.ReadFile(srcPath)
		if err != nil {
			r.Errors = append(r.Errors, fmt.Errorf("read %s: %w", srcPath, err))
			continue
		}

		content := ensureFrontmatter(data, strings.TrimSuffix(destName, ".md"))

		if err := os.MkdirAll(destDir, 0o755); err != nil {
			r.Errors = append(r.Errors, fmt.Errorf("mkdir %s: %w", destDir, err))
			continue
		}
		if err := os.WriteFile(destPath, []byte(content), 0o644); err != nil {
			r.Errors = append(r.Errors, fmt.Errorf("write %s: %w", destPath, err))
			continue
		}

		if err := os.MkdirAll(archiveDir, 0o755); err != nil {
			r.Errors = append(r.Errors, fmt.Errorf("mkdir archive %s: %w", archiveDir, err))
			continue
		}
		if err := os.Rename(srcPath, archivePath); err != nil {
			r.Errors = append(r.Errors, fmt.Errorf("archive %s: %w", srcPath, err))
		}
	}
}

// ensureFrontmatter returns the file content with valid v1 frontmatter.
// If the content already has a frontmatter block it is preserved; otherwise
// a default worker-facing block (type=reference) is prepended.
func ensureFrontmatter(data []byte, slug string) string {
	_, _, err := Parse(bytes.NewReader(data))
	if err == nil {
		// Already has valid frontmatter — return as-is.
		return string(data)
	}

	// No frontmatter: wrap the whole content as the body under type=reference.
	// Use slug (filename without ext) as the name.
	name := slug
	if name == "" || name == "INDEX" || name == "MEMORY" {
		name = "migrated-index"
	}

	fm := Frontmatter{
		Name:        name,
		Description: "Migrated from legacy memory layout (type: reference; re-classify as needed)",
		Type:        TypeReference,
	}

	var buf bytes.Buffer
	if err := Write(&buf, fm, strings.TrimSpace(string(data))); err != nil {
		// Fallback: return original content unchanged.
		return string(data)
	}
	return buf.String()
}

// exists reports whether path exists on the filesystem.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
