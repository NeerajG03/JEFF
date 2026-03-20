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
