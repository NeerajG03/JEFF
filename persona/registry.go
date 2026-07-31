package persona

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/NeerajG03/JEFF/internal/homepath"
)

const schemaURL = "https://raw.githubusercontent.com/NeerajG03/JEFF/main/schemas/personas.json"

// PersonaEntry describes a registered persona.
type PersonaEntry struct {
	Location    string `json:"location"`
	Description string `json:"description,omitempty"`
	MemoryHint  string `json:"memory_hint,omitempty"`
	Model       string `json:"model,omitempty"`
	Agent       string `json:"agent,omitempty"` // default agent tool (e.g. "claude", "gemini")
}

// PersonaConfig is the top-level structure of personas.json.
type PersonaConfig struct {
	Schema   string                   `json:"$schema,omitempty"`
	Personas map[string]*PersonaEntry `json:"personas"`
}

// PersonaInfo pairs a persona name with its entry for display.
type PersonaInfo struct {
	Name  string
	Entry *PersonaEntry
}

// PersonasPath returns the path to personas.json.
func PersonasPath(jeffHome string) string {
	return filepath.Join(jeffHome, ".personas", "personas.json")
}

// DefaultPersonasDir returns the default persona storage directory.
func DefaultPersonasDir(jeffHome string) string {
	return filepath.Join(jeffHome, ".personas")
}

// LoadPersonas reads personas.json. Returns empty config if the file is missing.
func LoadPersonas(jeffHome string) (*PersonaConfig, error) {
	data, err := os.ReadFile(PersonasPath(jeffHome))
	if err != nil {
		if os.IsNotExist(err) {
			return &PersonaConfig{Schema: schemaURL, Personas: make(map[string]*PersonaEntry)}, nil
		}
		return nil, fmt.Errorf("read personas.json: %w", err)
	}

	var pc PersonaConfig
	if err := json.Unmarshal(data, &pc); err != nil {
		return nil, fmt.Errorf("parse personas.json: %w", err)
	}
	if pc.Personas == nil {
		pc.Personas = make(map[string]*PersonaEntry)
	}
	// Locations are stored home-relative so the home stays relocatable; callers
	// always see absolute paths. Pre-existing absolute entries pass through
	// unchanged and are rewritten to relative form on the next save.
	for _, entry := range pc.Personas {
		entry.Location = homepath.Abs(jeffHome, entry.Location)
	}
	return &pc, nil
}

// SavePersonas writes personas.json with pretty formatting. Locations inside the
// home are persisted home-relative (see internal/homepath); the in-memory config
// keeps its absolute paths so callers can keep using it after a save.
func SavePersonas(jeffHome string, pc *PersonaConfig) error {
	if pc.Schema == "" {
		pc.Schema = schemaURL
	}

	onDisk := &PersonaConfig{Schema: pc.Schema, Personas: make(map[string]*PersonaEntry, len(pc.Personas))}
	for name, entry := range pc.Personas {
		copied := *entry
		copied.Location = homepath.Rel(jeffHome, entry.Location)
		onDisk.Personas[name] = &copied
	}

	data, err := json.MarshalIndent(onDisk, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal personas.json: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(PersonasPath(jeffHome))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create personas dir: %w", err)
	}
	return os.WriteFile(PersonasPath(jeffHome), data, 0o644)
}

// AddPersona registers a persona. If external is false, the persona directory is copied
// into .personas/<name>/. If external is true, only the location is recorded.
// name defaults to the base name of personaPath if empty.
func AddPersona(jeffHome, personaPath, name string, external bool) (*PersonaEntry, error) {
	absPath, err := filepath.Abs(personaPath)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	// Validate PERSONA.md exists.
	if _, err := os.Stat(filepath.Join(absPath, "PERSONA.md")); err != nil {
		return nil, fmt.Errorf("PERSONA.md not found in %s", absPath)
	}

	if name == "" {
		name = filepath.Base(absPath)
	}

	pc, err := LoadPersonas(jeffHome)
	if err != nil {
		return nil, err
	}

	if _, exists := pc.Personas[name]; exists {
		return nil, fmt.Errorf("persona %q already registered", name)
	}

	location := absPath
	if !external {
		dest := filepath.Join(DefaultPersonasDir(jeffHome), name)
		if err := copyDir(absPath, dest); err != nil {
			return nil, fmt.Errorf("copy persona: %w", err)
		}
		location = dest
	}

	// Extract description from PERSONA.md first line.
	desc := extractDescription(location)

	entry := &PersonaEntry{Location: location, Description: desc}
	pc.Personas[name] = entry
	if err := SavePersonas(jeffHome, pc); err != nil {
		return nil, err
	}
	return entry, nil
}

// RemovePersona unregisters a persona. If deleteFiles is true and the location is
// under .personas/, the directory is deleted.
func RemovePersona(jeffHome, name string, deleteFiles bool) error {
	pc, err := LoadPersonas(jeffHome)
	if err != nil {
		return err
	}

	entry, exists := pc.Personas[name]
	if !exists {
		return fmt.Errorf("persona %q not found", name)
	}

	if deleteFiles {
		personasDir := DefaultPersonasDir(jeffHome)
		if rel, err := filepath.Rel(personasDir, entry.Location); err == nil && !filepath.IsAbs(rel) {
			os.RemoveAll(entry.Location)
		}
	}

	delete(pc.Personas, name)
	return SavePersonas(jeffHome, pc)
}

