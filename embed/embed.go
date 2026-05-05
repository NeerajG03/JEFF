// Package embed provides default files shipped with the binary.
package embed

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed CLAUDE.md
var DefaultClaudeMD string

//go:embed claude-settings.json
var DefaultClaudeSettings string

//go:embed personas/marlowe/GOAL.md
var MarloweGoalMD string

// CreateContextAliases creates symlinks from each alias filename to CLAUDE.md
// in the given directory. CLAUDE.md is the single source of truth; other
// context files (e.g. GEMINI.md) are symlinks to it.
func CreateContextAliases(dir string, aliases []string) error {
	for _, alias := range aliases {
		link := filepath.Join(dir, alias)
		target := "CLAUDE.md"

		// If symlink already points to the right target, skip.
		if existing, err := os.Readlink(link); err == nil && existing == target {
			continue
		}

		// Remove any existing file/symlink.
		os.Remove(link)

		if err := os.Symlink(target, link); err != nil {
			return fmt.Errorf("symlink %s -> %s: %w", alias, target, err)
		}
	}
	return nil
}

// WriteClaudeMD writes the default CLAUDE.md to the target directory.
// homePath is substituted for {{.Home}} in the template (e.g. "~/.jeff/" or "./jeff/").
// If the file already exists, it is left untouched (user may have edited it).
// Use force=true to overwrite.
func WriteClaudeMD(dir, homePath string, force bool) error {
	path := filepath.Join(dir, "CLAUDE.md")

	if !force {
		if _, err := os.Stat(path); err == nil {
			return nil // already exists, don't overwrite
		}
	}

	content := strings.ReplaceAll(DefaultClaudeMD, "{{.Home}}", homePath)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
