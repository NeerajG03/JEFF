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
