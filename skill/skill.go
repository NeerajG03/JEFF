// Package skill manages a registry of Claude Code skills that can be
// auto-injected into task workspaces based on persona, task type, and tags.
package skill

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/NeerajG03/JEFF/internal/homepath"
)

//go:embed skill_mgmt.md
var Doc string

const schemaURL = "https://raw.githubusercontent.com/NeerajG03/JEFF/main/schemas/skills.json"

// SkillEntry describes a registered skill and its injection criteria.
type SkillEntry struct {
	Location string   `json:"location"`
	Tags     []string `json:"tags,omitempty"`
	Personas []string `json:"personas,omitempty"`
	GigTypes []string `json:"gig_type,omitempty"`
}

// SkillConfig is the top-level structure of skills.json.
type SkillConfig struct {
	Schema string                 `json:"$schema,omitempty"`
	Skills map[string]*SkillEntry `json:"skills"`
}

// SkillInfo pairs a skill name with its entry for display.
type SkillInfo struct {
	Name  string
	Entry *SkillEntry
}

// SkillsPath returns the path to skills.json.
func SkillsPath(jeffHome string) string {
	return filepath.Join(jeffHome, ".skills", "skills.json")
}

// DefaultSkillsDir returns the default skill storage directory.
func DefaultSkillsDir(jeffHome string) string {
	return filepath.Join(jeffHome, ".skills")
}

// LoadSkills reads skills.json. Returns an empty config if the file is missing.
func LoadSkills(jeffHome string) (*SkillConfig, error) {
	data, err := os.ReadFile(SkillsPath(jeffHome))
	if err != nil {
		if os.IsNotExist(err) {
			return &SkillConfig{Schema: schemaURL, Skills: make(map[string]*SkillEntry)}, nil
		}
		return nil, fmt.Errorf("read skills.json: %w", err)
	}

	var sc SkillConfig
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("parse skills.json: %w", err)
	}
	if sc.Skills == nil {
		sc.Skills = make(map[string]*SkillEntry)
	}
	// Locations are stored home-relative so the home stays relocatable; callers
	// always see absolute paths. Pre-existing absolute entries pass through
	// unchanged and are rewritten to relative form on the next save.
	for _, entry := range sc.Skills {
		entry.Location = homepath.Abs(jeffHome, entry.Location)
	}
	return &sc, nil
}

// SaveSkills writes skills.json with pretty formatting. Locations inside the home
// are persisted home-relative (see internal/homepath); the in-memory config keeps
// its absolute paths so callers can keep using it after a save.
func SaveSkills(jeffHome string, sc *SkillConfig) error {
	if sc.Schema == "" {
		sc.Schema = schemaURL
	}

	onDisk := &SkillConfig{Schema: sc.Schema, Skills: make(map[string]*SkillEntry, len(sc.Skills))}
	for name, entry := range sc.Skills {
		copied := *entry
		copied.Location = homepath.Rel(jeffHome, entry.Location)
		onDisk.Skills[name] = &copied
	}

	data, err := json.MarshalIndent(onDisk, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal skills.json: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(SkillsPath(jeffHome))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create skills dir: %w", err)
	}
	return os.WriteFile(SkillsPath(jeffHome), data, 0o644)
}

// Add registers a skill. If external is false, the skill directory is copied
// into .skills/<name>/. If external is true, only the location is recorded.
// name defaults to the base name of skillPath if empty.
func Add(jeffHome, skillPath, name string, external bool) (*SkillEntry, error) {
	absPath, err := filepath.Abs(skillPath)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	// Validate SKILL.md exists.
	if _, err := os.Stat(filepath.Join(absPath, "SKILL.md")); err != nil {
		return nil, fmt.Errorf("SKILL.md not found in %s", absPath)
	}

	if name == "" {
		name = filepath.Base(absPath)
	}

	sc, err := LoadSkills(jeffHome)
	if err != nil {
		return nil, err
	}

	if _, exists := sc.Skills[name]; exists {
		return nil, fmt.Errorf("skill %q already registered", name)
	}

	location := absPath
	if !external {
		dest := filepath.Join(DefaultSkillsDir(jeffHome), name)
		if err := copyDir(absPath, dest); err != nil {
			return nil, fmt.Errorf("copy skill: %w", err)
		}
		location = dest
	}

	entry := &SkillEntry{Location: location}
	sc.Skills[name] = entry
	if err := SaveSkills(jeffHome, sc); err != nil {
		return nil, err
	}
	return entry, nil
}

// Remove unregisters a skill. If deleteFiles is true and the location is
// under .skills/, the directory is deleted.
func Remove(jeffHome, name string, deleteFiles bool) error {
	sc, err := LoadSkills(jeffHome)
	if err != nil {
		return err
	}

	entry, exists := sc.Skills[name]
	if !exists {
		return fmt.Errorf("skill %q not found", name)
	}

	if deleteFiles {
		skillsDir := DefaultSkillsDir(jeffHome)
		if rel, err := filepath.Rel(skillsDir, entry.Location); err == nil && !filepath.IsAbs(rel) {
			os.RemoveAll(entry.Location)
		}
	}

	delete(sc.Skills, name)
	return SaveSkills(jeffHome, sc)
}

// SetTags updates the injection criteria for a skill.
// Non-nil slices replace the existing values; nil slices leave them unchanged.
func SetTags(jeffHome, name string, personas, gigTypes, tags []string) error {
	sc, err := LoadSkills(jeffHome)
	if err != nil {
		return err
	}

	entry, exists := sc.Skills[name]
	if !exists {
		return fmt.Errorf("skill %q not found", name)
	}

	if personas != nil {
		entry.Personas = personas
	}
	if gigTypes != nil {
		entry.GigTypes = gigTypes
	}
	if tags != nil {
		entry.Tags = tags
	}

	return SaveSkills(jeffHome, sc)
}

// List returns all registered skills sorted by name.
func List(jeffHome string) ([]*SkillInfo, error) {
	sc, err := LoadSkills(jeffHome)
	if err != nil {
		return nil, err
	}

	var result []*SkillInfo
	for name, entry := range sc.Skills {
		result = append(result, &SkillInfo{Name: name, Entry: entry})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// Get returns a single skill entry.
func Get(jeffHome, name string) (*SkillEntry, error) {
	sc, err := LoadSkills(jeffHome)
	if err != nil {
		return nil, err
	}
	entry, exists := sc.Skills[name]
	if !exists {
		return nil, fmt.Errorf("skill %q not found", name)
	}
	return entry, nil
}

// copyDir recursively copies src to dst.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
