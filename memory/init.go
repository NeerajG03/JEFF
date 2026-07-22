// init.go — Worker E: idempotent v1 layout provisioning and legacy migration.
// Initialize: greenfield setup. Update: additive only. Migrate: legacy → v1 tree.
package memory

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	jeff "github.com/NeerajG03/JEFF"
	jeffembed "github.com/NeerajG03/JEFF/embed"
	"github.com/NeerajG03/JEFF/skill"
	"gopkg.in/yaml.v3"
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
			migrateDir(srcDir, destDir, archiveDir, "persona:"+personaName, dryRun, &r)
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
			migrateDir(srcDir, destDir, archiveDir, "repo:"+repoName, dryRun, &r)
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
// scope is the canonical scope label ("persona:<x>" or "repo:<y>") that the
// destination represents — used by wrapAsCanonical to enrich entries.
func migrateDir(srcDir, destDir, archiveDir, scope string, dryRun bool, r *MigrateReport) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		r.Errors = append(r.Errors, fmt.Errorf("read dir %s: %w", srcDir, err))
		return
	}

	// Pre-read sibling INDEX.md / MEMORY.md to recover per-entry descriptions.
	descByslug := readLegacyDescriptions(srcDir)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			r.Skipped = append(r.Skipped, filepath.Join(srcDir, name))
			continue
		}

		isIndex := name == "MEMORY.md" || name == "INDEX.md"
		destName := name
		if isIndex {
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

		var content string
		if isIndex {
			// INDEX.md is filtered from ListEntries, so a minimal 3-field
			// frontmatter is sufficient (and harmless on regen by marlowe).
			content = ensureIndexFrontmatter(data)
		} else {
			slug := strings.TrimSuffix(destName, ".md")
			content = wrapAsCanonical(data, slug, scope, descByslug[slug])
		}

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

// indexEntryRe matches a markdown index line and captures (slug, description).
// Recognises both common shapes:
//
//	- [Title](slug.md) — description text
//	- **slug** (`type`): description text
var indexEntryRe = regexp.MustCompile(`(?m)^\s*-\s+(?:\[[^\]]+\]\(([^)]+?)\.md\)|\*\*([^*]+)\*\*[^:]*:)\s*[—–\-:]?\s*(.+)$`)

// readLegacyDescriptions scans MEMORY.md and INDEX.md in srcDir to extract
// per-slug descriptions. Returns an empty map if neither exists or parsing
// recovers nothing.
func readLegacyDescriptions(srcDir string) map[string]string {
	out := map[string]string{}
	for _, name := range []string{"MEMORY.md", "INDEX.md"} {
		data, err := os.ReadFile(filepath.Join(srcDir, name))
		if err != nil {
			continue
		}
		for _, m := range indexEntryRe.FindAllStringSubmatch(string(data), -1) {
			slug := m[1]
			if slug == "" {
				slug = strings.TrimSpace(m[2])
			}
			desc := strings.TrimSpace(m[3])
			if slug != "" && desc != "" {
				out[slug] = desc
			}
		}
	}
	return out
}

// wrapAsCanonical rewrites a legacy memory file as a canonical v1 entry.
// Legacy frontmatter (source: <string>, updated: <date>) is mapped:
//   - source        → source.task
//   - updated       → valid_from
//
// If the legacy file has no frontmatter, the whole content is treated as body.
// If the legacy file already has canonical frontmatter, this is idempotent —
// the canonical fields are re-emitted. Description preference order:
// existing canonical description → INDEX-derived description → fallback.
func wrapAsCanonical(data []byte, slug, scope, indexDescription string) string {
	yamlBlock, body, err := splitFrontmatter(bytes.NewReader(data))

	var legacy map[string]any
	if err == nil {
		_ = yaml.Unmarshal(yamlBlock, &legacy)
	} else {
		// No frontmatter: whole file is the body.
		body = string(data)
	}

	// Pull useful legacy fields, defensively typed.
	getStr := func(k string) string {
		switch v := legacy[k].(type) {
		case string:
			return v
		case time.Time:
			return v.Format(time.RFC3339)
		case nil:
			return ""
		default:
			return fmt.Sprintf("%v", v)
		}
	}
	legacyName := getStr("name")
	legacyDesc := getStr("description")
	legacyType := getStr("type")
	legacyTask := getStr("source")
	legacyUpdated := getStr("updated")
	// If updated decoded directly as time.Time, capture it for later use.
	var legacyUpdatedTime time.Time
	if t, ok := legacy["updated"].(time.Time); ok {
		legacyUpdatedTime = t.UTC()
	}

	name := slug
	if legacyName != "" {
		name = legacyName
	}

	description := legacyDesc
	if description == "" {
		description = indexDescription
	}
	if description == "" {
		description = "Migrated entry: " + slug + " (re-classify type as needed)"
	}

	memType := TypeReference
	if mt, err := ParseMemoryType(legacyType); err == nil {
		memType = mt
	}

	validFrom := time.Now().UTC()
	switch {
	case !legacyUpdatedTime.IsZero():
		validFrom = legacyUpdatedTime
	case legacyUpdated != "":
		if t, err := time.Parse("2006-01-02", legacyUpdated); err == nil {
			validFrom = t.UTC()
		} else if t, err := time.Parse(time.RFC3339, legacyUpdated); err == nil {
			validFrom = t.UTC()
		}
	}

	var sourcePersona string
	if strings.HasPrefix(scope, "persona:") {
		sourcePersona = strings.TrimPrefix(scope, "persona:")
	}

	fm := CanonicalFrontmatter{
		Frontmatter: Frontmatter{Name: name, Description: description, Type: memType},
		Status:      "accepted",
		Scope:       scope,
		ValidFrom:   validFrom,
		Provenance:  "review-required",
		Source: Source{
			Persona: sourcePersona,
			Task:    legacyTask,
			Trigger: "migration",
		},
	}

	var buf bytes.Buffer
	if err := writeCanonical(&buf, fm, strings.TrimSpace(body)); err != nil {
		// Fallback: return original content unchanged so we don't lose data.
		return string(data)
	}
	return buf.String()
}

// ensureIndexFrontmatter wraps INDEX.md content with minimal worker-facing
// frontmatter if it has none. INDEX.md is filtered out of ListEntries, so a
// 3-field block is enough — marlowe regenerates INDEX.md as entries change.
func ensureIndexFrontmatter(data []byte) string {
	if _, _, err := Parse(bytes.NewReader(data)); err == nil {
		return string(data)
	}
	fm := Frontmatter{
		Name:        "migrated-index",
		Description: "Migrated INDEX from legacy memory layout — regenerated by marlowe on next curate",
		Type:        TypeReference,
	}
	var buf bytes.Buffer
	if err := Write(&buf, fm, strings.TrimSpace(string(data))); err != nil {
		return string(data)
	}
	return buf.String()
}

// exists reports whether path exists on the filesystem.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
