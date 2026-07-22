// Package testutil provides shared test helpers for jeff packages.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TempHome creates a temporary JEFF_HOME directory for testing.
// If subdirs are provided, they are created under the home directory.
func TempHome(t *testing.T, subdirs ...string) string {
	t.Helper()
	dir := t.TempDir()
	home := filepath.Join(dir, ".jeff")
	for _, sub := range subdirs {
		_ = os.MkdirAll(filepath.Join(home, sub), 0o755)
	}
	return home
}
