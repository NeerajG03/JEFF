// Package embed provides the default CLAUDE.md shipped with the binary.
package embed

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed CLAUDE.md
var DefaultClaudeMD string

// WriteClaudeMD writes the default CLAUDE.md to the target directory.
// If the file already exists, it is left untouched (user may have edited it).
// Use force=true to overwrite.
func WriteClaudeMD(dir string, force bool) error {
	path := filepath.Join(dir, "CLAUDE.md")

	if !force {
		if _, err := os.Stat(path); err == nil {
			return nil // already exists, don't overwrite
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	return os.WriteFile(path, []byte(DefaultClaudeMD), 0o644)
}