// UpdatePersona updates the description, memory hint, and/or model.
// Empty strings leave the field unchanged.
func UpdatePersona(jeffHome, name, description, memoryHint, model string) error {
	pc, err := LoadPersonas(jeffHome)
	if err != nil {
		return err
	}

	entry, exists := pc.Personas[name]
	if !exists {
		return fmt.Errorf("persona %q not found", name)
	}

	if description != "" {
		entry.Description = description
	}
	if memoryHint != "" {
		entry.MemoryHint = memoryHint
	}
	if model != "" {
		entry.Model = model
	}

	return SavePersonas(jeffHome, pc)
}

// ListPersonas returns all registered personas sorted by name.
func ListPersonas(jeffHome string) ([]*PersonaInfo, error) {
	pc, err := LoadPersonas(jeffHome)
	if err != nil {
		return nil, err
	}

	var result []*PersonaInfo
	for name, entry := range pc.Personas {
		result = append(result, &PersonaInfo{Name: name, Entry: entry})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// GetPersona returns a single persona entry.
func GetPersona(jeffHome, name string) (*PersonaEntry, error) {
	pc, err := LoadPersonas(jeffHome)
	if err != nil {
		return nil, err
	}
	entry, exists := pc.Personas[name]
	if !exists {
		return nil, fmt.Errorf("persona %q not found", name)
	}
	return entry, nil
}

// GetTemplate reads the PERSONA.md content for a registered persona.
func GetTemplate(jeffHome, name string) (string, error) {
	entry, err := GetPersona(jeffHome, name)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(entry.Location, "PERSONA.md"))
	if err != nil {
		return "", fmt.Errorf("read PERSONA.md for %s: %w", name, err)
	}
	return string(data), nil
}

// RegisteredNames returns sorted names of all registered personas.
func RegisteredNames(jeffHome string) []string {
	pc, _ := LoadPersonas(jeffHome)
	if pc == nil {
		return nil
	}
	names := make([]string, 0, len(pc.Personas))
	for name := range pc.Personas {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// RegisteredNamesWithDescriptions returns persona names with descriptions
// formatted as "name\tdescription" for shell completion.
func RegisteredNamesWithDescriptions(jeffHome string) []string {
	personas, _ := ListPersonas(jeffHome)
	result := make([]string, 0, len(personas))
	for _, p := range personas {
		if p.Entry.Description != "" {
			result = append(result, p.Name+"\t"+p.Entry.Description)
		} else {
			result = append(result, p.Name)
		}
	}
	return result
}

// RegisteredDescription returns the description for a registered persona.
func RegisteredDescription(jeffHome, name string) string {
	entry, err := GetPersona(jeffHome, name)
	if err != nil {
		return ""
	}
	return entry.Description
}

// RegisteredMemoryHint returns the memory hint for a registered persona.
func RegisteredMemoryHint(jeffHome, name string) string {
	entry, err := GetPersona(jeffHome, name)
	if err != nil {
		return ""
	}
	return entry.MemoryHint
}

// RegisteredModel returns the default model for a registered persona.
func RegisteredModel(jeffHome, name string) string {
	entry, err := GetPersona(jeffHome, name)
	if err != nil {
		return ""
	}
	return entry.Model
}

// RegisteredAgent returns the default agent tool for a registered persona.
func RegisteredAgent(jeffHome, name string) string {
	entry, err := GetPersona(jeffHome, name)
	if err != nil {
		return ""
	}
	return entry.Agent
}

// SeedDefaults copies the embedded persona templates to disk and registers them.
// Skips any persona that already exists in the registry.
func SeedDefaults(jeffHome string) error {
	pc, err := LoadPersonas(jeffHome)
	if err != nil {
		return err
	}

	for _, name := range Names() {
		if _, exists := pc.Personas[name]; exists {
			continue
		}

		// Get embedded template content.
		content, err := Get(name)
		if err != nil {
			continue
		}

		// Write to disk.
		dir := filepath.Join(DefaultPersonasDir(jeffHome), name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create persona dir %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "PERSONA.md"), []byte(content), 0o644); err != nil {
			return fmt.Errorf("write PERSONA.md for %s: %w", name, err)
		}

		// Register with description, memory hint, default model, and default agent from embedded sources.
		desc := Description(name)
		hint := MemoryHint(name)
		model := DefaultModel(name)
		agent := DefaultAgent(name)
		pc.Personas[name] = &PersonaEntry{
			Location:    dir,
			Description: desc,
			MemoryHint:  hint,
			Model:       model,
			Agent:       agent,
		}
	}

	return SavePersonas(jeffHome, pc)
}

// extractDescription reads the first line of PERSONA.md and extracts the role description.
func extractDescription(location string) string {
	data, err := os.ReadFile(filepath.Join(location, "PERSONA.md"))
	if err != nil {
		return ""
	}
	firstLine := strings.SplitN(string(data), "\n", 2)[0]
	parts := strings.SplitN(firstLine, " — ", 2)
	if len(parts) < 2 {
		parts = strings.SplitN(firstLine, " - ", 2)
	}
	if len(parts) < 2 {
		return ""
	}
	desc := parts[1]
	if idx := strings.Index(desc, "."); idx != -1 {
		desc = desc[:idx]
	}
	return strings.TrimSpace(desc)
}

// copyDir recursively copies src to dst.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
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
