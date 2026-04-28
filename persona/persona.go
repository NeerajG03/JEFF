// Package persona provides embedded agent persona templates.
package persona

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed templates/*.md
var templateFS embed.FS

// Names returns all available persona names.
func Names() []string {
	entries, err := fs.ReadDir(templateFS, "templates")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, strings.TrimSuffix(e.Name(), ".md"))
		}
	}
	return names
}

// Get returns the persona template content by name.
func Get(name string) (string, error) {
	data, err := templateFS.ReadFile("templates/" + name + ".md")
	if err != nil {
		return "", fmt.Errorf("persona %q not found (available: %s)", name, strings.Join(Names(), ", "))
	}
	return string(data), nil
}

// IsValid returns true if a persona with the given name exists.
func IsValid(name string) bool {
	_, err := Get(name)
	return err == nil
}

// Description extracts the short role description from a persona's first line.
// Templates follow the pattern: "You are <Name> — <description>."
// Returns "" if the pattern doesn't match.
func Description(name string) string {
	content, err := Get(name)
	if err != nil {
		return ""
	}
	firstLine := strings.SplitN(content, "\n", 2)[0]
	// Split on " — " (em dash with spaces).
	parts := strings.SplitN(firstLine, " — ", 2)
	if len(parts) < 2 {
		// Try regular dash.
		parts = strings.SplitN(firstLine, " - ", 2)
	}
	if len(parts) < 2 {
		return ""
	}
	desc := parts[1]
	// Take only the first sentence for a clean description.
	if idx := strings.Index(desc, "."); idx != -1 {
		desc = desc[:idx]
	}
	return strings.TrimSpace(desc)
}

// NamesWithDescriptions returns persona names with descriptions
// formatted as "name\tdescription" for shell completion.
func NamesWithDescriptions() []string {
	names := Names()
	result := make([]string, 0, len(names))
	for _, name := range names {
		desc := Description(name)
		if desc != "" {
			result = append(result, name+"\t"+desc)
		} else {
			result = append(result, name)
		}
	}
	return result
}

// MemoryHint returns a persona-specific hint for what to capture in the scratchpad.
// Returns "" if persona has no custom hint.
func MemoryHint(name string) string {
	hints := map[string]string{
		"jenko":   "Code style corrections, test patterns the user prefers, build/lint quirks, implementation shortcuts that worked.",
		"schmidt":  "Root cause patterns, misleading error messages, debugging techniques that worked, investigation dead ends to avoid.",
		"eric":     "Where to find authoritative docs, research shortcuts, knowledge gaps in the codebase, architectural insights.",
		"hardy":    "Review standards the user enforces, common issues to flag, quality thresholds, approval criteria.",
		"dickson":  "Task decomposition patterns, delegation decisions that worked, scope tradeoffs, planning heuristics.",
	}
	return hints[name]
}

// DefaultModel returns the default Claude model for a persona.
// Personas that need deeper reasoning get "opus"; lighter roles get "sonnet".
// Returns "" if persona has no default (falls back to Claude's own default).
func DefaultModel(name string) string {
	models := map[string]string{
		"jenko":   "opus",
		"schmidt": "opus",
		"dickson": "opus",
		"eric":    "sonnet",
		"hardy":   "sonnet",
	}
	return models[name]
}

// DefaultAgent returns the default agent tool for a persona.
// Currently all embedded personas default to "claude".
// Returns "" to use the system default.
func DefaultAgent(name string) string {
	agents := map[string]string{
		"jenko":   "claude",
		"schmidt": "claude",
		"dickson": "claude",
		"eric":    "claude",
		"hardy":   "claude",
	}
	return agents[name]
}
